package workspace

import (
	"strings"
	"testing"
)

func TestEverySketchConstraintHasAViewportVisual(t *testing.T) {
	t.Parallel()
	value := 12.5
	entities := map[string]SketchEntity{
		"point-a":  {ID: "point-a", Kind: "POINT", Point: &SketchPoint2{X: 0, Y: 0}},
		"point-b":  {ID: "point-b", Kind: "POINT", Point: &SketchPoint2{X: 20, Y: 10}},
		"line-a":   {ID: "line-a", Kind: "LINE", Start: &SketchPoint2{X: 0, Y: 0}, End: &SketchPoint2{X: 20, Y: 0}},
		"line-b":   {ID: "line-b", Kind: "LINE", Start: &SketchPoint2{X: 0, Y: 0}, End: &SketchPoint2{X: 0, Y: 20}},
		"circle-a": {ID: "circle-a", Kind: "CIRCLE", Center: &SketchPoint2{X: 30, Y: 20}, Radius: 8},
		"arc-a":    {ID: "arc-a", Kind: "ARC", Center: &SketchPoint2{X: 50, Y: 20}, Radius: 8, StartAngle: 0, EndAngle: 3.14},
	}
	ref := func(entityID, subElement string) SketchGeometryRef {
		return SketchGeometryRef{Target: "ENTITY", EntityID: entityID, SubElement: subElement}
	}
	constraints := []SketchConstraint{
		{ID: "coincident", Kind: "COINCIDENT", References: []SketchGeometryRef{ref("point-a", "POINT"), ref("line-a", "START")}},
		{ID: "parallel", Kind: "PARALLEL", References: []SketchGeometryRef{ref("line-a", "DIRECTION"), ref("line-b", "DIRECTION")}},
		{ID: "fixed", Kind: "FIXED", References: []SketchGeometryRef{ref("line-a", "WHOLE")}},
		{ID: "fixed-point", Kind: "FIXED_POINT", References: []SketchGeometryRef{ref("point-a", "POINT")}, FixedPoint: &SketchPoint2{X: 0, Y: 0}},
		{ID: "horizontal", Kind: "HORIZONTAL", References: []SketchGeometryRef{ref("line-a", "DIRECTION")}},
		{ID: "vertical", Kind: "VERTICAL", References: []SketchGeometryRef{ref("line-b", "DIRECTION")}},
		{ID: "perpendicular", Kind: "PERPENDICULAR", References: []SketchGeometryRef{ref("line-a", "DIRECTION"), ref("line-b", "DIRECTION")}},
		{ID: "tangent", Kind: "TANGENT", References: []SketchGeometryRef{ref("line-a", "WHOLE"), ref("circle-a", "WHOLE")}},
		{ID: "equal", Kind: "EQUAL", References: []SketchGeometryRef{ref("line-a", "DIRECTION"), ref("line-b", "DIRECTION")}},
		{ID: "distance", Kind: "DISTANCE", References: []SketchGeometryRef{ref("point-a", "POINT"), ref("point-b", "POINT")}, Value: &value, Unit: "mm"},
		{ID: "length", Kind: "LENGTH", References: []SketchGeometryRef{ref("line-a", "DIRECTION")}, Value: &value, Unit: "mm"},
		{ID: "radius", Kind: "RADIUS", References: []SketchGeometryRef{ref("circle-a", "WHOLE")}, Value: &value, Unit: "mm"},
		{ID: "diameter", Kind: "DIAMETER", References: []SketchGeometryRef{ref("arc-a", "WHOLE")}, Value: &value, Unit: "mm"},
		{ID: "angle", Kind: "ANGLE", References: []SketchGeometryRef{ref("line-a", "DIRECTION"), ref("line-b", "DIRECTION")}, Value: &value, Unit: "deg"},
		{ID: "concentric", Kind: "CONCENTRIC", References: []SketchGeometryRef{ref("circle-a", "WHOLE"), ref("arc-a", "WHOLE")}},
		{ID: "point-on-object", Kind: "POINT_ON_OBJECT", References: []SketchGeometryRef{ref("point-a", "POINT"), ref("line-a", "WHOLE")}},
		{ID: "midpoint", Kind: "MIDPOINT", References: []SketchGeometryRef{ref("point-a", "POINT"), ref("line-a", "DIRECTION")}},
		{ID: "symmetry-line", Kind: "SYMMETRY", References: []SketchGeometryRef{ref("point-a", "POINT"), ref("line-a", "DIRECTION"), ref("point-b", "POINT")}},
		{ID: "symmetry-point", Kind: "SYMMETRY", References: []SketchGeometryRef{ref("point-a", "POINT"), ref("point-b", "POINT"), ref("line-a", "START")}},
	}
	for _, constraint := range constraints {
		constraint := constraint
		t.Run(constraint.Kind, func(t *testing.T) {
			visual, ok := constraintVisual(constraint, entities)
			if !ok || len(visual.Positions) == 0 {
				t.Fatalf("%s has no viewport visual: %#v", constraint.Kind, visual)
			}
			dimensional := constraint.Value != nil
			if dimensional && (visual.Kind != "LINE_SEGMENTS" || visual.Label == "" || visual.LabelPosition == nil || len(visual.Positions) < 6) {
				t.Fatalf("%s has no complete dimension leader: %#v", constraint.Kind, visual)
			}
			if !dimensional && visual.Kind != "POINTS" {
				t.Fatalf("%s must use a shader glyph anchor: %#v", constraint.Kind, visual)
			}
		})
	}
}

