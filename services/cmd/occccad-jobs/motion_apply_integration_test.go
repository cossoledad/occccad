package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/control"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/geometryrpc"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
)

func TestMotionApplyGeometryFrameAtomicHistory(t *testing.T) {
	binary := os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN")
	if binary == "" {
		t.Fatal("matching-source C++ Worker required")
	}
	db := testsupport.OpenDevelopmentDatabase(t)
	root := testsupport.Environment(t).DataDirectory
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	pool := control.NewGeometryPool(t.Context(), control.GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: root, LogDirectory: testsupport.Environment(t).LogDirectory, MinimumWorkers: 1, MaximumWorkers: 1, GeometryCapacity: 100})
	defer pool.Close()
	if e = pool.Start(); e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	router := grpc.NewServer(geometryrpc.ServerOptions()...)
	defer router.Stop()
	workerv1.RegisterGeometryWorkerServer(router, pool)
	go router.Serve(listener)
	store, local, e := artifact.OpenConfigured(t.Context(), root)
	if e != nil {
		t.Fatal(e)
	}
	client, e := geometry.Open(listener.Addr().String(), geometry.ArtifactStaging(store, local))
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	objects := artifact.NewService(db, store, local)
	service := workspace.NewWithArtifacts(db, client, objects)
	queue := jobs.New(db)
	actor := "00000000-0000-7000-8000-000000000001"

	view, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, RequestID: uuid.NewString(), Name: "propeller-conversion", Type: "PRODUCT"})
	if e != nil {
		t.Fatal(e)
	}
	view, e = buildShaftRotorFixture(t.Context(), service, view.Document.ID, actor, view.Document.VersionID)
	if e != nil {
		t.Fatal("geometry fixture", e)
	}
	if len(view.Product.Instances) != 2 || len(view.Product.Constraints) != 0 {
		t.Fatal("fixture bypassed generic definitions")
	}
	command := func(r workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		r.ActorID = actor
		r.RequestID = uuid.NewString()
		v, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
		if e != nil {
			t.Fatal("command", r.Type, e)
		}
		return v
	}
	// Locate actual precise planar/cylindrical topology. These picks are transient
	// input evidence only; SAVE binds Naming references and drops all local IDs.
	ends := []workspace.JointEndpoint{}
	for _, v := range view.ResolvedInstances {
		axis, plane := workspace.AssemblyGeometryRef{}, workspace.AssemblyGeometryRef{}
		for id := uint64(1); id < 24; id++ {
			prop, er := service.GetTopologyElementPropertiesAtVersion(t.Context(), v.InstancePath.Segments[len(v.InstancePath.Segments)-1].ReferencedDocumentID, v.InstancePath.Segments[len(v.InstancePath.Segments)-1].ResolvedVersionID, v.GeometryKey, "FACE", id)
			if er != nil {
				continue
			}
			pick := workspace.AssemblyGeometryRef{InstanceID: v.InstancePath.Segments[0].InstanceID, InstancePath: &v.InstancePath, Kind: "FACE", GeometryKey: v.GeometryKey, TopologyID: id}
			radius, _ := prop.Properties["radius"].(float64)
			expected := 4.
			if v.InstancePath.Segments[0].InstanceID != view.Product.Instances[0].ID {
				expected = 4.5
			}
			if prop.GeometryType == "CYLINDER" && axis.Kind == "" && math.Abs(radius-expected) < 1e-7 {
				pick.DerivedRole = "cylinder-axis"
				axis = pick
			}
			if prop.GeometryType == "PLANE" && plane.Kind == "" {
				pick.DerivedRole = ""
				inspection, ee := service.InspectAssemblySupports(t.Context(), view.Document.ID, []workspace.AssemblyGeometryRef{pick})
				if ee == nil && len(inspection.Supports) == 1 && inspection.Supports[0].Descriptor != nil && math.Abs(inspection.Supports[0].Descriptor.Direction[2]) > 1-1e-8 {
					plane = pick
				}
			}
		}
		if axis.Kind == "" || plane.Kind == "" {
			t.Fatalf("missing real supports %s axis=%+v plane=%+v", v.Name, axis, plane)
		}
		ends = append(ends, workspace.JointEndpoint{InstanceID: axis.InstanceID, Frame: workspace.InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, Axis: &axis, Plane: &plane})
	}
	if len(ends) != 2 {
		t.Fatal("expected two exact bodies", len(ends))
	}
	// Keep first endpoint on the shaft, independent of resolved traversal order.
	if ends[0].InstanceID != view.Product.Instances[0].ID {
		ends[0], ends[1] = ends[1], ends[0]
	}
	mid, jid, did, sid := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	k := workspace.KinematicsDefinitions{Mechanisms: []workspace.Mechanism{{ID: mid, Name: "shaft-propeller", UnitIDs: []string{ends[0].InstanceID, ends[1].InstanceID}, Joints: []workspace.MechanismJoint{{ID: uuid.NewString(), Name: "shaft grounded", Kind: "GROUND", First: workspace.JointEndpoint{InstanceID: ends[0].InstanceID, Frame: workspace.InstancePose{Rotation: [4]float64{0, 0, 0, 1}}}}, {ID: jid, Name: "geometric revolute", Kind: "REVOLUTE", First: ends[0], Second: &ends[1], AxialOffset: workspace.MotionQuantity{Value: -10, Unit: "mm"}, Coordinates: []workspace.JointCoordinate{{ID: jid + "-coordinate", Role: "AXIAL_ROTATION", Dimension: "ANGLE", Zero: workspace.MotionQuantity{Unit: "deg"}, Direction: 1, Lower: &workspace.MotionQuantity{Value: -720, Unit: "deg"}, Upper: &workspace.MotionQuantity{Value: 720, Unit: "deg"}}}}}}}, Drivers: []workspace.MotionDriver{{ID: did, Name: "angle", MechanismID: mid, JointID: jid, CoordinateID: jid + "-coordinate"}}}
	t.Run("mechanism command preview isolates formal constraints", func(t *testing.T) {
		originalHead := view.Document.VersionID
		fixRef := workspace.AssemblyGeometryRef{InstanceID: ends[1].InstanceID, Kind: "BODY"}
		view = command(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &fixRef, FixMode: "SPACE"})
		head := view.Document.VersionID
		raw, _ := json.Marshal(k.Mechanisms[0])
		var mechanism workspace.Mechanism
		json.Unmarshal(raw, &mechanism)
		for i := range mechanism.Joints {
			for _, end := range []*workspace.JointEndpoint{&mechanism.Joints[i].First, mechanism.Joints[i].Second} {
				if end == nil {
					continue
				}
				for _, ref := range []*workspace.AssemblyGeometryRef{end.Axis, end.Plane} {
					if ref != nil && ref.InstancePath != nil {
						ref.InstancePath.Segments[0].OwnerVersionID = head
					}
				}
			}
		}
		complete := mechanism
		raw, _ = json.Marshal(complete)
		mechanism = workspace.Mechanism{}
		json.Unmarshal(raw, &mechanism)
		mechanism.Joints[1].First.Plane = nil
		mechanism.Joints[1].Second.Plane = nil
		req := workspace.MechanismPreviewRequest{BaseRevisionID: head, Mechanism: mechanism, DraftJointID: jid}
		partial, err := service.PreviewMechanism(t.Context(), view.Document.ID, req)
		if err != nil || len(partial.InstancePoses) != 2 {
			t.Fatal("two-axis preview", err, partial)
		}
		for _, pose := range partial.InstancePoses {
			if pose.InstanceID == ends[1].InstanceID && (math.Abs(pose.Translation[0]) > 1e-7 || math.Abs(pose.Translation[1]) > 1e-7) {
				t.Fatal("axes did not align", pose)
			}
		}
		req.Mechanism = complete
		full, err := service.PreviewMechanism(t.Context(), view.Document.ID, req)
		if err != nil || len(full.InstancePoses) != 2 {
			t.Fatal("complete joint preview", err, full)
		}
		for _, pose := range full.InstancePoses {
			if pose.InstanceID == ends[1].InstanceID && (math.Abs(pose.Translation[0]) > 1e-7 || math.Abs(pose.Translation[1]) > 1e-7 || math.Abs(pose.Translation[2]-10) > 1e-7) {
				t.Fatal("axial location incorrect", pose)
			}
		}
		// The application workflow picks the moving rotor first and grounded shaft
		// second; the same two persisted relationships determine the whole pose.
		rotorFirst := complete
		rotorBytes, _ := json.Marshal(complete)
		rotorFirst = workspace.Mechanism{}
		json.Unmarshal(rotorBytes, &rotorFirst)
		rotorJoint := &rotorFirst.Joints[1]
		rotorJoint.First = *rotorJoint.Second
		// Copy the old first endpoint before assigning so references do not alias.
		shaftEnd := complete.Joints[1].First
		rotorJoint.Second = &shaftEnd
		rotorJoint.AxialOffset.Value = 10
		rotorPreview, err := service.PreviewMechanism(t.Context(), view.Document.ID, workspace.MechanismPreviewRequest{BaseRevisionID: head, Mechanism: rotorFirst, DraftJointID: jid})
		if err != nil {
			t.Fatal("moving-first rotor workflow", err)
		}
		for _, p := range rotorPreview.InstancePoses {
			if p.InstanceID == ends[1].InstanceID && (math.Abs(p.Translation[0]) > 1e-7 || math.Abs(p.Translation[1]) > 1e-7 || math.Abs(p.Translation[2]-10) > 1e-7) {
				t.Fatal("rotor workflow pose", p)
			}
		}
		// Without Ground, the first pick moves and the second pick retains its
		// nominal pose. This is the ordinary assembly creation hierarchy.
		ordered := req
		ordered.Mechanism.Joints = []workspace.MechanismJoint{complete.Joints[1]}
		orderedPreview, err := service.PreviewMechanism(t.Context(), view.Document.ID, ordered)
		if err != nil {
			t.Fatal("selection motion priority", err)
		}
		for _, p := range orderedPreview.InstancePoses {
			if p.InstanceID != complete.Joints[1].Second.InstanceID {
				continue
			}
			for _, nominal := range view.Product.Instances {
				if nominal.ID != p.InstanceID {
					continue
				}
				for k := 0; k < 3; k++ {
					if math.Abs(p.Translation[k]-nominal.Translation[k]) > 1e-7 {
						t.Fatal("second pick moved", p, nominal)
					}
				}
			}
		}
		unchanged, err := service.GetDocument(t.Context(), view.Document.ID)
		if err != nil || unchanged.Document.VersionID != head || len(unchanged.Product.Constraints) != 1 || len(unchanged.Product.Kinematics.Mechanisms) != 0 || !reflect.DeepEqual(unchanged.Product.Instances, view.Product.Instances) {
			t.Fatal("preview modified formal model", err)
		}
		req.Mechanism.UnitIDs = append(req.Mechanism.UnitIDs, "missing-preview-unit")
		if _, err = service.PreviewMechanism(t.Context(), view.Document.ID, req); err == nil {
			t.Fatal("declared invalid motion unit silently discarded")
		}
		req.Mechanism = complete
		req.Mechanism.SupplementalConstraintIDs = []string{view.Product.Constraints[0].ID}
		if _, err = service.PreviewMechanism(t.Context(), view.Document.ID, req); err == nil {
			t.Fatal("explicit conflicting supplemental Fix silently ignored")
		}
		req.Mechanism.SupplementalConstraintIDs = nil
		req.BaseRevisionID = originalHead
		if _, err = service.PreviewMechanism(t.Context(), view.Document.ID, req); err == nil {
			t.Fatal("stale preview accepted")
		}
		req.BaseRevisionID = head
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err = service.PreviewMechanism(ctx, view.Document.ID, req); err == nil {
			t.Fatal("canceled preview accepted")
		}
		view = command(workspace.CommandRequest{Type: "UNDO"})
		if len(view.Product.Constraints) != 0 {
			t.Fatal("fixture Fix undo failed")
		}
		for i := range k.Mechanisms[0].Joints {
			for _, end := range []*workspace.JointEndpoint{&k.Mechanisms[0].Joints[i].First, k.Mechanisms[0].Joints[i].Second} {
				if end == nil {
					continue
				}
				for _, ref := range []*workspace.AssemblyGeometryRef{end.Axis, end.Plane} {
					if ref != nil && ref.InstancePath != nil {
						ref.InstancePath.Segments[0].OwnerVersionID = view.Document.VersionID
					}
				}
			}
		}
	})

	view = command(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &k})
	opened, err := service.BeginMechanismSession(t.Context(), view.Document.ID, actor, workspace.MechanismSessionBegin{OwnerID: "fixture", Generation: 1, BaseRevisionID: view.Document.VersionID, MechanismID: mid, ActiveDriverIDs: []string{did}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.RecordMechanismKey(t.Context(), view.Document.ID, actor, opened.SessionID, 0, workspace.MotionKeyframe{ID: "key0", TimeSeconds: 0})
	if err != nil {
		t.Fatal(err)
	}
	qualified, err := service.UpdateMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 1, Values: map[string]workspace.
		MotionQuantity{did: workspace.MotionQuantity{Value: 360, Unit: "deg"}}})
	if err != nil || !qualified.Accepted {
		t.Fatal(qualified, err)
	}
	last, err := service.RecordMechanismKey(t.Context(), view.Document.ID, actor, opened.SessionID, 1, workspace.MotionKeyframe{ID: "key1", TimeSeconds: 4})
	if err != nil {
		t.Fatal(err)
	}
	study := workspace.MotionStudy{ID: sid, Name: "recorded", MechanismID: mid, Keyframes: []workspace.MotionKeyframe{first, last}, Frames: 25, BudgetMS: 60000, ActiveDriverIDs: []string{did}}
	final, err := service.FinalizeMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, workspace.MechanismSessionFinalize{Sequence: 1, Study: &study})
	if err != nil {
		t.Fatal(err)
	}
	view, err = service.ApplyCommand(t.Context(), view.Document.ID, final)
	if err != nil {
		t.Fatal(err)
	}
	service.CancelMechanismSession(view.Document.ID, actor, opened.SessionID)

	k = view.Product.Kinematics
	m := k.Mechanisms[0]
	if len(m.Poses) != len(view.Product.Instances) || len(m.Joints[0].Constraints) != 1 || len(m.Joints[1].Constraints) != 2 {
		t.Fatal("mechanism did not persist its complete constraint system and editing pose", m)
	}
	if m.Joints[1].Constraints[0].Family != "Coincidence" || m.Joints[1].Constraints[1].Family != "Offset" {
		t.Fatal("not ordinary assembly definitions", m.Joints[1].Constraints)
	}
	for _, v := range view.Product.Instances {
		if v.ID == ends[1].InstanceID {
			if v.Translation != ([3]float64{22, 15, 7}) || math.Abs(m.Poses[v.ID].Translation[2]-10) > 1e-7 {
				t.Fatal("mechanism/assembly pose ownership", v, m.Poses[v.ID])
			}
		}
	}
	t.Run("application transitions and conversion without a simulation", func(t *testing.T) {
		original := view.Product.Instances
		activate := func() {
			view = command(workspace.CommandRequest{Type: "ACTIVATE_MECHANISM", VersionID: view.Document.VersionID, MechanismApplication: &workspace.MechanismApplicationRequest{MechanismID: mid}})
		}
		activate()
		acceptedHead := view.Document.VersionID
		noop := workspace.CommandRequest{Type: "ACTIVATE_MECHANISM", ActorID: actor, RequestID: uuid.NewString(), VersionID: acceptedHead, MechanismApplication: &workspace.MechanismApplicationRequest{MechanismID: mid, Resume: true}}
		for range 2 {
			var er error
			view, er = service.ApplyCommand(t.Context(), view.Document.ID, noop)
			if er != nil || view.Document.VersionID != acceptedHead {
				t.Fatal("no-op activation created history or lost retry", er)
			}
		}
		var audits int
		if er := db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.commands WHERE request_id=$1`, noop.RequestID).Scan(&audits); er != nil || audits != 1 {
			t.Fatal("no-op audit not idempotent", er, audits)
		}
		empty := view.Product.Kinematics
		empty.Mechanisms = append(append([]workspace.Mechanism(nil), empty.Mechanisms...), workspace.Mechanism{ID: uuid.NewString(), Name: "empty", Joints: []workspace.MechanismJoint{}, UnitIDs: []string{}})
		view = command(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &empty})
		switchCandidate, er := service.PrepareMechanismApplication(t.Context(), view.Document.ID, workspace.MechanismApplicationRequest{BaseRevisionID: view.Document.VersionID, MechanismID: empty.Mechanisms[1].ID, SourceMechanismIDs: []string{mid}})
		if er != nil || switchCandidate.Mechanism == nil || len(switchCandidate.Mechanism.Poses) != 0 {
			t.Fatal("empty target owns environment", er)
		}
		for _, p := range switchCandidate.InstancePoses {
			if p.InstanceID == ends[1].InstanceID && math.Abs(p.Translation[2]-10) > 1e-7 {
				t.Fatal("empty application restored Product baseline", p)
			}
		}
		view = command(workspace.CommandRequest{Type: "UNDO"})
		evaluated := view.Product.Kinematics.Mechanisms[0].Evaluation
		if evaluated == nil || evaluated.RelativeDOF != 1 || evaluated.GaugeDOF != 0 || evaluated.DriverCount != 1 {
			t.Fatal("physical and command DOF conflated", evaluated)
		}
		if !reflect.DeepEqual(view.Product.Instances, original) {
			t.Fatal("activation wrote Product positions")
		}
		view = command(workspace.CommandRequest{Type: "EXIT_MECHANISM", VersionID: view.Document.VersionID, MechanismApplication: &workspace.MechanismApplicationRequest{SourceMechanismID: mid}})
		for _, v := range view.Product.Instances {
			p := view.Product.Kinematics.Mechanisms[0].Poses[v.ID]
			if v.Translation != p.Translation || v.Rotation != p.Rotation {
				t.Fatal("exit restored old Product pose", v, p)
			}
		}
		reopened, err := service.GetDocument(t.Context(), view.Document.ID)
		if err != nil || !reflect.DeepEqual(reopened.Product.Instances, view.Product.Instances) {
			t.Fatal("exit pose did not survive reopen", err)
		}
		view = command(workspace.CommandRequest{Type: "UNDO"})
		if !reflect.DeepEqual(view.Product.Instances, original) {
			t.Fatal("pose-only exit undo", view.Product.Instances)
		}
		view = command(workspace.CommandRequest{Type: "REDO"})
		view = command(workspace.CommandRequest{Type: "UNDO"})
		fixRef := workspace.AssemblyGeometryRef{InstanceID: ends[1].InstanceID, Kind: "BODY"}
		view = command(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &fixRef, FixMode: "SPACE"})
		fixed := view.Product.Instances
		activate()
		for _, v := range fixed {
			if v.ID == ends[1].InstanceID && v.Translation == view.Product.Kinematics.Mechanisms[0].Poses[v.ID].Translation {
				t.Fatal("Product Fix entered mechanism equations")
			}
		}
		view = command(workspace.CommandRequest{Type: "EXIT_MECHANISM", VersionID: view.Document.VersionID, MechanismApplication: &workspace.MechanismApplicationRequest{SourceMechanismID: mid}})
		if !reflect.DeepEqual(view.Product.Instances, fixed) || len(view.Product.Constraints) != 1 {
			t.Fatal("exit did not solve destination Product constraints")
		}
		// Undo activation (exit retained the already fixed Product position), then Fix.
		view = command(workspace.CommandRequest{Type: "UNDO"})
		if len(view.Product.Constraints) > 0 {
			view = command(workspace.CommandRequest{Type: "UNDO"})
		}
		if len(view.Product.Constraints) > 0 {
			view = command(workspace.CommandRequest{Type: "UNDO"})
		}
		if len(view.Product.Constraints) != 0 {
			t.Fatal("fixture cleanup retained Fix")
		}
		independent := view.Product.Kinematics
		independent.Drivers = nil
		independent.Studies = nil
		view = command(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &independent})
		plan, err := service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, workspace.MotionApplyRequest{MechanismID: mid})
		if err != nil || !plan.Ready || plan.DegreesOfFreedom != 1 || len(plan.Items) != 3 {
			t.Fatal("accepted mechanism conversion required a run", err, plan.Items)
		}
		view = command(workspace.CommandRequest{Type: "UNDO"})
		k = view.Product.Kinematics
		m = k.Mechanisms[0]
	})
	t.Run("removing relationships preserves owned free units", func(t *testing.T) {
		var free workspace.KinematicsDefinitions
		raw, _ := json.Marshal(view.Product.Kinematics)
		json.Unmarshal(raw, &free)
		free.Mechanisms[0].Joints = nil
		free.Mechanisms[0].Evaluation = nil
		free.Drivers, free.Studies = nil, nil
		view = command(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &free})
		accepted := view.Product.Kinematics.Mechanisms[0]
		if accepted.Evaluation == nil {
			t.Fatal("free units have no authoritative DOF")
		}
		if accepted.Evaluation.RelativeDOF+accepted.Evaluation.GaugeDOF != 12 || accepted.Evaluation.DriverCount != 0 {
			t.Fatal("incorrect free-unit DOF", *accepted.Evaluation)
		}
		for id, p := range accepted.Poses {
			old := m.Poses[id]
			for i, x := range p.Translation {
				if math.Abs(x-old.Translation[i]) > 1e-7 {
					t.Fatal("free unit moved when its relationships were removed", id, old, p)
				}
			}
		}
		view = command(workspace.CommandRequest{Type: "EXIT_MECHANISM", VersionID: view.Document.VersionID, MechanismApplication: &workspace.MechanismApplicationRequest{SourceMechanismID: mid}})
		for _, v := range view.Product.Instances {
			if p := accepted.Poses[v.ID]; v.Translation != p.Translation || v.Rotation != p.Rotation {
				t.Fatal("relationship removal restored an old Product pose", v)
			}
		}
		view = command(workspace.CommandRequest{Type: "UNDO"})
		view = command(workspace.CommandRequest{Type: "UNDO"})
	})
	// Saving another definition must keep the accepted mechanism pose and pair.
	reopened, e := service.GetDocument(t.Context(), view.Document.ID)
	if e != nil || !reflect.DeepEqual(reopened.Product.Kinematics.Mechanisms[0], m) {
		t.Fatal("reopened editing baseline", e)
	}
	// Constraint and accepted pose are one undoable mechanism edit; Product
	// placement and Product constraints remain independent throughout.
	delta := view.Product.Kinematics
	// Detach the payload from the current view before changing its offset.
	rawDelta, _ := json.Marshal(delta)
	delta = workspace.KinematicsDefinitions{}
	json.Unmarshal(rawDelta, &delta)
	delta.Mechanisms[0].Joints[1].AxialOffset.Value = -12
	changed := command(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &delta})
	if math.Abs(changed.Product.Kinematics.Mechanisms[0].Poses[ends[1].InstanceID].Translation[2]-12) > 1e-7 || len(changed.Product.Constraints) != 0 {
		t.Fatal("persistent constraints failed to determine editing pose", changed.Product.Kinematics)
	}
	view = command(workspace.CommandRequest{Type: "UNDO"})
	if !reflect.DeepEqual(view.Product.Kinematics.Mechanisms[0], m) {
		t.Fatal("mechanism undo did not restore constraints and pose together")
	}
	k = view.Product.Kinematics
	for _, bad := range []workspace.MotionRunRequest{{CurrentOnly: true}, {AnalysisID: "analysis"}, {DriverID: did, Trial: &workspace.MotionQuantity{Unit: "deg"}}} {
		bad.BaseRevisionID = view.Document.VersionID
		if _, err := service.FreezeMotionStudy(t.Context(), view.Document.ID, bad); err == nil {
			t.Fatal("retired motion capability accepted", bad)
		}
	}
	badDefinitions := view.Product.Kinematics
	badBytes, _ := json.Marshal(badDefinitions)
	badDefinitions = workspace.KinematicsDefinitions{}
	json.Unmarshal(badBytes, &badDefinitions)
	badDefinitions.Mechanisms[0].Joints[1].Kind = "PRISMATIC"
	if _, err := service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, ActorID: actor, RequestID: uuid.NewString(), Kinematics: &badDefinitions}); err == nil {
		t.Fatal("retired joint accepted")
	}
	for _, end := range []workspace.JointEndpoint{k.Mechanisms[0].Joints[1].First, *k.Mechanisms[0].Joints[1].Second} {
		if end.Axis.PersistentSelection == nil || end.Plane.PersistentSelection == nil || end.Axis.TopologyID != 0 || end.Axis.GeometryKey != "" || end.CapturedX == nil {
			t.Fatal("transient pick persisted", end)
		}
	}
	snapshot, er := service.FreezeMotionStudy(t.Context(), view.Document.ID, workspace.MotionRunRequest{BaseRevisionID: view.Document.VersionID, StudyID: sid})
	if er != nil {
		t.Fatal("freeze", er)
	}
	b, _ := json.Marshal(snapshot)
	o, er := objects.Put(t.Context(), artifact.Kind("MOTION_SNAPSHOT"), "application/json", bytes.NewReader(b))
	if er != nil {
		t.Fatal(er)
	}
	job, er := queue.Enqueue(t.Context(), jobs.EnqueueRequest{Type: "MOTION_STUDY", DocumentID: view.Document.ID, VersionID: &snapshot.RevisionID, RequestedBy: actor, InputObjectID: o.ID, IdempotencyKey: uuid.NewString(), Payload: map[string]any{"inputDigest": snapshot.Digest}, UserVisible: true})
	if er != nil {
		t.Fatal(er)
	}
	// Exercise the real claim path while selecting this fixture ahead of
	// unrelated temporary development jobs.
	if _, er = db.Exec(t.Context(), `UPDATE occccad.jobs SET priority=30000 WHERE id=$1`, job.ID); er != nil {
		t.Fatal(er)
	}
	claimed, er := queue.Claim(t.Context(), "conversion-test", time.Minute)
	if er != nil || claimed.ID != job.ID {
		t.Fatal(er)
	}
	h := handler{workerID: "conversion-test", database: db, queue: queue, artifacts: objects, access: access.New(db), geometry: client, workspace: service}
	if er = h.execute(t.Context(), claimed); er != nil {
		t.Fatal("job", er)
	}
	job, er = queue.Get(t.Context(), job.ID)
	if er != nil || job.ResultObjectID == nil {
		t.Fatal(job, er)
	}
	_, reader, er := objects.Open(t.Context(), *job.ResultObjectID)
	if er != nil {
		t.Fatal(er)
	}
	var run workspace.MotionRun
	er = json.NewDecoder(reader).Decode(&run)
	reader.Close()
	if er != nil || !run.Completed || len(run.Frames) != 25 {
		t.Fatalf("run %s %+v %v", run.Status, run.Failure, er)
	}
	if math.Abs(run.Frames[6].CommandValues[did]-math.Pi/2) > 1e-8 || math.Abs(run.Frames[24].Coordinates[jid+"-coordinate"]-2*math.Pi) > 1e-7 {
		t.Fatal("continuous angle lost")
	}
	// Closing a current simulation may adopt one accepted frame into the
	// mechanism editing baseline, without writing Product placement. The saved
	// owning Revision metadata must not invalidate unchanged joint definitions.
	adopted := view.Product.Kinematics
	adoptBytes, _ := json.Marshal(adopted)
	adopted = workspace.KinematicsDefinitions{}
	json.Unmarshal(adoptBytes, &adopted)
	adopted.Mechanisms[0].Poses = run.Frames[6].UnitPoses
	view = command(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &adopted})
	if len(view.Product.Constraints) != 0 {
		t.Fatal("closing playback wrote assembly constraints")
	}
	before := *view.Product
	request := workspace.MotionApplyRequest{JobID: job.ID, FrameIndex: 6}
	t.Run("historical conversion preserves newly added nonmembers", func(t *testing.T) {
		view = command(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: view.Product.Instances[1].ReferencedDocumentID})
		other := view.Product.Instances[len(view.Product.Instances)-1].ID
		position := [3]float64{90, 80, 70}
		view = command(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: other, Translation: position, Rotation: [4]float64{0, 0, 0, 1}})
		p, err := service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, request)
		if err != nil || !p.Ready {
			t.Fatal("nonmember structure blocked source", err, p.Items)
		}
		for _, i := range p.Candidate.Instances {
			if i.ID == other && i.Translation != position {
				t.Fatal("historical pose overwrote nonmember", i)
			}
		}
		for _, id := range p.PoseChanges {
			if id == other {
				t.Fatal("nonmember falsely included in impact")
			}
		}
		if len(run.Frames[6].UnitPoses) != 2 {
			t.Fatal("planning mutated frozen run")
		}
		view = command(workspace.CommandRequest{Type: "UNDO"})
		view = command(workspace.CommandRequest{Type: "UNDO"})
	})
	before = *view.Product
	plan, er := service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, request)
	if er != nil || !plan.Ready || plan.DegreesOfFreedom != 1 {
		t.Fatalf("plan %+v %v", plan, er)
	}
	bad := request
	bad.PlanDigest = "altered-plan"
	_, errBad := service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{Type: "APPLY_MOTION_FRAME", VersionID: view.Document.VersionID, ActorID: actor, RequestID: uuid.NewString(), MotionApply: &bad})
	if errBad == nil {
		t.Fatal("unreviewed plan accepted")
	}
	headAfterBad, errBad := service.GetDocument(t.Context(), view.Document.ID)
	if errBad != nil || headAfterBad.Document.VersionID != view.Document.VersionID || !reflect.DeepEqual(headAfterBad.Product, view.Product) {
		t.Fatal("rejected apply left partial mutation", errBad)
	}
	request.PlanDigest = plan.Digest
	// Two requests reviewed against the same version cannot both commit.
	type applyOutcome struct {
		view workspace.DocumentView
		err  error
	}
	outcomes := make(chan applyOutcome, 2)
	applyDocument, applyBase := view.Document.ID, view.Document.VersionID
	for range 2 {
		go func() {
			v, err := service.ApplyCommand(t.Context(), applyDocument, workspace.CommandRequest{Type: "APPLY_MOTION_FRAME", VersionID: applyBase, ActorID: actor, RequestID: uuid.NewString(), MotionApply: &request})
			outcomes <- applyOutcome{v, err}
		}()
	}
	committed, rejected := 0, 0
	for range 2 {
		outcome := <-outcomes
		if outcome.err != nil {
			rejected++
		} else {
			committed++
			view = outcome.view
		}
	}
	if committed != 1 || rejected != 1 {
		t.Fatalf("concurrent publication committed=%d rejected=%d", committed, rejected)
	}
	if len(view.Product.Constraints) != 3 || len(view.Product.Kinematics.Associations) != 3 {
		t.Fatal("connections missing", len(view.Product.Constraints))
	}
	selected := run.Frames[6].UnitPoses[ends[1].InstanceID]
	for _, v := range view.Product.Instances {
		if v.ID == ends[1].InstanceID && (v.Translation != selected.Translation || math.Abs(v.Rotation[2]-selected.Rotation[2]) > 1e-8) {
			t.Fatal("not selected 90-degree pose", v, selected)
		}
	}
	persisted, er := service.GetDocument(t.Context(), view.Document.ID)
	if er != nil || !reflect.DeepEqual(persisted.Product, view.Product) {
		t.Fatal("reopen", er)
	}
	// One compensation returns both definitions and poses to the pre-conversion baseline.
	undone := command(workspace.CommandRequest{Type: "UNDO"})
	if len(undone.Product.Constraints) != len(before.Constraints) || !reflect.DeepEqual(undone.Product.Instances, before.Instances) || !reflect.DeepEqual(undone.Product.Kinematics, before.Kinematics) {
		a, _ := json.Marshal(undone.Product)
		b, _ := json.Marshal(before)
		t.Fatalf("atomic undo lost baseline\nafter=%s\nbefore=%s", a, b)
	}
	view = command(workspace.CommandRequest{Type: "REDO"})
	plan, er = service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, request)
	if er != nil || !plan.Ready {
		t.Fatal("repeat plan", er, plan.Items)
	}
	for _, item := range plan.Items {
		if item.Action != "REUSE" {
			t.Fatal("repeat created relationship", item)
		}
	}
	request.PlanDigest = plan.Digest
	view = command(workspace.CommandRequest{Type: "APPLY_MOTION_FRAME", VersionID: view.Document.VersionID, MotionApply: &request})
	if len(view.Product.Constraints) != 3 {
		t.Fatal("duplicate constraints")
	}
	t.Run("deleted targets get new identities and changed sources do not update", func(t *testing.T) {
		oldAxis := ""
		for _, a := range view.Product.Kinematics.Associations {
			if a.Role == "axis" {
				oldAxis = a.ConstraintID
			}
		}
		view = command(workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "ASSEMBLY_CONSTRAINT", TargetID: oldAxis})
		p, err := service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, request)
		if err != nil || !p.Ready {
			t.Fatal("target regeneration", err, p.Items)
		}
		found := false
		for _, i := range p.Items {
			if i.Role == "axis" {
				found = i.Action == "ADD" && i.ConstraintID != oldAxis && i.Detail != ""
			}
		}
		if !found {
			t.Fatal("deleted identity was revived", p.Items)
		}
		r := request
		r.PlanDigest = p.Digest
		view = command(workspace.CommandRequest{Type: "APPLY_MOTION_FRAME", VersionID: view.Document.VersionID, MotionApply: &r})
		changed := view.Product.Kinematics
		b, _ := json.Marshal(changed)
		json.Unmarshal(b, &changed)
		for i, j := range changed.Mechanisms[0].Joints {
			if j.ID == jid {
				q := j.AxialOffset
				q.Value += 2
				changed.Mechanisms[0].Joints[i].AxialOffset = q
			}
		}
		view = command(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &changed})
		p, err = service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, workspace.MotionApplyRequest{MechanismID: mid})
		if err != nil || p.Ready {
			t.Fatal("source edit automatically updated Product", err, p.Items)
		}
		conflict := false
		for _, i := range p.Items {
			if i.Action == "UPDATE" {
				t.Fatal("implicit update", i)
			}
			conflict = conflict || i.Action == "CONFLICT"
		}
		if !conflict {
			t.Fatal("definition difference did not report conflict", p.Items)
		}
		view = command(workspace.CommandRequest{Type: "UNDO"})
	})
	request.LockCoordinateIDs = []string{run.Snapshot.Mechanism.Joints[1].Coordinates[0].ID}
	request.PlanDigest = ""
	plan, er = service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, request)
	if er != nil || !plan.Ready || plan.DegreesOfFreedom != 0 {
		t.Fatal("angle lock", er, plan.Items)
	}
	request.PlanDigest = plan.Digest
	view = command(workspace.CommandRequest{Type: "APPLY_MOTION_FRAME", VersionID: view.Document.VersionID, MotionApply: &request})
	if len(view.Product.Constraints) != 4 {
		t.Fatal("missing explicit angle lock")
	}
	stale := view.Document.VersionID
	request.FrameIndex = 12
	request.LockCoordinateIDs = nil
	request.PlanDigest = ""
	plan, er = service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, request)
	if er != nil || plan.Ready {
		t.Fatal("existing angle silently omitted", er, plan)
	}
	lockID := ""
	for _, a := range view.Product.Kinematics.Associations {
		if a.Role == "coordinate-lock/"+jid+"-coordinate" {
			lockID = a.ConstraintID
		}
	}
	view = command(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{lockID}, Suppressed: func() *bool { v := true; return &v }()})
	request.Resolutions = nil
	plan, er = service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, request)
	if er != nil || !plan.Ready || plan.DegreesOfFreedom != 1 {
		t.Fatal("ordinary assembly suppression then incremental apply", er, plan.Items)
	}
	request.PlanDigest = plan.Digest
	view = command(workspace.CommandRequest{Type: "APPLY_MOTION_FRAME", VersionID: view.Document.VersionID, MotionApply: &request})
	_, er = service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{Type: "APPLY_MOTION_FRAME", ActorID: actor, RequestID: uuid.NewString(), VersionID: stale, MotionApply: &request})
	if er == nil {
		t.Fatal("stale CAS accepted")
	}
	// Ordinary solve recognizes every published connection and leaves the allowed
	// coordinate free; a complete solve snapshot still contains all definitions.
	frozen, er := service.FreezeAssemblyInput(t.Context(), view.Document.ID, view.Document.VersionID, *view.Product)
	if er != nil || len(frozen.Constraints) != 3 {
		t.Fatal("ordinary compiler omitted published relation", er, len(frozen.Constraints))
	}

	// An unrelated incomplete mechanism is saveable and cannot prevent running
	// the selected mechanism and its dependencies.
	dk := view.Product.Kinematics
	dk.Mechanisms = append(dk.Mechanisms, workspace.Mechanism{ID: uuid.NewString(), Name: "empty draft", UnitIDs: []string{}, Joints: []workspace.MechanismJoint{}})
	view = command(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &dk})
	if _, er = service.FreezeMotionStudy(t.Context(), view.Document.ID, workspace.MotionRunRequest{BaseRevisionID: view.Document.VersionID, StudyID: sid}); er != nil {
		t.Fatal("unrelated draft blocked run", er)
	}
	// Explicit replace of a conflicting Fix replaces only that definition, while
	// default export still consists of a ground plus freely rotating connection.
	ref := workspace.AssemblyGeometryRef{Kind: "BODY", InstanceID: ends[1].InstanceID}
	var fixed workspace.InstancePose
	for _, v := range view.Product.Instances {
		if v.ID == ends[1].InstanceID {
			fixed = workspace.InstancePose{Translation: v.Translation, Rotation: v.Rotation}
		}
	}
	view = command(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &ref, FixedPose: &fixed, FixMode: "SPACE"})
	fixID := ""
	for _, c := range view.Product.Constraints {
		if c.Kind == "FIX" && c.First.InstanceID == ref.InstanceID {
			fixID = c.ID
		}
	}
	request.FrameIndex = 6
	request.PlanDigest = ""
	request.Resolutions = nil
	plan, er = service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, request)
	if er != nil || plan.Ready {
		t.Fatal("conflicting Fix omitted", er, plan.Items)
	}
	request.Resolutions = map[string]string{fixID: "REPLACE"}
	if _, er = service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, request); er == nil {
		t.Fatal("incremental conversion accepted destructive resolution")
	}
	t.Log("real Pad geometry -> Naming picks -> 360-degree Worker Job -> 90-degree apply/reopen/undo/redo/reuse/angle-lock/conflict/CAS passed")
}
