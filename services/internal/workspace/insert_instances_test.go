package workspace

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
)

func TestInsertInstancesPatternIsOneChangeSet(t *testing.T) {
	t.Parallel()
	base := ProductModel{Instances: []ProductInstance{{ID: "source", Name: "Block.1", ReferencedDocumentID: "part",
		ReferencedVersionID: "revision", Translation: [3]float64{3, 4, 5}, Rotation: [4]float64{0, 0, 0, 1}, ReferenceMode: "PINNED"}}}
	base.Instances = append(base.Instances, ProductInstance{ID: "sibling", Name: "Block.2", ReferencedDocumentID: "part"})
	before, _ := json.Marshal(base)
	kind, payload, err := (&Service{}).adaptLegacyCommand(t.Context(), "product", "PRODUCT", before,
		CommandRequest{Type: "INSERT_INSTANCES", InstanceID: "source", PatternAxis: "Y", PatternCount: 4,
			PatternSpacing: 12.5, PatternReversed: true})
	if err != nil || kind != typeInsertInstances {
		t.Fatalf("adapt pattern: kind=%s err=%v", kind, err)
	}
	payloadJSON, _ := json.Marshal(payload)
	after, changes, err := applyInsertInstances(before, payloadJSON)
	if err != nil || len(changes.Changes) != 3 {
		t.Fatalf("apply pattern: changes=%d err=%v", len(changes.Changes), err)
	}
	var model ProductModel
	if err := json.Unmarshal(after, &model); err != nil {
		t.Fatal(err)
	}
	if len(model.Instances) != 5 || model.Instances[0] != base.Instances[0] {
		t.Fatalf("source changed: %+v", model.Instances)
	}
	for index, instance := range model.Instances[2:] {
		want := [3]float64{3, 4 - float64(index+1)*12.5, 5}
		if instance.Name != fmt.Sprintf("Block.%d", index+3) || instance.Translation != want || instance.Rotation != base.Instances[0].Rotation || instance.ReferenceMode != "PINNED" ||
			instance.ReferencedVersionID != "revision" || instance.ID == "source" {
			t.Fatalf("copy %d: %+v", index, instance)
		}
	}
}

func TestInsertInstancesRejectsInvalidBatchAtomically(t *testing.T) {
	t.Parallel()
	before, _ := json.Marshal(ProductModel{Instances: []ProductInstance{{ID: "source", Name: "Block.1", ReferencedDocumentID: "part"}}})
	for _, input := range []CommandRequest{
		{Type: "INSERT_INSTANCES", InstanceID: "source", PatternAxis: "X", PatternCount: 2, PatternSpacing: 0},
		{Type: "INSERT_INSTANCES", InstanceID: "source", PatternAxis: "Y", PatternCount: 129, PatternSpacing: 1},
		{Type: "INSERT_INSTANCES", InstanceID: "source", PatternAxis: "Z", PatternCount: 2, PatternSpacing: math.NaN()},
	} {
		if _, _, err := (&Service{}).adaptLegacyCommand(t.Context(), "product", "PRODUCT", before, input); err == nil {
			t.Fatalf("accepted invalid pattern: %+v", input)
		}
	}
	payload, _ := json.Marshal(insertInstancesPayload{Instances: []ProductInstance{
		{ID: "new", Name: "Block.2", ReferencedDocumentID: "part"},
		{ID: "new", Name: "Block.3", ReferencedDocumentID: "part"},
	}})
	if _, _, err := applyInsertInstances(before, payload); err == nil {
		t.Fatal("duplicate ID must reject entire batch")
	}
}

func TestInsertInstancesDocumentBatchCreatesOneRevisionPayload(t *testing.T) {
	t.Parallel()
	before, _ := json.Marshal(ProductModel{Instances: []ProductInstance{{ID: "existing", Name: "Wheel.1", ReferencedDocumentID: "wheel"}}})
	payload, _ := json.Marshal(insertInstancesPayload{Instances: []ProductInstance{
		{ID: "a", Name: "Wheel.2", ReferencedDocumentID: "wheel", ReferencedVersionID: "r1", ReferenceMode: "FOLLOW_HEAD"},
		{ID: "b", Name: "Axle.1", ReferencedDocumentID: "axle", ReferencedVersionID: "r2", ReferenceMode: "FOLLOW_HEAD"},
	}})
	after, changes, err := applyInsertInstances(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Changes) != 2 || len(changes.ImpactSeeds) != 2 {
		t.Fatalf("one batch ChangeSet: %+v", changes)
	}
	var model ProductModel
	if err := json.Unmarshal(after, &model); err != nil {
		t.Fatal(err)
	}
	if len(model.Instances) != 3 || model.Instances[0].ID != "existing" || model.Instances[1].ID != "a" || model.Instances[2].ID != "b" {
		t.Fatalf("unexpected batch result: %+v", model.Instances)
	}
}

func TestInstanceReferenceNameRemovesOnlyFinalOrdinal(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{"Part1.1": "Part1", "Part.1.12": "Part.1", "Custom": "Custom", "Part.abc": "Part.abc"} {
		if got := instanceReferenceName(input); got != want {
			t.Errorf("%q: got %q, want %q", input, got, want)
		}
	}
}
