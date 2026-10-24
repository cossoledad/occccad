package control

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAssociationUpdateAndRepairThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	part, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Shared mounting block"})
	if e != nil {
		t.Fatal(e)
	}
	product, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PRODUCT", Name: "Direct Part association repair"})
	if e != nil {
		t.Fatal(e)
	}
	seq := 0
	apply := func(v *workspace.DocumentView, r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-association-%d", v.Document.ID, seq)
		next, e := service.ApplyCommand(t.Context(), v.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		*v = next
	}
	apply(&part, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := part.Part.Features[0].ID
	apply(&part, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	apply(&part, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	base := part.Part.Features[len(part.Part.Features)-1]
	originalPart := part.Document.VersionID
	for i := 0; i < 3; i++ {
		apply(&product, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: fmt.Sprintf("Block %d", i+1)})
	}
	ids := []string{product.Product.Instances[0].ID, product.Product.Instances[1].ID, product.Product.Instances[2].ID}
	for i, id := range ids {
		apply(&product, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: id, Translation: [3]float64{float64(i) * 40, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}})
	}
	apply(&product, workspace.CommandRequest{Type: "SET_REFERENCE_MODE", InstanceID: ids[2], ReferenceMode: "PINNED"})
	for _, id := range []string{ids[0], ids[2]} {
		apply(&product, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FixMode: "SPACE", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: id, Kind: "BODY"}})
	}
	fixed := []string{product.Product.Constraints[0].ID, product.Product.Constraints[1].ID}
	pick := findP7TopologyPick(t, service, part, "FACE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == base.ID && strings.HasPrefix(s.Anchor.OutputSlot, "END_CAP/")
	})
	apply(&product, p7Coincident(p7AssemblyPick(ids[0], pick), p7AssemblyPick(ids[1], pick)))
	mate := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	initialPoses := append([]workspace.ProductInstance(nil), product.Product.Instances...)
	plan := func() workspace.ProductUpdatePlan {
		t.Helper()
		p, e := service.GetProductUpdatePlan(t.Context(), product.Document.ID)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	accept := func(p workspace.ProductUpdatePlan) error {
		seq++
		next, e := service.AcceptParametricProductUpdate(t.Context(), product.Document.ID, fmt.Sprintf("%s-accept-%d", product.Document.ID, seq), p.Digest, p6Actor, func(string, bool) error { return nil })
		if e == nil {
			product = next
		}
		return e
	}
	verify := func(want string, status modelcore.AssemblyConstraintEvaluationStatus) {
		t.Helper()
		for i, v := range product.Product.Instances {
			version := want
			if i == 2 {
				version = originalPart
			}
			if v.ID != ids[i] || v.ReferencedVersionID != version || v.Translation != initialPoses[i].Translation || v.Rotation != initialPoses[i].Rotation {
				t.Fatal("instance identity/version/pose confused", v)
			}
		}
		for _, id := range fixed {
			for _, c := range product.Product.Constraints {
				if c.ID == id && c.EvaluationStatus != modelcore.AssemblyConstraintVerified {
					t.Fatal("unrelated FIX invalidated", c)
				}
			}
		}
		assertP7Constraint(t, product, mate, status, map[bool]modelcore.SupportingElementStatus{true: modelcore.SupportingElementConnected, false: modelcore.SupportingElementNotConnected}[status == modelcore.AssemblyConstraintVerified])
	}
	editLength := func(value float64) {
		t.Helper()
		current := base
		current.Length = value
		apply(&part, workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: base.ID, Feature: &current, ExpectedFeatureDigest: findP6FeatureDigest(t, part.StructureTree, base.ID)})
	}
	editLength(12)
	stale := plan()
	accepted := product.Document.VersionID
	// Conservative affected IDs are not a candidate solve result; accepted mate is still Verified.
	if !stale.HasUpdates || !stale.CanAccept || len(stale.AffectedConstraintIDs) != 3 {
		t.Fatal("unexpected conservative plan", stale)
	}
	verify(originalPart, modelcore.AssemblyConstraintVerified)
	editLength(14)
	if e = accept(stale); e == nil || !strings.Contains(e.Error(), "PRODUCT_UPDATE_PLAN_STALE") {
		t.Fatal("old plan accepted", e)
	}
	unchanged, _ := service.GetDocument(t.Context(), product.Document.ID, p6Actor)
	if unchanged.Document.VersionID != accepted {
		t.Fatal("stale update advanced Head")
	}
	if e = accept(plan()); e != nil {
		t.Fatal(e)
	}
	verify(part.Document.VersionID, modelcore.AssemblyConstraintVerified)
	apply(&part, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 7}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	apply(&part, workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: part.Part.DatumPlanes[len(part.Part.DatumPlanes)-1].ID})
	cutSketch := part.Part.Features[len(part.Part.Features)-1].ID
	apply(&part, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: cutSketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -10, Y: -10}, Second: &workspace.SketchPoint2{X: 10, Y: 10}}}})
	apply(&part, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: cutSketch, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: 7})
	cut := part.Part.Features[len(part.Part.Features)-1]
	if e = accept(plan()); e != nil {
		t.Fatal("topology changes must allow Broken acceptance", e)
	}
	verify(part.Document.VersionID, modelcore.AssemblyConstraintBroken)
	broken := plan()
	if broken.HasUpdates || !broken.CanAccept {
		t.Fatal("Broken creates update loop/gate", broken)
	}
	for _, entry := range broken.Entries {
		if entry.Kind == "ASSEMBLY_SOLVE" && (entry.AcceptedRevisionID != product.Document.VersionID || entry.CandidateRevisionID != product.Document.VersionID) {
			t.Fatal("accepted-state diagnostic impersonates candidate solve")
		}
	}
	replacement := findP7TopologyPick(t, service, part, "FACE", func(p workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.Anchor.FeatureID == cut.ID && p.GeometryType == "PLANE" && math.Abs(s.CreationEvidence.Centroid[2]-7) < 1e-6
	})
	request := workspace.CommandRequest{ActorID: p6Actor, RequestID: product.Document.ID + "-repair", Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: mate, FirstAssemblyRef: p7AssemblyPick(ids[0], replacement), SecondAssemblyRef: p7AssemblyPick(ids[1], replacement)}
	head := product.Document.VersionID
	preview, e := service.PreviewCommand(t.Context(), product.Document.ID, request)
	if e != nil {
		t.Fatal("repair preview", e)
	}
	now, _ := service.GetDocument(t.Context(), product.Document.ID, p6Actor)
	if now.Document.VersionID != head {
		t.Fatal("cancel/preview wrote Head")
	}
	request.PreviewID = preview.PreviewID
	next, e := service.ApplyCommand(t.Context(), product.Document.ID, request)
	if e != nil {
		t.Fatal("repair", e)
	}
	product = next
	verify(part.Document.VersionID, modelcore.AssemblyConstraintVerified)
	apply(&product, workspace.CommandRequest{Type: "UNDO"})
	verify(part.Document.VersionID, modelcore.AssemblyConstraintBroken)
	apply(&product, workspace.CommandRequest{Type: "REDO"})
	verify(part.Document.VersionID, modelcore.AssemblyConstraintVerified)
	// Failed source update retains the accepted legal Product and its poses.
	accepted = product.Document.VersionID
	validPart := part.Document.VersionID
	edge := part.Part.Features[0].Sketch.Entities[0].ID
	apply(&part, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{edge}}}})
	blocked := plan()
	if blocked.CanAccept || !blocked.HasUpdates {
		t.Fatal("failed source was not blocked", blocked)
	}
	for _, entry := range blocked.Entries {
		if entry.Kind == "OCCURRENCE_REFERENCE" && (entry.DiagnosticCode != "REFERENCE_PART_NOT_READY" || !strings.Contains(entry.Diagnostic, base.ID)) {
			t.Fatal("missing actual source failure", entry)
		}
	}
	if e = accept(blocked); e == nil {
		t.Fatal("failed source revision was accepted")
	}
	now, e = service.GetDocument(t.Context(), product.Document.ID, p6Actor)
	if e != nil || now.Document.VersionID != accepted || !reflect.DeepEqual(now.Product.Constraints, product.Product.Constraints) {
		t.Fatal("failed update changed accepted constraint evidence", e)
	}
	for i, instance := range now.Product.Instances {
		previous := product.Product.Instances[i]
		// HeadChanged is current read-model availability, not accepted model state.
		instance.HeadChanged, previous.HeadChanged = false, false
		if !reflect.DeepEqual(instance, previous) {
			t.Fatal("failed update changed an accepted instance", instance)
		}
	}
	product = now
	verify(validPart, modelcore.AssemblyConstraintVerified)
	apply(&part, workspace.CommandRequest{Type: "UNDO"})
	if e = accept(plan()); e != nil {
		t.Fatal("retry after source recovery", e)
	}
	verify(part.Document.VersionID, modelcore.AssemblyConstraintVerified)
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	opened, e := fresh.GetDocument(t.Context(), product.Document.ID, p6Actor)
	if e != nil || !reflect.DeepEqual(opened.Product, product.Product) {
		t.Fatal("reopen", e)
	}
}
