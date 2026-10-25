package workspace

import "testing"

func TestSketchDimensionPlacementsFollowPointEdits(t *testing.T) {
	s := SketchFeature{SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 10}}}, Constraints: []SketchConstraint{{ID: "length", Kind: "LENGTH", Unit: "mm", LabelPosition: &SketchPoint2{X: 5, Y: -8}, References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "line", SubElement: "WHOLE"}}}}}
	if err := applySketchOperations(&s, []SketchOperation{{Type: "UPDATE_ENTITY_POINT", EntityID: "line", SubElement: "START", Point: &SketchPoint2{X: 20, Y: 30}}, {Type: "UPDATE_ENTITY_POINT", EntityID: "line", SubElement: "END", Point: &SketchPoint2{X: 30, Y: 30}}}); err != nil {
		t.Fatal(err)
	}
	if got := *s.Constraints[0].LabelPosition; got != (SketchPoint2{X: 25, Y: 22}) {
		t.Fatal(got)
	}
	before := *s.Constraints[0].LabelPosition
	carrySketchDimensionPlacements(&s, sketchDimensionPlacements(&s))
	if *s.Constraints[0].LabelPosition != before {
		t.Fatal("unchanged geometry moved label")
	}
	positions := sketchDimensionPlacements(&s)
	s.Constraints[0].LabelPosition = &SketchPoint2{X: 99, Y: 88}
	s.Entities[0].End = &SketchPoint2{X: 40, Y: 30}
	carrySketchDimensionPlacements(&s, positions)
	if *s.Constraints[0].LabelPosition != (SketchPoint2{X: 99, Y: 88}) {
		t.Fatal("explicit layout overridden")
	}
}
func TestDeleteSketchProjectionAndDependentDimensions(t *testing.T) {
	s := SketchFeature{Entities: []SketchEntity{{ID: "local", Kind: "POINT", Role: "PROFILE", Point: &SketchPoint2{}}}, ExternalGeometry: []SketchExternalGeometry{{ID: "broken", Status: "BROKEN"}, {ID: "source", Status: "CONNECTED"}}, Constraints: []SketchConstraint{{ID: "distance", Kind: "DISTANCE", References: []SketchGeometryRef{{Target: "EXTERNAL", EntityID: "source", SubElement: "POINT"}, {Target: "ENTITY", EntityID: "local", SubElement: "POINT"}}}}}
	if err := applySketchGeometryEdit(&s, SketchOperation{Type: "DELETE_ENTITIES", EntityIDs: []string{"source", "broken"}}); err != nil {
		t.Fatal(err)
	}
	if len(s.ExternalGeometry) != 0 || len(s.Constraints) != 0 || len(s.Entities) != 1 {
		t.Fatalf("wrong deletion scope: %+v", s)
	}
}
