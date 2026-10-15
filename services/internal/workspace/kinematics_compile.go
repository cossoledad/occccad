package workspace

import (
	"fmt"
	"math"
	"slices"

	"github.com/occccad/occccad/internal/geometry"
)

// Compile joints into the existing solver's hard equations. Never use DragTarget.
type mechanismEquations struct {
	Bodies                []geometry.AssemblyBody       `json:"bodies"`
	Geometry              []geometry.AssemblyGeometry   `json:"geometry"`
	Constraints           []geometry.AssemblyConstraint `json:"constraints"`
	RetainedConstraintIDs []string                      `json:"retainedConstraintIds"`
}

// The plane branch is independent of the axis and coordinate directions.
func prismaticOrientation(j MechanismJoint) (float64, error) {
	angle, err := motionValue(j.OrientationAngle, true)
	if j.AxialDirection == "OPPOSITE" {
		angle += math.Pi
	}
	return angle, err
}

func compileMechanism(frozen AssemblySolveManifest, m Mechanism, s MotionStudy) (mechanismEquations, error) {
	return compileMechanismEquations(frozen, m, s, true)
}

func compileMechanismEquations(frozen AssemblySolveManifest, m Mechanism, s MotionStudy, requireDriver bool) (mechanismEquations, error) {
	out := mechanismEquations{Bodies: append([]geometry.AssemblyBody(nil), frozen.Bodies...), Geometry: append([]geometry.AssemblyGeometry(nil), frozen.Geometry...), RetainedConstraintIDs: []string{}}
	if len(frozen.RelativeFixUpdates) > 0 {
		return out, fmt.Errorf("%w: mechanism cannot edit relative Fix baselines", ErrValidation)
	}
	equations, err := interactionFrozenConstraints(frozen)
	if err != nil {
		return out, err
	}
	// Reject missing active equations; diagnostic compilation is not admission.
	covered := map[string]bool{}
	for _, c := range equations {
		covered[c.ID] = true
		if c.GroupID != "" {
			covered[c.GroupID] = true
		}
	}
	for _, c := range frozen.Definitions {
		if !c.Suppressed && c.Mode != "MEASURED" && !covered[c.ID] {
			return out, fmt.Errorf("%w: active assembly constraint %s was not compiled", ErrValidation, c.ID)
		}
	}
	poses := map[string]geometry.AssemblyPose{}
	members := map[string]bool{}
	for _, id := range m.UnitIDs {
		members[id] = true
	}
	for _, body := range out.Bodies {
		poses[body.ID] = body.Pose
		if !members[body.ID] {
			p := body.Pose
			out.Constraints = append(out.Constraints, geometry.AssemblyConstraint{ID: "mechanism/frozen/" + body.ID, Kind: "FIX", FirstBodyID: body.ID, FixedPose: &p})
		}
	}
	for _, j := range m.Joints {
		if !slices.Contains([]string{"GROUND", "REVOLUTE", "PRISMATIC", "CYLINDRICAL", "PLANAR", "RIGID", "SPHERICAL"}, j.Kind) {
			return out, fmt.Errorf("%w: unsupported joint", ErrValidation)
		}
		prefix := "mechanism/" + m.ID + "/" + j.ID
		if j.Kind == "GROUND" {
			if len(j.Constraints) > 0 {
				continue
			}
			p, ok := poses[j.First.InstanceID]
			if !ok {
				return out, fmt.Errorf("%w: missing Ground body", ErrValidation)
			}
			out.Constraints = append(out.Constraints, geometry.AssemblyConstraint{ID: prefix + "/ground", Kind: "FIX", FirstBodyID: j.First.InstanceID, FixedPose: &p})
			continue
		}
		if j.Second == nil {
			return out, fmt.Errorf("%w: joint endpoint missing", ErrValidation)
		}
		if j.Kind == "RIGID" {
			if len(j.Constraints) != 1 || j.Constraints[0].Kind != "FIX_TOGETHER" {
				return out, fmt.Errorf("%w: rigid capture missing", ErrValidation)
			}
			c := j.Constraints[0]
			rigid, e := groupRigidConstraints(c.ID, []string{j.First.InstanceID, j.Second.InstanceID}, c.GroupRelations)
			if e != nil {
				return out, e
			}
			out.Constraints = append(out.Constraints, rigid...)
			continue
		}
		a, b := j.First, *j.Second
		add := func(end JointEndpoint, side string) {
			for _, axis := range []struct {
				name, kind string
				direction  [3]float64
			}{{"p", "POINT", [3]float64{}}, {"x", "AXIS", [3]float64{1, 0, 0}}, {"z", "AXIS", [3]float64{0, 0, 1}}, {"plane", "PLANE", [3]float64{0, 0, 1}}} {
				out.Geometry = append(out.Geometry, geometry.AssemblyGeometry{ID: prefix + "/" + side + "/" + axis.name, BodyID: end.InstanceID, Kind: axis.kind, Origin: end.Frame.Translation, Direction: rotateByPose(end.Frame, axis.direction), LengthUnit: "mm"})
			}
		}
		add(a, "a")
		add(b, "b")
		eq := func(name, kind, first, second string) geometry.AssemblyConstraint {
			return geometry.AssemblyConstraint{ID: prefix + "/" + name, Kind: kind, FirstBodyID: a.InstanceID, FirstGeometryID: prefix + "/a/" + first, SecondBodyID: b.InstanceID, SecondGeometryID: prefix + "/b/" + second, DirectionRelation: "SAME"}
		}
		// Persisted relations are compiled by the shared Product compiler. Pure
		// local-frame numerical fixtures use the same primitive definitions.
		if len(j.Constraints) == 0 {
			switch j.Kind {
			case "REVOLUTE", "PRISMATIC", "CYLINDRICAL":
				out.Constraints = append(out.Constraints, eq("axis", "COINCIDENT", "z", "z"))
				if j.Kind == "REVOLUTE" {
					location := eq("axial-location", "DISTANCE", "plane", "plane")
					location.DistanceRelation = selectedPlaneNormalV1
					out.Constraints = append(out.Constraints, location)
				}
				if j.Kind == "PRISMATIC" {
					angle := eq("orientation", "ANGLE", "x", "x")
					angle.AngleReferenceBodyID = a.InstanceID
					angle.AngleReferenceGeometryID = prefix + "/a/z"
					angle.Value, _ = prismaticOrientation(j)
					out.Constraints = append(out.Constraints, angle)
				}
			case "PLANAR":
				out.Constraints = append(out.Constraints, eq("plane", "COINCIDENT", "plane", "plane"))
			case "SPHERICAL":
				out.Constraints = append(out.Constraints, eq("center", "COINCIDENT", "p", "p"))
			}
		}
	}

	for _, c := range equations {
		out.Constraints = append(out.Constraints, c)
		out.RetainedConstraintIDs = append(out.RetainedConstraintIDs, c.ID)
	}
	return out, nil
}

