package control

import (
	"context"
	"fmt"
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

func TestMechanismAnalysisThroughRouter(t *testing.T) {
	var mode atomic.Int32
	started, release := make(chan struct{}, 1), make(chan struct{})
	f := newCompositionControlFixture(t, grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		r, ok := req.(*workerv1.SolveAssemblyRequest)
		if ok && strings.HasPrefix(r.RequestId, "mechanism-analysis/") {
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
				return status.Error(codes.Unavailable, "injected analysis failure")
			}
		}
		return invoke(ctx, method, req, reply, cc, opts...)
	}))
	part := f.importPart(t, "cylinder-r6-h12")
	p, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Read-only saved mechanisms"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(req workspace.CommandRequest) {
		t.Helper()
		seq++
		req.ActorID = f.actor
		req.RequestID = fmt.Sprintf("%s-analysis-%d", p.Document.ID, seq)
		next, e := f.service.ApplyCommand(t.Context(), p.Document.ID, req)
		if e != nil {
			t.Fatal(req.Type, e)
		}
		p = next
	}
	for i := 0; i < 3; i++ {
		apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	a, b, environment := p.Product.Instances[0].ID, p.Product.Instances[1].ID, p.Product.Instances[2].ID
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FixMode: "SPACE", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: environment, Kind: "BODY"}})
	end := func(id string) workspace.JointEndpoint {
		return workspace.JointEndpoint{InstanceID: id, Frame: workspace.InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, Axis: &workspace.AssemblyGeometryRef{InstanceID: id, Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Z"}, Plane: &workspace.AssemblyGeometryRef{InstanceID: id, Kind: "PLANE", GeometryID: "datum-xy"}}
	}
	second := end(b)
	m := workspace.Mechanism{ID: "analysis-revolute", Name: "Ground and revolute", UnitIDs: []string{a, b}, Joints: []workspace.MechanismJoint{{ID: "analysis-ground", Name: "Ground", Kind: "GROUND", First: end(a)}, {ID: "analysis-rev", Name: "Revolute", Kind: "REVOLUTE", First: end(a), Second: &second, Coordinates: []workspace.JointCoordinate{{ID: "analysis-rev-angle", Role: "AXIAL_ROTATION", Dimension: "ANGLE", Direction: 1, Zero: workspace.MotionQuantity{Unit: "deg"}}}}}}
	cylinder := m
	cylinder.ID = "analysis-cylinder"
	cylinder.Name = "Cylinder two commands"
	cylinder.Joints = append([]workspace.MechanismJoint(nil), m.Joints...)
	cylinder.Joints[0].ID = "cylinder-ground"
	cylinder.Joints[1].ID = "cylinder-joint"
	cylinder.Joints[1].Kind = "CYLINDRICAL"
	cylinder.Joints[1].Coordinates = []workspace.JointCoordinate{{ID: "cylinder-angle", Role: "AXIAL_ROTATION", Dimension: "ANGLE", Direction: 1, Zero: workspace.MotionQuantity{Unit: "deg"}}, {ID: "cylinder-slide", Role: "AXIAL_TRANSLATION", Dimension: "LENGTH", Direction: 1, Zero: workspace.MotionQuantity{Unit: "mm"}}}
	rigid := workspace.Mechanism{ID: "analysis-rigid", Name: "Ungrounded rigid pair", UnitIDs: []string{a, b}, Joints: []workspace.MechanismJoint{{ID: "rigid-joint", Name: "Rigid", Kind: "RIGID", First: end(a), Second: &second}}}
	for i := range cylinder.Joints[1].Coordinates {
		c := &cylinder.Joints[1].Coordinates[i]
		c.Lower = &workspace.MotionQuantity{Value: -180, Unit: c.Zero.Unit}
		c.Upper = &workspace.MotionQuantity{Value: 180, Unit: c.Zero.Unit}
	}
	defs := workspace.KinematicsDefinitions{Mechanisms: []workspace.Mechanism{m, cylinder, rigid}, Drivers: []workspace.MotionDriver{{ID: "angle-command", Name: "Rotation", MechanismID: cylinder.ID, JointID: "cylinder-joint", CoordinateID: "cylinder-angle"}, {ID: "slide-command", Name: "Translation", MechanismID: cylinder.ID, JointID: "cylinder-joint", CoordinateID: "cylinder-slide"}}}
	apply(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: p.Document.VersionID, Kinematics: &defs})
	type snapshot struct {
		Head, Model                            string
		Commands, Versions, Results, Manifests int
	}
	state := func() snapshot {
		t.Helper()
		var v snapshot
		var raw []byte
		e := f.db.QueryRow(t.Context(), `SELECT d.head_version_id::text,v.model_json,(SELECT count(*) FROM occccad.commands c WHERE c.document_id=d.id),(SELECT count(*) FROM occccad.document_versions r WHERE r.document_id=d.id),(SELECT count(*) FROM occccad.product_solve_results r JOIN occccad.product_solve_manifests m ON m.digest=r.manifest_digest WHERE m.root_product_document_id=d.id),(SELECT count(*) FROM occccad.product_solve_manifests m WHERE m.root_product_document_id=d.id) FROM occccad.documents d JOIN occccad.document_versions v ON v.id=d.head_version_id WHERE d.id=$1`, p.Document.ID).Scan(&v.Head, &raw, &v.Commands, &v.Versions, &v.Results, &v.Manifests)
		if e != nil {
			t.Fatal(e)
		}
		v.Model = string(raw)
		return v
	}
	baseline := state()
	for _, tc := range []struct {
		id              string
		relative, gauge uint64
	}{{m.ID, 1, 0}, {cylinder.ID, 2, 0}, {rigid.ID, 0, 6}} {
		for _, numeric := range []bool{false, true} {
			report, e := f.service.AnalyzeMechanism(t.Context(), p.Document.ID, p.Document.VersionID, tc.id, numeric)
			if e != nil {
				t.Fatal(e)
			}
			if len(report.MemberIDs) != 2 || len(report.Joints) == 0 || report.InputDigest == "" || report.PolicyDigest == "" {
				t.Fatalf("incomplete report %+v", report)
			}
			for _, r := range report.Joints {
				if r.SupportStatus != "VALID" {
					t.Fatalf("supports: %+v", r)
				}
			}
			if numeric {
				if !report.Evidence.Available || len(report.Evidence.Components) != 1 || report.Evidence.Components[0].RelativeDof != tc.relative || report.Evidence.Components[0].GaugeDof != tc.gauge {
					t.Fatalf("%s physical evidence %+v", tc.id, report.Evidence)
				}
			}
			if tc.id == cylinder.ID && len(report.Joints[1].Drivers) != 2 {
				t.Fatal("lost second command", report.Joints)
			}
			if after := state(); !reflect.DeepEqual(baseline, after) {
				t.Fatal("read-only analysis changed model/poses/zeros/history/solve records")
			}
		}
	}
	mode.Store(2)
	if _, e := f.service.AnalyzeMechanism(t.Context(), p.Document.ID, p.Document.VersionID, m.ID, true); e == nil {
		t.Fatal("infrastructure failure swallowed")
	}
	if !reflect.DeepEqual(baseline, state()) {
		t.Fatal("failure wrote state")
	}
	mode.Store(1)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, e := f.service.AnalyzeMechanism(ctx, p.Document.ID, p.Document.VersionID, m.ID, true)
		done <- e
	}()
	<-started
	cancel()
	if e := <-done; e == nil {
		t.Fatal("cancel accepted")
	}
	if !reflect.DeepEqual(baseline, state()) {
		t.Fatal("cancel wrote state")
	}
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	go func() {
		_, e := f.service.AnalyzeMechanism(ctx, p.Document.ID, p.Document.VersionID, m.ID, true)
		done <- e
	}()
	<-started
	old := p.Document.VersionID
	visible := false
	apply(workspace.CommandRequest{Type: "SET_DEFINITION_VISIBILITY", TargetKind: "ASSEMBLY_CONSTRAINT", TargetID: p.Product.Constraints[0].ID, Visible: visible})
	close(release)
	if e := <-done; e == nil {
		t.Fatal("late response accepted")
	}
	mode.Store(0)
	if _, e := f.service.AnalyzeMechanism(t.Context(), p.Document.ID, old, m.ID, false); e == nil {
		t.Fatal("stale revision accepted")
	}
	// A fresh service reconstructs the analysis exclusively from accepted definitions.
	fresh := workspace.NewWithArtifacts(f.db, f.client, f.store)
	r, e := fresh.AnalyzeMechanism(t.Context(), p.Document.ID, p.Document.VersionID, cylinder.ID, true)
	if e != nil || !r.Evidence.Available {
		t.Fatal("reopen", r.Evidence, e)
	}
}
