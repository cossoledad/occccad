package workspace

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/modelcore"
	"reflect"
	"strings"
	"testing"
)

func moveUnitModel() PartModel {
	m := newPartModel()
	body := m.ActiveBodyID
	m.Features = []Feature{{ID: "sketch", Type: "SKETCH", BodyID: body, Sketch: &SketchFeature{SchemaVersion: SketchSchemaVersion, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"}}}, {ID: "base", Type: "PAD", Profile: "sketch", BodyID: body, Operation: "ADD", Length: 10}, {ID: "a", Type: "PAD", Profile: "sketch", BodyID: body, Operation: "ADD", Length: 10}, {ID: "b", Type: "PAD", Profile: "sketch", BodyID: body, Operation: "ADD", Length: 10}}
	normalizePartModel(&m)
	return m
}
func TestMoveFeaturePreservesDefinitionsAndCompensation(t *testing.T) {
	m := moveUnitModel()
	before, _ := json.Marshal(m)
	digest, _ := featureDefinitionDigest(m, "b")
	p, _ := json.Marshal(moveFeaturePayload{FeatureID: "b", AnchorFeatureID: "a", Placement: "BEFORE", ExpectedFeatureDigest: digest})
	after, set, err := applyMoveFeature(before, p)
	if err != nil {
		t.Fatal(err)
	}
	var candidate PartModel
	_ = json.Unmarshal(after, &candidate)
	if candidate.Features[2].ID != "b" || candidate.Features[3].ID != "a" || len(set.Changes) != 2 || !reflect.DeepEqual(candidate.Parameters, m.Parameters) {
		t.Fatal("single-item move changed unrelated definitions", candidate, set)
	}
	values := map[modelcore.PropertyAddress]json.RawMessage{}
	for _, c := range set.Changes {
		values[c.Target] = c.Before
	}
	restored, err := applyPartHistoryValues(after, values)
	if err != nil {
		t.Fatal(err)
	}
	var original PartModel
	_ = json.Unmarshal(restored, &original)
	if !reflect.DeepEqual(original.Features, m.Features) || !reflect.DeepEqual(original.Parameters, m.Parameters) {
		t.Fatal("compensation lost original order or parameter definitions")
	}
}
func TestMoveFeatureFixedNodesAndRanges(t *testing.T) {
	for _, kind := range []string{"IMPORT_BODY", "BOOLEAN", "SOLID_PATTERN"} {
		m := moveUnitModel()
		fixed := Feature{ID: "fixed", Name: kind, Type: kind, BodyID: m.ActiveBodyID, Order: 4}
		m.Features = append(m.Features[:3], append([]Feature{fixed}, m.Features[3:]...)...)
		m.Features[4].Order = 5
		raw, _ := json.Marshal(m)
		digest, _ := featureDefinitionDigest(m, "b")
		p, _ := json.Marshal(moveFeaturePayload{FeatureID: "b", AnchorFeatureID: "a", Placement: "BEFORE", ExpectedFeatureDigest: digest})
		if _, _, err := applyMoveFeature(raw, p); err == nil || !strings.Contains(err.Error(), "fixed feature cannot be crossed") {
			t.Fatal("crossed fixed node", kind, err)
		}
	}
	m := moveUnitModel()
	for _, id := range []string{"base", "sketch"} {
		raw, _ := json.Marshal(m)
		digest, _ := featureDefinitionDigest(m, id)
		p, _ := json.Marshal(moveFeaturePayload{FeatureID: id, AnchorFeatureID: "b", Placement: "AFTER", ExpectedFeatureDigest: digest})
		if _, _, err := applyMoveFeature(raw, p); err == nil {
			t.Fatal("moved fixed/source sketch", id)
		}
	}
	m = moveUnitModel()
	m.Features[2].Type = "FILLET"
	m.Features = append(m.Features, Feature{ID: "range", Type: "SOLID_PATTERN", BodyID: m.ActiveBodyID, Pattern: &FeaturePattern{SourceKind: "FEATURE_DELTA", StartFeatureID: "base", Source: FeatureStageRef{BodyID: m.ActiveBodyID, FeatureID: "a"}}})
	raw, _ := json.Marshal(m)
	digest, _ := featureDefinitionDigest(m, "b")
	p, _ := json.Marshal(moveFeaturePayload{FeatureID: "b", AnchorFeatureID: "a", Placement: "BEFORE", ExpectedFeatureDigest: digest})
	if _, _, err := applyMoveFeature(raw, p); err == nil || !strings.Contains(err.Error(), "FEATURE_DELTA") {
		t.Fatal("changed protected range", err)
	}
}
