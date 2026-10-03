package workspace

import (
	"encoding/json"
	"math"
	"testing"
)

func rationalQuarterSpline() SketchEntity {
	return SketchEntity{ID: "spline", Kind: "SPLINE", Role: "PROFILE", Mode: "CONTROL", Degree: 2, Poles: []SketchPoint2{{X: 1}, {X: 1, Y: 1}, {Y: 1}}, PoleIDs: []string{"a", "b", "c"}, Knots: []float64{0, 1}, Multiplicities: []uint32{3, 3}, Weights: []float64{1, math.Sqrt(.5), 1}, ParameterStart: 0, ParameterEnd: 1}
}
func TestSketchControlSplineExactInsertionRemoval(t *testing.T) {
	original := rationalQuarterSpline()
	sketch := SketchFeature{Entities: []SketchEntity{original}}
	u := .37
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "EDIT_SPLINE_POINT", OperationID: "insert", EntityID: original.ID, PointAction: "INSERT", KnotParameter: &u}}); err != nil {
		t.Fatal(err)
	}
	inserted := sketch.Entities[0]
	if len(inserted.Poles) != 4 || inserted.PoleIDs[0] != "a" || inserted.PoleIDs[3] != "c" {
		t.Fatal("unaffected identities lost")
	}
	for i := 0; i <= 100; i++ {
		parameter := float64(i) / 100
		a, _ := evaluateCanonicalSpline(original, parameter)
		b, err := evaluateCanonicalSpline(inserted, parameter)
		if err != nil || math.Hypot(a.X-b.X, a.Y-b.Y) > 1e-12 || math.Abs(b.X*b.X+b.Y*b.Y-1) > 1e-12 {
			t.Fatal("exact rational curve changed")
		}
	}
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "EDIT_SPLINE_POINT", OperationID: "remove", EntityID: original.ID, PointAction: "DELETE", KnotParameter: &u}}); err != nil {
		t.Fatal(err)
	}
	removed := sketch.Entities[0]
	for i, p := range removed.Poles {
		if math.Hypot(p.X-original.Poles[i].X, p.Y-original.Poles[i].Y) > 1e-12 || math.Abs(removed.Weights[i]-original.Weights[i]) > 1e-12 {
			t.Fatal("inverse knot operation altered coefficients")
		}
	}
	inserted.Poles[1].Y += .2
	sketch.Entities[0] = inserted
	before, _ := json.Marshal(sketch)
	err := applySketchOperations(&sketch, []SketchOperation{{Type: "EDIT_SPLINE_POINT", OperationID: "illegal-remove", EntityID: original.ID, PointAction: "DELETE", KnotParameter: &u}})
	after, _ := json.Marshal(sketch)
	if err == nil || string(before) != string(after) {
		t.Fatal("nonremovable knot did not fail atomically")
	}
}
func TestSketchFitSplineStablePointEditsAndModeConversion(t *testing.T) {
	sketch := SketchFeature{Entities: []SketchEntity{{ID: "fit", Kind: "SPLINE", Role: "PROFILE", Mode: "FIT", Degree: 3, ControlPoints: []SketchPoint2{{}, {X: 3, Y: 4}, {X: 8, Y: 3}, {X: 10}}, ControlPointIDs: []string{"f0", "f1", "f2", "f3"}}}}
	i := 1
	p := SketchPoint2{X: 2, Y: 1}
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "EDIT_SPLINE_POINT", OperationID: "fit-insert", EntityID: "fit", PointAction: "INSERT", PointIndex: &i, Point: &p}}); err != nil {
		t.Fatal(err)
	}
	if sketch.Entities[0].ControlPointIDs[2] != "f1" || sketch.Entities[0].ControlPointIDs[4] != "f3" {
		t.Fatal("index shift changed identities")
	}
	id := sketch.Entities[0].ControlPointIDs[1]
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "EDIT_SPLINE_POINT", OperationID: "fit-delete", EntityID: "fit", PointAction: "DELETE", ControlPointID: id}}); err != nil {
		t.Fatal(err)
	}
	if len(sketch.Entities[0].ControlPoints) != 4 || sketch.Entities[0].ControlPointIDs[1] != "f1" {
		t.Fatal("delete identity mapping")
	}
	e := rationalQuarterSpline()
	e.Mode = "FIT"
	e.ControlPoints = []SketchPoint2{{X: 1}, {X: math.Sqrt(.5), Y: math.Sqrt(.5)}, {Y: 1}}
	e.ControlPointIDs = []string{"fit-a", "fit-b", "fit-c"}
	sketch.Entities = []SketchEntity{e}
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "CONVERT_SPLINE_TO_CONTROL", OperationID: "convert", EntityID: e.ID}}); err != nil {
		t.Fatal(err)
	}
	if sketch.Entities[0].Mode != "CONTROL" || len(sketch.Entities[0].ControlPoints) != 3 || len(sketch.Entities[0].Poles) != 3 {
		t.Fatal("mode conversion discarded fit provenance or exact poles")
	}
}
func TestSketchSplineDeletionRequiresExplicitReferenceRelease(t *testing.T) {
	e := rationalQuarterSpline()
	sketch := SketchFeature{Entities: []SketchEntity{e}, Constraints: []SketchConstraint{{ID: "fixed-pole", Kind: "FIXED_POINT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: e.ID, SubElement: "CONTROL", ControlPointID: "b"}}, FixedPoint: &SketchPoint2{X: 1, Y: 1}}}}
	u := .5
	op := SketchOperation{Type: "EDIT_SPLINE_POINT", OperationID: "ins", EntityID: e.ID, PointAction: "INSERT", KnotParameter: &u}
	before, _ := json.Marshal(sketch)
	if err := applySketchOperations(&sketch, []SketchOperation{op}); err == nil {
		t.Fatal("silently rewired blended pole")
	}
	after, _ := json.Marshal(sketch)
	if string(before) != string(after) {
		t.Fatal("failed operation mutated original")
	}
	op.DetachConstraintIDs = []string{"fixed-pole"}
	if err := applySketchOperations(&sketch, []SketchOperation{op}); err != nil {
		t.Fatal(err)
	}
	if len(sketch.Constraints) != 0 {
		t.Fatal("explicit release not applied")
	}
}

func TestSketchSplineOpenClosedLifecycleAndEndpointImpact(t *testing.T) {
	e := rationalQuarterSpline()
	sketch := SketchFeature{Entities: []SketchEntity{e}}
	closed := true
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "SET_SPLINE_CLOSED", OperationID: "close", EntityID: e.ID, Closed: &closed}}); err != nil {
		t.Fatal(err)
	}
	result := sketch.Entities[0]
	if !result.Closed || result.Poles[0] != result.Poles[len(result.Poles)-1] || result.Weights[1] != e.Weights[1] {
		t.Fatal("closure failed or discarded rational weight")
	}
	if err := validateCanonicalSpline(result); err != nil {
		t.Fatal(err)
	}
	closed = false
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "SET_SPLINE_CLOSED", OperationID: "open", EntityID: e.ID, Closed: &closed}}); err != nil {
		t.Fatal(err)
	}
	if sketch.Entities[0].Closed || len(sketch.Entities[0].Poles) != len(e.Poles) {
		t.Fatal("open did not remove explicit closing pole")
	}
	if err := normalizeSketchPointIdentities(&sketch); err != nil {
		t.Fatal(err)
	}
	sketch.Constraints = []SketchConstraint{{ID: "end-link", Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: e.ID, SubElement: "START"}, {Target: "SKETCH_ORIGIN", SubElement: "POINT"}}}}
	closed = true
	before, _ := json.Marshal(sketch)
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "SET_SPLINE_CLOSED", OperationID: "close-again", EntityID: e.ID, Closed: &closed}}); err == nil {
		t.Fatal("closure silently retained old endpoint constraint")
	}
	after, _ := json.Marshal(sketch)
	if string(before) != string(after) {
		t.Fatal("invalid closure not atomic")
	}
}

