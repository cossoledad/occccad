package workspace

import (
	"fmt"
	"math"
)

// Distance measures supporting lines, not nearest points on finite segments.
// Its nonnegative magnitude deliberately carries no camera-dependent sign.
func validSketchDimensionValue(kind string, value float64) bool {
	return finite(value) && (kind == "HORIZONTAL_DISTANCE" || kind == "VERTICAL_DISTANCE" || value > 0 || (kind == "DISTANCE" && value == 0))
}

func sketchLineReference(ref SketchGeometryRef, kinds map[string]string) bool {
	return ref.Target == "SKETCH_X_AXIS" || ref.Target == "SKETCH_Y_AXIS" ||
		((ref.Target == "ENTITY" || ref.Target == "EXTERNAL") && kinds[ref.EntityID] == "LINE" && (ref.SubElement == "WHOLE" || ref.SubElement == "DIRECTION"))
}
func sameSketchLine(a, b SketchGeometryRef) bool {
	return a.Target == b.Target && a.EntityID == b.EntityID
}
func sketchLinePair(refs []SketchGeometryRef, a, b SketchGeometryRef) bool {
	return len(refs) == 2 && ((sameSketchLine(refs[0], a) && sameSketchLine(refs[1], b)) || (sameSketchLine(refs[0], b) && sameSketchLine(refs[1], a)))
}
func sketchParallelRelation(sketch *SketchFeature, a, b SketchGeometryRef) bool {
	if sameSketchLine(a, b) {
		return true
	}
	axis := func(ref SketchGeometryRef) string {
		if ref.Target == "SKETCH_X_AXIS" {
			return "HORIZONTAL"
		}
		if ref.Target == "SKETCH_Y_AXIS" {
			return "VERTICAL"
		}
		return ""
	}
	aa, bb := axis(a), axis(b)
	for _, c := range sketch.Constraints {
		if c.Suppressed {
			continue
		}
		if c.Kind == "PARALLEL" && sketchLinePair(c.References, a, b) {
			return true
		}
		if (c.Kind == "HORIZONTAL" || c.Kind == "VERTICAL") && len(c.References) == 1 {
			if sameSketchLine(c.References[0], a) {
				aa = c.Kind
			}
			if sameSketchLine(c.References[0], b) {
				bb = c.Kind
			}
		}
	}
	return aa != "" && aa == bb
}

// The visible parallel relation is a normal persistent constraint, with its
// own identity and diagnostics. It is never replaced by hidden fixed points.
func ensureSketchDistanceRelations(sketch *SketchFeature) error {
	kinds := map[string]string{}
	for _, e := range sketch.Entities {
		kinds[e.ID] = e.Kind
	}
	for _, e := range sketch.ExternalGeometry {
		kinds[e.ID] = e.GeometryKind
		if e.Snapshot != nil {
			kinds[e.ID] = e.Snapshot.Kind
		}
	}
	for _, c := range append([]SketchConstraint(nil), sketch.Constraints...) {
		if c.Suppressed || c.Reference || c.Kind != "DISTANCE" || len(c.References) != 2 {
			continue
		}
		a, b := c.References[0], c.References[1]
		if !sketchLineReference(a, kinds) || !sketchLineReference(b, kinds) {
			continue
		}
		if sameSketchLine(a, b) && c.Value != nil && *c.Value != 0 {
			return fmt.Errorf("%w: a line has zero distance to itself", ErrValidation)
		}
		for _, other := range sketch.Constraints {
			if other.Suppressed || !sketchLinePair(other.References, a, b) {
				continue
			}
			if other.Kind == "PERPENDICULAR" || (other.Kind == "ANGLE" && other.Value != nil && math.Remainder(*other.Value, 180) != 0) {
				return fmt.Errorf("%w: parallel spacing conflicts with constraint %s", ErrValidation, other.ID)
			}
		}
		if !sketchParallelRelation(sketch, a, b) {
			id := macroID(c.ID, "parallel-spacing")
			for _, other := range sketch.Constraints {
				if other.ID == id {
					return fmt.Errorf("%w: parallel spacing identity collision", ErrValidation)
				}
			}
			sketch.Constraints = append(sketch.Constraints, SketchConstraint{ID: id, Kind: "PARALLEL", References: []SketchGeometryRef{a, b}})
		}
	}
	return nil
}
