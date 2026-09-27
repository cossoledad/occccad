package workspace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFeatureOwnedBodyDeletionLifecycle(t *testing.T) {
	m := newPartModel()
	m.Features = append(m.Features, testRectangleSketch("profile", "XY"))
	normalizePartModel(&m)
	before, _ := json.Marshal(m)
	payload, _ := json.Marshal(createFeaturePayload{Feature: Feature{ID: "extrude", Type: "LINEAR_EXTRUDE", Profile: "profile", Operation: "NEW_BODY", Length: 10}})
	created, _, err := applyCreateFeature(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	var solid PartModel
	json.Unmarshal(created, &solid)
	if len(solid.Bodies) != 2 || solid.Bodies[1].CreatedByFeatureID != "extrude" {
		t.Fatal("missing generator ownership")
	}
	deletion, _ := json.Marshal(deleteNodePayload{TargetKind: "FEATURE", TargetID: "extrude"})
	after, changes, err := applyDeletePartNode(created, deletion)
	if err != nil {
		t.Fatal(err)
	}
	var result PartModel
	json.Unmarshal(after, &result)
	if len(result.Bodies) != 1 || len(result.Features) != 1 || result.Features[0].ID != "profile" || result.ActiveBodyID != m.ActiveBodyID {
		t.Fatal("generator deletion did not preserve the profile and original Body")
	}
	if err = changes.Finalize(); err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PART", after, changes)
	if err != nil {
		t.Fatal(err)
	}
	restoredValues, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PART", after, restoredValues)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(restored, &result)
	if len(result.Bodies) != 2 || result.Bodies[1].CreatedByFeatureID != "extrude" || result.ActiveBodyID != solid.ActiveBodyID {
		t.Fatal("compensation lost Body ownership")
	}

	// Later operations must never vanish as a side effect of generator deletion.
	solid.Features = append(solid.Features, Feature{ID: "later", BodyID: solid.ActiveBodyID, Type: "LINEAR_EXTRUDE", Profile: "profile", Operation: "ADD", Length: 20})
	dependent, _ := json.Marshal(solid)
	if _, _, err = applyDeletePartNode(dependent, deletion); err == nil || !strings.Contains(err.Error(), "depends on it") {
		t.Fatalf("dependent Body operation discarded: %v", err)
	}
}

func TestBodyRejectsMissingCreatingFeature(t *testing.T) {
	m := newPartModel()
	m.Bodies[0].CreatedByFeatureID = "missing"
	if err := validatePartStructure(m); err == nil {
		t.Fatal("dangling Body owner accepted")
	}
}