func TestSketchSplineTangentRequiresExplicitControlEndpoint(t *testing.T) {
	f := dimensionalSketchFeature("tangent-mode")
	e := rationalQuarterSpline()
	e.Mode = "FIT"
	e.ControlPoints = []SketchPoint2{{X: 1}, {X: .7, Y: .7}, {Y: 1}}
	e.ControlPointIDs = []string{"f0", "f1", "f2"}
	f.Sketch.Entities = []SketchEntity{e, {ID: "line", Kind: "LINE", Role: "CONSTRUCTION", Start: &SketchPoint2{X: 1}, End: &SketchPoint2{X: 1, Y: 3}}}
	f.Sketch.Constraints = []SketchConstraint{{ID: "tangent", Kind: "TANGENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: e.ID, SubElement: "START"}, {Target: "ENTITY", EntityID: "line", SubElement: "WHOLE"}}}}
	if err := normalizeSketchPointIdentities(f.Sketch); err != nil {
		t.Fatal(err)
	}
	if err := validateSketch(*f.Sketch); err == nil {
		t.Fatal("FIT point chord accepted as derivative")
	}
	f.Sketch.Entities[0].Mode = "CONTROL"
	if err := validateSketch(*f.Sketch); err != nil {
		t.Fatal(err)
	}
	f.Sketch.Constraints[0].References[0].SubElement = "WHOLE"
	if err := validateSketch(*f.Sketch); err == nil {
		t.Fatal("whole spline tangent lacks endpoint meaning")
	}
}
