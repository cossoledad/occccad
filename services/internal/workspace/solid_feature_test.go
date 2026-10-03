package workspace

import (
	"encoding/json"
	"testing"
)

func TestBooleanExplicitStagesAndHistory(t *testing.T) {
	m := newPartModel()
	m.Bodies = append(m.Bodies, PartBody{ID: "tool", Name: "Tool", Visible: true})
	m.Features = []Feature{{ID: "base", BodyID: m.ActiveBodyID, Type: "IMPORT_BODY", GeometryKey: "base"}, {ID: "tool-stage", BodyID: "tool", Type: "IMPORT_BODY", GeometryKey: "tool"}}
	normalizePartModel(&m)
	f := Feature{ID: "cut", Type: "BOOLEAN", BodyID: m.ActiveBodyID, Operation: "REMOVE", Tools: []FeatureStageRef{{BodyID: "tool", FeatureID: "tool-stage"}}}
	before, _ := json.Marshal(m)
	payload, _ := json.Marshal(createFeaturePayload{Feature: f})
	after, changes, err := applyCreateFeature(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	var created PartModel
	_ = json.Unmarshal(after, &created)
	if !created.Bodies[1].Consumed || !created.Bodies[1].Visible {
		t.Fatal("consumption must preserve visibility")
	}
	digest, _ := featureDefinitionDigest(created, "cut")
	f = created.Features[len(created.Features)-1]
	f.KeepTools = true
	edit, _ := json.Marshal(editFeaturePayload{FeatureID: f.ID, ExpectedFeatureDigest: digest, Definition: &f})
	edited, _, err := applyEditFeature(after, edit)
	if err != nil {
		t.Fatal(err)
	}
	var retained PartModel
	_ = json.Unmarshal(edited, &retained)
	if retained.Bodies[1].Consumed {
		t.Fatal("keep tools did not restore output")
	}
	// A tool is bound to its explicit upstream feature, not the Body's future tip.
	_, _, err = buildPartEvaluation(created, "revision", "model", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PART", after, changes)
	if err != nil {
		t.Fatal(err)
	}
	compensation, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PART", after, compensation)
	if err != nil {
		t.Fatal(err)
	}
	var original PartModel
	_ = json.Unmarshal(restored, &original)
	if len(original.Features) != 2 || original.Bodies[1].Consumed {
		t.Fatal("undo did not restore body output")
	}
	for _, bad := range []Feature{
		{ID: "self", Type: "BOOLEAN", BodyID: m.ActiveBodyID, Operation: "REMOVE", Tools: []FeatureStageRef{{m.ActiveBodyID, "base"}}},
		{ID: "future", Type: "BOOLEAN", BodyID: m.ActiveBodyID, Operation: "REMOVE", Tools: []FeatureStageRef{{"tool", "later"}}},
		{ID: "multi", Type: "BOOLEAN", BodyID: m.ActiveBodyID, Operation: "INTERSECT", Tools: []FeatureStageRef{{"tool", "tool-stage"}, {"tool", "tool-stage"}}},
	} {
		p, _ := json.Marshal(createFeaturePayload{Feature: bad})
		if _, _, err := applyCreateFeature(before, p); err == nil {
			t.Fatalf("accepted invalid stage %s", bad.ID)
		}
	}
}

func TestSolidDefinitionEditParametersAndUndo(t *testing.T) {
	m := newPartModel()
	m.Features = []Feature{{ID: "s", Type: "SKETCH", BodyID: m.ActiveBodyID, Plane: "XY", Sketch: &SketchFeature{SchemaVersion: 2, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"}}}, {ID: "pad", Type: "LINEAR_EXTRUDE", BodyID: m.ActiveBodyID, Profile: "s", Operation: "ADD", Length: 10}}
	normalizePartModel(&m)
	original, _ := json.Marshal(m)
	digest, _ := featureDefinitionDigest(m, "pad")
	f := m.Features[1]
	f.Extent = "TWO_SIDED"
	f.Length2 = 5
	f.Length = 20
	payload, _ := json.Marshal(editFeaturePayload{FeatureID: f.ID, ExpectedFeatureDigest: digest, Definition: &f})
	next, changes, err := applyEditFeature(original, payload)
	if err != nil {
		t.Fatal(err)
	}
	var edited PartModel
	_ = json.Unmarshal(next, &edited)
	if edited.Features[1].Length != 20 || edited.Features[1].Length2 != 5 || edited.Features[1].BodyID != f.BodyID {
		t.Fatal("incomplete edit")
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
	var model PartModel
	_ = json.Unmarshal(restored, &model)
	if model.Features[1].Length != 10 || model.Features[1].Extent != "" || len(model.Parameters) != 1 {
		t.Fatalf("incorrect restore: %+v", model.Features[1])
	}
}
