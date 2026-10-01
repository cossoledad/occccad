package workspace

import (
	"github.com/occccad/occccad/internal/geometry"
	"math"
)

// Static branch infeasibility is separate from command syntax and numerical
// convergence. This gate does not compute residuals or adopt candidate poses.
// It isolates only the failed definition, never via user suppression.
func contactDefinitionInfeasibility(c geometry.AssemblyConstraint, a, b geometry.AssemblyGeometry) string {
	if c.Kind != "CONTACT" {
		return ""
	}
	closeParameter := func(x, y float64) bool { return math.Abs(x-y) <= 1e-12*math.Max(1, math.Max(math.Abs(x), math.Abs(y))) }
	material := func(g geometry.AssemblyGeometry) int32 {
		if g.MaterialSide == 0 {
			return 1
		}
		return g.MaterialSide
	}
	external := c.ContactSide == "EXTERNAL"
	if a.Kind == "PLANE" || b.Kind == "PLANE" {
		other := b
		if b.Kind == "PLANE" {
			other = a
		}
		if other.Kind == "SPHERE" || other.Kind == "CYLINDER" {
			branch := material(other)
			if !external {
				branch = -branch
			}
			if c.ContactBranch != branch {
				return "CONTACT_PLANE_MATERIAL_BRANCH_MISMATCH"
			}
		}
		return ""
	}
	if c.ContactKind == "FACE" && a.Kind == b.Kind {
		if a.Kind == "CONE" {
			if !closeParameter(a.HalfAngle, b.HalfAngle) {
				return "CONTACT_FACE_ANGLE_MISMATCH"
			}
		} else if !closeParameter(a.Radius, b.Radius) {
			return "CONTACT_FACE_RADIUS_MISMATCH"
		}
	}
	if c.ContactKind == "FACE" || (c.ContactKind == "RING" && ((a.Kind == "SPHERE" && b.Kind == "CONE") || (b.Kind == "SPHERE" && a.Kind == "CONE"))) {
		expected := int32(1)
		if external {
			expected = -1
		}
		if material(a)*material(b) != expected {
			return "CONTACT_MATERIAL_SIDE_MISMATCH"
		}
	}
	if c.ContactKind == "RING" && ((a.Kind == "SPHERE" && b.Kind == "CIRCLE") || (b.Kind == "SPHERE" && a.Kind == "CIRCLE")) {
		sphere, circle := a, b
		if b.Kind == "SPHERE" {
			sphere, circle = b, a
		}
		if circle.Radius > sphere.Radius {
			return "CONTACT_CIRCLE_EXCEEDS_SPHERE"
		}
	}
	if c.ContactKind == "LINE" && a.Kind == b.Kind && !((external) == (material(a)*material(b) > 0)) {
		if (a.Kind == "CYLINDER" && a.Radius == b.Radius) || (a.Kind == "CONE" && a.HalfAngle == b.HalfAngle) {
			return "CONTACT_LINE_DEGENERATES_TO_FACE"
		}
	}
	return ""
}
