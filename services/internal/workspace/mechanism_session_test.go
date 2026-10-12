package workspace

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
)

func TestMechanismSessionRecordingCASAndHistory(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "mechanism_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	local, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewWithArtifacts(db, motionWorker(t), artifact.NewService(db, local))
	actor := access.DefaultUserID
	part, err := svc.CreateDocument(t.Context(), CreateDocumentRequest{Type: "PART", Name: "datum part", ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.CreateDocument(t.Context(), CreateDocumentRequest{Type: "PRODUCT", Name: "recorded motion", ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	command := func(req CommandRequest) DocumentView {
		t.Helper()
		req.ActorID = actor
		req.RequestID = uuid.NewString()
		v, e := svc.ApplyCommand(t.Context(), view.Document.ID, req)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	view = command(CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	view = command(CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	id := view.Product.Instances[1].ID
	view = command(CommandRequest{Type: "MOVE_INSTANCE", InstanceID: id, Rotation: [4]float64{0, 0, math.Sin(math.Pi / 12), math.Cos(math.Pi / 12)}})
	view = command(CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	environmentID := view.Product.Instances[2].ID
	view = command(CommandRequest{Type: "MOVE_INSTANCE", InstanceID: environmentID, Translation: [3]float64{77, -25, 12}, Rotation: [4]float64{0, 0, 0, 1}})
	end := func(id string) JointEndpoint {
		return JointEndpoint{InstanceID: id, Axis: &AssemblyGeometryRef{InstanceID: id, Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Z"}, Plane: &AssemblyGeometryRef{InstanceID: id, Kind: "PLANE", GeometryID: "datum-xy"}, Frame: InstancePose{Rotation: [4]float64{0, 0, 0, 1}}}
	}
	second := end(id)
	upper := MotionQuantity{Value: 60, Unit: "deg"}
	m := Mechanism{ID: "m", Name: "M", UnitIDs: []string{view.Product.Instances[0].ID, id}, Joints: []MechanismJoint{{ID: "g", Name: "ground", Kind: "GROUND", First: end(view.Product.Instances[0].ID)}, {ID: "r", Name: "revolute", Kind: "REVOLUTE", First: end(view.Product.Instances[0].ID), Second: &second, Coordinates: []JointCoordinate{{ID: "r" + "-coordinate", Role: "AXIAL_ROTATION", Dimension: "ANGLE", Zero: MotionQuantity{Unit: "deg"}, Direction: 1, Lower: &MotionQuantity{Value: -720, Unit: "deg"}, Upper: &upper}}}}}
	k := KinematicsDefinitions{Mechanisms: []Mechanism{m}, Drivers: []MotionDriver{{ID: "d", Name: "command", MechanismID: "m", JointID: "r", CoordinateID: "r" + "-coordinate"}}}
	view = command(CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &k})
	generation := uint64(0)
	nextGeneration := func() uint64 { generation++; return generation }
	baseline := view
	opened, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: view.Document.VersionID, MechanismID: "m", ActiveDriverIDs: []string{"d"}})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(opened.Frame.CommandValues["d"]) > 1e-8 {
		t.Fatal("current configuration is not operation zero", opened.Frame.CommandValues["d"])
	}
	if _, err = svc.UpdateMechanismSession(t.Context(), view.Document.ID, "other", opened.SessionID, MechanismSessionUpdate{Sequence: 1, Values: map[string]MotionQuantity{"d": MotionQuantity{Value: 1, Unit: "deg"}}}); err == nil {
		t.Fatal("another actor used transient session")
	}
	key0, err := svc.RecordMechanismKey(t.Context(), view.Document.ID, actor, opened.SessionID, 0, MotionKeyframe{ID: "key0", TimeSeconds: 0})
	if err != nil {
		t.Fatal(err)
	}
	f, err := svc.UpdateMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, MechanismSessionUpdate{Sequence: 1, Values: map[string]MotionQuantity{"d": MotionQuantity{Value: 10, Unit: "deg"}}})
	if err != nil || !f.Accepted {
		t.Fatal(f, err)
	}
	key1, err := svc.RecordMechanismKey(t.Context(), view.Document.ID, actor, opened.SessionID, 1, MotionKeyframe{ID: "key1", TimeSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := svc.UpdateMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, MechanismSessionUpdate{Sequence: 2, Values: map[string]MotionQuantity{"d": MotionQuantity{Value: 90, Unit: "deg"}}})
	if err != nil || rejected.Accepted || rejected.Failure == nil || rejected.Failure.Code != "LIMIT_REACHED" || !reflect.DeepEqual(f.Frame, rejected.Frame) {
		t.Fatal("failed target did not retain the entire checkpoint", rejected, err)
	}
	current, _ := svc.GetDocument(t.Context(), view.Document.ID)
	if current.Document.VersionID != baseline.Document.VersionID {
		t.Fatal("trial or recording created a Revision")
	}
	study := MotionStudy{ID: "s", Name: "Simulation", MechanismID: "m", Frames: 5, BudgetMS: 10000, Keyframes: []MotionKeyframe{key0, key1}, ActiveDriverIDs: []string{"d"}}
	path, err := svc.PreviewMechanismRecording(t.Context(), view.Document.ID, actor, opened.SessionID, study)
	if err != nil || !path.Completed {
		t.Fatal("recorded finite path", path.Failure, err)
	}
	if !reflect.DeepEqual(path.Snapshot.View.Product, baseline.Product) || !reflect.DeepEqual(path.Snapshot.View.ResolvedInstances, baseline.ResolvedInstances) {
		t.Fatal("recording playback mixed numerical nominal poses into the display document")
	}
	t.Run("frozen environment is numerical input, not a display document mutation", func(t *testing.T) {
		raw, _ := json.Marshal(path.Snapshot)
		var snapshot MotionSnapshot
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			t.Fatal(err)
		}
		pose := InstancePose{Translation: [3]float64{90, -35, 15}, Rotation: [4]float64{0, 0, math.Sin(math.Pi / 8), math.Cos(math.Pi / 8)}}
		snapshot.Study.EnvironmentPoses[environmentID] = pose
		if err := svc.compileMotionSnapshot(t.Context(), &snapshot); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(snapshot.View.Product, baseline.Product) || !reflect.DeepEqual(snapshot.View.ResolvedInstances, baseline.ResolvedInstances) {
			t.Fatal("historical environment changed the source occurrence coordinate system")
		}
		snapshot.Digest = motionDigest(snapshot)
		engine, err := newMechanismMotionSolver(t.Context(), svc, snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if !conflictPoseClose(geometry.AssemblyPose(engine.state.Frame.UnitPoses[environmentID]), geometry.AssemblyPose(pose), 1e-7, 1e-8) {
			t.Fatal("display isolation lost the frozen environment in the solver")
		}
	})
	playhead, err := svc.motionPlayhead(t.Context(), path, .37)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(playhead.CommandValues["d"]-3.7*math.Pi/180) > 1e-8 || playhead.TimeSeconds != .37 {
		t.Fatal("exact playhead adopted a neighbouring sample", playhead)
	}
	if !reflect.DeepEqual(path.Frames[0].UnitPoses, opened.Frame.UnitPoses) {
		t.Fatal("path evaluation changed its recorded initial configuration")
	}
	canceled, cancelConfirmation := context.WithCancel(t.Context())
	cancelConfirmation()
	if _, err = svc.FinalizeMechanismSession(canceled, view.Document.ID, actor, opened.SessionID, MechanismSessionFinalize{Sequence: 2, Study: &study}); err == nil {
		t.Fatal("canceled confirmation unexpectedly succeeded")
	}
	final, err := svc.FinalizeMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, MechanismSessionFinalize{Sequence: 2, Study: &study})
	if err != nil {
		t.Fatal(err)
	}
	if final.PreviewID == "" {
		t.Fatal("confirmation bypassed authoritative candidate")
	}
	final.ActorID = actor
	view, err = svc.ApplyCommand(t.Context(), view.Document.ID, final)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Product.Kinematics.Studies) != 1 || !reflect.DeepEqual(view.Product.Instances, baseline.Product.Instances) {
		t.Fatal("recording failed or changed Product assembly truth")
	}
	tampered, _ := json.Marshal(view.Product.Kinematics)
	var unchecked KinematicsDefinitions
	if err := json.Unmarshal(tampered, &unchecked); err != nil {
		t.Fatal(err)
	}
	unchecked.Studies[0].Keyframes[1].Values["d"] = MotionQuantity{Value: 90, Unit: "deg"}
	if _, err := svc.ApplyCommand(t.Context(), view.Document.ID, CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, RequestID: uuid.NewString(), ActorID: actor, Kinematics: &unchecked}); err == nil {
		t.Fatal("direct Domain Command accepted an unqualified recorded key")
	}
	unchecked.Studies[0].Keyframes[1].Values["d"] = key1.Values["d"]
	badInitial := unchecked.Studies[0].InitialPoses[id]
	badInitial.Translation[2] += 1
	unchecked.Studies[0].InitialPoses[id] = badInitial
	if _, err := svc.ApplyCommand(t.Context(), view.Document.ID, CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, RequestID: uuid.NewString(), ActorID: actor, Kinematics: &unchecked}); err == nil {
		t.Fatal("direct Domain Command silently solved away an invalid recording initial configuration")
	}
	if !reflect.DeepEqual(view.Product.Kinematics.Mechanisms[0].Poses, mechanismMemberPoses(m, f.Frame.UnitPoses)) {
		t.Fatal("final confirmation adopted another pose")
	}
	t.Run("zero preserves limits and the recorded physical path", func(t *testing.T) {
		before := view
		clone := func() KinematicsDefinitions {
			raw, _ := json.Marshal(view.Product.Kinematics)
			var k KinematicsDefinitions
			json.Unmarshal(raw, &k)
			return k
		}
		zero := clone()
		zero.Drivers[0].Reference, zero.Drivers[0].ReferenceDigest = nil, ""
		view = command(CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &zero})
		q, _ := motionValue(*view.Product.Kinematics.Drivers[0].Reference, true)
		expectedZero := before.Product.Kinematics.Mechanisms[0].Coordinates["r-coordinate"]
		if math.Abs(q-expectedZero) > 1e-8 || !reflect.DeepEqual(view.Product.Kinematics.Studies, before.Product.Kinematics.Studies) {
			t.Fatalf("zero capture: got %g want %g records equal %v", q, expectedZero, reflect.DeepEqual(view.Product.Kinematics.Studies, before.Product.Kinematics.Studies))
		}
		for id, p := range before.Product.Kinematics.Mechanisms[0].Poses {
			if !conflictPoseClose(geometry.AssemblyPose(p), geometry.AssemblyPose(view.Product.Kinematics.Mechanisms[0].Poses[id]), 1e-7, 1e-8) {
				t.Fatal("zero moved a member", id)
			}
		}
		if !reflect.DeepEqual(view.Product.Kinematics.Mechanisms[0].Joints, before.Product.Kinematics.Mechanisms[0].Joints) {
			t.Fatal("zero moved a stop or changed the coordinate definition")
		}
		frozen, err := svc.FreezeMotionStudy(t.Context(), view.Document.ID, MotionRunRequest{BaseRevisionID: view.Document.VersionID, StudyID: "s"})
		if err != nil {
			t.Fatal(err)
		}
		afterPath, err := svc.RunMotionStudy(t.Context(), frozen, nil)
		if err != nil || !afterPath.Completed || len(afterPath.Frames) != len(path.Frames) {
			t.Fatal(afterPath.Failure, err)
		}
		for i, frame := range path.Frames {
			for id, p := range frame.UnitPoses {
				if !conflictPoseClose(geometry.AssemblyPose(p), geometry.AssemblyPose(afterPath.Frames[i].UnitPoses[id]), 1e-7, 1e-8) {
					t.Fatal("zero shifted the physical path", i, id)
				}
			}
		}
		opened, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: view.Document.VersionID, MechanismID: "m", StudyID: "s", ActiveDriverIDs: []string{"d"}})
		if err != nil {
			t.Fatal(err)
		}
		first, _ := motionValue(opened.Study.Keyframes[0].Values["d"], true)
		last, _ := motionValue(opened.Study.Keyframes[1].Values["d"], true)
		if math.Abs(first+10*math.Pi/180) > 1e-8 || math.Abs(last) > 1e-8 || math.Abs(opened.Frame.CommandValues["d"]-first) > 1e-8 || opened.Study.Keyframes[0].ID != key0.ID {
			t.Fatal("editing did not rebase the stable keys to today's zero", opened.Study)
		}
		confirmation, err := svc.FinalizeMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, MechanismSessionFinalize{Sequence: 0, Study: opened.Study})
		if err != nil {
			t.Fatal("rebased recording cannot prepare a qualified confirmation", err)
		}
		view = command(confirmation)
		if !reflect.DeepEqual(view.Product.Kinematics.Studies[0].Keyframes, opened.Study.Keyframes) {
			t.Fatal("confirmation lost rebased key identities or coordinates")
		}
		frozen, err = svc.FreezeMotionStudy(t.Context(), view.Document.ID, MotionRunRequest{BaseRevisionID: view.Document.VersionID, StudyID: "s"})
		if err != nil {
			t.Fatal(err)
		}
		confirmedPath, err := svc.RunMotionStudy(t.Context(), frozen, nil)
		if err != nil || !confirmedPath.Completed || len(confirmedPath.Frames) != len(path.Frames) {
			t.Fatal("confirmed recording lost its qualified path", confirmedPath.Failure, err)
		}
		for i, frame := range path.Frames {
			for id, p := range frame.UnitPoses {
				if !conflictPoseClose(geometry.AssemblyPose(p), geometry.AssemblyPose(confirmedPath.Frames[i].UnitPoses[id]), 1e-7, 1e-8) {
					t.Fatal("confirming rebased keys shifted the physical path", i, id)
				}
			}
		}
		svc.CancelMechanismSession(view.Document.ID, actor, opened.SessionID)
		view = command(CommandRequest{Type: "UNDO"})
		head := view.Document.VersionID
		for _, test := range []struct {
			name, want string
			mutate     func(*KinematicsDefinitions)
		}{
			{"referenced deletion", "命令被模拟引用", func(k *KinematicsDefinitions) { k.Drivers = nil }},
			{"retarget", "命令所属接合不能更改", func(k *KinematicsDefinitions) { k.Drivers[0].JointID = "g" }},
			{"missing driven limit", "requires lower and upper limits", func(k *KinematicsDefinitions) { k.Mechanisms[0].Joints[1].Coordinates[0].Lower = nil }},
			{"equal joint limits", "reversed coordinate limits", func(k *KinematicsDefinitions) { k.Mechanisms[0].Joints[1].Coordinates[0].Lower = k.Mechanisms[0].Joints[1].Coordinates[0].Upper }},
			{"current position outside stop", "LIMIT_REACHED", func(k *KinematicsDefinitions) {
				k.Mechanisms[0].Joints[1].Coordinates[0].Upper = &MotionQuantity{Value: expectedZero - 5*math.Pi/180, Unit: "rad"}
			}},
		} {
			t.Run(test.name, func(t *testing.T) {
				k := clone()
				test.mutate(&k)
				_, err := svc.ApplyCommand(t.Context(), view.Document.ID, CommandRequest{Type: "SAVE_KINEMATICS", VersionID: head, ActorID: actor, RequestID: uuid.NewString(), Kinematics: &k})
				if err == nil || !strings.Contains(err.Error(), test.want) {
					t.Fatal("invalid edit accepted or misclassified", err)
				}
				current, _ := svc.GetDocument(t.Context(), view.Document.ID)
				if current.Document.VersionID != head {
					t.Fatal("rejected edit wrote a revision")
				}
			})
		}
		view = command(CommandRequest{Type: "UNDO"})
		if !reflect.DeepEqual(view.Product.Kinematics, before.Product.Kinematics) {
			t.Fatal("undo did not restore command zero and recorded state together")
		}
		view = command(CommandRequest{Type: "REDO"})
		if value, _ := motionValue(*view.Product.Kinematics.Drivers[0].Reference, true); math.Abs(value-q) > 1e-8 {
			t.Fatal("redo lost the new zero")
		}
		view = command(CommandRequest{Type: "UNDO"})
	})
	if _, err = svc.UpdateMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, MechanismSessionUpdate{Sequence: 3, Values: map[string]MotionQuantity{"d": MotionQuantity{Unit: "deg"}}}); err == nil {
		t.Fatal("old Head session overwrote a new Revision")
	}
	view = command(CommandRequest{Type: "UNDO"})
	if len(view.Product.Kinematics.Studies) != 0 || !reflect.DeepEqual(view.Product.Kinematics.Mechanisms[0].Poses, baseline.Product.Kinematics.Mechanisms[0].Poses) {
		t.Fatal("one Undo did not restore both recording and entry pose")
	}
	view = command(CommandRequest{Type: "REDO"})
	if len(view.Product.Kinematics.Studies) != 1 {
		t.Fatal("Redo lost recording")
	}
	limited, _ := json.Marshal(view.Product.Kinematics)
	var restricted KinematicsDefinitions
	if err := json.Unmarshal(limited, &restricted); err != nil {
		t.Fatal(err)
	}
	restricted.Mechanisms[0].Poses = baseline.Product.Kinematics.Mechanisms[0].Poses
	restricted.Mechanisms[0].Coordinates = baseline.Product.Kinematics.Mechanisms[0].Coordinates
	reference, err := motionValue(*restricted.Drivers[0].Reference, true)
	if err != nil {
		t.Fatal(err)
	}
	restricted.Mechanisms[0].Joints[1].Coordinates[0].Upper = &MotionQuantity{Value: reference + 5*math.Pi/180, Unit: "rad"}
	view = command(CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &restricted})
	beforeValidation := view.Document.VersionID
	if _, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: beforeValidation, MechanismID: "m", StudyID: "s", Revalidate: true, ActiveDriverIDs: []string{"d"}}); err == nil {
		t.Fatal("revalidation discarded an authored key outside today's physical limits")
	}
	current, _ = svc.GetDocument(t.Context(), view.Document.ID)
	if current.Document.VersionID != beforeValidation || len(current.Product.Kinematics.Studies[0].Keyframes) != 2 {
		t.Fatal("failed revalidation changed Head or erased the user's recording")
	}
	fullTurn := view.Product.Kinematics
	fullTurn.Mechanisms[0].Joints[1].Coordinates[0].Upper = &MotionQuantity{Value: 720, Unit: "deg"}
	view = command(CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &fullTurn})
	if _, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: view.Document.VersionID, MechanismID: "m", StudyID: "s", ActiveDriverIDs: []string{"d"}}); err == nil {
		t.Fatal("changed limits silently adopted a recorded basis")
	}
	revalidated, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: view.Document.VersionID, MechanismID: "m", StudyID: "s", Revalidate: true, ActiveDriverIDs: []string{"d"}})
	if err != nil || revalidated.Study == nil || revalidated.Study.Keyframes[0].ID != key0.ID || math.Abs(revalidated.Frame.CommandValues["d"]) > 1e-8 {
		t.Fatal("explicit revalidation lost authored coordinates or initial identity", revalidated, err)
	}
	if err := svc.TouchMechanismSession(t.Context(), view.Document.ID, actor, revalidated.SessionID); err != nil {
		t.Fatal("idle lease cannot renew", err)
	}
	svc.CancelMechanismSession(view.Document.ID, actor, revalidated.SessionID)
	turn, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: view.Document.VersionID, MechanismID: "m", ActiveDriverIDs: []string{"d"}})
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := svc.UpdateMechanismSession(t.Context(), view.Document.ID, actor, turn.SessionID, MechanismSessionUpdate{Sequence: 1, Values: map[string]MotionQuantity{"d": MotionQuantity{Value: 440, Unit: "deg"}}})
	if err != nil || !rotated.Accepted {
		t.Fatal("full turn", rotated.Failure, err)
	}
	turnCommand, err := svc.FinalizeMechanismSession(t.Context(), view.Document.ID, actor, turn.SessionID, MechanismSessionFinalize{Sequence: 1})
	if err != nil {
		t.Fatal(err)
	}
	turnCommand.ActorID = actor
	view, err = svc.ApplyCommand(t.Context(), view.Document.ID, turnCommand)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: view.Document.VersionID, MechanismID: "m", ActiveDriverIDs: []string{"d"}})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(reopened.Frame.CommandValues["d"]-440*math.Pi/180) > 1e-8 {
		t.Fatal("reopen discarded winding", reopened.Frame.CommandValues["d"])
	}
	svc.CancelMechanismSession(view.Document.ID, actor, reopened.SessionID)
	if _, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: generation, BaseRevisionID: view.Document.VersionID, MechanismID: "m", ActiveDriverIDs: []string{"d"}}); err == nil {
		t.Fatal("late Begin recreated a canceled dialog generation")
	}
	svc.CancelMechanismSession(view.Document.ID, actor, turn.SessionID)
	svc.CancelMechanismSession(view.Document.ID, actor, opened.SessionID)
	if _, err = svc.RecordMechanismKey(t.Context(), view.Document.ID, actor, opened.SessionID, 2, key1); err == nil {
		t.Fatal("cancel did not release session")
	}
}
