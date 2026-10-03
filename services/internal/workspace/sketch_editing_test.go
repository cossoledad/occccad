package workspace

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"math"
	"testing"
)

func TestSketchParallelSpacingCompositeAndAtomicity(t *testing.T) {
	sketch := dimensionalSketchFeature("spacing").Sketch
	sketch.Constraints = nil
	sketch.Entities[3].Start = &SketchPoint2{X: 30, Y: 12}
	sketch.Entities[3].End = &SketchPoint2{X: 10, Y: 12}
	value := 12.0
	c := SketchConstraint{ID: "spacing", Kind: "DISTANCE", Value: &value, Unit: "mm", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "line-a", SubElement: "WHOLE"}, {Target: "ENTITY", EntityID: "line-b", SubElement: "DIRECTION"}}}
	if err := applySketchOperations(sketch, []SketchOperation{{Type: "ADD_CONSTRAINT", Constraint: &c}}); err != nil {
		t.Fatal(err)
	}
	if len(sketch.Constraints) != 2 || sketch.Constraints[1].Kind != "PARALLEL" || sketch.Constraints[1].Internal {
		t.Fatalf("visible composite missing: %#v", sketch.Constraints)
	}
	relationID := sketch.Constraints[1].ID
	zero := 0.0
	if err := applySketchOperations(sketch, []SketchOperation{{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: c.ID, Value: &zero}}); err != nil {
		t.Fatal(err)
	}
	if len(sketch.Constraints) != 2 || sketch.Constraints[1].ID != relationID {
		t.Fatal("edit duplicated parallel identity")
	}
	before, _ := json.Marshal(sketch)
	point := SketchPoint2{X: 99, Y: 99}
	err := applySketchOperations(sketch, []SketchOperation{{Type: "UPDATE_ENTITY_POINT", EntityID: "point-a", SubElement: "POINT", Point: &point}, {Type: "UNSUPPORTED"}})
	after, _ := json.Marshal(sketch)
	if err == nil || string(before) != string(after) {
		t.Fatal("failed compound edit mutated sketch")
	}
}

func TestSketchSpacingTypedParameterAndConflict(t *testing.T) {
	model := newPartModel()
	feature := dimensionalSketchFeature("spacing")
	feature.Sketch.Constraints = nil
	model.Features = append(model.Features, feature)
	normalizePartModel(&model)
	value := 5.0
	refs := []SketchGeometryRef{{Target: "ENTITY", EntityID: "line-a", SubElement: "WHOLE"}, {Target: "ENTITY", EntityID: "line-b", SubElement: "WHOLE"}}
	c := SketchConstraint{ID: "gap", Kind: "DISTANCE", Value: &value, Unit: "mm", References: refs}
	before, _ := json.Marshal(model)
	payload, _ := json.Marshal(editSketchPayload{SketchID: feature.ID, Operations: []SketchOperation{{Type: "ADD_CONSTRAINT", Constraint: &c}}})
	after, changes, err := workspaceCommandRegistry.Apply("PART", before, modelcore.DomainCommand{CommandID: "gap", TypeURI: typeEditSketch, SchemaVersion: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	var edited PartModel
	_ = json.Unmarshal(after, &edited)
	if len(changes.Changes) != 2 || len(edited.Parameters) != 1 {
		t.Fatalf("parameter lifecycle changes: %#v", changes)
	}
	if err = validateSketch(*edited.Features[0].Sketch); err != nil {
		t.Fatal(err)
	}
	if edited.Parameters[0].Dimension != modelcore.LengthDimension || edited.Parameters[0].EvaluatedValue.SIValue != 0.005 {
		t.Fatal("wrong quantity")
	}
	angle := 90.0
	conflict := SketchConstraint{ID: "angle", Kind: "ANGLE", References: refs, Value: &angle, Unit: "deg"}
	original := edited.Features[0].Sketch
	snapshot, _ := json.Marshal(original)
	if err = applySketchOperations(original, []SketchOperation{{Type: "ADD_CONSTRAINT", Constraint: &conflict}}); err == nil {
		t.Fatal("incompatible angle accepted")
	}
	unchanged, _ := json.Marshal(original)
	if string(snapshot) != string(unchanged) {
		t.Fatal("conflict was not atomic")
	}
}

func TestSketchLineSpacingDisplayUsesNormalProjection(t *testing.T) {
	entities := map[string]SketchEntity{
		"a": {Kind: "LINE", Start: &SketchPoint2{X: 10, Y: 20}, End: &SketchPoint2{X: 30, Y: 40}},
		"b": {Kind: "LINE", Start: &SketchPoint2{X: 5, Y: 35}, End: &SketchPoint2{X: 25, Y: 55}},
	}
	c := SketchConstraint{Kind: "DISTANCE", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "a", SubElement: "WHOLE"}, {Target: "ENTITY", EntityID: "b", SubElement: "WHOLE"}}}
	visual, ok := constraintVisual(c, entities)
	if !ok {
		t.Fatal("no spacing visualization")
	}
	a, b := visual.Positions[0], visual.Positions[2]
	if (b.X-a.X)+(b.Y-a.Y) != 0 {
		t.Fatalf("dimension did not use normal: %#v %#v", a, b)
	}
}

