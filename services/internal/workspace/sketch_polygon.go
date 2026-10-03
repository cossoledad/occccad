package workspace

import (
	"fmt"
	"math"
)

// Onshape's naming describes the reference circle: INSCRIBED has tangent
// sides outside the circle; CIRCUMSCRIBED has vertices on the circle.
func applySketchPolygon(sketch *SketchFeature, op SketchOperation) error {
	if op.OperationID == "" || op.Point == nil || !finite(op.Point.X) || !finite(op.Point.Y) || op.Value == nil || !finite(*op.Value) || *op.Value <= 0 || !finite(op.Angle) || op.Sides < 3 || op.Sides > 50 || (op.Mode != "INSCRIBED" && op.Mode != "CIRCUMSCRIBED") || (op.Role != "PROFILE" && op.Role != "CONSTRUCTION") {
		return fmt.Errorf("%w: polygon requires a finite center/radius, mode and 3–50 sides", ErrValidation)
	}
	circleID := macroID(op.OperationID, "circle")
	radius := *op.Value
	angle := op.Angle
	step := 2 * math.Pi / float64(op.Sides)
	if op.Mode == "INSCRIBED" {
		radius /= math.Cos(math.Pi / float64(op.Sides))
		angle -= math.Pi / float64(op.Sides)
	}
	ids := make([]string, op.Sides)
	vertices := make([]SketchPoint2, op.Sides)
	for i := range ids {
		ids[i] = macroID(op.OperationID, fmt.Sprintf("edge/%d", i))
		a := angle + float64(i)*step
		vertices[i] = SketchPoint2{X: op.Point.X + radius*math.Cos(a), Y: op.Point.Y + radius*math.Sin(a)}
	}
	ops := []SketchOperation{{Type: "ADD_ENTITY", Entity: &SketchEntity{ID: circleID, Kind: "CIRCLE", Role: "CONSTRUCTION", Center: op.Point, Radius: *op.Value, CreatedByOperationID: op.OperationID}}}
	ref := func(id, sub string) SketchGeometryRef {
		return SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: sub}
	}
	add := func(name, kind string, refs []SketchGeometryRef, value *float64) {
		ops = append(ops, SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &SketchConstraint{ID: macroID(op.OperationID, name), Kind: kind, References: refs, Value: value, Internal: true}})
	}
	for i, id := range ids {
		a, b := vertices[i], vertices[(i+1)%len(ids)]
		ops = append(ops, SketchOperation{Type: "ADD_ENTITY", Entity: &SketchEntity{ID: id, Kind: "LINE", Role: op.Role, Start: &a, End: &b, CreatedByOperationID: op.OperationID}})
	}
	for i, id := range ids {
		add(fmt.Sprintf("join/%d", i), "COINCIDENT", []SketchGeometryRef{ref(id, "END"), ref(ids[(i+1)%len(ids)], "START")}, nil)
		if op.Mode == "INSCRIBED" {
			add(fmt.Sprintf("circle/%d", i), "TANGENT", []SketchGeometryRef{ref(id, "WHOLE"), ref(circleID, "WHOLE")}, nil)
		} else {
			add(fmt.Sprintf("circle/%d", i), "POINT_ON_OBJECT", []SketchGeometryRef{ref(id, "START"), ref(circleID, "WHOLE")}, nil)
		}
		if i > 0 && !(op.Mode == "INSCRIBED" && op.Sides%2 == 0 && i == op.Sides-1) {
			add(fmt.Sprintf("equal/%d", i), "EQUAL", []SketchGeometryRef{ref(ids[0], "WHOLE"), ref(id, "WHOLE")}, nil)
		}
	}
	// Even tangential polygons have an alternating tangent-length freedom.
	// Anchor one tangency at a side midpoint to remove it. The last equal-side
	// equation is then implied by closure; omit it rather than add redundancy.
	// For odd side counts the existing equal-side equations already imply this.
	if op.Mode == "INSCRIBED" && op.Sides%2 == 0 {
		midpointID := macroID(op.OperationID, "midpoint-radius")
		a, b := vertices[0], vertices[1]
		midpoint := SketchPoint2{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
		ops = append(ops, SketchOperation{Type: "ADD_ENTITY", Entity: &SketchEntity{ID: midpointID, Kind: "LINE", Role: "CONSTRUCTION", Start: op.Point, End: &midpoint, CreatedByOperationID: op.OperationID}})
		add("midpoint-on-side", "MIDPOINT", []SketchGeometryRef{ref(midpointID, "END"), ref(ids[0], "WHOLE")}, nil)
		add("midpoint-radius-center", "COINCIDENT", []SketchGeometryRef{ref(midpointID, "START"), ref(circleID, "CENTER")}, nil)
		add("midpoint-radius-normal", "PERPENDICULAR", []SketchGeometryRef{ref(midpointID, "WHOLE"), ref(ids[0], "WHOLE")}, nil)
	}
	if op.FirstReference != nil {
		add("center-connection", "COINCIDENT", []SketchGeometryRef{ref(circleID, "CENTER"), *op.FirstReference}, nil)
	}
	return applySketchOperationsCandidate(sketch, ops)
}
