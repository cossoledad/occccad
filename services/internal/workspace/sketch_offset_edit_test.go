package workspace

import (
	"encoding/json"
	"math"
	"testing"
)

func TestSketchOffsetLineCircleArcSignedLeftNormalAndIdentity(t *testing.T) {
	distance := 2.0
	fixtures := []SketchEntity{
		{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{1, 3}, End: &SketchPoint2{5, 3}},
		{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{20, 30}, Radius: 5},
		{ID: "arc", Kind: "ARC", Role: "PROFILE", Center: &SketchPoint2{40, 30}, Radius: 5, StartAngle: 0, EndAngle: -math.Pi / 2},
	}
	for _, source := range fixtures {
		s := SketchFeature{SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{source}}
		original, _ := json.Marshal(s)
		op := SketchOperation{Type: "OFFSET_ENTITIES", OperationID: "offset-" + source.ID, EntityIDs: []string{source.ID}, Value: &distance}
		if err := applySketchOperations(&s, []SketchOperation{op}); err != nil {
			t.Fatal(err)
		}
		if err := validateSketch(s); err != nil {
			t.Fatal(err)
		}
		if len(s.Entities) != 2 {
			t.Fatal("copy missing")
		}
		e := s.Entities[1]
		if e.ID == source.ID || e.SourceEntityID != source.ID || e.CreatedByOperationID != op.OperationID {
			t.Fatal("unstable copy identity")
		}
		switch source.Kind {
		case "LINE":
			if e.Start.Y != 5 || e.End.Y != 5 {
				t.Fatal("line left normal")
			}
		case "CIRCLE":
			if e.Radius != 3 {
				t.Fatal("circle CCW inward")
			}
		case "ARC":
			if e.Radius != 7 || e.EndAngle != source.EndAngle {
				t.Fatal("negative arc sweep left is outward")
			}
		}
		again := SketchFeature{SchemaVersion: SketchSchemaVersion}
		if err := json.Unmarshal(original, &again); err != nil {
			t.Fatal(err)
		}
		if err := applySketchOperations(&again, []SketchOperation{op}); err != nil {
			t.Fatal(err)
		}
		x, _ := json.Marshal(s)
		y, _ := json.Marshal(again)
		if string(x) != string(y) {
			t.Fatal("deterministic operation failed")
		}
	}
}
func TestSketchOffsetRectangleMiterAndRoundClosedProfile(t *testing.T) {
	for _, mode := range []string{"MITER", "ROUND"} {
		t.Run(mode, func(t *testing.T) {
			s := cornerRectangleFixture()
			d := -1.0
			if err := applySketchOffsetEdit(&s, SketchOperation{Type: "OFFSET_ENTITIES", OperationID: "rectangle-" + mode, EntityIDs: []string{"bottom", "top", "right", "left"}, Value: &d, Mode: mode}); err != nil {
				t.Fatal(err)
			}
			if err := validateSketch(s); err != nil {
				t.Fatal(err)
			}
			count := 4
			if mode == "ROUND" {
				count = 8
			}
			if len(s.Entities) != 4+count {
				t.Fatalf("wrong output count %d", len(s.Entities))
			}
			for i := 0; i < 4; i++ {
				s.Entities[i].Role = "CONSTRUCTION"
			}
			regions, err := buildProfileRegions(Feature{Sketch: &s})
			if err != nil || len(regions) != 1 {
				t.Fatalf("offset topology not closed: %v", err)
			}
			minX, maxX, minY, maxY := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
			for _, e := range s.Entities[4:] {
				for _, sub := range []string{"START", "END"} {
					p := cornerEndpoint(e, sub)
					minX = math.Min(minX, p.X)
					maxX = math.Max(maxX, p.X)
					minY = math.Min(minY, p.Y)
					maxY = math.Max(maxY, p.Y)
				}
			}
			if math.Abs(minX+1) > 1e-8 || math.Abs(maxX-11) > 1e-8 || math.Abs(minY+1) > 1e-8 || math.Abs(maxY-9) > 1e-8 {
				t.Fatal("wrong offset bounds")
			}
		})
	}
}
func TestSketchOffsetMixedLineArcChainUsesFormalConnections(t *testing.T) {
	s := SketchFeature{SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{-3, 0}, End: &SketchPoint2{0, 0}}, {ID: "arc", Kind: "ARC", Role: "PROFILE", Center: &SketchPoint2{0, 2}, Radius: 2, StartAngle: -math.Pi / 2, EndAngle: 0}}, Constraints: []SketchConstraint{{ID: "join", Kind: "COINCIDENT", References: []SketchGeometryRef{cornerRef("line", "END"), cornerRef("arc", "START")}}}}
	d := 0.5
	if err := applySketchOffsetEdit(&s, SketchOperation{Type: "OFFSET_ENTITIES", OperationID: "mixed", EntityIDs: []string{"line", "arc"}, Value: &d, Mode: "ROUND"}); err != nil {
		t.Fatal(err)
	}
	if len(s.Entities) != 4 || len(s.Constraints) != 2 {
		t.Fatal("tangent chain should not create a rounded gap")
	}
	if cornerDistance(cornerEndpoint(s.Entities[2], "END"), cornerEndpoint(s.Entities[3], "START")) > 1e-8 {
		t.Fatal("line/arc offset tangent connection broken")
	}
	if s.Entities[3].Radius != 1.5 {
		t.Fatal("wrong analytic offset radius")
	}
}
func TestSketchOffsetRejectsNonpositiveRadiusSelfIntersectionAndUnsupportedAtomically(t *testing.T) {
	s := SketchFeature{SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 2}}}
	d := 2.0
	before, _ := json.Marshal(s)
	if err := applySketchOffsetEdit(&s, SketchOperation{OperationID: "zero-radius", EntityIDs: []string{"circle"}, Value: &d}); err == nil {
		t.Fatal("zero radius accepted")
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("failure mutated source")
	}
	s = cornerRectangleFixture()
	d = 5.0
	before, _ = json.Marshal(s)
	if err := applySketchOffsetEdit(&s, SketchOperation{OperationID: "collapsed", EntityIDs: []string{"bottom", "right", "top", "left"}, Value: &d}); err == nil {
		t.Fatal("collapsed inward chain accepted")
	}
	after, _ = json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("failed chain partially committed")
	}
	s.Entities = []SketchEntity{{ID: "ellipse", Kind: "ELLIPSE", Role: "PROFILE", Center: &SketchPoint2{}, MajorRadius: 5, MinorRadius: 2}}
	d = 1
	if err := applySketchOffsetEdit(&s, SketchOperation{OperationID: "unsupported", EntityIDs: []string{"ellipse"}, Value: &d}); err == nil {
		t.Fatal("ellipse silently approximated")
	}
}