func TestSketchEllipseProfileAndStableLoopIdentity(t *testing.T) {
	e := SketchEntity{ID: "ellipse", Kind: "ELLIPSE", Role: "PROFILE", Center: &SketchPoint2{X: 23, Y: -11}, MajorRadius: 12, MinorRadius: 4, Rotation: 0.7}
	sketch := SketchFeature{SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{e}}
	if err := validateSketch(sketch); err != nil {
		t.Fatal(err)
	}
	feature := Feature{ID: "sketch", Type: "SKETCH", Sketch: &sketch}
	regions, err := buildProfileRegions(feature)
	if err != nil || len(regions) != 1 {
		t.Fatalf("ellipse profile: %v %#v", err, regions)
	}
	curve := regions[0].Outer.Curves[0]
	if curve.Kind != "ELLIPSE" || curve.MajorRadius != 12 || curve.Rotation != 0.7 {
		t.Fatal("ellipse was flattened")
	}
	first := regions[0].Outer.ID
	sketch.Entities[0].Rotation = 1.5
	regions, err = buildProfileRegions(feature)
	if err != nil || regions[0].Outer.ID != first {
		t.Fatal("dimension edit changed loop identity")
	}
	curves := []geometry.ProfileCurve{{EntityID: "a"}, {EntityID: "b"}, {EntityID: "c"}}
	original := loopID(curves)
	reverse := []geometry.ProfileCurve{{EntityID: "b", Reversed: true}, {EntityID: "a", Reversed: true}, {EntityID: "c", Reversed: true}}
	if original != loopID(reverse) {
		t.Fatal("cyclic shift and traversal reversal changed loop identity")
	}
}

func TestSketchLinkedMirrorClosesHalfProfileWithArc(t *testing.T) {
	sketch := SketchFeature{SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{
		{ID: "bottom", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 0, Y: 0}, End: &SketchPoint2{X: 10, Y: 0}},
		{ID: "arc", Kind: "ARC", Role: "PROFILE", Center: &SketchPoint2{X: 10, Y: 5}, Radius: 5, StartAngle: -math.Pi / 2, EndAngle: math.Pi / 2},
		{ID: "top", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 10, Y: 10}, End: &SketchPoint2{X: 0, Y: 10}},
	}, Constraints: []SketchConstraint{
		{ID: "bottom-arc", Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "bottom", SubElement: "END"}, {Target: "ENTITY", EntityID: "arc", SubElement: "START"}}},
		{ID: "arc-top", Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "arc", SubElement: "END"}, {Target: "ENTITY", EntityID: "top", SubElement: "START"}}},
	}}
	axis := SketchGeometryRef{Target: "SKETCH_Y_AXIS", SubElement: "DIRECTION"}
	op := SketchOperation{Type: "MIRROR_ENTITIES", OperationID: "mirror", EntityIDs: []string{"top", "arc", "bottom"}, Axis: &axis, MirrorMode: "LINKED"}
	if err := applySketchOperations(&sketch, []SketchOperation{op}); err != nil {
		t.Fatal(err)
	}
	if err := validateSketch(sketch); err != nil {
		t.Fatal(err)
	}
	linked := 0
	for _, c := range sketch.Constraints {
		if c.Kind == "MIRROR" {
			linked++
		}
	}
	if linked != 3 {
		t.Fatalf("missing formal associations: %d", linked)
	}
	regions, err := buildProfileRegions(Feature{ID: "half", Sketch: &sketch})
	if err != nil || len(regions) != 1 {
		t.Fatalf("formal mirror did not close: %v", err)
	}
	// The exact geometry encloses a 20x10 rectangle plus two radius-five halves.
	area := polygonArea(sampleProfileLoop(regions[0].Outer))
	if math.Abs(math.Abs(area)-(200+math.Pi*25)) > 0.2 {
		t.Fatalf("unexpected mirror region area %g", area)
	}
}

