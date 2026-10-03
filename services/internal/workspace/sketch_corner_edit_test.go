package workspace

import (
	"encoding/json"
	"math"
	"testing"
)

func cornerRectangleFixture() SketchFeature {
	s := SketchFeature{SchemaVersion: SketchSchemaVersion}
	points := []SketchPoint2{{0, 0}, {10, 0}, {10, 8}, {0, 8}}
	ids := []string{"bottom", "right", "top", "left"}
	for i, id := range ids {
		a, b := points[i], points[(i+1)%4]
		s.Entities = append(s.Entities, SketchEntity{ID: id, Kind: "LINE", Role: "PROFILE", Start: &a, End: &b})
		s.Constraints = append(s.Constraints, SketchConstraint{ID: "join-" + id, Kind: "COINCIDENT", References: []SketchGeometryRef{cornerRef(id, "END"), cornerRef(ids[(i+1)%4], "START")}})
	}
	width := 10.0
	s.Constraints = append(s.Constraints, SketchConstraint{ID: "width", Kind: "LENGTH", References: []SketchGeometryRef{cornerRef("bottom", "WHOLE")}, Value: &width, Unit: "mm", ParameterID: "width-parameter"})
	return s
}
func cornerOperation(kind, id, a, b, as, bs string) SketchOperation {
	r := 1.0
	first, second := cornerRef(a, as), cornerRef(b, bs)
	return SketchOperation{Type: kind, OperationID: id, EntityIDs: []string{a, b}, FirstReference: &first, SecondReference: &second, Value: &r, ChamferFirst: 1}
}
func TestSketchCornerFilletRectangleBatchPreservesDriversAndClosedProfile(t *testing.T) {
	s := cornerRectangleFixture()
	ops := []SketchOperation{cornerOperation("FILLET_ENTITIES", "ll", "bottom", "left", "START", "END"), cornerOperation("FILLET_ENTITIES", "lr", "bottom", "right", "END", "START"), cornerOperation("FILLET_ENTITIES", "ur", "right", "top", "END", "START"), cornerOperation("FILLET_ENTITIES", "ul", "top", "left", "END", "START")}
	if e := applySketchOperations(&s, ops); e != nil {
		t.Fatal(e)
	}
	if e := validateSketch(s); e != nil {
		t.Fatal(e)
	}
	for _, c := range s.Constraints {
		if c.ID == "width" && (c.ParameterID != "width-parameter" || c.References[0].EntityID != "bottom" || *c.Value != 10) {
			t.Fatal("source dimension rewritten")
		}
	}
	regions, err := buildProfileRegions(Feature{ID: "sketch", Sketch: &s})
	if err != nil || len(regions) != 1 {
		t.Fatalf("closed profile lost: %v", err)
	}
	area := 0.0
	for _, c := range regions[0].Outer.Curves {
		term := 0.0
		switch c.Kind {
		case "LINE":
			term = (c.Start[0]*c.End[1] - c.Start[1]*c.End[0]) / 2
		case "ARC":
			term = (c.Radius*c.Center[0]*(math.Sin(c.EndAngle)-math.Sin(c.StartAngle)) + c.Radius*c.Center[1]*(math.Cos(c.StartAngle)-math.Cos(c.EndAngle)) + c.Radius*c.Radius*(c.EndAngle-c.StartAngle)) / 2
		}
		if c.Reversed {
			term = -term
		}
		area += term
	}
	if math.Abs(math.Abs(area)-(80-4+math.Pi)) > 1e-8 {
		t.Fatalf("wrong exact area %g", area)
	}
}
func TestSketchCornerChamferModesUseStableVirtualIntersectionAndDimensions(t *testing.T) {
	for _, mode := range []string{"EQUAL", "TWO_LENGTHS", "LENGTH_ANGLE"} {
		t.Run(mode, func(t *testing.T) {
			s := cornerRectangleFixture()
			op := cornerOperation("CHAMFER_ENTITIES", "chamfer", "bottom", "left", "START", "END")
			op.ChamferMode = mode
			op.ChamferSecond = 2
			op.ChamferAngle = 30
			if e := applySketchOperations(&s, []SketchOperation{op}); e != nil {
				t.Fatal(e)
			}
			if e := validateSketch(s); e != nil {
				t.Fatal(e)
			}
			var bridge SketchEntity
			virtualRelations, angles := 0, 0
			for _, e := range s.Entities {
				if e.ID == macroID(op.OperationID, "corner/bridge") {
					bridge = e
				}
			}
			first, second := *bridge.Start, *bridge.End
			if first.X == 0 {
				first, second = second, first
			}
			expected := 1.0
			if mode == "TWO_LENGTHS" {
				expected = 2
			}
			if mode == "LENGTH_ANGLE" {
				expected = math.Tan(math.Pi / 6)
			}
			if math.Abs(first.X-1) > 1e-10 || math.Abs(first.Y) > 1e-10 || math.Abs(second.X) > 1e-10 || math.Abs(second.Y-expected) > 1e-10 {
				t.Fatalf("wrong chamfer points %#v %#v", first, second)
			}
			for _, c := range s.Constraints {
				if c.Kind == "POINT_ON_OBJECT" && c.References[0].EntityID == macroID(op.OperationID, "corner/virtual") {
					virtualRelations++
				}
				if c.Kind == "ANGLE" {
					angles++
					if c.ParameterID == "" || *c.Value != 30 {
						t.Fatal("angle parameter lost")
					}
				}
			}
			if virtualRelations != 2 {
				t.Fatal("virtual intersection is free")
			}
			if mode == "LENGTH_ANGLE" && angles != 1 {
				t.Fatal("missing angle equation")
			}
			if r, e := buildProfileRegions(Feature{ID: "sketch", Sketch: &s}); e != nil || len(r) != 1 {
				t.Fatalf("profile lost: %v", e)
			}
		})
	}
}
func TestSketchCornerFilletLineArcAndArcArcAreAnalyticallyTangent(t *testing.T) {
	fixtures := []SketchFeature{{SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{{ID: "first", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{5, 0}, End: &SketchPoint2{10, 0}}, {ID: "second", Kind: "ARC", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 5, EndAngle: math.Pi / 2}}}, {SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{{ID: "first", Kind: "ARC", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 5, EndAngle: math.Pi / 2}, {ID: "second", Kind: "ARC", Role: "PROFILE", Center: &SketchPoint2{10, 0}, Radius: 5, StartAngle: math.Pi / 2, EndAngle: math.Pi}}}}
	for i, s := range fixtures {
		secondSub := "START"
		if i == 1 {
			secondSub = "END"
		}
		op := cornerOperation("FILLET_ENTITIES", "fillet", "first", "second", "START", secondSub)
		if e := applySketchOperations(&s, []SketchOperation{op}); e != nil {
			t.Fatal(e)
		}
		if e := validateSketch(s); e != nil {
			t.Fatal(e)
		}
		var bridge SketchEntity
		for _, e := range s.Entities {
			if e.ID == macroID(op.OperationID, "corner/bridge") {
				bridge = e
			}
		}
		for _, source := range s.Entities[:2] {
			if source.Kind == "LINE" {
				d := sub2(*source.End, *source.Start)
				distance := math.Abs(cornerCross(sub2(*bridge.Center, *source.Start), d)) / math.Hypot(d.X, d.Y)
				if math.Abs(distance-bridge.Radius) > 1e-10 {
					t.Fatal("line not tangent")
				}
			} else {
				d := cornerDistance(*source.Center, *bridge.Center)
				if math.Abs(d-source.Radius-bridge.Radius) > 1e-10 && math.Abs(d-math.Abs(source.Radius-bridge.Radius)) > 1e-10 {
					t.Fatal("arc not tangent")
				}
			}
		}
	}
}
func TestSketchCornerFailureIsAtomicAndLateRangeGateRejectsOversize(t *testing.T) {
	s := cornerRectangleFixture()
	before, _ := json.Marshal(s)
	first := cornerOperation("FILLET_ENTITIES", "good", "bottom", "left", "START", "END")
	bad := cornerOperation("FILLET_ENTITIES", "bad", "bottom", "right", "END", "START")
	r := 100.0
	bad.Value = &r
	if e := applySketchOperations(&s, []SketchOperation{first, bad}); e == nil {
		t.Fatal("oversize accepted")
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("partial batch mutation")
	}
	if e := applySketchOperations(&s, []SketchOperation{first}); e != nil {
		t.Fatal(e)
	}
	for i := range s.Entities {
		e := &s.Entities[i]
		if e.SourceEntityID == "bottom" && e.Kind == "LINE" && e.Role == "PROFILE" {
			e.Start = &SketchPoint2{20, 0}
			break
		}
	}
	if e := validateSketchCornerGeometry(&s); e == nil {
		t.Fatal("late solver oversize accepted")
	}
}
func TestSketchCornerKeepLeavesOriginalProfile(t *testing.T) {
	s := cornerRectangleFixture()
	op := cornerOperation("FILLET_ENTITIES", "keep", "bottom", "left", "START", "END")
	op.TrimMode = "KEEP"
	if e := applySketchOperations(&s, []SketchOperation{op}); e != nil {
		t.Fatal(e)
	}
	for _, e := range s.Entities {
		if e.CreatedByOperationID == op.OperationID && e.Role != "CONSTRUCTION" {
			t.Fatal("KEEP duplicated profile")
		}
	}
	regions, e := buildProfileRegions(Feature{ID: "sketch", Sketch: &s})
	if e != nil || len(regions) != 1 || len(regions[0].Outer.Curves) != 4 {
		t.Fatalf("original profile damaged: %v", e)
	}
}
