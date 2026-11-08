package workspace

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/modelcore"
	"strings"
	"testing"
)

func testGeometryLine() Feature {
	return Feature{ID: "line", Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "LINE", Curve: &GeometryCurveDefinition{Points: []GeometryControlPoint{{ID: "start", Position: [3]float64{0, 0, 0}}, {ID: "end", Position: [3]float64{50, 0, 0}}}}}}
}
func TestGeometryFeatureOwnerParametersHistoryAndDependencies(t *testing.T) {
	before, _ := json.Marshal(newPartModel())
	payload, _ := json.Marshal(createFeaturePayload{Feature: testGeometryLine()})
	created, changes, err := applyCreateFeature(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	var m PartModel
	json.Unmarshal(created, &m)
	f := m.Features[0]
	if f.BodyID != "" || f.GeometricalSetID == "" || len(m.GeometricalSets) != 1 || len(m.Parameters) != 6 {
		t.Fatalf("incorrect independent ownership/parameters: %+v", m)
	}
	values, err := partHistoryValues(created, changes)
	if err != nil {
		t.Fatal(err)
	}
	restore, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := applyModelValues("PART", created, restore)
	if err != nil {
		t.Fatal(err)
	}
	var empty PartModel
	json.Unmarshal(raw, &empty)
	if len(empty.Features) != 0 || len(empty.GeometricalSets) != 0 || len(empty.Parameters) != 0 {
		t.Fatal("creation undo left geometry records")
	}
	for i := range m.Parameters {
		if m.Parameters[i].PropertySlot == "geometry:point:end:x" {
			q, _ := modelcore.NewQuantity(75, "mm")
			m.Parameters[i].Source = modelcore.ValueSource{Literal: &q}
		}
	}
	if err := validateAndResolvePartParameters(&m); err != nil {
		t.Fatal(err)
	}
	if m.Features[0].Geometry.Curve.Points[1].Position[0] != 75 {
		t.Fatal("parameter did not reach curve definition")
	}
	surface := Feature{ID: "surface", Type: "GEOMETRY", GeometricalSetID: f.GeometricalSetID, Order: 2, Geometry: &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: "line", ResultID: "result"}}, Parameters: GeometryOperationParameters{Direction: [3]float64{0, 0, 1}, Distance: 20}}}
	m.Features = append(m.Features, surface)
	normalizePartModel(&m)
	graph, _, err := buildPartEvaluation(m, "revision", "hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range graph.Edges {
		if e.Source == "feature:line" && e.Target == "feature:surface" {
			found = true
		}
	}
	if !found {
		t.Fatal("curve/surface dependency omitted")
	}
	m.Features[0].Result = &FeatureGeometryResult{ID: "result", Domain: "WIRE_SET", GeometryKey: "derived"}
	if featureHistoryDefinition(m.Features[0]).Result != nil || geometryFeatureDefinition(m.Features[0]).Result != nil {
		t.Fatal("derived result entered feature definition")
	}
	bad := testGeometryLine()
	bad.BodyID = m.ActiveBodyID
	input, _ := json.Marshal(createFeaturePayload{Feature: bad})
	if _, _, err := applyCreateFeature(before, input); err == nil {
		t.Fatal("accepted Body-owned independent curve")
	}
	m.Features[1].Geometry.Inputs[0].FeatureID = "later"
	if err := validatePartStructure(m); err == nil {
		t.Fatal("accepted forward/missing input")
	}
}
func TestGeometrySourceOwnerIsExclusiveAndCrossDocumentReferencesAreRejected(t *testing.T) {
	selection := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: "part", SourceResult: &modelcore.GeometryResultReference{FeatureID: "curve", ResultID: "result"}, Anchor: modelcore.SemanticTopologyRef{FeatureID: "curve", OutputSlot: "curve"}, ExpectedType: modelcore.PersistentTopologyEdge, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}}
	if err := selection.Validate(); err != nil {
		t.Fatal(err)
	}
	selection.SourceBodyID = "body"
	if err := selection.Validate(); err == nil {
		t.Fatal("accepted ambiguous source owner")
	}
	selection.SourceBodyID = ""
	f := testGeometryLine()
	f.Geometry = &GeometricFeatureDefinition{Kind: "EXTRACT", Inputs: []GeometricInput{{Kind: "TOPOLOGY", Selection: &FeatureSelection{Selection: selection, SourceVersionID: "revision", SourceFeatureID: "curve"}}}}
	if err := qualifyGeometryInputs("other-part", f); err == nil || !strings.Contains(err.Error(), "cross-document") {
		t.Fatalf("unqualified source: %v", err)
	}
}