func TestSketchMirrorSelfMappedGeometryDoesNotDuplicateLoops(t *testing.T) {
	sketch := SketchFeature{SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{Y: 5}, Radius: 2}, {ID: "axis-line", Kind: "LINE", Role: "CONSTRUCTION", Start: &SketchPoint2{Y: 1}, End: &SketchPoint2{Y: 9}}}}
	axis := SketchGeometryRef{Target: "SKETCH_Y_AXIS", SubElement: "DIRECTION"}
	op := SketchOperation{Type: "MIRROR_ENTITIES", OperationID: "self", EntityIDs: []string{"circle", "axis-line"}, Axis: &axis, MirrorMode: "LINKED"}
	if err := applySketchOperations(&sketch, []SketchOperation{op}); err != nil {
		t.Fatal(err)
	}
	if len(sketch.Entities) != 2 || len(sketch.Constraints) != 2 {
		t.Fatal("self mapped geometry must avoid duplicates and retain formal association")
	}
	for _, c := range sketch.Constraints {
		if c.Kind != "MIRROR" || c.References[0].EntityID != c.References[2].EntityID || c.SelfMirrorMode != "ON_AXIS" {
			t.Fatal("self mapped association was not preserved")
		}
	}
	if regions, err := buildProfileRegions(Feature{ID: "self", Sketch: &sketch}); err != nil || len(regions) != 1 {
		t.Fatalf("self mirror profile %v", err)
	}
}

func TestSketchTransformCopyAndSplitMaintainExplicitReferences(t *testing.T) {
	sketch := SketchFeature{SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{
		{ID: "a", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 10}},
		{ID: "b", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 10}, End: &SketchPoint2{X: 10, Y: 10}},
	}, Constraints: []SketchConstraint{{ID: "join", Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "a", SubElement: "END"}, {Target: "ENTITY", EntityID: "b", SubElement: "START"}}}}}
	copy := SketchOperation{Type: "COPY_ENTITIES", OperationID: "copy", EntityIDs: []string{"b", "a"}, ConstraintPolicy: "INTERNAL", Translation: &SketchPoint2{X: 30}}
	if err := applySketchOperations(&sketch, []SketchOperation{copy}); err != nil {
		t.Fatal(err)
	}
	if sketch.Entities[2].Start.X != 30 || sketch.Constraints[1].References[0].EntityID == "a" {
		t.Fatal("copy failed to map internal references")
	}
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "SPLIT_ENTITY", OperationID: "split", EntityIDs: []string{"a"}, Parameters: []float64{0.3, 0.7}}}); err != nil {
		t.Fatal(err)
	}
	if sketch.Entities[0].End.X != 3 {
		t.Fatal("split used wrong parameter")
	}
	last := macroID("split", "segment/a/2")
	if sketch.Constraints[0].References[0].EntityID != last {
		t.Fatal("old END did not retain original endpoint")
	}
	if err := validateSketch(sketch); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(sketch)
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "TRANSFORM_ENTITIES", OperationID: "move-one", EntityIDs: []string{"a"}, Translation: &SketchPoint2{Y: 9}}}); err == nil {
		t.Fatal("cross-selection relation silently released")
	}
	after, _ := json.Marshal(sketch)
	if string(before) != string(after) {
		t.Fatal("rejected move mutated sketch")
	}
}

func TestCanonicalRationalSplineEvaluatesTrueCurve(t *testing.T) {
	e := SketchEntity{Kind: "SPLINE", Mode: "CONTROL", Degree: 2, Poles: []SketchPoint2{{X: 1}, {X: 1, Y: 1}, {Y: 1}}, Knots: []float64{0, 1}, Multiplicities: []uint32{3, 3}, Weights: []float64{1, math.Sqrt(0.5), 1}, ParameterStart: 0, ParameterEnd: 1}
	for _, u := range []float64{0, 0.13, 0.5, 0.83, 1} {
		p, err := evaluateCanonicalSpline(e, u)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(p.X*p.X+p.Y*p.Y-1) > 1e-12 {
			t.Fatalf("rational circle deviated: %#v", p)
		}
	}
	if err := validateCanonicalSpline(e); err != nil {
		t.Fatal(err)
	}
	e.Multiplicities[1] = 2
	if err := validateCanonicalSpline(e); err == nil {
		t.Fatal("malformed canonical knot vector accepted")
	}
}
