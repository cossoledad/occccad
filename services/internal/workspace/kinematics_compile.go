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
	ReplacedConstraintID  string                        `json:"replacedConstraintId,omitempty"`
	Driver                geometry.AssemblyConstraint   `json:"driver"`
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
		prefix := "mechanism/" + m.ID + "/" + j.ID
		if j.Kind == "GROUND" {
			p, ok := poses[j.First.InstanceID]
			if !ok {
				return out, fmt.Errorf("%w: missing Ground body", ErrValidation)
			}
			out.Constraints = append(out.Constraints, geometry.AssemblyConstraint{ID: prefix + "/ground", Kind: "FIX", FirstBodyID: j.First.InstanceID, FixedPose: &p})
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
		// A joint owns the same relations as assembly design. Frames have already
		// incorporated the signed axial offset; their locating planes coincide.
		axis := eq("axis", "COINCIDENT", "z", "z")
		if err := compileAssemblyRelation(AssemblyConstraint{Kind: "COINCIDENT", Family: "Coincidence"}, &axis, geometry.AssemblyGeometry{Kind: "AXIS"}, geometry.AssemblyGeometry{Kind: "AXIS"}); err != nil {
			return out, err
		}
		out.Constraints = append(out.Constraints, axis)
		if j.Kind != "PRISMATIC" {
			location := eq("axial-location", "DISTANCE", "plane", "plane")
			location.DirectionRelation = "UNORIENTED"
			if err := compileAssemblyRelation(AssemblyConstraint{Kind: "DISTANCE", DistanceRelation: selectedPlaneNormalV1}, &location, geometry.AssemblyGeometry{Kind: "PLANE"}, geometry.AssemblyGeometry{Kind: "PLANE"}); err != nil {
				return out, err
			}
			out.Constraints = append(out.Constraints, location)
		}
		if j.Kind == "RIGID" || j.Kind == "PRISMATIC" {
			out.Constraints = append(out.Constraints, eq("rotation", "PARALLEL", "x", "x"))
		}
		if j.ID == s.DriverJointID {
			if j.Kind == "REVOLUTE" {
				out.Driver = eq("driver", "ANGLE", "x", "x")
				out.Driver.AngleReferenceBodyID = a.InstanceID
				out.Driver.AngleReferenceGeometryID = prefix + "/a/z"
				out.Driver.DirectionRelation = ""
			} else {
				out.Driver = geometry.AssemblyConstraint{ID: prefix + "/driver", Kind: "DISTANCE", FirstBodyID: b.InstanceID, FirstGeometryID: prefix + "/b/p", SecondBodyID: a.InstanceID, SecondGeometryID: prefix + "/a/plane", DistanceRelation: "ALONG_SECOND_NORMAL"}
			}
		}
	}
	if requireDriver && out.Driver.ID == "" {
		return out, fmt.Errorf("%w: no independent driver", ErrValidation)
	}
	for _, c := range equations {
		if c.ID == s.ReplaceDriverConstraintID {
			if !sameMotionCoordinate(c, out.Driver, out.Geometry) {
				return out, fmt.Errorf("%w: replacement is not the same directed joint coordinate", ErrValidation)
			}
			out.ReplacedConstraintID = c.ID
		} else {
			out.Constraints = append(out.Constraints, c)
			out.RetainedConstraintIDs = append(out.RetainedConstraintIDs, c.ID)
		}
	}
	if s.ReplaceDriverConstraintID != "" && out.ReplacedConstraintID == "" {
		return out, fmt.Errorf("%w: driver replacement equation missing", ErrValidation)
	}
	return out, nil
}
func sameMotionCoordinate(c, d geometry.AssemblyConstraint, values []geometry.AssemblyGeometry) bool {
	if c.Kind != d.Kind || c.FirstBodyID != d.FirstBodyID || c.SecondBodyID != d.SecondBodyID || c.GroupID != "" || c.Mode == "MEASURED" {
		return false
	}
	get := func(body, id string) (geometry.AssemblyGeometry, bool) {
		i := slices.IndexFunc(values, func(g geometry.AssemblyGeometry) bool { return g.BodyID == body && g.ID == id })
		if i < 0 {
			return geometry.AssemblyGeometry{}, false
		}
		return values[i], true
	}
	equal := func(ab, ai, bb, bi string) bool {
		a, ok := get(ab, ai)
		b, ok2 := get(bb, bi)
		if !ok || !ok2 || a.Kind != b.Kind || a.LengthUnit != b.LengthUnit {
			return false
		}
		for i := range 3 {
			if math.Abs(a.Origin[i]-b.Origin[i]) > 1e-9 || math.Abs(a.Direction[i]-b.Direction[i]) > 1e-10 {
				return false
			}
		}
		return true
	}
	if !equal(c.FirstBodyID, c.FirstGeometryID, d.FirstBodyID, d.FirstGeometryID) || !equal(c.SecondBodyID, c.SecondGeometryID, d.SecondBodyID, d.SecondGeometryID) {
		return false
	}
	if c.Kind == "ANGLE" {
		return c.AngleReferenceDirection == nil && !c.ReverseAngleReference && c.AngleReferenceBodyID == d.AngleReferenceBodyID && equal(c.AngleReferenceBodyID, c.AngleReferenceGeometryID, d.AngleReferenceBodyID, d.AngleReferenceGeometryID)
	}
	return c.DistanceRelation == d.DistanceRelation
}
func motionJointCoordinate(j MechanismJoint, poses map[string]InstancePose, previous *float64) (float64, error) {
	a := composeInstancePose(poses[j.First.InstanceID], j.First.Frame)
	b := composeInstancePose(poses[j.Second.InstanceID], j.Second.Frame)
	z := rotateByPose(a, [3]float64{0, 0, 1})
	zero, _ := motionValue(j.Zero, j.Kind == "REVOLUTE")
	raw := 0.0
	if j.Kind == "REVOLUTE" {
		x, y := rotateByPose(a, [3]float64{1, 0, 0}), rotateByPose(b, [3]float64{1, 0, 0})
		cross := [3]float64{x[1]*y[2] - x[2]*y[1], x[2]*y[0] - x[0]*y[2], x[0]*y[1] - x[1]*y[0]}
		raw = math.Atan2(z[0]*cross[0]+z[1]*cross[1]+z[2]*cross[2], x[0]*y[0]+x[1]*y[1]+x[2]*y[2])
	} else {
		for i := range 3 {
			raw += (b.Translation[i] - a.Translation[i]) * z[i]
		}
	}
	q := (raw - zero) * float64(j.Direction)
	if j.Kind == "REVOLUTE" && previous != nil {
		q += 2 * math.Pi * math.Round((*previous-q)/(2*math.Pi))
	}
	if !finiteMotion(q) {
		return 0, fmt.Errorf("joint %s coordinate overflow", j.ID)
	}
	return q, nil
}
func verifyMechanismJoints(m Mechanism, poses map[string]InstancePose, previous map[string]float64, driverID string, target float64) (map[string]float64, error) {
	result := map[string]float64{}
	norm := func(v [3]float64) float64 { return math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2]) }
	sub := func(a, b [3]float64) [3]float64 { return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
	for _, j := range m.Joints {
		if j.Kind == "GROUND" {
			continue
		}
		a := composeInstancePose(poses[j.First.InstanceID], j.First.Frame)
		b := composeInstancePose(poses[j.Second.InstanceID], j.Second.Frame)
		if !validMotionPose(a) || !validMotionPose(b) {
			return nil, fmt.Errorf("joint %s invalid world frame", j.ID)
		}
		za, zb := rotateByPose(a, [3]float64{0, 0, 1}), rotateByPose(b, [3]float64{0, 0, 1})
		if norm(sub(za, zb)) > 1e-8 {
			return nil, fmt.Errorf("joint %s axis residual", j.ID)
		}
		delta := sub(b.Translation, a.Translation)
		if j.Kind == "PRISMATIC" {
			projection := delta[0]*za[0] + delta[1]*za[1] + delta[2]*za[2]
			for i := range 3 {
				delta[i] -= projection * za[i]
			}
		}
		if length := norm(delta); !finiteMotion(length) || length > 1e-7 {
			return nil, fmt.Errorf("joint %s closure residual", j.ID)
		}
		if j.Kind == "RIGID" || j.Kind == "PRISMATIC" {
			if norm(sub(rotateByPose(a, [3]float64{1, 0, 0}), rotateByPose(b, [3]float64{1, 0, 0}))) > 1e-8 {
				return nil, fmt.Errorf("joint %s rotation residual", j.ID)
			}
		}
		if j.Kind != "REVOLUTE" && j.Kind != "PRISMATIC" {
			continue
		}
		var prior *float64
		if p, ok := previous[j.ID]; ok {
			prior = &p
		}
		if j.ID == driverID {
			prior = &target
		}
		q, err := motionJointCoordinate(j, poses, prior)
		if err != nil {
			return nil, err
		}
		if j.ID == driverID && math.Abs(q-target) > 1e-7 {
			return nil, fmt.Errorf("hard driver coordinate residual %g", q-target)
		}
		if j.Kind == "REVOLUTE" && j.ID != driverID && prior != nil && math.Abs(q-*prior) > math.Pi/2 {
			return nil, fmt.Errorf("joint %s branch discontinuity", j.ID)
		}
		result[j.ID] = q
	}
	return result, nil
}
