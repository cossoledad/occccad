package workspace

import (
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
	"math"
	"slices"
)

func jointCoordinateRoles(kind string) []string {
	switch kind {
	case "REVOLUTE":
		return []string{"AXIAL_ROTATION"}
	case "PRISMATIC":
		return []string{"AXIAL_TRANSLATION"}
	case "CYLINDRICAL":
		return []string{"AXIAL_ROTATION", "AXIAL_TRANSLATION"}
	case "GROUND", "PLANAR", "RIGID", "SPHERICAL":
		return nil
	}
	return nil
}
func (c JointCoordinate) angular() bool { return c.Dimension == "ANGLE" }
func (c JointCoordinate) unit() string {
	if c.angular() {
		return "rad"
	}
	return "mm"
}
func findJointCoordinate(m Mechanism, id string) (MechanismJoint, JointCoordinate, bool) {
	for _, j := range m.Joints {
		for _, c := range j.Coordinates {
			if c.ID == id {
				return j, c, true
			}
		}
	}
	return MechanismJoint{}, JointCoordinate{}, false
}
func driverCoordinate(m Mechanism, d MotionDriver) (MechanismJoint, JointCoordinate, error) {
	j, c, ok := findJointCoordinate(m, d.CoordinateID)
	if !ok || j.ID != d.JointID || d.MechanismID != m.ID {
		return j, c, fmt.Errorf("%w: invalid command coordinate", ErrValidation)
	}
	return j, c, nil
}
func motionJointCoordinate(j MechanismJoint, c JointCoordinate, poses map[string]InstancePose, previous *float64) (float64, error) {
	if j.Second == nil {
		return 0, fmt.Errorf("%w: coordinate endpoint missing", ErrValidation)
	}
	a := composeInstancePose(poses[j.First.InstanceID], j.First.Frame)
	b := composeInstancePose(poses[j.Second.InstanceID], j.Second.Frame)
	if !validMotionPose(a) || !validMotionPose(b) {
		return 0, fmt.Errorf("%w: invalid coordinate pose", ErrValidation)
	}
	z := rotateByPose(a, [3]float64{0, 0, 1})
	raw := 0.0
	switch c.Role {
	case "AXIAL_ROTATION":
		x, y := rotateByPose(a, [3]float64{1, 0, 0}), rotateByPose(b, [3]float64{1, 0, 0})
		x = conflictSub(x, conflictScale(z, conflictDot(x, z)))
		y = conflictSub(y, conflictScale(z, conflictDot(y, z)))
		if conflictNorm(x) < 1e-10 || conflictNorm(y) < 1e-10 {
			return 0, fmt.Errorf("%w: degenerate angular coordinate", ErrValidation)
		}
		raw = math.Atan2(conflictDot(z, conflictCross(x, y)), conflictDot(x, y))
	case "AXIAL_TRANSLATION":
		raw = conflictDot(conflictSub(b.Translation, a.Translation), z)
	default:
		return 0, fmt.Errorf("%w: unknown coordinate role", ErrValidation)
	}
	zero, err := motionValue(c.Zero, c.angular())
	if err != nil {
		return 0, err
	}
	q := (raw - zero) * float64(c.Direction)
	if c.angular() && previous != nil {
		q += 2 * math.Pi * math.Round((*previous-q)/(2*math.Pi))
	}
	if !finiteMotion(q) {
		return 0, fmt.Errorf("coordinate %s overflow", c.ID)
	}
	return q, nil
}
func mechanismCoordinateEquation(mechanismID string, j MechanismJoint, c JointCoordinate) geometry.AssemblyConstraint {
	prefix := "mechanism/" + mechanismID + "/" + j.ID
	d := geometry.AssemblyConstraint{ID: prefix + "/coordinate/" + c.ID, FirstBodyID: j.First.InstanceID, SecondBodyID: j.Second.InstanceID}
	if c.angular() {
		d.Kind = "ANGLE"
		d.FirstGeometryID = prefix + "/a/x"
		d.SecondGeometryID = prefix + "/b/x"
		d.AngleReferenceBodyID = j.First.InstanceID
		d.AngleReferenceGeometryID = prefix + "/a/z"
	} else {
		// Signed point-to-plane distance, along the first endpoint's frozen Z.
		d.Kind = "DISTANCE"
		d.FirstGeometryID = prefix + "/a/plane"
		d.SecondGeometryID = prefix + "/b/p"
		d.DistanceRelation = selectedPlaneNormalV1
	}
	return d
}
func mechanismCoordinates(m Mechanism, poses map[string]InstancePose, previous, targets map[string]float64) (map[string]float64, error) {
	out := map[string]float64{}
	for _, j := range m.Joints {
		for _, c := range j.Coordinates {
			var prior *float64
			if q, ok := previous[c.ID]; ok {
				prior = &q
			}
			target, driven := targets[c.ID]
			if driven {
				prior = &target
			}
			q, err := motionJointCoordinate(j, c, poses, prior)
			if err != nil {
				return nil, err
			}
			if driven && math.Abs(q-target) > 1e-7 {
				return nil, fmt.Errorf("hard coordinate %s residual %g", c.ID, q-target)
			}
			if c.angular() && !driven && prior != nil && math.Abs(q-*prior) > math.Pi/2 {
				return nil, fmt.Errorf("coordinate %s branch discontinuity", c.ID)
			}
			out[c.ID] = q
		}
	}
	return out, nil
}
func validateJointCoordinates(j MechanismJoint, claim func(string) bool) error {
	roles := jointCoordinateRoles(j.Kind)
	if len(roles) != len(j.Coordinates) {
		return fmt.Errorf("%w: incomplete joint coordinates", ErrValidation)
	}
	seen := map[string]bool{}
	for _, c := range j.Coordinates {
		if !claim(c.ID) || seen[c.Role] || !slices.Contains(roles, c.Role) || c.Direction != 1 && c.Direction != -1 ||
			(c.Role == "AXIAL_ROTATION" && c.Dimension != "ANGLE") || (c.Role == "AXIAL_TRANSLATION" && c.Dimension != "LENGTH") {
			return fmt.Errorf("%w: invalid joint coordinate", ErrValidation)
		}
		seen[c.Role] = true
		if _, err := motionValue(c.Zero, c.angular()); err != nil {
			return err
		}
		for _, q := range []*MotionQuantity{c.Lower, c.Upper} {
			if q != nil {
				if _, err := motionValue(*q, c.angular()); err != nil {
					return err
				}
			}
		}
		if c.Lower != nil && c.Upper != nil {
			lo, _ := motionValue(*c.Lower, c.angular())
			hi, _ := motionValue(*c.Upper, c.angular())
			if lo >= hi {
				return fmt.Errorf("%w: reversed coordinate limits", ErrValidation)
			}
		}
	}
	return nil
}
func motionLimits(m Mechanism, coords map[string]float64) error {
	for _, j := range m.Joints {
		for _, c := range j.Coordinates {
			q, ok := coords[c.ID]
			if !ok {
				return fmt.Errorf("INPUT_ERROR: coordinate %s missing", c.ID)
			}
			if err := motionCoordinateLimit(c, q); err != nil {
				return err
			}
		}
	}
	return nil
}
func motionCoordinateLimit(c JointCoordinate, q float64) error {
	for _, bound := range []struct {
		quantity *MotionQuantity
		lower    bool
	}{{c.Lower, true}, {c.Upper, false}} {
		if bound.quantity == nil {
			continue
		}
		v, err := motionValue(*bound.quantity, c.angular())
		if err != nil {
			return err
		}
		if bound.lower && q < v-1e-8 || !bound.lower && q > v+1e-8 {
			return fmt.Errorf("LIMIT_REACHED: coordinate %s", c.ID)
		}
	}
	return nil
}
func commandReferences(ds []MotionDriver) map[string]MotionQuantity {
	out := map[string]MotionQuantity{}
	for _, d := range ds {
		if d.Reference != nil {
			out[d.ID] = *d.Reference
		}
	}
	return out
}
func commandValues(m Mechanism, ds []MotionDriver, coordinates map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for _, d := range ds {
		_, c, err := driverCoordinate(m, d)
		if err != nil || d.Reference == nil {
			continue
		}
		ref, _ := motionValue(*d.Reference, c.angular())
		out[d.ID] = coordinates[c.ID] - ref
	}
	return out
}
func physicalTargets(m Mechanism, ds []MotionDriver, values map[string]MotionQuantity, refs map[string]MotionQuantity) (map[string]float64, error) {
	if len(values) != len(ds) || len(refs) != len(ds) {
		return nil, fmt.Errorf("%w: incomplete command vector", ErrValidation)
	}
	out := map[string]float64{}
	for _, d := range ds {
		_, c, err := driverCoordinate(m, d)
		if err != nil {
			return nil, err
		}
		v, ok := values[d.ID]
		r, found := refs[d.ID]
		if !ok || !found {
			return nil, fmt.Errorf("%w: command vector identity mismatch", ErrValidation)
		}
		q, err := motionValue(v, c.angular())
		if err != nil {
			return nil, err
		}
		ref, err := motionValue(r, c.angular())
		if err != nil || !finiteMotion(q+ref) {
			return nil, fmt.Errorf("%w: invalid physical command target", ErrValidation)
		}
		out[c.ID] = q + ref
	}
	return out, nil
}
func motionVectorQuantities(m Mechanism, ds []MotionDriver, values map[string]float64) map[string]MotionQuantity {
	out := map[string]MotionQuantity{}
	for _, d := range ds {
		_, c, err := driverCoordinate(m, d)
		if err == nil {
			out[d.ID] = MotionQuantity{Value: values[d.ID], Unit: c.unit()}
		}
	}
	return out
}
func sameMotionVector(a, b map[string]MotionQuantity) bool {
	if len(a) != len(b) {
		return false
	}
	for id, q := range a {
		v, ok := b[id]
		angular := q.Unit == "rad" || q.Unit == "deg"
		x, e := motionValue(q, angular)
		y, f := motionValue(v, angular)
		if !ok || e != nil || f != nil || math.Abs(x-y) > 1e-9 {
			return false
		}
	}
	return true
}

