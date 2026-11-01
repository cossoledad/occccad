package workspace

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestReferenceConstructionParametersPropagationAndHistory(t *testing.T) {
	model := newPartModel()
	raw, _ := json.Marshal(model)
	point := DatumPoint{ID: "p1", Name: "Point.1", Construction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{1, 2, 3}}}
	payload, _ := json.Marshal(referencePointPayload{Action: "CREATE", Point: point})
	next, changes, err := applyReferencePoint(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	var candidate PartModel
	_ = json.Unmarshal(next, &candidate)
	if len(candidate.GeometricalSets) != 1 || candidate.DatumPoints[0].GeometricalSetID != candidate.GeometricalSets[0].ID || len(candidate.Parameters) != 3 {
		t.Fatal("point ownership and parameter lifecycle", candidate)
	}
	service := &Service{}
	if err := service.resolveDatumDefinitions(context.Background(), "doc", "request", &candidate, 0); err != nil {
		t.Fatal(err)
	}
	if candidate.DatumPoints[0].Position != point.Construction.Coordinates {
		t.Fatal("coordinate point", candidate.DatumPoints)
	}
	second := DatumPoint{ID: "p2", Name: "Point.2", GeometricalSetID: candidate.GeometricalSets[0].ID, Construction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{5, 2, 3}}}
	candidate.DatumPoints = append(candidate.DatumPoints, second)
	candidate.DatumAxes = append(candidate.DatumAxes, DatumAxis{ID: "line", Name: "Line", GeometricalSetID: second.GeometricalSetID, Construction: &ReferenceConstruction{Mode: "LINE_TWO_POINTS", Inputs: []DatumReference{{Kind: "POINT", EntityID: "p1"}, {Kind: "POINT", EntityID: "p2"}}}})
	normalizePartModel(&candidate)
	if err := validateAndResolvePartParameters(&candidate); err != nil {
		t.Fatal(err)
	}
	if err := service.resolveDatumDefinitions(context.Background(), "doc", "request", &candidate, 0); err != nil {
		t.Fatal(err)
	}
	if candidate.DatumAxes[0].Direction != [3]float64{1, 0, 0} {
		t.Fatal("point dependency", candidate.DatumAxes)
	}
	graph, _, err := buildPartEvaluation(candidate, "r", "hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	closure := graph.DirtyClosure([]modelcore.DependencyKey{"parameter:parameter:p2:reference:y"})
	found := false
	for _, key := range closure {
		if key == "datum:line" {
			found = true
		}
	}
	if !found {
		t.Fatal("parameter -> point -> line dependency missing", closure)
	}
	values, err := modelValues("PART", next, changes)
	if err != nil {
		t.Fatal(err)
	}
	undo, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PART", next, undo)
	if err != nil {
		t.Fatal(err)
	}
	var original PartModel
	_ = json.Unmarshal(restored, &original)
	if len(original.DatumPoints) != 0 || len(original.GeometricalSets) != 0 || len(original.Parameters) != 0 {
		t.Fatal("point creation compensation leaked owned objects", original)
	}
}

func TestReferenceConstructionPlaneModesAndDegeneracy(t *testing.T) {
	frames := map[string]datumFrame{"a": {origin: [3]float64{0, 0, 0}, point: true}, "b": {origin: [3]float64{1, 0, 0}, point: true}, "c": {origin: [3]float64{0, 1, 0}, point: true}, "base": {origin: [3]float64{0, 0, 2}, direction: [3]float64{0, 0, 1}, u: [3]float64{1, 0, 0}, plane: true}}
	resolve := func(ref DatumReference) (datumFrame, error) { return frames[ref.EntityID], nil }
	plane, err := constructReference(&ReferenceConstruction{Mode: "PLANE_THREE_POINTS", Inputs: []DatumReference{{EntityID: "a"}, {EntityID: "b"}, {EntityID: "c"}}}, resolve)
	if err != nil || plane.direction != [3]float64{0, 0, 1} {
		t.Fatal(plane, err)
	}
	offset, err := constructReference(&ReferenceConstruction{Mode: "PLANE_OFFSET", Offset: 10, Inputs: []DatumReference{{EntityID: "base"}}}, resolve)
	if err != nil || offset.origin[2] != 12 {
		t.Fatal(offset, err)
	}
	equation, err := constructReference(&ReferenceConstruction{Mode: "PLANE_EQUATION", Coefficients: [4]float64{0, 0, 2, 10}}, resolve)
	if err != nil || math.Abs(equation.origin[2]-5) > 1e-12 {
		t.Fatal(equation, err)
	}
	if _, err := constructReference(&ReferenceConstruction{Mode: "LINE_TWO_POINTS", Inputs: []DatumReference{{EntityID: "a"}, {EntityID: "a"}}}, resolve); err == nil {
		t.Fatal("degenerate line accepted")
	}
	if _, err := constructReference(&ReferenceConstruction{Mode: "PLANE_THREE_POINTS", Inputs: []DatumReference{{EntityID: "a"}, {EntityID: "b"}, {EntityID: "b"}}}, resolve); err == nil {
		t.Fatal("collinear plane accepted")
	}
}
