package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/control"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/geometryrpc"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
	"math"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestMotionApplyGeometryFrameAtomicHistory(t *testing.T) {
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Fatal("matching-source C++ Worker required")
	}
	db, e := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "motion.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(t.Context(), db); e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	pool := control.NewGeometryPool(t.Context(), control.GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: root, LogDirectory: t.TempDir(), MinimumWorkers: 1, MaximumWorkers: 1, GeometryCapacity: 100})
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
	local, e := artifact.NewLocalStore(root)
	if e != nil {
		t.Fatal(e)
	}
	remote, e := artifact.NewLocalStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	var store artifact.Store = motionRemoteStore{remote}
	if os.Getenv("OCCCCAD_TEST_MOTION_S3") == "1" {
		store, local, e = artifact.OpenConfigured(t.Context(), root)
		if e != nil {
			t.Fatal(e)
		}
		if store.Backend() != "S3" {
			t.Fatal("S3 verification requires configured S3 backend")
		}
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
	k := workspace.KinematicsDefinitions{Mechanisms: []workspace.Mechanism{{ID: mid, Name: "shaft-propeller", UnitIDs: []string{ends[0].InstanceID, ends[1].InstanceID}, Joints: []workspace.MechanismJoint{{ID: uuid.NewString(), Name: "shaft grounded", Kind: "GROUND", First: workspace.JointEndpoint{InstanceID: ends[0].InstanceID, Frame: workspace.InstancePose{Rotation: [4]float64{0, 0, 0, 1}}}, Direction: 1}, {ID: jid, Name: "geometric revolute", Kind: "REVOLUTE", First: ends[0], Second: &ends[1], Direction: 1, Zero: workspace.MotionQuantity{Unit: "deg"}, AxialOffset: workspace.MotionQuantity{Value: -10, Unit: "mm"}}}}}, Drivers: []workspace.MotionDriver{{ID: did, Name: "angle", MechanismID: mid, JointID: jid}}, Studies: []workspace.MotionStudy{{ID: sid, Name: "0..360", MechanismID: mid, DriverID: did, Start: workspace.MotionQuantity{Unit: "deg"}, End: workspace.MotionQuantity{Value: 360, Unit: "deg"}, DurationSeconds: 4, Frames: 25, BudgetMS: 60000, Clearance: workspace.MotionQuantity{Unit: "mm"}}}}
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
	if math.Abs(run.Frames[6].DriverValue-math.Pi/2) > 1e-8 || math.Abs(run.Frames[24].Coordinates[jid]-2*math.Pi) > 1e-7 {
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
	request.LockAngle = true
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
	request.LockAngle = false
	request.PlanDigest = ""
	plan, er = service.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, request)
	if er != nil || plan.Ready {
		t.Fatal("existing angle silently omitted", er, plan)
	}
	lockID := ""
	for _, a := range view.Product.Kinematics.Associations {
		if a.Role == "angle-lock" {
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
