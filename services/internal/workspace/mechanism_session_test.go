package workspace

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
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
	end := func(id string) JointEndpoint {
		return JointEndpoint{InstanceID: id, Axis: &AssemblyGeometryRef{InstanceID: id, Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Z"}, Plane: &AssemblyGeometryRef{InstanceID: id, Kind: "PLANE", GeometryID: "datum-xy"}, Frame: InstancePose{Rotation: [4]float64{0, 0, 0, 1}}}
	}
	second := end(id)
	upper := MotionQuantity{Value: 60, Unit: "deg"}
	m := Mechanism{ID: "m", Name: "M", UnitIDs: []string{view.Product.Instances[0].ID, id}, Joints: []MechanismJoint{{ID: "g", Name: "ground", Kind: "GROUND", First: end(view.Product.Instances[0].ID), Direction: 1}, {ID: "r", Name: "revolute", Kind: "REVOLUTE", First: end(view.Product.Instances[0].ID), Second: &second, Direction: 1, Zero: MotionQuantity{Unit: "deg"}, Upper: &upper}}}
	k := KinematicsDefinitions{Mechanisms: []Mechanism{m}, Drivers: []MotionDriver{{ID: "d", Name: "command", MechanismID: "m", JointID: "r"}}}
	view = command(CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &k})
	generation := uint64(0)
	nextGeneration := func() uint64 { generation++; return generation }
	baseline := view
	opened, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: view.Document.VersionID, MechanismID: "m", DriverID: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(opened.Frame.DriverValue) > 1e-8 {
		t.Fatal("current configuration is not operation zero", opened.Frame.DriverValue)
	}
	if _, err = svc.UpdateMechanismSession(t.Context(), view.Document.ID, "other", opened.SessionID, MechanismSessionUpdate{Sequence: 1, Value: &MotionQuantity{Value: 1, Unit: "deg"}}); err == nil {
		t.Fatal("another actor used transient session")
	}
	key0, err := svc.RecordMechanismKey(t.Context(), view.Document.ID, actor, opened.SessionID, 0, MotionKeyframe{ID: "key0", TimeSeconds: 0})
	if err != nil {
		t.Fatal(err)
	}
	f, err := svc.UpdateMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, MechanismSessionUpdate{Sequence: 1, Value: &MotionQuantity{Value: 10, Unit: "deg"}})
	if err != nil || !f.Accepted {
		t.Fatal(f, err)
	}
	key1, err := svc.RecordMechanismKey(t.Context(), view.Document.ID, actor, opened.SessionID, 1, MotionKeyframe{ID: "key1", TimeSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := svc.UpdateMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, MechanismSessionUpdate{Sequence: 2, Value: &MotionQuantity{Value: 90, Unit: "deg"}})
	if err != nil || rejected.Accepted || rejected.Failure == nil || rejected.Failure.Code != "LIMIT_REACHED" || !reflect.DeepEqual(f.Frame, rejected.Frame) {
		t.Fatal("failed target did not retain the entire checkpoint", rejected, err)
	}
	current, _ := svc.GetDocument(t.Context(), view.Document.ID)
	if current.Document.VersionID != baseline.Document.VersionID {
		t.Fatal("trial or recording created a Revision")
	}
	study := MotionStudy{ID: "s", Name: "Simulation", MechanismID: "m", DriverID: "d", Frames: 5, BudgetMS: 10000, Keyframes: []MotionKeyframe{key0, key1}}
	path, err := svc.PreviewMechanismRecording(t.Context(), view.Document.ID, actor, opened.SessionID, study)
	if err != nil || !path.Completed {
		t.Fatal("recorded finite path", path.Failure, err)
	}
	playhead, err := svc.motionPlayhead(t.Context(), path, .37)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(playhead.DriverValue-3.7*math.Pi/180) > 1e-8 || playhead.TimeSeconds != .37 {
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
	unchecked.Studies[0].Keyframes[1].Value = MotionQuantity{Value: 90, Unit: "deg"}
	if _, err := svc.ApplyCommand(t.Context(), view.Document.ID, CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, RequestID: uuid.NewString(), ActorID: actor, Kinematics: &unchecked}); err == nil {
		t.Fatal("direct Domain Command accepted an unqualified recorded key")
	}
	unchecked.Studies[0].Keyframes[1].Value = key1.Value
	badInitial := unchecked.Studies[0].InitialPoses[id]
	badInitial.Translation[2] += 1
	unchecked.Studies[0].InitialPoses[id] = badInitial
	if _, err := svc.ApplyCommand(t.Context(), view.Document.ID, CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, RequestID: uuid.NewString(), ActorID: actor, Kinematics: &unchecked}); err == nil {
		t.Fatal("direct Domain Command silently solved away an invalid recording initial configuration")
	}
	if !reflect.DeepEqual(view.Product.Kinematics.Mechanisms[0].Poses, mechanismMemberPoses(m, f.Frame.UnitPoses)) {
		t.Fatal("final confirmation adopted another pose")
	}
	if _, err = svc.UpdateMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, MechanismSessionUpdate{Sequence: 3, Value: &MotionQuantity{Unit: "deg"}}); err == nil {
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
	restricted.Mechanisms[0].Joints[1].Upper = &MotionQuantity{Value: reference + 5*math.Pi/180, Unit: "rad"}
	view = command(CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &restricted})
	beforeValidation := view.Document.VersionID
	if _, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: beforeValidation, MechanismID: "m", DriverID: "d", StudyID: "s", Revalidate: true}); err == nil {
		t.Fatal("revalidation discarded an authored key outside today's physical limits")
	}
	current, _ = svc.GetDocument(t.Context(), view.Document.ID)
	if current.Document.VersionID != beforeValidation || len(current.Product.Kinematics.Studies[0].Keyframes) != 2 {
		t.Fatal("failed revalidation changed Head or erased the user's recording")
	}
	fullTurn := view.Product.Kinematics
	fullTurn.Mechanisms[0].Joints[1].Upper = nil
	view = command(CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &fullTurn})
	if _, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: view.Document.VersionID, MechanismID: "m", DriverID: "d", StudyID: "s"}); err == nil {
		t.Fatal("changed limits silently adopted a recorded basis")
	}
	revalidated, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: view.Document.VersionID, MechanismID: "m", DriverID: "d", StudyID: "s", Revalidate: true})
	if err != nil || revalidated.Study == nil || revalidated.Study.Keyframes[0].ID != key0.ID || math.Abs(revalidated.Frame.DriverValue) > 1e-8 {
		t.Fatal("explicit revalidation lost authored coordinates or initial identity", revalidated, err)
	}
	if err := svc.TouchMechanismSession(t.Context(), view.Document.ID, actor, revalidated.SessionID); err != nil {
		t.Fatal("idle lease cannot renew", err)
	}
	svc.CancelMechanismSession(view.Document.ID, actor, revalidated.SessionID)
	turn, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: view.Document.VersionID, MechanismID: "m", DriverID: "d"})
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := svc.UpdateMechanismSession(t.Context(), view.Document.ID, actor, turn.SessionID, MechanismSessionUpdate{Sequence: 1, Value: &MotionQuantity{Value: 440, Unit: "deg"}})
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
	reopened, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: nextGeneration(), BaseRevisionID: view.Document.VersionID, MechanismID: "m", DriverID: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(reopened.Frame.DriverValue-440*math.Pi/180) > 1e-8 {
		t.Fatal("reopen discarded winding", reopened.Frame.DriverValue)
	}
	svc.CancelMechanismSession(view.Document.ID, actor, reopened.SessionID)
	if _, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "dialog", Generation: generation, BaseRevisionID: view.Document.VersionID, MechanismID: "m", DriverID: "d"}); err == nil {
		t.Fatal("late Begin recreated a canceled dialog generation")
	}
	svc.CancelMechanismSession(view.Document.ID, actor, turn.SessionID)
	svc.CancelMechanismSession(view.Document.ID, actor, opened.SessionID)
	if _, err = svc.RecordMechanismKey(t.Context(), view.Document.ID, actor, opened.SessionID, 2, key1); err == nil {
		t.Fatal("cancel did not release session")
	}
}
