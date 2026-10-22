package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

type debugTopologyWorkspace struct {
	*syntheticStageWorkspace
	topology workspace.BodyTopology
}

func (w *debugTopologyWorkspace) ReadBodyTopology(_ context.Context, doc, revision, body, kind string) (workspace.BodyTopology, error) {
	if doc != w.head.DocumentID || revision != w.head.RevisionID || body != "body" || kind != "EDGE" {
		return workspace.BodyTopology{}, workspace.ErrNotFound
	}
	return w.topology, nil
}

func TestAssistantDebugQueryFixedSnapshotAndActualOutput(t *testing.T) {
	s, _, provider, r, _ := questionFixture(t)
	old, err := s.automation.ReadSnapshot(t.Context(), r.ActorID, r.DocumentID, r.Scope.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	var part workspace.PartModel
	if err = json.Unmarshal(old.Model, &part); err != nil {
		t.Fatal(err)
	}
	// One projected item fits the service's item budget, but its enclosing
	// snapshot/page exceeds the actual tool-result boundary.
	part.Features = []workspace.Feature{{ID: "feature", BodyID: part.ActiveBodyID, Type: "PAD", Name: strings.Repeat("x", 32400)}}
	old.Model = marshalJSON(part)
	newer := old
	newer.RevisionID = uuid.NewString()
	newer.Model = marshalJSON(workspace.PartModel{Units: "mm", Bodies: part.Bodies, ActiveBodyID: part.ActiveBodyID})
	ws := &syntheticStageWorkspace{head: newer, versions: map[string]workspace.ModelSnapshot{old.RevisionID: old, newer.RevisionID: newer}}
	service := automation.New(nil, ws, access.New(s.db))
	scope := r.Scope
	input := QueryDebugInput{Scope: scope, Tool: "cad_query_model", Arguments: marshalJSON(map[string]any{"id": "feature", "fields": []string{"name"}})}
	out, err := DebugQuery(t.Context(), service, r.ActorID, r.DocumentID, input)
	if err != nil || out.ServiceResult == nil || out.ServiceResult.IsError || !out.OutputReplaced || !out.ToolResult.IsError || out.ServiceBytes <= MaxToolResultBytes || out.ToolBytes > MaxToolResultBytes {
		t.Fatal("raw and model-visible results were not distinguished", out.ServiceBytes, out.ToolBytes, err)
	}
	actual := NewHost(service, r.ActorID, r.DocumentID, false).Call(t.Context(), out.ToolCall)
	if !bytes.Equal(actual.Content, out.ToolResult.Content) || out.Supplement.Snapshot.RevisionID != old.RevisionID {
		t.Fatal("debug output was a simulation or used latest Head")
	}
	if replay, replayErr := DebugQuery(t.Context(), service, r.ActorID, r.DocumentID, out.Input); replayErr != nil || !bytes.Equal(replay.ToolResult.Content, out.ToolResult.Content) {
		t.Fatal("the exported fixed input could not be replayed", replayErr)
	}
	if out.Supplement.Page["complete"] != true || out.Supplement.Page["returnedCount"] != 1 {
		t.Fatal("service page completeness was lost", out.Supplement.Page)
	}
	for _, change := range []func(*QueryDebugInput){
		func(i *QueryDebugInput) { i.Scope.RevisionID = "" },
		func(i *QueryDebugInput) { i.Scope.RevisionID = uuid.NewString() },
		func(i *QueryDebugInput) { i.Scope.WorkingBodyID = "other-body" },
		func(i *QueryDebugInput) { i.Tool = "cad_create_task" },
		func(i *QueryDebugInput) { i.Scope.EditContextID = "unsupported-context" },
		func(i *QueryDebugInput) {
			i.SelectedOnly = true
			i.Scope.SelectionCount = 2
			i.Scope.Selections = []SelectedReference{{ID: "feature", SourceDocumentID: r.DocumentID, SourceVersionID: old.RevisionID}}
		},
		func(i *QueryDebugInput) { i.Arguments = marshalJSON(map[string]any{"revisionId": newer.RevisionID}) },
	} {
		changed := input
		change(&changed)
		if _, err = DebugQuery(t.Context(), service, r.ActorID, r.DocumentID, changed); err == nil {
			t.Fatal("unfixed or mismatched query scope was accepted", changed)
		}
	}
	if _, err = DebugQuery(t.Context(), service, uuid.NewString(), r.DocumentID, input); err == nil {
		t.Fatal("debug bypassed document ACL")
	}
	var runs, calls int
	if err = s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.assistant_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.assistant_model_calls`).Scan(&calls); err != nil || runs != 1 || calls != 0 || provider.calls.Load() != 0 {
		t.Fatal("debug created a Run or contacted a model", runs, calls, err)
	}
}

func TestAssistantDebugTopologySetAndPageCompleteness(t *testing.T) {
	s, _, _, r, _ := questionFixture(t)
	snapshot, err := s.automation.ReadSnapshot(t.Context(), r.ActorID, r.DocumentID, r.Scope.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Model = marshalJSON(workspace.PartModel{Units: "mm", ActiveBodyID: "body", Bodies: []workspace.PartBody{{ID: "body"}}})
	ws := &debugTopologyWorkspace{syntheticStageWorkspace: &syntheticStageWorkspace{head: snapshot, versions: map[string]workspace.ModelSnapshot{snapshot.RevisionID: snapshot}}, topology: workspace.BodyTopology{ManifestDigest: "exact-manifest", Items: []workspace.BodyTopologyItem{{ID: "edge-a", GeometryType: "LINE"}, {ID: "edge-b", GeometryType: "CIRCLE"}}}}
	service := automation.New(nil, ws, access.New(s.db))
	r.Scope.WorkingBodyID = "body"
	input := QueryDebugInput{Scope: r.Scope, Tool: "cad_query_topology", Arguments: marshalJSON(map[string]any{"kind": "EDGE", "limit": 1})}
	first, err := DebugQuery(t.Context(), service, r.ActorID, r.DocumentID, input)
	if err != nil || first.ToolResult.IsError || first.Supplement.Page["selectionSetComplete"] != true || first.Supplement.Page["pageComplete"] != false || first.Supplement.Page["hasMore"] != true {
		t.Fatal(first, err)
	}
	var args map[string]any
	_ = json.Unmarshal(input.Arguments, &args)
	args["cursor"] = first.Supplement.Page["nextCursor"]
	input.Arguments = marshalJSON(args)
	last, err := DebugQuery(t.Context(), service, r.ActorID, r.DocumentID, input)
	if err != nil || last.ToolResult.IsError || last.Supplement.Page["hasMore"] != false || last.Supplement.Page["pageComplete"] != false {
		t.Fatal("last page was confused with the full collection", last, err)
	}
	args["ids"] = []string{"edge-a"}
	input.Arguments = marshalJSON(args)
	changed, err := DebugQuery(t.Context(), service, r.ActorID, r.DocumentID, input)
	if err != nil || !changed.ToolResult.IsError || !bytes.Contains(changed.ToolResult.Content, []byte("INVALID_CURSOR")) {
		t.Fatal("cursor retargeted a different selection", changed, err)
	}
}
