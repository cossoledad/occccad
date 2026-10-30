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
	p, _ := json.Marshal(moveFeaturePayload{FeatureIDs: []string{"b"}, ExpectedFeatureOrderDigest: featureOrderDigest(m), AnchorFeatureID: "a", Placement: "BEFORE", ExpectedFeatureDigests: map[string]string{"b": digest}})
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
		p, _ := json.Marshal(moveFeaturePayload{FeatureIDs: []string{"b"}, ExpectedFeatureOrderDigest: featureOrderDigest(m), AnchorFeatureID: "a", Placement: "BEFORE", ExpectedFeatureDigests: map[string]string{"b": digest}})
		if _, _, err := applyMoveFeature(raw, p); err == nil || !strings.Contains(err.Error(), "fixed feature cannot be crossed") {
			t.Fatal("crossed fixed node", kind, err)
		}
	}
	m := moveUnitModel()
	for _, id := range []string{"base", "sketch"} {
		raw, _ := json.Marshal(m)
		digest, _ := featureDefinitionDigest(m, id)
		p, _ := json.Marshal(moveFeaturePayload{FeatureIDs: []string{id}, ExpectedFeatureOrderDigest: featureOrderDigest(m), AnchorFeatureID: "b", Placement: "AFTER", ExpectedFeatureDigests: map[string]string{id: digest}})
		if _, _, err := applyMoveFeature(raw, p); err == nil {
			t.Fatal("moved fixed/source sketch", id)
		}
	}
	m = moveUnitModel()
	m.Features[2].Type = "FILLET"
	m.Features = append(m.Features, Feature{ID: "range", Type: "SOLID_PATTERN", BodyID: m.ActiveBodyID, Pattern: &FeaturePattern{SourceKind: "FEATURE_DELTA", StartFeatureID: "base", Source: FeatureStageRef{BodyID: m.ActiveBodyID, FeatureID: "a"}}})
	raw, _ := json.Marshal(m)
	digest, _ := featureDefinitionDigest(m, "b")
	p, _ := json.Marshal(moveFeaturePayload{FeatureIDs: []string{"b"}, ExpectedFeatureOrderDigest: featureOrderDigest(m), AnchorFeatureID: "a", Placement: "BEFORE", ExpectedFeatureDigests: map[string]string{"b": digest}})
	if _, _, err := applyMoveFeature(raw, p); err == nil || !strings.Contains(err.Error(), "FEATURE_DELTA") {
		t.Fatal("changed protected range", err)
	}
}

func TestMoveFeatureCollectionOrderAndFrozenInputs(t *testing.T) {
	m := moveUnitModel()
	body := m.ActiveBodyID
	other := "other"
	m.Bodies = append(m.Bodies, PartBody{ID: other, Name: "Body.2", Order: 2, Visible: true})
	extra := Feature{ID: "extra-sketch", Type: "SKETCH", BodyID: body, Sketch: &SketchFeature{SchemaVersion: SketchSchemaVersion, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"}}}
	m.Features[3].Suppressed = true
	m.Features = append(m.Features[:3], append([]Feature{{ID: "other-base", Type: "PAD", BodyID: other, Profile: "sketch", Operation: "ADD", Length: 4}, extra}, m.Features[3:]...)...)
	m.Features = append(m.Features, Feature{ID: "c", Type: "PAD", BodyID: body, Profile: "sketch", Operation: "ADD", Length: 4})
	normalizePartModel(&m)
	for i := range m.Features {
		m.Features[i].Order = i + 1
	}
	raw, _ := json.Marshal(m)
	request := func(ids []string, anchor string) moveFeaturePayload {
		p := moveFeaturePayload{FeatureIDs: ids, AnchorFeatureID: anchor, Placement: "AFTER", ExpectedFeatureDigests: map[string]string{}, ExpectedFeatureOrderDigest: featureOrderDigest(m)}
		for _, id := range ids {
			p.ExpectedFeatureDigests[id], _ = featureDefinitionDigest(m, id)
		}
		return p
	}
	for _, ids := range [][]string{{"a", "c"}, {"a", "a"}, {"a", "other-base"}, {"base", "a"}} {
		p, _ := json.Marshal(request(ids, "c"))
		if _, _, e := applyMoveFeature(raw, p); e == nil {
			t.Fatal("invalid collection accepted", ids)
		}
	}
	p := request([]string{"a", "b"}, "b")
	data, _ := json.Marshal(p)
	if _, _, e := applyMoveFeature(raw, data); e == nil {
		t.Fatal("selected anchor accepted")
	}
	p = request([]string{"a", "b"}, "c")
	p.ExpectedFeatureOrderDigest = "stale"
	data, _ = json.Marshal(p)
	if _, _, e := applyMoveFeature(raw, data); e == nil {
		t.Fatal("stale global order accepted")
	}
	p = request([]string{"a", "b"}, "c")
	p.ExpectedFeatureDigests["b"] = "stale"
	data, _ = json.Marshal(p)
	if _, _, e := applyMoveFeature(raw, data); e == nil {
		t.Fatal("stale member accepted")
	}
	p = request([]string{"a", "b"}, "c")
	data, _ = json.Marshal(p)
	next, set, e := applyMoveFeature(raw, data)
	if e != nil {
		t.Fatal(e)
	}
	var after PartModel
	_ = json.Unmarshal(next, &after)
	ids := []string{}
	for _, f := range after.Features {
		ids = append(ids, f.ID)
	}
	if !reflect.DeepEqual(ids, []string{"sketch", "base", "other-base", "extra-sketch", "c", "a", "b"}) || !after.Features[len(after.Features)-1].Suppressed || !reflect.DeepEqual(m.Parameters, after.Parameters) {
		t.Fatal("collection extraction changed unrelated objects", ids)
	}
	old := map[modelcore.PropertyAddress]json.RawMessage{}
	for _, change := range set.Changes {
		old[change.Target] = change.Before
	}
	restored, e := applyPartHistoryValues(next, old)
	if e != nil {
		t.Fatal(e)
	}
	var before PartModel
	_ = json.Unmarshal(restored, &before)
	if !reflect.DeepEqual(before.Features, m.Features) {
		t.Fatal("one compensation did not restore interleaving")
	}
}
