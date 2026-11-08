package workspace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeometryReferencesKeepDependenciesAndScalarParameterOwnership(t *testing.T) {
	m := newPartModel()
	m.GeometricalSets = []GeometricalSet{{ID: "geometry-set", Name: "Geometry"}}
	m.DatumPoints = []DatumPoint{{ID: "a", GeometricalSetID: "geometry-set", Construction: &ReferenceConstruction{Mode: "POINT_COORDINATES"}}, {ID: "b", GeometricalSetID: "geometry-set", Position: [3]float64{0, 0, 10}, Construction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{0, 0, 10}}}}
	line := testGeometryLine()
	line.GeometricalSetID = "geometry-set"
	for i, id := range []string{"a", "b"} {
		line.Geometry.Curve.Points[i].Reference = &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "POINT", EntityID: id}}
	}
	direction := GeometricInput{Kind: "RESULT", FeatureID: line.ID, ResultID: "result"}
	sheet := Feature{ID: "sheet", Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{{Kind: "PLANE", PlaneID: "datum-xy"}}, Direction: &direction, Parameters: GeometryOperationParameters{Distance: 20}}}
	sheet.GeometricalSetID = "geometry-set"
	m.Features = []Feature{line, sheet}
	normalizePartModel(&m)
	if err := validatePartStructure(m); err != nil {
		t.Fatal(err)
	}
	if len(geometryParameters(line.Geometry)) != 0 {
		t.Fatal("picked point gained independent coordinate parameters")
	}
	parameters := geometryParameters(sheet.Geometry)
	if len(parameters) != 1 || parameters[0].slot != "geometry:distance" {
		t.Fatal("reference direction gained independent vector parameters", parameters)
	}
	graph, _, err := buildPartEvaluation(m, "r", "h", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][2]string{{"datum:a", "feature:line"}, {"datum:b", "feature:line"}, {"feature:line", "feature:sheet"}} {
		found := false
		for _, edge := range graph.Edges {
			if string(edge.Source) == want[0] && string(edge.Target) == want[1] {
				found = true
			}
		}
		if !found {
			t.Fatal("reference dependency missing", want)
		}
	}
}

func TestGeometryReferenceFramesAndInvalidRoles(t *testing.T) {
	m := newPartModel()
	m.DatumAxes = []DatumAxis{{ID: "axis", Origin: [3]float64{1, 2, 3}, Direction: [3]float64{0, 2, 0}}}
	ref := GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "AXIS", EntityID: "axis"}, Reversed: true}
	f := Feature{ID: "move", Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "TRANSLATE", Inputs: []GeometricInput{{Kind: "PLANE", PlaneID: "datum-xy"}}, Direction: &ref, Parameters: GeometryOperationParameters{Distance: 5}}}
	spec, err := (&Service{}).geometryFeatureSpec(t.Context(), "unit", m, f)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Parameters.Direction.Y != -1 || spec.Parameters.Distance != 5 || len(spec.References) != 1 {
		t.Fatal("reference frame was not compiled", spec)
	}
	a, _ := json.Marshal(spec.References)
	m.DatumAxes[0].Direction = [3]float64{1, 0, 0}
	next, err := (&Service{}).geometryFeatureSpec(t.Context(), "unit", m, f)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(next.References)
	if string(a) == string(b) || next.Parameters.Direction.X != -1 {
		t.Fatal("upstream frame change lost its frozen dependency")
	}
	f.Geometry.Kind = "EXTRACT"
	if err := validateGeometricFeature(f, nil, m); err == nil || !strings.Contains(err.Error(), "REFERENCE_ROLE") {
		t.Fatal("accepted a reference in the wrong operation", err)
	}
	bad := ref
	bad.FeatureID = "another-owner"
	if err := validateGeometryReference(bad, nil, m); err == nil {
		t.Fatal("accepted two owners")
	}
	point := GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "POINT", EntityID: "missing"}}
	if err := validateGeometryReference(point, nil, m); err == nil {
		t.Fatal("accepted missing point")
	}
}