func TestPointLineDistanceVisualUsesPerpendicularFoot(t *testing.T) {
	value := 7.0
	entities := map[string]SketchEntity{
		"point": {ID: "point", Kind: "POINT", Point: &SketchPoint2{X: 4, Y: 7}},
		"line":  {ID: "line", Kind: "LINE", Start: &SketchPoint2{X: -10, Y: 0}, End: &SketchPoint2{X: 20, Y: 0}},
	}
	visual, ok := constraintVisual(SketchConstraint{Kind: "DISTANCE", Value: &value, Unit: "mm", References: []SketchGeometryRef{
		{Target: "ENTITY", EntityID: "point", SubElement: "POINT"},
		{Target: "ENTITY", EntityID: "line", SubElement: "WHOLE"},
	}}, entities)
	if !ok || len(visual.Positions) < 4 {
		t.Fatalf("missing point-line dimension: %#v", visual)
	}
	if visual.Positions[2].X != 4 || visual.Positions[2].Y != 0 {
		t.Fatalf("dimension uses line midpoint instead of perpendicular foot: %#v", visual.Positions[:4])
	}
}

func TestEntityRoleUpdateIsAtomic(t *testing.T) {
	sketch := SketchFeature{SchemaVersion: 1, Entities: []SketchEntity{{ID: "line", Kind: "LINE", Role: "PROFILE",
		Start: &SketchPoint2{X: 0, Y: 0}, End: &SketchPoint2{X: 10, Y: 0}}}}
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "UPDATE_ENTITY_ROLE", EntityID: "line", Role: "CONSTRUCTION"}}); err != nil {
		t.Fatal(err)
	}
	if sketch.Entities[0].Role != "CONSTRUCTION" {
		t.Fatalf("role was not updated: %#v", sketch.Entities[0])
	}
}

