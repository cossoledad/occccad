package workspace

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/modelcore"
	"strings"
	"testing"
)

func TestSurfaceControlStableReferencesParametersGraphAndUndo(t *testing.T) {
	m := newPartModel()
	m.GeometricalSets = []GeometricalSet{{ID: "set", Name: "Geometry"}}
	source := testGeometryLine()
	source.GeometricalSetID = "set"
	source.Order = 1
	m.Features = []Feature{source}
	m.DatumPoints = []DatumPoint{{ID: "support-point", Name: "Station", GeometricalSetID: "set", Construction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{0, 0, 0}}}}
	input := GeometricInput{Kind: "RESULT", FeatureID: source.ID, ResultID: "result"}
	plane := GeometricInput{Kind: "PLANE", PlaneID: "datum-xy"}
	id := uuid.NewString()
	blend := Feature{ID: "blend", Type: "GEOMETRY", GeometricalSetID: "set", Geometry: &GeometricFeatureDefinition{Kind: "SURFACE_BLEND", Inputs: []GeometricInput{input, {Kind: "PLANE", PlaneID: "datum-yz"}}, Surface: &GeometrySurfaceDefinition{Tolerance: 1e-6, Boundaries: []GeometryBoundaryControl{{ID: id, Boundary: input, Support: &plane, Continuity: "G2", Tension: 1}}}}}
	// Geometric kind is checked by the authoritative evaluator; definition/history
	// checks here concern stable control identity, parameter slots and references.
	raw, _ := json.Marshal(m)
	payload, _ := json.Marshal(createFeaturePayload{Feature: blend})
	created, changes, err := applyCreateFeature(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(created, &m); err != nil {
		t.Fatal(err)
	}
	found := false
	for i := range m.Parameters {
		p := &m.Parameters[i]
		if p.OwnerFeatureID == blend.ID && p.PropertySlot == "geometry:tension:"+id {
			q, _ := modelcore.NewQuantity(2, "1")
			p.Source = modelcore.ValueSource{Literal: &q}
			found = true
		}
	}
	if !found {
		t.Fatal("support tension omitted from formal parameters")
	}
	if err = validateAndResolvePartParameters(&m); err != nil {
		t.Fatal(err)
	}
	if m.Features[1].Geometry.Surface.Boundaries[0].Tension != 2 {
		t.Fatal("parameter did not reach control")
	}
	graph, _, err := buildPartEvaluation(m, "revision", "digest", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, e := range graph.Edges {
		if e.Target == "feature:blend" && strings.Contains(string(e.Source), "datum-xy") {
			found = true
		}
	}
	if !found {
		t.Fatal("support missing from dependency graph")
	}
	values, err := partHistoryValues(created, changes)
	if err != nil {
		t.Fatal(err)
	}
	undo, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	undone, err := applyModelValues("PART", created, undo)
	if err != nil {
		t.Fatal(err)
	}
	var restored PartModel
	json.Unmarshal(undone, &restored)
	if len(restored.Features) != 1 {
		t.Fatal("undo retained controlled patch")
	}
	for _, p := range restored.Parameters {
		if p.OwnerFeatureID == blend.ID {
			t.Fatal("undo retained tension")
		}
	}
	// Reordering inputs cannot move the constraint to a different boundary.
	d := m.Features[1].Geometry
	d.Inputs[0], d.Inputs[1] = d.Inputs[1], d.Inputs[0]
	if surfaceInputIndex(d.Inputs, d.Surface.Boundaries[0].Boundary) != 1 {
		t.Fatal("control followed a row index")
	}
	d.Surface.Boundaries[0].ID = "0"
	if validateSurfaceControls(d) == nil {
		t.Fatal("accepted invalid stable control ID")
	}
}
func TestSurfaceControlPointAndGuideContracts(t *testing.T) {
	a := GeometricInput{Kind: "RESULT", FeatureID: "a", ResultID: "result"}
	b := GeometricInput{Kind: "RESULT", FeatureID: "b", ResultID: "result"}
	p := GeometricInput{Kind: "DATUM", Datum: &DatumReference{Kind: "POINT", EntityID: "p"}}
	id := uuid.NewString()
	d := &GeometricFeatureDefinition{Kind: "SURFACE_LOFT", Inputs: []GeometricInput{a, b}, Surface: &GeometrySurfaceDefinition{Spine: &a, Guide: &b, FrameMode: "GUIDE_FRAME", Sections: []GeometrySectionControl{{ID: uuid.NewString(), Section: a, StationPoint: &p, Couplings: []GeometryCouplingPoint{{ID: id, Point: &p}}}, {ID: uuid.NewString(), Section: b, StationPoint: &p, Couplings: []GeometryCouplingPoint{{ID: id, Point: &p}}}}}}
	if err := validateSurfaceControls(d); err != nil {
		t.Fatal(err)
	}
	if len(geometryReferences(d)) != 6 {
		t.Fatal("guide, spine or correspondence omitted from references")
	}
	d.Surface.Sections[1].Couplings[0].Point = nil
	if validateSurfaceControls(d) == nil {
		t.Fatal("accepted incomplete coupling")
	}
	d.Surface.Sections[1].Couplings[0].Point = &p
	d.Surface.Sections[1].StationPoint = nil
	if validateSurfaceControls(d) == nil {
		t.Fatal("accepted implicit spine station")
	}
}
