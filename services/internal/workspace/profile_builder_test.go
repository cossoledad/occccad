package workspace

import (
	"math"
	"strings"
	"testing"
)

func profileSketch(entities []SketchEntity, constraints []SketchConstraint) Feature {
	return Feature{ID: "sketch", Type: "SKETCH", Sketch: &SketchFeature{SchemaVersion: SketchSchemaVersion,
		Support:  SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "xy", Plane: "XY"},
		Entities: entities, Constraints: constraints}}
}

func TestAvailableProfileLoopsPreserveClosedComponents(t *testing.T) {
	f := profileSketch([]SketchEntity{{ID: "outer", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 10}}, nil)
	before, err := buildProfileLoops(f, true)
	if err != nil {
		t.Fatal(err)
	}
	f.Sketch.Entities = append(f.Sketch.Entities, SketchEntity{ID: "unfinished", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 20}, End: &SketchPoint2{X: 30}})
	if _, err = buildProfileLoops(f, true); err == nil {
		t.Fatal("whole sketch ignored open geometry")
	}
	available, err := buildAvailableProfileLoops(f, true)
	if err != nil || len(available) != 1 || available[0].value.ID != before[0].value.ID {
		t.Fatal("independent open component changed closed identity", err)
	}
	// A branched rectangle is wholly unavailable, rather than being silently
	// cut into a different region. The independent circle remains available.
	ops, err := rectangleMacro("branched", SketchPoint2{X: 40}, SketchPoint2{X: 50, Y: 10})
	if err != nil {
		t.Fatal(err)
	}
	var corner SketchGeometryRef
	for _, op := range ops {
		if op.Entity != nil {
			f.Sketch.Entities = append(f.Sketch.Entities, *op.Entity)
			if corner.EntityID == "" {
				corner = SketchGeometryRef{Target: "ENTITY", EntityID: op.Entity.ID, SubElement: "START"}
			}
		}
		if op.Constraint != nil {
			f.Sketch.Constraints = append(f.Sketch.Constraints, *op.Constraint)
		}
	}
	f.Sketch.Entities = append(f.Sketch.Entities, SketchEntity{ID: "branch", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 40}, End: &SketchPoint2{X: 35}})
	f.Sketch.Constraints = append(f.Sketch.Constraints, SketchConstraint{ID: "branch-join", Kind: "COINCIDENT", References: []SketchGeometryRef{corner, {Target: "ENTITY", EntityID: "branch", SubElement: "START"}}})
	available, err = buildAvailableProfileLoops(f, true)
	if err != nil || len(available) != 1 || available[0].value.ID != before[0].value.ID {
		t.Fatal("branched component leaked into available loops", err)
	}
	f.Sketch.Solve.Status = "CONFLICTING"
	if _, err = buildAvailableProfileLoops(f, true); err == nil {
		t.Fatal("partial profile bypassed sketch solve failure")
	}
}

func TestBuildProfileRegionsClassifiesCircleHole(t *testing.T) {
	feature := profileSketch([]SketchEntity{
		{ID: "outer", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 20},
		{ID: "hole", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 8},
	}, nil)
	regions, err := buildProfileRegions(feature)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 || len(regions[0].Holes) != 1 {
		t.Fatalf("expected one annular region, got %#v", regions)
	}
	if regions[0].Outer.Curves[0].EntityID != "outer" || regions[0].Holes[0].Curves[0].EntityID != "hole" {
		t.Fatalf("wrong containment classification: %#v", regions[0])
	}
}

func TestBuildProfileRegionsConsumesRectangleMacroRelations(t *testing.T) {
	operations, err := rectangleMacro("profile-test", SketchPoint2{X: -3, Y: 2}, SketchPoint2{X: 12, Y: 9})
	if err != nil {
		t.Fatal(err)
	}
	entities := []SketchEntity{}
	constraints := []SketchConstraint{}
	for _, operation := range operations {
		if operation.Entity != nil {
			entities = append(entities, *operation.Entity)
		}
		if operation.Constraint != nil {
			constraints = append(constraints, *operation.Constraint)
		}
	}
	regions, err := buildProfileRegions(profileSketch(entities, constraints))
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 || len(regions[0].Outer.Curves) != 4 {
		t.Fatalf("expected one rectangle region, got %#v", regions)
	}
}

func TestBuildProfileRegionsAcceptsArcAndLineLoop(t *testing.T) {
	feature := profileSketch([]SketchEntity{
		{ID: "arc", Kind: "ARC", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 10, StartAngle: 0, EndAngle: math.Pi},
		{ID: "diameter", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: -10}, End: &SketchPoint2{X: 10}},
	}, []SketchConstraint{
		{ID: "join-a", Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "arc", SubElement: "END"}, {Target: "ENTITY", EntityID: "diameter", SubElement: "START"}}},
		{ID: "join-b", Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "diameter", SubElement: "END"}, {Target: "ENTITY", EntityID: "arc", SubElement: "START"}}},
	})
	regions, err := buildProfileRegions(feature)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 || len(regions[0].Outer.Curves) != 2 {
		t.Fatalf("expected one two-curve region, got %#v", regions)
	}
}

func TestBuildProfileRegionsAcceptsClosedSpline(t *testing.T) {
	feature := profileSketch([]SketchEntity{{ID: "curve", Kind: "SPLINE", Role: "PROFILE", ControlPoints: []SketchPoint2{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 20, Y: 20}, {X: 0, Y: 20}}, Degree: 3, Closed: true}}, nil)
	regions, err := buildProfileRegions(feature)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 || regions[0].Outer.Curves[0].Kind != "SPLINE" {
		t.Fatalf("expected one spline region, got %#v", regions)
	}
}

func TestBuildProfileRegionsRejectsOpenAndBranchedProfiles(t *testing.T) {
	feature := profileSketch([]SketchEntity{
		{ID: "a", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 10}},
		{ID: "b", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 10}, End: &SketchPoint2{X: 10, Y: 10}},
	}, []SketchConstraint{{ID: "join", Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "a", SubElement: "END"}, {Target: "ENTITY", EntityID: "b", SubElement: "START"}}}})
	_, err := buildProfileRegions(feature)
	if err == nil || !strings.Contains(err.Error(), "open or has a T-junction") {
		t.Fatalf("expected open-profile diagnostic, got %v", err)
	}
}