func TestConstraintReferenceCompatibilityRejectsUnsupportedPairs(t *testing.T) {
	t.Parallel()
	kinds := map[string]string{"line": "LINE", "circle": "CIRCLE", "spline": "SPLINE", "point": "POINT"}
	ref := func(id, sub string) SketchGeometryRef {
		return SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: sub}
	}
	tests := []SketchConstraint{
		{Kind: "TANGENT", References: []SketchGeometryRef{ref("line", "WHOLE"), ref("line", "WHOLE")}},
		{Kind: "TANGENT", References: []SketchGeometryRef{ref("spline", "WHOLE"), ref("circle", "WHOLE")}},
		{Kind: "EQUAL", References: []SketchGeometryRef{ref("line", "WHOLE"), ref("circle", "WHOLE")}},
		{Kind: "POINT_ON_OBJECT", References: []SketchGeometryRef{ref("point", "POINT"), ref("spline", "WHOLE")}},
		{Kind: "MIDPOINT", References: []SketchGeometryRef{ref("point", "POINT"), ref("circle", "WHOLE")}},
	}
	for _, constraint := range tests {
		if constraintReferencesCompatible(constraint, kinds) {
			t.Fatalf("accepted incompatible %s references: %#v", constraint.Kind, constraint.References)
		}
	}
}

func TestSketchValidationEnforcesConstraintReferenceSignature(t *testing.T) {
	t.Parallel()
	sketch := SketchFeature{SchemaVersion: 1,
		Entities: []SketchEntity{
			{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 0, Y: 0}, End: &SketchPoint2{X: 10, Y: 0}},
			{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{X: 20, Y: 0}, Radius: 5},
		},
		Constraints: []SketchConstraint{{ID: "invalid-equal", Kind: "EQUAL", References: []SketchGeometryRef{
			{Target: "ENTITY", EntityID: "line", SubElement: "WHOLE"},
			{Target: "ENTITY", EntityID: "circle", SubElement: "WHOLE"},
		}}},
	}
	err := validateSketch(sketch)
	if err == nil || !strings.Contains(err.Error(), "incompatible reference types") {
		t.Fatalf("invalid constraint signature reached the solver: %v", err)
	}
}

func TestSymmetryAcceptsLineOrPointCenterAndRejectsCurveCenter(t *testing.T) {
	t.Parallel()
	kinds := map[string]string{"a": "POINT", "b": "POINT", "center": "POINT", "axis": "LINE", "circle": "CIRCLE"}
	ref := func(id, sub string) SketchGeometryRef {
		return SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: sub}
	}
	for _, constraint := range []SketchConstraint{
		{Kind: "SYMMETRY", References: []SketchGeometryRef{ref("a", "POINT"), ref("axis", "DIRECTION"), ref("b", "POINT")}},
		{Kind: "SYMMETRY", References: []SketchGeometryRef{ref("a", "POINT"), ref("center", "POINT"), ref("b", "POINT")}},
	} {
		if !constraintReferencesCompatible(constraint, kinds) {
			t.Fatalf("rejected symmetry: %#v", constraint)
		}
	}
	invalid := SketchConstraint{Kind: "SYMMETRY", References: []SketchGeometryRef{ref("a", "POINT"), ref("circle", "WHOLE"), ref("b", "POINT")}}
	if constraintReferencesCompatible(invalid, kinds) {
		t.Fatal("accepted circular symmetry center")
	}
}

func TestSketchConstraintPlacementAndValueUpdatesAreAtomicOperations(t *testing.T) {
	t.Parallel()
	value := 10.0
	sketch := SketchFeature{SchemaVersion: 1, Constraints: []SketchConstraint{{ID: "length", Kind: "LENGTH", Value: &value, Unit: "mm"}}}
	updatedValue := 24.0
	position := SketchPoint2{X: 12, Y: -8}
	err := applySketchOperations(&sketch, []SketchOperation{
		{Type: "UPDATE_CONSTRAINT_PLACEMENT", ConstraintID: "length", LabelPosition: &position},
		{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: "length", Value: &updatedValue},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sketch.Constraints[0].Value == nil || *sketch.Constraints[0].Value != updatedValue ||
		sketch.Constraints[0].LabelPosition == nil || *sketch.Constraints[0].LabelPosition != position {
		t.Fatalf("constraint edit did not preserve placement and value: %#v", sketch.Constraints[0])
	}
}
