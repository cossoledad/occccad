package assistant

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

// Controlled ledger rows are not sent to any provider. They exercise the same
// recording/recovery protocol while retaining actual tool payloads and trace.
func controlledToolCall(t *testing.T, s *Service, r Run, message Message) CallRecord {
	t.Helper()
	snapshot, err := s.automation.ReadSnapshot(t.Context(), r.ActorID, r.DocumentID, "")
	if err != nil {
		t.Fatal(err)
	}
	c := CallRecord{ID: uuid.NewString(), RunID: r.ID, LogicalID: uuid.NewString(), Attempt: 1, Epoch: r.Epoch, Status: "RECEIVED", Provider: "TEST_NO_LLM", Baseline: snapshot.CommandBaseline, Response: &message, StartedAt: timestamp(s.now())}
	_, err = s.db.Exec(t.Context(), `INSERT INTO occccad.assistant_model_calls(id,run_id,logical_id,attempt,epoch,status,payload,started_at) VALUES($1,$2,$3,1,$4,'RECEIVED',$5,$6)`, c.ID, r.ID, c.LogicalID, r.Epoch, marshalJSON(c), c.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func recordedQuery(t *testing.T, s *Service, r Run, args any) (QueryResultReference, Result) {
	t.Helper()
	message := toolMessage("cad_query_topology", args)
	c := controlledToolCall(t, s, r, message)
	result := s.executeTool(t.Context(), r, c, message.ToolCalls[0])
	if result.IsError {
		t.Fatal(string(result.Content))
	}
	result, err := s.recordTool(t.Context(), r, &c, 0, message.ToolCalls[0], result)
	if err != nil {
		t.Fatal(err)
	}
	ref := queryResultReference(c.ID, message.ToolCalls[0].ID, "cad_query_topology", result)
	if ref == nil {
		t.Fatal("missing query reference")
	}
	return *ref, result
}

func TestAssistantQueryResultReferencesRecoveryScopeAndDamage(t *testing.T) {
	s, _, provider, r, _ := questionFixture(t)
	snapshot, err := s.automation.ReadSnapshot(t.Context(), r.ActorID, r.DocumentID, r.Scope.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Model = marshalJSON(workspace.PartModel{Units: "mm", ActiveBodyID: "body", Bodies: []workspace.PartBody{{ID: "body"}}})
	ws := &debugTopologyWorkspace{syntheticStageWorkspace: &syntheticStageWorkspace{head: snapshot, versions: map[string]workspace.ModelSnapshot{snapshot.RevisionID: snapshot}}, topology: workspace.BodyTopology{ManifestDigest: "manifest", GeometryKey: "geometry", Items: []workspace.BodyTopologyItem{{ID: "edge-a", GeometryType: "LINE"}, {ID: "edge-b", GeometryType: "CIRCLE"}}}}
	s.automation = automation.New(nil, ws, access.New(s.db))
	r.Scope.WorkingBodyID = "body"
	ref, result := recordedQuery(t, s, r, automation.TopologyQuery{RevisionID: snapshot.RevisionID, BodyID: "body", Kind: "EDGE", Limit: 1})
	var page map[string]any
	_ = json.Unmarshal(result.Content, &page)
	step := func() plannedStep {
		return plannedStep{StepSpec: automation.StepSpec{OperationID: "part.fillet", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": "body", "size": 2, "unit": "mm"})}, ResultBindings: []QueryResultBinding{{Target: "selectionSet", Source: ref}}}
	}
	restored := New(s.db, s.automation, Options{KeyFile: filepath.Join(t.TempDir(), "key")})
	t.Cleanup(restored.Close)
	valid := step()
	if err = restored.bindQueryResult(t.Context(), r, snapshot.CommandBaseline, &valid); err != nil {
		t.Fatal(err)
	}
	var bound map[string]any
	_ = json.Unmarshal(valid.Arguments, &bound)
	if bound["selectionSet"] != page["selectionSet"] {
		t.Fatal("restart changed the referenced output")
	}
	denied := r
	denied.ActorID = uuid.NewString()
	deniedStep := step()
	if e := restored.bindQueryResult(t.Context(), denied, snapshot.CommandBaseline, &deniedStep); !errors.Is(e, access.ErrNotFound) && !errors.Is(e, access.ErrForbidden) {
		t.Fatal("reference bypassed source permissions", e)
	}
	// A live tool transcript keeps the original payload and adds a separate
	// deterministic application note only after all tool replies are present.
	writer, e := s.mutateRun(t.Context(), r.ID, func(_ database.Tx, current *Run) error {
		current.ActiveTaskID = ""
		current.Status = "PLANNING"
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	message := toolMessage("cad_query_topology", automation.TopologyQuery{RevisionID: snapshot.RevisionID, BodyID: "body", Kind: "EDGE", Limit: 1})
	call := controlledToolCall(t, s, writer, message)
	online := &planningSession{messages: []Message{message}}
	if e = s.processCall(t.Context(), writer, call, online); e != nil || len(online.messages) != 3 || online.messages[1].Role != "tool" || !bytes.Equal([]byte(online.messages[1].Content), result.Content) || !strings.Contains(online.messages[2].Content, call.ID) {
		t.Fatal("online reference annotation changed the actual tool reply", online.messages, e)
	}
	for _, test := range []struct {
		name, code string
		change     func(*plannedStep, *Run, *workspace.CommandBaseline)
	}{
		{"missing", "QUERY_RESULT_REF_INVALID", func(p *plannedStep, _ *Run, _ *workspace.CommandBaseline) {
			p.ResultBindings[0].Source.ToolID = "missing"
		}},
		{"bad-id", "QUERY_RESULT_REF_INVALID", func(p *plannedStep, _ *Run, _ *workspace.CommandBaseline) { p.ResultBindings[0].Source.CallID = "bad" }},
		{"wrong-source-slot", "QUERY_RESULT_REF_INVALID", func(p *plannedStep, _ *Run, _ *workspace.CommandBaseline) {
			p.ResultBindings[0].Source.Slot = "items[0]"
		}},
		{"wrong-target-slot", "QUERY_RESULT_REF_INVALID", func(p *plannedStep, _ *Run, _ *workspace.CommandBaseline) { p.ResultBindings[0].Target = "bodyId" }},
		{"undeclared-operation", "QUERY_RESULT_REF_INVALID", func(p *plannedStep, _ *Run, _ *workspace.CommandBaseline) { p.OperationID = "part.cut" }},
		{"collision", "QUERY_RESULT_REF_INVALID", func(p *plannedStep, _ *Run, _ *workspace.CommandBaseline) {
			p.Arguments = marshalJSON(map[string]any{"bodyId": "body", "selectionSet": "literal"})
		}},
		{"other-run", "QUERY_RESULT_REF_SCOPE", func(_ *plannedStep, r *Run, _ *workspace.CommandBaseline) { r.ID = uuid.NewString() }},
		{"other-body", "QUERY_RESULT_REF_SCOPE", func(p *plannedStep, _ *Run, _ *workspace.CommandBaseline) {
			p.Arguments = marshalJSON(map[string]any{"bodyId": "other"})
		}},
		{"stale", "QUERY_RESULT_REF_STALE", func(_ *plannedStep, _ *Run, b *workspace.CommandBaseline) { b.RevisionID = uuid.NewString() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, run, baseline := step(), r, snapshot.CommandBaseline
			test.change(&p, &run, &baseline)
			e := restored.bindQueryResult(t.Context(), run, baseline, &p)
			if e == nil || !strings.Contains(e.Error(), test.code) {
				t.Fatal(e)
			}
		})
	}
	// A corrupt recorded output is not reported as a geometry failure.
	damaged := result
	damaged.Content = marshalJSON(map[string]any{"scope": page["scope"], "selectionSet": "damaged"})
	if _, err = s.db.Exec(t.Context(), `UPDATE occccad.assistant_tool_results SET payload=$1 WHERE call_id=$2 AND tool_id=$3`, marshalJSON(damaged), ref.CallID, ref.ToolID); err != nil {
		t.Fatal(err)
	}
	p := step()
	if err = restored.bindQueryResult(t.Context(), r, snapshot.CommandBaseline, &p); err == nil || !strings.Contains(err.Error(), "QUERY_RESULT_REF_INVALID") {
		t.Fatal("damaged token mislabeled", err)
	}
	if _, err = s.db.Exec(t.Context(), `UPDATE occccad.assistant_tool_results SET payload=$1 WHERE call_id=$2 AND tool_id=$3`, marshalJSON(result), ref.CallID, ref.ToolID); err != nil {
		t.Fatal(err)
	}
	trace, err := s.CallTrace(t.Context(), r.ActorID, r.DocumentID, r.ID, ref.CallID)
	if err != nil || len(trace.Tools) != 1 || !bytes.Equal(trace.Tools[0].Feedback, result.Content) {
		t.Fatal("trace lost full original feedback", err)
	}
	summary := recentResultContext(ref.CallID, "cad_query_topology", result, uuid.NewString())
	raw := marshalJSON(summary)
	if summary["historical"] != true || summary["selectionSetRef"] == nil || bytes.Contains(raw, []byte(page["selectionSet"].(string))) {
		t.Fatal("context expanded a token or hid historical scope", summary)
	}
	if summary["digest"] != modelcore.ValueDigest(result.Content) || !bytes.Equal(raw, marshalJSON(recentResultContext(ref.CallID, "cad_query_topology", result, "another-head"))) {
		t.Fatal("non-deterministic context projection")
	}
	if provider.calls.Load() != 0 {
		t.Fatal("query references invoked a model")
	}
	// More mandatory checks than fit one service request remain a single goal.
	checks := []automation.PartCheck{}
	for range 130 {
		checks = append(checks, automation.PartCheck{ID: uuid.NewString(), Kind: "ENTITY_EXISTS", BodyID: "body"})
	}
	results, e := s.verifyPartGoalChecks(t.Context(), r, automation.PartVerificationRequest{RevisionID: snapshot.RevisionID, GoalVersion: r.GoalVersion, Checks: checks})
	if e != nil || len(results) != 130 {
		t.Fatal("per-request boundary became a total goal limit", len(results), e)
	}
	for i, result := range results {
		if !result.Passed || result.ID != checks[i].ID {
			t.Fatal("verification pages lost requirements", result)
		}
	}
}
