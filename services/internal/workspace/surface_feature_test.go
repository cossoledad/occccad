package workspace

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestSurfaceKeepPointDependencyAndToleranceParameter(t *testing.T) {
	m := newPartModel()
	m.GeometricalSets = []GeometricalSet{{ID: "set", Name: "Geometry"}}
	m.DatumPoints = []DatumPoint{{ID: "keep", Name: "Keep", GeometricalSetID: "set", Construction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{2, 0, 3}}}}
	source := testGeometryLine()
	source.GeometricalSetID = "set"
	source.Order = 1
	m.Features = []Feature{source}
	keep := &GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "POINT", EntityID: "keep"}}
	trim := Feature{ID: "trim", Type: "GEOMETRY", GeometricalSetID: "set", Geometry: &GeometricFeatureDefinition{Kind: "SURFACE_TRIM", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: source.ID, ResultID: "result"}, {Kind: "PLANE", PlaneID: "datum-yz"}}, KeepPoint: keep, Surface: &GeometrySurfaceDefinition{Tolerance: 1e-6}}}
	raw, _ := json.Marshal(m)
	payload, _ := json.Marshal(createFeaturePayload{Feature: trim})
	created, _, err := applyCreateFeature(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(created, &m); err != nil {
		t.Fatal(err)
	}
	found := false
	for i := range m.Parameters {
		p := &m.Parameters[i]
		if p.OwnerFeatureID == "trim" && p.PropertySlot == "geometry:tolerance" {
			found = true
			q, _ := modelcore.NewQuantity(.002, "mm")
			p.Source = modelcore.ValueSource{Literal: &q}
		}
	}
	if !found {
		t.Fatal("construction tolerance missing from parameter model")
	}
	if err := validateAndResolvePartParameters(&m); err != nil {
		t.Fatal(err)
	}
	if m.Features[1].Geometry.Surface.Tolerance != .002 {
		t.Fatal("tolerance parameter did not reach surface definition")
	}
	graph, _, err := buildPartEvaluation(m, "revision", "digest", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	pointDependency := false
	for _, edge := range graph.Edges {
		if strings.Contains(string(edge.Source), "keep") && edge.Target == "feature:trim" {
			pointDependency = true
		}
	}
	if !pointDependency {
		t.Fatal("retained design point omitted from dependency graph")
	}
	earlier := map[string]Feature{source.ID: source}
	for _, tolerance := range []float64{0, 1e-8, .1} {
		bad := m.Features[1]
		definition := *bad.Geometry
		surface := *definition.Surface
		surface.Tolerance = tolerance
		definition.Surface = &surface
		bad.Geometry = &definition
		if err := validateGeometricFeature(bad, earlier, m); err == nil {
			t.Fatalf("accepted invalid tolerance %g", tolerance)
		}
	}
	bad := m.Features[1]
	definition := *bad.Geometry
	definition.KeepPoint = nil
	bad.Geometry = &definition
	if err := validateGeometricFeature(bad, earlier, m); err == nil {
		t.Fatal("trim accepted implicit retained region")
	}
}
