//go:build integration

package assistant

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAssistantFrozenSelectionCompactContextAndExplicitEvidence(t *testing.T) {
	s, _, _, r, _ := questionFixture(t)
	r.Scope.SelectionCount = 1
	r.Scope.Selections = []SelectedReference{{Kind: "EDGE", BodyID: r.Scope.WorkingBodyID, SourceDocumentID: r.DocumentID, SourceVersionID: r.Scope.RevisionID, Selection: &modelcore.PersistentSelection{SchemaVersion: 2, SourceDocumentID: r.DocumentID, SourceBodyID: r.Scope.WorkingBodyID, ExpectedType: "EDGE", Anchor: modelcore.SemanticTopologyRef{FeatureID: "owner-feature", OutputSlot: strings.Repeat("FORMAL_EVIDENCE_SENTINEL", 120)}, Selector: modelcore.SelectionRecipe{Kind: "SELF"}}}}
	messages, _, err := s.buildContext(t.Context(), r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(messages[1].Content, "FORMAL_EVIDENCE_SENTINEL") || strings.Contains(messages[1].Content, "creationEvidence") {
		t.Fatal("full frozen evidence was injected into the default context")
	}
	call := toolMessage("cad_get_selection", map[string]any{"limit": 1}).ToolCalls[0]
	compact := s.executeTool(t.Context(), r, CallRecord{}, call)
	if compact.IsError || bytes.Contains(compact.Content, []byte("FORMAL_EVIDENCE_SENTINEL")) || !bytes.Contains(compact.Content, []byte("referenceDigest")) {
		t.Fatal(string(compact.Content))
	}
	call.Function.Arguments = `{"limit":1,"includeEvidence":true}`
	full := s.executeTool(t.Context(), r, CallRecord{}, call)
	if full.IsError || !bytes.Contains(full.Content, []byte("FORMAL_EVIDENCE_SENTINEL")) || len(full.Content) <= len(compact.Content) {
		t.Fatal("explicit evidence did not read the original reference")
	}
	t.Logf("same frozen page: compact %d bytes, explicit formal evidence %d bytes; initial context contains no evidence copy", len(compact.Content), len(full.Content))
	var page struct {
		Items []SelectedReference `json:"items"`
	}
	if json.Unmarshal(full.Content, &page) != nil || !bytes.Equal(marshalJSON(page.Items[0].Selection), marshalJSON(r.Scope.Selections[0].Selection)) {
		t.Fatal("evidence read changed the execution reference")
	}
}

func TestAssistantFrozenSelectionUsesActualFeatureAndSketchEntityTypes(t *testing.T) {
	part := workspace.PartModel{Features: []workspace.Feature{{ID: "fillet", Type: "FILLET", BodyID: "body", Name: "Fillet 2"}, {ID: "sketch", Type: "SKETCH", BodyID: "body", Sketch: &workspace.SketchFeature{Entities: []workspace.SketchEntity{{ID: "circle", Kind: "CIRCLE"}}}}}}
	feature := SelectedReference{Kind: "PAD", ID: "fillet", EntityType: "PAD"}
	selectionMetadata(&feature, &part)
	if feature.Kind != "FEATURE" || feature.EntityType != "FILLET" || feature.BodyID != "body" {
		t.Fatal("display PAD replaced actual type", feature)
	}
	entity := SelectedReference{Kind: "VISUAL", ID: "circle", FeatureID: "sketch", EntityType: "EDGE"}
	selectionMetadata(&entity, &part)
	if entity.Kind != "SKETCH_ENTITY" || entity.EntityType != "CIRCLE" || entity.OwnerID != "sketch" || entity.BodyID != "body" {
		t.Fatal("sketch geometry became BRep topology", entity)
	}
}
