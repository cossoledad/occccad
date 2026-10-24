package control

import (
	"context"
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
	"math"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestReadOnlyEngineeringRefreshThroughRouter(t *testing.T) {
	var mode atomic.Int32
	started, release := make(chan struct{}, 1), make(chan struct{})
	interceptor := grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		r, ok := req.(*workerv1.SolveAssemblyRequest)
		if !ok || !strings.HasPrefix(r.RequestId, "engineering/") {
			return invoke(ctx, method, req, reply, cc, opts...)
		}
		switch mode.Load() {
		case 1:
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-release:
			}
		case 2:
			return status.Error(codes.Unavailable, "injected analysis Worker failure")
		}
		return invoke(ctx, method, req, reply, cc, opts...)
	})
	f := newCompositionControlFixture(t, interceptor)
	part := f.importPart(t, "cylinder-r6-h12")
	p, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Accepted-pose read-only motion"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = f.actor
		r.RequestID = fmt.Sprintf("%s-engineering-%d", v.Document.ID, seq)
		next, e := f.service.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = next
	}
	for i := 0; i < 5; i++ {
		apply(&p, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	ids := []string{}
	for _, i := range p.Product.Instances {
		ids = append(ids, i.ID)
	}
	for i, pos := range [][3]float64{{0, 0, 0}, {0, 0, 20}, {40, 0, 0}, {50, 0, 0}, {80, 0, 0}} {
		apply(&p, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: ids[i], Translation: pos, Rotation: [4]float64{0, 0, 0, 1}})
	}
	axis := func(id string) *workspace.AssemblyGeometryRef {
		return &workspace.AssemblyGeometryRef{InstanceID: id, Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Z"}
	}
	apply(&p, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FixMode: "SPACE", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: ids[0], Kind: "BODY"}})
	apply(&p, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "CONCENTRIC", FirstAssemblyRef: axis(ids[0]), SecondAssemblyRef: axis(ids[1])})
	apply(&p, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX_TOGETHER", GroupMembers: []workspace.AssemblyGroupMember{{InstanceID: ids[2]}, {InstanceID: ids[3]}}})
	group := p.Product.Constraints[len(p.Product.Constraints)-1].ID
	first, second := f.support(t, part, "PLANE"), f.support(t, part, "PLANE")
	first.InstanceID = ids[0]
	second.InstanceID = ids[1]
	apply(&p, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", Value: 20, FirstAssemblyRef: &first, SecondAssemblyRef: &second})
	offset := p.Product.Constraints[len(p.Product.Constraints)-1].ID
	measured := "MEASURED"
	apply(&p, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", Value: 80, ConstraintMode: &measured, FirstAssemblyRef: axis(ids[0]), SecondAssemblyRef: axis(ids[4])})
	apply(&p, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FixMode: "SPACE", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: ids[4], Kind: "BODY"}})
	suppressed := true
	apply(&p, workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{p.Product.Constraints[len(p.Product.Constraints)-1].ID}, Suppressed: &suppressed})
	// A legal but inconsistent new definition is retained as unaccepted.
	apply(&p, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FixMode: "SPACE", FixedPose: &workspace.InstancePose{Translation: [3]float64{0, 0, 30}, Rotation: [4]float64{0, 0, 0, 1}}, FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: ids[1], Kind: "BODY"}})
	pending := p.Product.Constraints[len(p.Product.Constraints)-1]
	if pending.EvaluationStatus != "NOT_UPDATED" {
		t.Fatal("fixture must retain pending definition", pending)
	}
	type snapshot struct {
		Head                                   string
		Model                                  string
		Commands, Versions, Results, Manifests int
	}
	state := func() snapshot {
		t.Helper()
		var s snapshot
		var raw []byte
		e := f.db.QueryRow(t.Context(), `SELECT d.head_version_id::text,v.model_json,(SELECT count(*) FROM occccad.commands c WHERE c.document_id=d.id),(SELECT count(*) FROM occccad.document_versions r WHERE r.document_id=d.id),(SELECT count(*) FROM occccad.product_solve_results r JOIN occccad.product_solve_manifests m ON m.digest=r.manifest_digest WHERE m.root_product_document_id=d.id),(SELECT count(*) FROM occccad.product_solve_manifests m WHERE m.root_product_document_id=d.id) FROM occccad.documents d JOIN occccad.document_versions v ON v.id=d.head_version_id WHERE d.id=$1`, p.Document.ID).Scan(&s.Head, &raw, &s.Commands, &s.Versions, &s.Results, &s.Manifests)
		if e != nil {
			t.Fatal(e)
		}
		s.Model = string(raw)
		return s
	}
	visibility := false
	metadata := func(_ string) {
		apply(&p, workspace.CommandRequest{Type: "SET_DEFINITION_VISIBILITY", TargetKind: "ASSEMBLY_CONSTRAINT", TargetID: p.Product.Constraints[0].ID, Visible: visibility})
		visibility = !visibility
	}

	metadata("unconstrained metadata revision")
	absent, e := f.service.GetAssemblyEngineeringEvidence(t.Context(), p.Document.ID, p.Document.VersionID)
	if e != nil || absent.Available {
		t.Fatal("metadata must not borrow older result", absent, e)
	}
	check := func(wantRelative uint64) workspace.AssemblyEngineeringEvidence {
		t.Helper()
		before := state()
		r, e := f.service.RefreshAssemblyEngineeringEvidence(t.Context(), p.Document.ID, p.Document.VersionID)
		if e != nil {
			t.Fatal(e)
		}
		if !r.Available || r.Status != "AVAILABLE" || r.Source != "READ_ONLY_ANALYSIS" || r.InputDigest == "" || r.SolverBuildPolicy == "" {
			t.Fatal("analysis evidence missing", r)
		}
		for _, id := range []string{ids[0], ids[1], ids[2], ids[3], ids[4]} {
			found := false
			for _, c := range r.Components {
				for _, body := range c.BodyIDs {
					if body != id {
						continue
					}
					found = true
					if !c.Solved {
						t.Fatal("unexpected unknown", c)
					}
					if id == ids[1] && c.RelativeDof != wantRelative {
						t.Fatal("accepted hard set confused with pending definitions", c)
					}
					if (id == ids[2] || id == ids[4]) && c.GaugeDof != 6 {
						t.Fatal("numeric anchor treated as physical ground", c)
					}
				}
			}
			if !found {
				t.Fatal("missing repeated instance", id)
			}
		}
		for _, x := range r.References {
			if x.DocumentID != part.Document.ID || x.RevisionID != part.Document.VersionID {
				t.Fatal("reference not frozen", x)
			}
		}
		if state() != before {
			t.Fatal("read-only refresh changed model, pose, branch, capture, solve records or history")
		}
		return r
	}
	r := check(1)
	if len(r.ConstraintIDs) != 4 || len(r.Excluded) != 3 {
		t.Fatal("wrong accepted scope", r.ConstraintIDs, r.Excluded)
	}
	cached, e := f.service.GetAssemblyEngineeringEvidence(t.Context(), p.Document.ID, p.Document.VersionID)
	if e != nil || !reflect.DeepEqual(cached, r) {
		t.Fatal("reopen analysis cache", cached, e)
	}
	fresh := workspace.NewWithArtifacts(f.db, f.client, f.store)
	cold, e := fresh.GetAssemblyEngineeringEvidence(t.Context(), p.Document.ID, p.Document.VersionID)
	if e != nil || cold.Available {
		t.Fatal("cache is rebuildable, never another command result", cold, e)
	}
	// Remove the old supporting cap by valid upstream geometry; accepted datum/rigid components remain.
	apply(&part, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 6}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	apply(&part, workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: part.Part.DatumPlanes[len(part.Part.DatumPlanes)-1].ID})
	sketch := part.Part.Features[len(part.Part.Features)-1].ID
	apply(&part, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -20, Y: -20}, Second: &workspace.SketchPoint2{X: 20, Y: 20}}}})
	apply(&part, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: 12})
	apply(&p, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	broken := false
	for _, c := range p.Product.Constraints {
		if c.ID == offset {
			broken = c.EvaluationStatus == "BROKEN"
		}
	}
	if !broken {
		t.Fatal("real supporting topology must be missing")
	}
	// Re-admission during this source update may accept the pending Fix. Suppress it explicitly for a two-DOF shaft.
	apply(&p, workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{pending.ID}, Suppressed: &suppressed})
	metadata("broken support plus healthy components")
	r = check(2)
	hasBroken := false
	for _, x := range r.Excluded {
		if x.ConstraintID == offset && x.Reason == "BROKEN" {
			hasBroken = true
		}
	}
	if !hasBroken {
		t.Fatal("Broken exclusion lost", r.Excluded)
	}
	replacement := findP7TopologyPick(t, f.service, part, "FACE", func(properties workspace.TopologyElementProperties, selection modelcore.PersistentSelection) bool {
		return properties.GeometryType == "PLANE" && math.Abs(selection.CreationEvidence.Centroid[2]-6) < 1e-6
	})
	ref := func(id string) *workspace.AssemblyGeometryRef {
		return &workspace.AssemblyGeometryRef{InstanceID: id, Kind: "FACE", PersistentSelection: &replacement.Selection, SourceVersionID: replacement.SourceVersionID}
	}
	apply(&p, workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: offset, FirstAssemblyRef: ref(ids[0]), SecondAssemblyRef: ref(ids[1]), Value: 20})
	check(1)

	// Cancellation/failure/stale responses never create evidence or any history.
	metadata("cancel analysis")
	before := state()
	mode.Store(1)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, e := f.service.RefreshAssemblyEngineeringEvidence(ctx, p.Document.ID, p.Document.VersionID)
		done <- e
	}()
	<-started
	cancel()
	if e = <-done; e == nil {
		t.Fatal("cancel accepted")
	}
	if state() != before {
		t.Fatal("cancel mutated model")
	}
	mode.Store(2)
	if _, e = f.service.RefreshAssemblyEngineeringEvidence(t.Context(), p.Document.ID, p.Document.VersionID); e == nil {
		t.Fatal("Worker failure hidden")
	}
	if state() != before {
		t.Fatal("failure mutated model")
	}
	mode.Store(1)
	oldRevision := p.Document.VersionID
	go func() {
		_, e := f.service.RefreshAssemblyEngineeringEvidence(t.Context(), p.Document.ID, oldRevision)
		done <- e
	}()
	<-started
	metadata("newer while analysis running")
	afterRename := state()
	close(release)
	if e = <-done; e == nil || !strings.Contains(e.Error(), "ASSEMBLY_EVIDENCE_STALE") {
		t.Fatal("stale response accepted", e)
	}
	if state() != afterRename {
		t.Fatal("late analysis wrote model")
	}
	mode.Store(0)
	check(1)
	apply(&p, workspace.CommandRequest{Type: "UNDO"})
	check(1)
	apply(&p, workspace.CommandRequest{Type: "REDO"})
	check(1)
	// Captured group identity and its saved relative pose remain formal and unchanged.
	for _, c := range p.Product.Constraints {
		if c.ID == group && (c.GroupCapturePending || len(c.GroupRelations) != 2) {
			t.Fatal("group capture corrupted", c)
		}
	}
}
