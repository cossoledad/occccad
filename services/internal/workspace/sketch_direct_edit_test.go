package workspace

import (
	"context"
	"encoding/json"
	"math"
	"testing"
)

func TestFreeSketchExtensionUsesAnalyticSupportAndStableUncutEndpoint(t *testing.T) {
	for _, source := range []SketchEntity{
		{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 3, Y: 7}, End: &SketchPoint2{X: 8, Y: 7}},
		{ID: "arc", Kind: "ARC", Role: "PROFILE", Center: &SketchPoint2{X: 3, Y: 7}, Radius: 5, StartAngle: 5.8, EndAngle: 6.4},
		{ID: "ellipse", Kind: "ELLIPTICAL_ARC", Role: "PROFILE", Center: &SketchPoint2{X: 3, Y: 7}, MajorRadius: 8, MinorRadius: 3, Rotation: .4, StartAngle: 5.8, EndAngle: 6.4},
	} {
		t.Run(source.Kind, func(t *testing.T) {
			source.StartPointID = "original-start"
			source.EndPointID = "original-end"
			target := SketchPoint2{X: 13, Y: 9}
			if source.Kind != "LINE" {
				x, y := source.Radius*math.Cos(7), source.Radius*math.Sin(7)
				if source.Kind == "ELLIPTICAL_ARC" {
					x, y = source.MajorRadius*math.Cos(7), source.MinorRadius*math.Sin(7)
				}
				c, s := math.Cos(source.Rotation), math.Sin(source.Rotation)
				target = SketchPoint2{X: source.Center.X + x*c - y*s, Y: source.Center.Y + x*s + y*c}
			}
			sketch := SketchFeature{SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{source}}
			before, _ := json.Marshal(sketch)
			ref := cornerRef(source.ID, "END")
			op, err := (&Service{}).prepareSketchExtension(context.Background(), &sketch, SketchOperation{Type: "EXTEND_ENTITY", OperationID: "extend", EntityIDs: []string{source.ID}, FirstReference: &ref, Point: &target}, "test")
			if err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(sketch)
			if string(after) != string(before) {
				t.Fatal("preview mutated baseline")
			}
			if len(op.CutConnections) != 0 {
				t.Fatal("free extension invented a boundary constraint")
			}
			if op.Intervals[0].End <= 1 {
				t.Fatal("did not extend")
			}
			result := op.ComputedEntities[0]
			if result.StartPointID != source.StartPointID || result.EndPointID == source.EndPointID {
				t.Fatal("wrong endpoint provenance")
			}
			if source.Kind == "LINE" {
				if result.End.X != 13 || result.End.Y != 7 {
					t.Fatalf("wrong projected endpoint %+v", result.End)
				}
			} else if math.Abs(result.EndAngle-7) > 1e-12 {
				t.Fatalf("wrong seam branch %g", result.EndAngle)
			}
			if err := applySketchOperations(&sketch, []SketchOperation{op}); err != nil {
				t.Fatal(err)
			}
			if err := validateSketch(sketch); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestSketchScalingIsRejectedWithoutMutatingGeometry(t *testing.T) {
	sketch := cornerRectangleFixture()
	before, _ := json.Marshal(sketch)
	scale := 2.0
	err := applySketchOperations(&sketch, []SketchOperation{{Type: "TRANSFORM_ENTITIES", OperationID: "scale", EntityIDs: []string{"bottom", "right", "top", "left"}, Scale: &scale}})
	if err == nil {
		t.Fatal("scaling still accepted")
	}
	after, _ := json.Marshal(sketch)
	if string(after) != string(before) {
		t.Fatal("rejected scaling changed model")
	}
}
func TestCornerGeneratedRelationsDoNotDuplicateSupportMembership(t *testing.T) {
	for _, kind := range []string{"FILLET_ENTITIES", "CHAMFER_ENTITIES"} {
		sketch := cornerRectangleFixture()
		if err := applySketchOperations(&sketch, []SketchOperation{cornerOperation(kind, "corner", "bottom", "left", "START", "END")}); err != nil {
			t.Fatal(err)
		}
		for _, c := range sketch.Constraints {
			if c.Kind == "POINT_ON_OBJECT" && len(c.References) > 0 && c.References[0].EntityID == macroID("corner", "corner/trim/bottom") {
				t.Fatal("cut endpoint has duplicate support equation")
			}
			if c.Kind == "TANGENT" && c.References[0].EntityID == "bottom" {
				t.Fatal("tangency bypasses joined trimmed curve")
			}
		}
	}
}
