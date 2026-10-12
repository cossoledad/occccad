package workspace

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
)

func TestMechanismCommandZeroAndPrivateRollback(t *testing.T) {
	service := New(nil, motionWorker(t))
	s := motionFixture(t, true, "REVOLUTE")
	engine, err := newMechanismMotionSolver(t.Context(), service, s)
	if err != nil {
		t.Fatal(err)
	}
	initial := engine.state.Frame
	for _, b := range s.Equations.Bodies {
		if !conflictPoseClose(geometry.AssemblyPose(initial.UnitPoses[b.ID]), b.Pose, 1e-6, 1e-8) {
			t.Fatal("opening the current coordinate moved the initial pose", b.ID)
		}
	}
	upper := MotionQuantity{Value: initial.DriverValue + math.Pi/18, Unit: "rad"}
	engine.snapshot.Mechanism.Joints[1].Upper = &upper
	before, _ := json.Marshal(engine.state)
	if _, err = engine.Advance(t.Context(), initial.DriverValue+math.Pi/6); err == nil {
		t.Fatal("target beyond physical limit accepted")
	}
	after, _ := json.Marshal(engine.state)
	if string(before) != string(after) {
		t.Fatal("failed target leaked private continuation poses or angle/warm state")
	}
	if _, err = engine.Advance(t.Context(), initial.DriverValue-math.Pi/18); err != nil {
		t.Fatal("legal reverse motion after rejection failed", err)
	}
	if reflect.DeepEqual(initial.UnitPoses, engine.state.Frame.UnitPoses) {
		t.Fatal("reverse request did not actually move the mechanism")
	}
}

func TestMechanismCommandWithoutGroundEndpoint(t *testing.T) {
	service := New(nil, motionWorker(t))
	s := motionFixture(t, true, "REVOLUTE")
	s.Study.DriverJointID = "j3"
	frozen := AssemblySolveManifest{Bodies: s.Equations.Bodies}
	var err error
	s.Equations, err = compileMechanism(frozen, s.Mechanism, s.Study)
	if err != nil {
		t.Fatal(err)
	}
	s.Digest = motionDigest(s)
	engine, err := newMechanismMotionSolver(t.Context(), service, s)
	if err != nil {
		t.Fatal("non-ground active coordinate refused", err)
	}
	if engine.driver.First.InstanceID == "ground" || engine.driver.Second.InstanceID == "ground" {
		t.Fatal("fixture accidentally directly grounded")
	}
	initial := engine.state.Frame.DriverValue
	f, err := engine.Advance(t.Context(), initial+0.05)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = verifyMechanismJoints(s.Mechanism, f.UnitPoses, engine.state.Frame.Coordinates, "j3", initial+.05); err != nil {
		t.Fatal(err)
	}
	before := engine.state.Frame
	joint := s.Mechanism.Joints[1]
	q := before.Coordinates[joint.ID]
	driver := MotionDriver{ID: "other", MechanismID: s.Mechanism.ID, JointID: joint.ID, Reference: &MotionQuantity{Value: q, Unit: "rad"}, ReferenceDigest: commandCoordinateBasis(joint)}
	if err := engine.SelectDriver(t.Context(), driver); err != nil {
		t.Fatal("coordinate switch", err)
	}
	for id, p := range before.UnitPoses {
		if !conflictPoseClose(geometry.AssemblyPose(p), geometry.AssemblyPose(engine.state.Frame.UnitPoses[id]), 1e-7, 1e-8) {
			t.Fatal("switch reset a member pose", id)
		}
	}
	if engine.snapshot.Digest != motionDigest(engine.snapshot) || engine.driver.ID != joint.ID {
		t.Fatal("switch did not bind its new frozen coordinate identity")
	}
}

func TestMechanismCommandReferenceCapturedOnce(t *testing.T) {
	s := motionFixture(t, false, "REVOLUTE")
	m := s.Mechanism
	m.Poses = map[string]InstancePose{}
	for _, b := range s.Equations.Bodies {
		m.Poses[b.ID] = InstancePose(b.Pose)
	}
	k := KinematicsDefinitions{Mechanisms: []Mechanism{m}, Drivers: []MotionDriver{{ID: "d", MechanismID: m.ID, JointID: "drive"}}}
	if err := bindMotionDrivers(&k); err != nil {
		t.Fatal(err)
	}
	first := *k.Drivers[0].Reference
	if math.Abs(first.Value-math.Pi/3) > 1e-10 {
		t.Fatal("current pose was not command zero", first)
	}
	p := k.Mechanisms[0].Poses["crank"]
	p.Rotation = [4]float64{0, 0, math.Sin(.7), math.Cos(.7)}
	k.Mechanisms[0].Poses["crank"] = p
	if err := bindMotionDrivers(&k); err != nil {
		t.Fatal(err)
	}
	if *k.Drivers[0].Reference != first {
		t.Fatal("reopen silently reset operation zero")
	}
	digest := recordingBasis(k.Mechanisms[0], k.Drivers[0])
	k.Mechanisms[0].Name = "renamed"
	k.Mechanisms[0].UnitIDs = []string{"crank", "ground"}
	if recordingBasis(k.Mechanisms[0], k.Drivers[0]) != digest {
		t.Fatal("display rename or order invalidated recording")
	}
}

func TestMechanismPassiveJointLimitRetainsQualifiedCheckpoint(t *testing.T) {
	service := New(nil, motionWorker(t))
	snapshot := motionFixture(t, true, "REVOLUTE")
	engine, err := newMechanismMotionSolver(t.Context(), service, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	initial := engine.state.Frame
	target := initial.DriverValue + .05
	qualified, err := engine.Advance(t.Context(), target)
	if err != nil {
		t.Fatal("unlimited target has no feasible witness", err)
	}
	start, end := initial.Coordinates["j2"], qualified.Coordinates["j2"]
	if math.Abs(start-end) < 1e-5 {
		t.Fatal("passive joint fixture did not move")
	}
	for i := range snapshot.Mechanism.Joints {
		j := &snapshot.Mechanism.Joints[i]
		if j.ID == "j2" {
			bound := &MotionQuantity{Value: (start + end) / 2, Unit: "rad"}
			if end > start {
				j.Lower = nil
				j.Upper = bound
			} else {
				j.Lower = bound
				j.Upper = nil
			}
		}
	}
	snapshot.Digest = motionDigest(snapshot)
	limited, err := newMechanismMotionSolver(t.Context(), service, snapshot)
	if err != nil {
		t.Fatal("one-sided passive stop rejected its valid entry", err)
	}
	before, _ := json.Marshal(limited.state)
	if _, err = limited.Advance(t.Context(), target); err == nil {
		t.Fatal("active coordinate ignored a passive joint stop")
	}
	after, _ := json.Marshal(limited.state)
	if string(before) != string(after) {
		t.Fatal("passive limit failure leaked continuation or member poses")
	}
}