func mechanismHasCoordinate(m Mechanism, id string) bool {
	_, _, ok := findJointCoordinate(m, id)
	return ok
}
func snapshotDrivers(s MotionSnapshot, ids []string) ([]MotionDriver, error) {
	out := []MotionDriver{}
	seen := map[string]bool{}
	for _, id := range ids {
		index := slices.IndexFunc(s.Drivers, func(d MotionDriver) bool { return d.ID == id && d.MechanismID == s.Mechanism.ID })
		if index < 0 || seen[id] {
			return nil, fmt.Errorf("%w: invalid active command set", ErrValidation)
		}
		seen[id] = true
		out = append(out, s.Drivers[index])
	}
	return out, nil
}
func sameDriverSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, id := range a {
		if seen[id] {
			return false
		}
		seen[id] = true
	}
	for _, id := range b {
		if !seen[id] {
			return false
		}
		delete(seen, id)
	}
	return len(seen) == 0
}
func validateJointSupports(j MechanismJoint) error {
	if j.Kind == "GROUND" {
		return nil
	}
	if j.Second == nil {
		return fmt.Errorf("%w: missing second endpoint", ErrValidation)
	}
	for _, end := range []JointEndpoint{j.First, *j.Second} {
		valid := false
		switch j.Kind {
		case "REVOLUTE", "PRISMATIC":
			valid = end.Axis != nil && end.Plane != nil
		case "CYLINDRICAL":
			valid = end.Axis != nil
		case "PLANAR":
			valid = end.Plane != nil
		case "SPHERICAL":
			valid = end.Point != nil
		case "RIGID":
			valid = end.InstanceID != ""
		}
		if !valid {
			return fmt.Errorf("%w: incomplete joint supports", ErrValidation)
		}
	}
	return nil
}

func wrappedMotionAngle(angle float64) float64 {
	v := math.Mod(angle, 2*math.Pi)
	if v < 0 {
		v += 2 * math.Pi
	}
	return v
}
func angularCoordinateFrame(frame InstancePose) (InstancePose, error) {
	direction := rotateByPose(frame, [3]float64{1, 0, 0})
	axis := geometry.AssemblyGeometry{Kind: "AXIS", Origin: frame.Translation, Direction: direction}
	plane := geometry.AssemblyGeometry{Kind: "PLANE", Origin: frame.Translation, Direction: direction}
	return motionFrameFromSupports(axis, plane, rotateByPose(frame, [3]float64{0, 0, 1}))
}
