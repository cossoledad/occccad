package workspace

import (
	"encoding/json"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestRepairImportNamingPreservesFeatureAndCompensates(t *testing.T) {
	model := newPartModel()
	original := Feature{ID: "import-root", Type: "IMPORT_BODY", Name: "Original", GeometryKey: "frozen-brep", SourceFormat: "BREP"}
	model.Features = append(model.Features, original)
	before, _ := json.Marshal(model)
	repaired := original
	repaired.ImportDefinitionID = "frozen-definition"
	payload, _ := json.Marshal(createFeaturePayload{Feature: repaired})
	after, changes, err := workspaceCommandRegistry.Apply("PART", before, modelcore.DomainCommand{CommandID: "repair", TypeURI: typeRepairImportNaming, SchemaVersion: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelValues("PART", after, changes)
	if err != nil {
		t.Fatal(err)
	}
	undo, err := changes.Compensate(current)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PART", after, undo)
	if err != nil {
		t.Fatal(err)
	}
	var actual PartModel
	if err = json.Unmarshal(restored, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Features[0] != original {
		t.Fatalf("repair undo changed original import: %+v", actual.Features[0])
	}
	if _, _, err = applyRepairImportNaming(after, payload); err == nil {
		t.Fatal("must not reallocate already named import")
	}
}
