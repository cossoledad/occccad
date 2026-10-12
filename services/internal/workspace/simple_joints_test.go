package workspace

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
)

func simpleJointSnapshot(t *testing.T, kind string, grounded bool) MotionSnapshot {
	t.Helper()
	a, b := jointEnd("a", 0, 0), jointEnd("b", 0, 0)
	j := MechanismJoint{ID: "joint", Name: "joint", Kind: kind, First: a, Second: &b, OrientationAngle: MotionQuantity{Unit: "rad"}, AxialDirection: "SAME"}
	for _, role := range jointCoordinateRoles(kind) {
		dimension, unit := "LENGTH", "mm"
		if role == "AXIAL_ROTATION" {
			dimension, unit = "ANGLE", "rad"
		}
		j.Coordinates = append(j.Coordinates, JointCoordinate{ID: "coordinate/" + role, Role: role, Dimension: dimension, Direction: 1, Zero: MotionQuantity{Unit: unit}, Lower: &MotionQuantity{Value: -1000, Unit: unit}, Upper: &MotionQuantity{Value: 1000, Unit: unit}})
	}
	p := InstancePose{Rotation: [4]float64{0, 0, 0, 1}}
	m := Mechanism{ID: "mechanism", Name: "mechanism", UnitIDs: []string{"a", "b"}, Joints: []MechanismJoint{j}, Poses: map[string]InstancePose{"a": p, "b": p}}
	if grounded {
		m.Joints = append([]MechanismJoint{{ID: "ground", Name: "ground", Kind: "GROUND", First: a}}, m.Joints...)
	}
	if kind == "RIGID" {
		m.Joints[len(m.Joints)-1].Constraints = []AssemblyConstraint{{ID: "rigid", Name: "rigid", Kind: "FIX_TOGETHER", RelationRole: "rigid", GroupMembers: []AssemblyGroupMember{{InstanceID: "a"}, {InstanceID: "b"}}, GroupRelations: []AssemblyGroupRelation{{InstanceID: "a", RelativePose: p}, {InstanceID: "b", RelativePose: p}}}}
	}
	frozen := AssemblySolveManifest{Bodies: []geometry.AssemblyBody{{ID: "a", Pose: geometry.AssemblyPose(p)}, {ID: "b", Pose: geometry.AssemblyPose(p)}}}
	eq, err := compileMechanismEquations(frozen, m, MotionStudy{}, false)
	if err != nil {
		t.Fatal(err)
	}
	s := MotionSnapshot{Schema: motionSchema, Policy: motionPolicy, Mechanism: m, Equations: eq, SolverProfile: defaultAssemblySolverProfile(), LinearStepMM: 2, Study: MotionStudy{MechanismID: m.ID, Frames: 9, BudgetMS: 30000}}
	for _, c := range j.Coordinates {
		s.Drivers = append(s.Drivers, MotionDriver{ID: "driver/" + c.Role, Name: c.Role, MechanismID: m.ID, JointID: j.ID, CoordinateID: c.ID, Reference: &MotionQuantity{Unit: c.unit()}, ReferenceDigest: commandCoordinateBasis(j, c.ID)})
	}
	s.Digest = motionDigest(s)
	return s
}
func TestSimpleJointPhysicalFreedom(t *testing.T) {
	svc := New(nil, motionWorker(t))
	for _, tc := range []struct {
		kind       string
		grounded   bool
		dof, gauge uint64
	}{{"PRISMATIC", true, 1, 0}, {"CYLINDRICAL", true, 2, 0}, {"PLANAR", true, 3, 0}, {"SPHERICAL", true, 3, 0}, {"RIGID", true, 0, 0}, {"RIGID", false, 0, 6}} {
		t.Run(tc.kind+map[bool]string{true: "-grounded", false: "-free"}[tc.grounded], func(t *testing.T) {
			s := simpleJointSnapshot(t, tc.kind, tc.grounded)
			e, err := newMechanismMotionSolver(t.Context(), svc, s)
			if err != nil {
				t.Fatal(err)
			}
			dof, gauge := motionDof(*e.last)
			if dof != tc.dof || gauge != tc.gauge {
				t.Fatalf("actual DOF/gauge %d/%d, want %d/%d", dof, gauge, tc.dof, tc.gauge)
			}
			if e.state.Qualified {
				t.Fatal("passive entry acquired fake recording qualification")
			}
		})
	}
}
func TestCylindricalVectorAtomicLimitsAndPath(t *testing.T) {
	svc := New(nil, motionWorker(t))
	s := simpleJointSnapshot(t, "CYLINDRICAL", true)
	e, err := newMechanismMotionSolver(t.Context(), svc, s)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.SelectDrivers(t.Context(), []string{s.Drivers[0].ID}); err == nil {
		t.Fatal("one coordinate hid the other physical freedom")
	}
	if err = e.SelectDrivers(t.Context(), []string{s.Drivers[0].ID, s.Drivers[1].ID}); err != nil {
		t.Fatal(err)
	}
	angular, linear := s.Drivers[0].CoordinateID, s.Drivers[1].CoordinateID
	f, err := e.Advance(t.Context(), map[string]float64{angular: math.Pi / 3, linear: 27})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(f.UnitPoses["b"].Translation[2]-27) > 1e-7 || math.Abs(f.Coordinates[angular]-math.Pi/3) > 1e-7 {
		t.Fatal("coupled vector does not match rigid-body pose", f)
	}
	e.snapshot.Mechanism.Joints[1].Coordinates[1].Upper = &MotionQuantity{Value: 30, Unit: "mm"}
	before, _ := json.Marshal(e.state)
	if _, err = e.Advance(t.Context(), map[string]float64{angular: math.Pi / 2, linear: 40}); err == nil {
		t.Fatal("linear stop ignored")
	}
	after, _ := json.Marshal(e.state)
	if string(before) != string(after) {
		t.Fatal("length rejection partially published angular pose or continuation state")
	}
	if _, err = e.Advance(t.Context(), map[string]float64{angular: math.Pi / 2}); err == nil {
		t.Fatal("incomplete vector accepted")
	}
	s.Study.ActiveDriverIDs = []string{s.Drivers[0].ID, s.Drivers[1].ID}
	s.Study.References = commandReferences(s.Drivers)
	s.Study.Keyframes = []MotionKeyframe{{ID: "start", Values: map[string]MotionQuantity{s.Drivers[0].ID: {Unit: "rad"}, s.Drivers[1].ID: {Unit: "mm"}}}, {ID: "end", TimeSeconds: 2, Values: map[string]MotionQuantity{s.Drivers[0].ID: {Value: math.Pi, Unit: "rad"}, s.Drivers[1].ID: {Value: 20, Unit: "mm"}}}}
	s.Digest = motionDigest(s)
	run, err := svc.RunMotionStudy(t.Context(), s, nil)
	if err != nil || !run.Completed {
		t.Fatal(run.Failure, err)
	}
	exact, err := svc.motionPlayhead(t.Context(), run, .73)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(exact.Coordinates[linear]-7.3) > 1e-7 || math.Abs(exact.Coordinates[angular]-math.Pi*.365) > 1e-7 {
		t.Fatal("exact vector seek adopted an approximate/cache pose", exact)
	}
}
func TestSphericalClosureDoesNotLockRotation(t *testing.T) {
	s := simpleJointSnapshot(t, "SPHERICAL", true)
	p := s.Mechanism.Poses["b"]
	p.Rotation = [4]float64{math.Sin(.6), 0, 0, math.Cos(.6)}
	s.Mechanism.Poses["b"] = p
	if _, err := verifyMechanismJoints(s.Mechanism, s.Mechanism.Poses, nil, nil); err != nil {
		t.Fatal("ball-center closure incorrectly checks z alignment", err)
	}
}
func TestPassivePlanarAndSphericalReduceCylindricalDriveBasis(t *testing.T) {
	svc := New(nil, motionWorker(t))
	for _, kind := range []string{"PLANAR", "SPHERICAL"} {
		t.Run(kind, func(t *testing.T) {
			s := simpleJointSnapshot(t, "CYLINDRICAL", true)
			passive := MechanismJoint{ID: "passive", Name: kind, Kind: kind, First: jointEnd("a", 0, 0), Second: ptrEnd(jointEnd("b", 0, 0)), AxialDirection: "SAME"}
			s.Mechanism.Joints = append(s.Mechanism.Joints, passive)
			var err error
			s.Equations, err = compileMechanismEquations(AssemblySolveManifest{Bodies: s.Equations.Bodies}, s.Mechanism, s.Study, false)
			if err != nil {
				t.Fatal(err)
			}
			s.Digest = motionDigest(s)
			e, err := newMechanismMotionSolver(t.Context(), svc, s)
			if err != nil {
				t.Fatal(err)
			}
			ids, err := e.UniqueDriveSet(t.Context())
			if err != nil || !reflect.DeepEqual(ids, []string{s.Drivers[0].ID}) {
				t.Fatal("native rank did not select the sole surviving angular coordinate", ids, err)
			}
			if err = e.SelectDrivers(t.Context(), ids); err != nil {
				t.Fatal(err)
			}
			if _, err = e.Advance(t.Context(), map[string]float64{s.Drivers[0].CoordinateID: .4}); err != nil {
				t.Fatal("passive closure rejected driven motion", err)
			}
			before, _ := json.Marshal(e.state)
			if err = e.SelectDrivers(t.Context(), []string{s.Drivers[0].ID, s.Drivers[1].ID}); err == nil {
				t.Fatal("eliminated length coordinate admitted as another independent drive")
			}
			after, _ := json.Marshal(e.state)
			if string(before) != string(after) {
				t.Fatal("rejected basis changed the accepted frame")
			}
		})
	}
}

