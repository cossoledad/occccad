package control

import (
	"encoding/json"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAssemblyDiagnosticNestedSourcesThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	insert := func(sourceID string) workspace.DocumentView {
		t.Helper()
		product, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Diagnostic nesting"})
		if err != nil {
			t.Fatal(err)
		}
		product, err = f.service.ApplyCommand(t.Context(), product.Document.ID, workspace.CommandRequest{ActorID: f.actor, Type: "INSERT_INSTANCE", ReferencedDocumentID: sourceID})
		if err != nil {
			t.Fatal(err)
		}
		return product
	}
	child := insert(part.Document.ID)
	root := insert(child.Document.ID)
	bundle, err := f.service.ExportAssemblyDiagnostic(t.Context(), root.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	var file geometry.AssemblyReplay
	if err := json.Unmarshal(bundle, &file); err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Sources []struct {
			DocumentID string          `json:"documentId"`
			RevisionID string          `json:"revisionId"`
			Model      json.RawMessage `json:"model"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(file.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sources) != 2 {
		t.Fatal("nested source closure incomplete", len(snapshot.Sources))
	}
	for _, source := range snapshot.Sources {
		if source.DocumentID != part.Document.ID {
			continue
		}
		var restored workspace.PartModel
		if err := json.Unmarshal(source.Model, &restored); err != nil {
			t.Fatal(err)
		}
		if source.RevisionID != part.Document.VersionID || len(restored.Bodies) != len(part.Part.Bodies) || len(restored.Bodies) == 0 {
			t.Fatal("Part revision or CAD Body lost")
		}
	}
	current, err := f.service.GetDocument(t.Context(), root.Document.ID)
	if err != nil || current.Document.VersionID != root.Document.VersionID {
		t.Fatal("nested export changed head", err)
	}
}
