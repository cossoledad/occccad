package automation_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/assistant"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

// This constructs an explicit CAD case and exports evidence for manual review.
// It does not identify hole mouths or treat a preselection as semantic success.
func TestAutomationDebugNativeCubeChamferHole(t *testing.T) {
	db, domain, view := automationFixture(t)
	service := automation.New(db, domain, access.New(db))
	actor, document, body := access.DefaultUserID, view.Document.ID, view.Part.ActiveBodyID
	stages := []map[string]any{}
	run := func(operation string, args any) automation.ActionResult {
		t.Helper()
		snapshot, err := domain.ReadModelSnapshot(t.Context(), document, "")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(args)
		action := automation.ActionRequest{OperationID: operation, Version: 1, Arguments: raw, RequestID: uuid.NewString(), ExpectedHead: snapshot.CommandBaseline}
		preview, err := service.Prepare(t.Context(), actor, document, action)
		if err != nil {
			t.Fatal(operation, err)
		}
		action.PreviewID = preview.PreviewID
		result, err := service.Commit(t.Context(), actor, document, action)
		if err != nil {
			t.Fatal(operation, err)
		}
		stages = append(stages, map[string]any{"operation": operation, "arguments": args, "revisionId": result.Receipt.RevisionID, "output": result.Output})
		return result
	}
	sketch := run("sketch.create", map[string]any{"bodyId": body, "plane": "XY"})
	run("sketch.rectangle", map[string]any{"sketchId": sketch.Output.ID, "width": 20, "height": 20, "unit": "mm"})
	cube := run("part.extrude", map[string]any{"bodyId": body, "sketchId": sketch.Output.ID, "length": 20, "unit": "mm"})
	edges, err := service.QueryTopology(t.Context(), actor, document, automation.TopologyQuery{RevisionID: cube.Receipt.RevisionID, BodyID: body, Kind: "EDGE", Limit: 1})
	if err != nil || edges["selectionSetCount"] != 12 || edges["selectionSetComplete"] != true || edges["pageComplete"] != false {
		t.Fatal("complete all-edge input was not available", edges, err)
	}
	run("part.chamfer", map[string]any{"bodyId": body, "selectionSet": edges["selectionSet"], "size": 1, "unit": "mm"})
	holeSketch := run("sketch.create", map[string]any{"bodyId": body, "plane": "XY"})
	run("sketch.circle", map[string]any{"sketchId": holeSketch.Output.ID, "centerX": 10, "centerY": 10, "diameter": 6, "unit": "mm"})
	cut := run("part.cut", map[string]any{"bodyId": body, "sketchId": holeSketch.Output.ID, "extent": "THROUGH_ALL", "unit": "mm"})
	scope := assistant.FrozenScope{RootDocumentID: document, DocumentID: document, RevisionID: cut.Receipt.RevisionID, WorkingBodyID: body, Selections: []assistant.SelectedReference{}, Complete: true}
	read := func(tool string, args any) assistant.QueryDebugResult {
		t.Helper()
		raw, _ := json.Marshal(args)
		out, e := assistant.DebugQuery(t.Context(), service, actor, document, assistant.QueryDebugInput{Scope: scope, Tool: tool, Arguments: raw})
		if e != nil || out.ServiceResult == nil || out.ServiceResult.IsError {
			t.Fatalf("native read-only query failed %s: %s; %v", tool, out.ToolResult.Content, e)
		}
		if out.ToolResult.IsError && !out.OutputReplaced {
			t.Fatalf("unexpected model-visible error: %s", out.ToolResult.Content)
		}
		return out
	}
	defaultEdges := read("cad_query_topology", map[string]any{"kind": "EDGE", "limit": 5})
	detailedFive := read("cad_query_topology", map[string]any{"kind": "EDGE", "limit": 5, "includeSelections": true})
	pages := map[string][]assistant.QueryDebugResult{}
	for _, kind := range []string{"EDGE", "FACE"} {
		cursor := ""
		for {
			page := read("cad_query_topology", map[string]any{"kind": kind, "limit": 1, "includeSelections": true, "cursor": cursor})
			pages[kind] = append(pages[kind], page)
			if page.Supplement.Page["selectionSetComplete"] != true {
				t.Fatal("formal set lost completeness")
			}
			if page.Supplement.Page["hasMore"] != true {
				break
			}
			next, _ := page.Supplement.Page["nextCursor"].(string)
			if next == "" || next == cursor {
				t.Fatal("pagination made no progress")
			}
			cursor = next
		}
	}
	// Exercise frozen selection resolution with an arbitrary caller-chosen
	// edge. Its position is not used as persistent identity or as a hole-mouth
	// classifier; the exact formal reference and revision are the input.
	var firstPage struct {
		Items []struct {
			ID        string                     `json:"id"`
			Selection workspace.FeatureSelection `json:"selection"`
		} `json:"items"`
	}
	if err = json.Unmarshal(pages["EDGE"][0].ServiceResult.Content, &firstPage); err != nil || len(firstPage.Items) == 0 {
		t.Fatal(err)
	}
	chosen := firstPage.Items[0]
	selectedScope := scope
	selectedScope.SelectionCount = 1
	selectedScope.Selections = []assistant.SelectedReference{{Kind: "EDGE", BodyID: body, SourceDocumentID: document, SourceVersionID: scope.RevisionID, Selection: &chosen.Selection.Selection}}
	selected, err := assistant.DebugQuery(t.Context(), service, actor, document, assistant.QueryDebugInput{Scope: selectedScope, Tool: "cad_query_topology", SelectedOnly: true, Arguments: json.RawMessage(`{"kind":"EDGE","limit":1,"includeSelections":true}`)})
	if err != nil || selected.ToolResult.IsError || selected.Supplement.Page["matchedCount"] != float64(1) || len(selected.Supplement.SelectionResolutions) != 1 || selected.Supplement.SelectionResolutions[0].TopologyID != chosen.ID {
		t.Fatal("frozen native edge scope did not survive formal resolution", selected.ToolResult, err)
	}
	definitions := read("cad_query_model", map[string]any{"kind": "FEATURE", "fields": []string{"definition"}, "limit": 5})
	sketchDefinitions := read("cad_query_model", map[string]any{"kind": "SKETCH", "fields": []string{"definition"}, "limit": 2})
	// A historical read stays historical after creating the hole.
	oldScope := scope
	oldScope.RevisionID = cube.Receipt.RevisionID
	old, err := assistant.DebugQuery(t.Context(), service, actor, document, assistant.QueryDebugInput{Scope: oldScope, Tool: "cad_query_topology", Arguments: json.RawMessage(`{"kind":"EDGE","limit":1}`)})
	if err != nil || old.Supplement.Page["matchedCount"] != float64(12) || old.Supplement.Snapshot.RevisionID != cube.Receipt.RevisionID {
		t.Fatal("historical cube was replaced by current hole geometry", old.Supplement, err)
	}
	after, err := domain.ReadModelSnapshot(t.Context(), document, "")
	if err != nil || after.RevisionID != cut.Receipt.RevisionID {
		t.Fatal("query changed Head", err)
	}
	for _, check := range []struct{ table, query string }{
		{"assistant_runs", `SELECT count(*) FROM occccad.assistant_runs WHERE document_id=$1`},
		{"assistant_model_calls", `SELECT count(*) FROM occccad.assistant_model_calls WHERE run_id IN (SELECT id FROM occccad.assistant_runs WHERE document_id=$1)`},
		{"automation_tasks", `SELECT count(*) FROM occccad.automation_tasks WHERE document_id=$1`},
	} {
		var count int
		if err = db.QueryRow(t.Context(), check.query, document).Scan(&count); err != nil || count != 0 {
			t.Fatal("model-free regression created execution state", check.table, count, err)
		}
	}
	report := map[string]any{"documentId": document, "bodyId": body, "revisionId": cut.Receipt.RevisionID, "dimensionsMM": []int{20, 20, 20}, "chamferMM": 1, "holeDiameterMM": 6, "stages": stages, "defaultEdgePage": defaultEdges, "detailedFiveEdgePage": detailedFive, "pages": pages, "definitions": definitions, "sketchDefinitions": sketchDefinitions, "selectedArbitraryEdgePage": selected, "historicalCubePage": old, "semanticLocation": "NOT_TESTED: inspect exported stable identities manually; no mouth classifier or preselection-based semantic claim"}
	path, err := filepath.Abs("../../../build/agent-check/automation-debug/native-case.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("native Cube → all 12 edges chamfered → centered Ø6 through hole; read-only evidence %s; document %s", path, document)
}