// Check closure independently of the solver's residual report. Each joint
// checks precisely its physical relation, never the revolute's extra locks.
func verifyMechanismJoints(m Mechanism, poses map[string]InstancePose, previous, targets map[string]float64) (map[string]float64, error) {
	for _, j := range m.Joints {
		if j.Kind == "GROUND" {
			continue
		}
		if j.Second == nil {
			return nil, fmt.Errorf("joint %s second endpoint missing", j.ID)
		}
		a := composeInstancePose(poses[j.First.InstanceID], j.First.Frame)
		b := composeInstancePose(poses[j.Second.InstanceID], j.Second.Frame)
		if !validMotionPose(a) || !validMotionPose(b) {
			return nil, fmt.Errorf("joint %s invalid world frame", j.ID)
		}
		za, zb := rotateByPose(a, [3]float64{0, 0, 1}), rotateByPose(b, [3]float64{0, 0, 1})
		delta := conflictSub(b.Translation, a.Translation)
		fail := false
		switch j.Kind {
		case "REVOLUTE", "PRISMATIC", "CYLINDRICAL":
			fail = conflictNorm(conflictSub(za, zb)) > 1e-8 || conflictNorm(conflictSub(delta, conflictScale(za, conflictDot(delta, za)))) > 1e-7
			if j.Kind == "REVOLUTE" {
				fail = fail || conflictNorm(delta) > 1e-7
			}
			if j.Kind == "PRISMATIC" {
				x, y := rotateByPose(a, [3]float64{1, 0, 0}), rotateByPose(b, [3]float64{1, 0, 0})
				angle := math.Atan2(conflictDot(za, conflictCross(x, y)), conflictDot(x, y))
				goal, _ := prismaticOrientation(j)
				fail = fail || math.Abs(math.Remainder(angle-goal, 2*math.Pi)) > 1e-8
			}
		case "PLANAR":
			if j.AxialDirection == "OPPOSITE" {
				zb = conflictScale(zb, -1)
			}
			fail = conflictNorm(conflictSub(za, zb)) > 1e-8 || math.Abs(conflictDot(delta, za)) > 1e-7
		case "SPHERICAL":
			fail = conflictNorm(delta) > 1e-7
		case "RIGID":
			if len(j.Constraints) != 1 {
				return nil, fmt.Errorf("joint %s capture missing", j.ID)
			}
			c := j.Constraints[0]
			eq, err := groupRigidConstraints(c.ID, []string{j.First.InstanceID, j.Second.InstanceID}, c.GroupRelations)
			if err != nil {
				return nil, err
			}
			actual := inverseRelativePose(poses[j.Second.InstanceID], poses[j.First.InstanceID])
			fail = !conflictPoseClose(geometry.AssemblyPose(actual), *eq[0].FixedPose, 1e-7, 1e-8)
		default:
			return nil, fmt.Errorf("joint %s unsupported kind", j.ID)
		}
		if fail {
			return nil, fmt.Errorf("joint %s closure residual", j.ID)
		}
	}
	return mechanismCoordinates(m, poses, previous, targets)
}
