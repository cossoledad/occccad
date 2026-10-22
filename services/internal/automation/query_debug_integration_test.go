package automation_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/assistant"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/modelcore"
)

// Native, model-free baseline: compact observations choose an explicitly known
// cylindrical face, then server predicates select its circular boundaries. This
// is not a general hole recognizer and uses no preselected topology references.
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

	readAt := func(frozen assistant.FrozenScope, args any) assistant.QueryDebugResult {
		t.Helper()
		raw, _ := json.Marshal(args)
		out, e := assistant.DebugQuery(t.Context(), service, actor, document, assistant.QueryDebugInput{Scope: frozen, Tool: "cad_query_topology", Arguments: raw})
		if e != nil || out.ToolResult.IsError || out.OutputReplaced || out.ServiceBytes > assistant.MaxToolResultBytes {
			t.Fatalf("bounded native query failed: %s; %v", out.ToolResult.Content, e)
		}
		actual := assistant.NewHost(service, actor, document, false).Call(t.Context(), out.ToolCall)
		if !bytes.Equal(actual.Content, out.ToolResult.Content) || actual.IsError != out.ToolResult.IsError {
			t.Fatal("debug and model tools differ")
		}
		return out
	}
	read := func(args any) assistant.QueryDebugResult { return readAt(scope, args) }
	rows := func(out assistant.QueryDebugResult) []automation.TopologyObservation {
		t.Helper()
		var page struct {
			Items []automation.TopologyObservation `json:"items"`
		}
		if e := json.Unmarshal(out.ToolResult.Content, &page); e != nil {
			t.Fatal(e)
		}
		return page.Items
	}
	radiusFilter := func(face string) *automation.TopologyFilter {
		r := 3.0
		return &automation.TopologyFilter{GeometryType: "CIRCLE", Radius: &automation.TopologyRadiusFilter{EqualMM: &r, ToleranceMM: 1e-6}, AdjacentFaceID: face}
	}
	firstFive := read(automation.TopologyQuery{Kind: "EDGE", Limit: 5})
	detailedFive := read(automation.TopologyQuery{Kind: "EDGE", Limit: 5, IncludeSelections: true})
	// Reconstruct the pre-change response format from this exact manifest and
	// first five stable IDs. This isolates projection changes from random IDs or
	// different model history, and includes the old scope/token/cursor envelope.
	full, err := domain.ReadBodyTopology(t.Context(), document, scope.RevisionID, body, "EDGE")
	if err != nil {
		t.Fatal(err)
	}
	legacy := func(detailed bool) []byte {
		oldSet := struct {
			Document string `json:"document"`
			Revision string `json:"revision"`
			Body     string `json:"body"`
			Kind     string `json:"kind"`
			Digest   string `json:"digest"`
			Count    int    `json:"count"`
		}{document, scope.RevisionID, body, "EDGE", full.ManifestDigest, len(full.Items)}
		raw, _ := json.Marshal(oldSet)
		token := base64.RawURLEncoding.EncodeToString(raw)
		items := []map[string]any{}
		for _, item := range full.Items[:5] {
			row := map[string]any{"id": item.ID, "geometryType": item.GeometryType}
			if detailed {
				row["selection"] = item.Selection
			}
			items = append(items, row)
		}
		cursorRaw, _ := json.Marshal(struct{ Set, After string }{modelcore.ValueDigest([]byte(token)), full.Items[4].ID})
		page := map[string]any{"revisionId": scope.RevisionID, "bodyId": body, "kind": "EDGE", "matchedCount": len(full.Items), "items": items, "hasMore": true, "nextCursor": base64.RawURLEncoding.EncodeToString(cursorRaw), "selectionSet": token, "selectionSetCount": len(full.Items), "selectionSetComplete": true, "pageComplete": false}
		raw, _ = json.Marshal(page)
		return raw
	}
	legacyMinimal, legacyDetailed := legacy(false), legacy(true)
	if len(legacyDetailed) <= firstFive.ToolBytes || len(rows(firstFive)) != 5 {
		t.Fatal("compact projection did not reduce the equivalent full-reference page")
	}
	if bytes.Contains(firstFive.ToolResult.Content, []byte("creationEvidence")) || bytes.Contains(firstFive.ToolResult.Content, []byte("sourceIds")) {
		t.Fatal("full evidence leaked into default tools")
	}
	// Read compact faces, not all PersistentSelections. Explicit fixture intent:
	// the single r3 cavity at x=y=10. The filter service does not classify holes.
	faceR := 3.0
	faceQuery := read(automation.TopologyQuery{Kind: "FACE", Limit: 20, Filter: &automation.TopologyFilter{GeometryType: "CYLINDER", Radius: &automation.TopologyRadiusFilter{EqualMM: &faceR, ToleranceMM: 1e-6}}})
	faceRows := rows(faceQuery)
	if len(faceRows) != 1 || faceRows[0].Geometry.MaterialSide == nil || *faceRows[0].Geometry.MaterialSide != -1 {
		t.Fatal("native compact cylinder evidence missing", faceRows)
	}
	wallID := faceRows[0].ID
	query := automation.TopologyQuery{Kind: "EDGE", Limit: 1, Filter: radiusFilter(wallID)}
	filtered := read(query)
	if filtered.Supplement.Page["matchedCount"] != float64(2) || filtered.Supplement.Page["selectionSetCount"] != float64(2) || filtered.Supplement.Page["selectionSetComplete"] != true || filtered.Supplement.Page["pageComplete"] != false {
		t.Fatal("filtered set must cover both rims before pagination", filtered.Supplement.Page)
	}
	query.Cursor = filtered.Supplement.Page["nextCursor"].(string)
	second := read(query)
	if second.Supplement.Page["hasMore"] != false || second.Supplement.Page["pageComplete"] != false || second.Supplement.Page["selectionSetCount"] != float64(2) {
		t.Fatal("last page confused with whole set", second.Supplement.Page)
	}
	targetRows := append(rows(filtered), rows(second)...)
	expectedIDs := []string{}
	for _, row := range targetRows {
		if row.Selection != nil || row.Geometry.OriginMM == nil || row.Geometry.RadiusMM == nil || math.Abs(*row.Geometry.RadiusMM-3) > 1e-6 || math.Abs((*row.Geometry.OriginMM)[0]-10) > 1e-6 || math.Abs((*row.Geometry.OriginMM)[1]-10) > 1e-6 || !row.AdjacentFaces.Complete || !slices.Contains(row.AdjacentFaces.IDs, wallID) {
			t.Fatal("unreliable native compact edge", row)
		}
		expectedIDs = append(expectedIDs, row.ID)
	}
	zs := []float64{(*targetRows[0].Geometry.OriginMM)[2], (*targetRows[1].Geometry.OriginMM)[2]}
	slices.Sort(zs)
	if math.Abs(zs[0]) > 1e-6 || math.Abs(zs[1]-20) > 1e-6 {
		t.Fatal("did not find the fixture's two circular boundaries", zs)
	}
	// Explicit evidence expansion remains pinned to the same revision and ID.
	evidence := read(automation.TopologyQuery{Kind: "EDGE", IDs: []string{expectedIDs[0]}, Limit: 1, IncludeSelections: true})
	if e := rows(evidence); len(e) != 1 || e[0].Selection == nil || e[0].ID != expectedIDs[0] {
		t.Fatal("explicit evidence expansion lost identity", e)
	}
	// Empty sets cannot reach the evaluator as all edges.
	emptyR := 100.0
	empty := read(automation.TopologyQuery{Kind: "EDGE", Filter: &automation.TopologyFilter{GeometryType: "CIRCLE", Radius: &automation.TopologyRadiusFilter{EqualMM: &emptyR, ToleranceMM: 1e-6}}})
	before, _ := domain.ReadModelSnapshot(t.Context(), document, "")
	// selectionSet lives in the actual output, not the human page supplement.
	var emptyContent map[string]any
	_ = json.Unmarshal(empty.ToolResult.Content, &emptyContent)
	badArgs, _ := json.Marshal(map[string]any{"bodyId": body, "selectionSet": emptyContent["selectionSet"], "size": 0.3, "unit": "mm"})
	_, err = service.Prepare(t.Context(), actor, document, automation.ActionRequest{OperationID: "part.chamfer", Version: 1, Arguments: badArgs, RequestID: uuid.NewString(), ExpectedHead: before.CommandBaseline})
	if err == nil || !strings.Contains(err.Error(), "TOPOLOGY_SELECTION_EMPTY") {
		t.Fatal("zero matches were not rejected", err)
	}
	var filteredContent map[string]any
	_ = json.Unmarshal(filtered.ToolResult.Content, &filteredContent)
	// Only the complete filtered collection, never page rows, enters native preview/commit.
	rimChamfer := run("part.chamfer", map[string]any{"bodyId": body, "selectionSet": filteredContent["selectionSet"], "size": 0.3, "unit": "mm"})
	after, err := domain.GetDocument(t.Context(), document)
	if err != nil {
		t.Fatal(err)
	}
	feature := after.Part.Features[len(after.Part.Features)-1]
	if feature.Type != "CHAMFER" || feature.ID != rimChamfer.Output.ID || len(feature.Selections) != 2 {
		t.Fatal("rim chamfer consumed another set", feature)
	}
	committedIDs := []string{}
	for _, selection := range feature.Selections {
		if selection.SourceVersionID != scope.RevisionID {
			t.Fatal("commit silently changed selection revision")
		}
		raw, _ := json.Marshal(selection.Selection)
		committedIDs = append(committedIDs, modelcore.ValueDigest(raw))
	}
	slices.Sort(expectedIDs)
	slices.Sort(committedIDs)
	if !slices.Equal(expectedIDs, committedIDs) {
		t.Fatal("committed selections differ from filtered full set", expectedIDs, committedIDs)
	}
	fresh, _ := domain.ReadModelSnapshot(t.Context(), document, "")
	staleArgs, _ := json.Marshal(map[string]any{"bodyId": body, "selectionSet": filteredContent["selectionSet"], "size": 0.2, "unit": "mm"})
	if _, err = service.Prepare(t.Context(), actor, document, automation.ActionRequest{OperationID: "part.chamfer", Version: 1, Arguments: staleArgs, RequestID: uuid.NewString(), ExpectedHead: fresh.CommandBaseline}); err == nil || !strings.Contains(err.Error(), "TOPOLOGY_SELECTION_STALE") {
		t.Fatal("set rebound to latest revision", err)
	}
	// Same-radius interference in the same Body: another through hole and a boss.
	otherSketch := run("sketch.create", map[string]any{"bodyId": body, "plane": "XY"})
	run("sketch.circle", map[string]any{"sketchId": otherSketch.Output.ID, "centerX": 5, "centerY": 15, "diameter": 6, "unit": "mm"})
	run("part.cut", map[string]any{"bodyId": body, "sketchId": otherSketch.Output.ID, "extent": "THROUGH_ALL", "unit": "mm"})
	bossSketch := run("sketch.create", map[string]any{"bodyId": body, "plane": "XY"})
	run("sketch.circle", map[string]any{"sketchId": bossSketch.Output.ID, "centerX": 15, "centerY": 5, "diameter": 6, "unit": "mm"})
	boss := run("part.extrude", map[string]any{"bodyId": body, "sketchId": bossSketch.Output.ID, "length": 24, "unit": "mm"})
	interferenceScope := scope
	interferenceScope.RevisionID = boss.Receipt.RevisionID
	interferenceFaces := readAt(interferenceScope, automation.TopologyQuery{Kind: "FACE", Limit: 20, Filter: &automation.TopologyFilter{GeometryType: "CYLINDER", Radius: &automation.TopologyRadiusFilter{EqualMM: &faceR, ToleranceMM: 1e-6}}})
	cavityCount, bossCount := 0, 0
	currentWall := ""
	for _, face := range rows(interferenceFaces) {
		g := face.Geometry
		if g.MaterialSide == nil || g.OriginMM == nil {
			t.Fatal("missing cylinder evidence", face)
		}
		if *g.MaterialSide == -1 {
			cavityCount++
		} else if *g.MaterialSide == 1 {
			bossCount++
		}
		if *g.MaterialSide == -1 && math.Abs((*g.OriginMM)[0]-10) <= 1e-6 && math.Abs((*g.OriginMM)[1]-10) <= 1e-6 {
			if currentWall != "" {
				t.Fatal("fixture target ambiguous")
			}
			currentWall = face.ID
		}
	}
	if cavityCount != 2 || bossCount != 1 || currentWall == "" {
		t.Fatal("interference fixture not established", cavityCount, bossCount, currentWall)
	}
	radiusOnly := readAt(interferenceScope, automation.TopologyQuery{Kind: "EDGE", Limit: 1, Filter: radiusFilter("")})
	associated := readAt(interferenceScope, automation.TopologyQuery{Kind: "EDGE", Limit: 1, Filter: radiusFilter(currentWall)})
	if radiusOnly.Supplement.Page["matchedCount"].(float64) <= 2 || associated.Supplement.Page["matchedCount"] != float64(2) || associated.Supplement.Page["selectionSetCount"] != float64(2) || associated.Supplement.Page["unknownCount"] != float64(0) {
		t.Fatal("face adjacency failed to exclude same-radius interference", radiusOnly.Supplement.Page, associated.Supplement.Page)
	}
	// Also execute the filtered set with same-radius interference present. Query
	// the second compact page solely to compare committed stable identities.
	interferenceNext := readAt(interferenceScope, automation.TopologyQuery{Kind: "EDGE", Limit: 1, Filter: radiusFilter(currentWall), Cursor: associated.Supplement.Page["nextCursor"].(string)})
	interferenceIDs := []string{rows(associated)[0].ID, rows(interferenceNext)[0].ID}
	var associatedContent map[string]any
	_ = json.Unmarshal(associated.ToolResult.Content, &associatedContent)
	interferenceChamfer := run("part.chamfer", map[string]any{"bodyId": body, "selectionSet": associatedContent["selectionSet"], "size": 0.1, "unit": "mm"})
	interferenceModel, err := domain.GetDocument(t.Context(), document)
	if err != nil {
		t.Fatal(err)
	}
	interferenceFeature := interferenceModel.Part.Features[len(interferenceModel.Part.Features)-1]
	actualInterferenceIDs := []string{}
	for _, selection := range interferenceFeature.Selections {
		if selection.SourceVersionID != interferenceScope.RevisionID {
			t.Fatal("interference set changed Revision")
		}
		raw, _ := json.Marshal(selection.Selection)
		actualInterferenceIDs = append(actualInterferenceIDs, modelcore.ValueDigest(raw))
	}
	slices.Sort(interferenceIDs)
	slices.Sort(actualInterferenceIDs)
	if interferenceFeature.ID != interferenceChamfer.Output.ID || !slices.Equal(interferenceIDs, actualInterferenceIDs) {
		t.Fatal("interference chamfer consumed other edges", actualInterferenceIDs)
	}
	// Replaying the original fixed scope after all changes still returns the same
	// bytes and same stable set. It must not substitute the new Head or face IDs.
	replay := readAt(scope, automation.TopologyQuery{Kind: "EDGE", Limit: 1, Filter: radiusFilter(wallID)})
	if !bytes.Equal(replay.ToolResult.Content, filtered.ToolResult.Content) {
		t.Fatal("frozen query changed after later modeling")
	}
	oldScope := scope
	oldScope.RevisionID = cube.Receipt.RevisionID
	old := readAt(oldScope, automation.TopologyQuery{Kind: "EDGE", Limit: 1})
	if old.Supplement.Page["matchedCount"] != float64(12) || old.Supplement.Snapshot.RevisionID != cube.Receipt.RevisionID {
		t.Fatal("historical cube changed")
	}
	final, err := domain.ReadModelSnapshot(t.Context(), document, "")
	if err != nil || final.RevisionID != interferenceChamfer.Receipt.RevisionID {
		t.Fatal("read-only queries changed Head", err)
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
	firstFiveIDs := []string{}
	for _, row := range rows(firstFive) {
		firstFiveIDs = append(firstFiveIDs, row.ID)
	}
	comparison := map[string]any{"sameScopeRevision": scope.RevisionID, "sameFirstFiveIDs": firstFiveIDs, "legacyMinimalBytes": len(legacyMinimal), "legacyDetailedBytes": len(legacyDetailed), "compactFiveBytes": firstFive.ToolBytes, "newDetailedRequestedFiveBytes": detailedFive.ToolBytes, "newDetailedReturnedCount": len(rows(detailedFive)), "filteredOneOfTwoBytes": filtered.ToolBytes, "legacyDetailedFormat": json.RawMessage(legacyDetailed)}
	report := map[string]any{"documentId": document, "bodyId": body, "baselineRevisionId": scope.RevisionID, "finalRevisionId": interferenceChamfer.Receipt.RevisionID, "stages": stages, "comparison": comparison, "compactFive": firstFive, "budgetedDetailed": detailedFive, "cylinderFaces": faceQuery, "filteredFirst": filtered, "filteredSecond": second, "expandedEvidence": evidence, "zeroMatches": empty, "committedIDs": committedIDs, "interferenceFaces": interferenceFaces, "interferenceRadiusOnly": radiusOnly, "interferenceAssociated": associated, "interferenceCommittedIDs": actualInterferenceIDs, "historicalCube": old, "semanticLocation": "EXPLICIT_FIXTURE_INTENT: human/test chooses cavity face using compact current center/radius/material side, server matches adjacent circles; no generic hole classifier or preselection"}
	path, err := filepath.Abs("../../../build/agent-check/automation-debug/compact-native-case.json")
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
	t.Logf("same-scope first 5 edges: old IDs %d B, old detailed %d B, compact %d B; new detailed %d B/%d rows; selected %v; report %s; document %s", len(legacyMinimal), len(legacyDetailed), firstFive.ToolBytes, detailedFive.ToolBytes, len(rows(detailedFive)), committedIDs, path, document)
}