func TestCylindricalAlternativeBasesAndPassiveLimitRollback(t *testing.T) {
	svc := New(nil, motionWorker(t))
	s := simpleJointSnapshot(t, "CYLINDRICAL", true)
	duplicate := s.Mechanism.Joints[1]
	duplicate.ID = "alternative"
	duplicate.Coordinates = append([]JointCoordinate(nil), duplicate.Coordinates...)
	for i := range duplicate.Coordinates {
		duplicate.Coordinates[i].ID = "alternative/" + duplicate.Coordinates[i].Role
	}
	s.Mechanism.Joints = append(s.Mechanism.Joints, duplicate)
	for _, c := range duplicate.Coordinates {
		s.Drivers = append(s.Drivers, MotionDriver{ID: "alternative-driver/" + c.Role, Name: c.Role, MechanismID: s.Mechanism.ID, JointID: duplicate.ID, CoordinateID: c.ID, Reference: &MotionQuantity{Unit: c.unit()}, ReferenceDigest: commandCoordinateBasis(duplicate, c.ID)})
	}
	var err error
	s.Equations, err = compileMechanismEquations(AssemblySolveManifest{Bodies: s.Equations.Bodies}, s.Mechanism, s.Study, false)
	if err != nil {
		t.Fatal(err)
	}
	s.Digest = motionDigest(s)
	e, err := newMechanismMotionSolver(t.Context(), svc, s)
	if err != nil {
		t.Fatal(err)
	}
	if ids, err := e.UniqueDriveSet(t.Context()); err != nil || len(ids) != 0 || e.state.Qualified {
		t.Fatal("ambiguous candidate set was silently chosen", ids, err)
	}
	if err = e.SelectDrivers(t.Context(), []string{s.Drivers[0].ID, s.Drivers[1].ID, s.Drivers[2].ID}); err == nil {
		t.Fatal("redundant angular command passed rank qualification")
	}
	if err = e.SelectDrivers(t.Context(), []string{s.Drivers[3].ID, s.Drivers[0].ID}); err != nil {
		t.Fatal("independent mixed-source basis rejected", err)
	}
	if _, err = e.Advance(t.Context(), map[string]float64{s.Drivers[0].CoordinateID: .4, s.Drivers[3].CoordinateID: 12}); err != nil {
		t.Fatal(err)
	}
	checkpoint, _ := json.Marshal(e.state.Frame.UnitPoses)
	if err = e.SelectDrivers(t.Context(), []string{s.Drivers[2].ID, s.Drivers[1].ID}); err != nil {
		t.Fatal("alternative complete basis rejected", err)
	}
	after, _ := json.Marshal(e.state.Frame.UnitPoses)
	if string(checkpoint) != string(after) {
		t.Fatal("basis switch changed the physical checkpoint")
	}
	// This coordinate is now passive; its stop must still constrain the path.
	e.snapshot.Mechanism.Joints[1].Coordinates[0].Upper = &MotionQuantity{Value: .5, Unit: "rad"}
	before, _ := json.Marshal(e.state)
	if _, err = e.Advance(t.Context(), map[string]float64{s.Drivers[2].CoordinateID: .8, s.Drivers[1].CoordinateID: 15}); err == nil {
		t.Fatal("passive angular stop ignored")
	}
	after, _ = json.Marshal(e.state)
	if string(before) != string(after) {
		t.Fatal("failed mixed continuation retained private successful substeps")
	}
}
