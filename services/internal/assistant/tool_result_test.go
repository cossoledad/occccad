//go:build integration

package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestAssistantToolResultByteBoundary(t *testing.T) {
	exact := Result{CallID: "tool", Content: json.RawMessage(`"` + strings.Repeat("x", MaxToolResultBytes-2) + `"`)}
	if got := NormalizeToolResult(exact); got.IsError || !bytes.Equal(got.Content, exact.Content) {
		t.Fatal("a complete boundary-sized JSON result changed")
	}
	large := Result{CallID: "tool", Content: marshalJSON(strings.Repeat("孔口", MaxToolResultBytes))}
	got := NormalizeToolResult(large)
	if !got.IsError || got.CallID != large.CallID || len(got.Content) > MaxToolResultBytes || !json.Valid(got.Content) || !bytes.Contains(got.Content, []byte("TOOL_PAGE_RESOURCE")) {
		t.Fatal("oversize output was not replaced as a complete result", got)
	}
	if next := NormalizeToolResult(got); !bytes.Equal(next.Content, got.Content) || !next.IsError {
		t.Fatal("normalization is not idempotent")
	}
	if invalid := NormalizeToolResult(Result{Content: json.RawMessage(`{"partial":`)}); !invalid.IsError || !json.Valid(invalid.Content) {
		t.Fatal("invalid JSON survived the output boundary")
	}
}

func TestAssistantToolResultOnlineLedgerRecoveryAndTraceAgree(t *testing.T) {
	for _, question := range []bool{false, true} {
		t.Run(map[bool]string{false: "planner", true: "question"}[question], func(t *testing.T) {
			s, _, provider, r, input := questionFixture(t)
			// A synthetic large but structurally valid frozen selection page.
			// No model configuration, Provider invocation or CAD evaluation.
			for range 20 {
				selection := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: r.DocumentID, SourceBodyID: r.Scope.WorkingBodyID, ExpectedType: "EDGE", Anchor: modelcore.SemanticTopologyRef{FeatureID: uuid.NewString(), OutputSlot: "EDGE"}, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}}
				for range 20 {
					selection.CreationEvidence.Adjacent = append(selection.CreationEvidence.Adjacent, modelcore.SemanticTopologyRef{FeatureID: uuid.NewString(), OutputSlot: "FACE"})
				}
				if err := selection.Validate(); err != nil {
					t.Fatal(err)
				}
				r.Scope.Selections = append(r.Scope.Selections, SelectedReference{Kind: "EDGE", Selection: &selection, BodyID: r.Scope.WorkingBodyID, SourceDocumentID: r.DocumentID, SourceVersionID: r.Scope.RevisionID})
			}
			r.Scope.SelectionCount = len(r.Scope.Selections)
			ctx := t.Context()
			inputID := ""
			if question {
				input.Scope = r.Scope
				if err := saveInput(ctx, s.db, input); err != nil {
					t.Fatal(err)
				}
				token := uuid.NewString()
				if _, err := s.db.Exec(ctx, `UPDATE occccad.assistant_inputs SET lease_token=$1,lease_until=$2 WHERE id=$3`, token, s.now().Add(questionLeaseDuration).UnixMilli(), input.ID); err != nil {
					t.Fatal(err)
				}
				ctx = context.WithValue(ctx, questionLeaseKey{}, questionLease{inputID: input.ID, token: token})
				inputID = input.ID
			} else {
				r.Status, r.ActiveTaskID, r.PendingCallID = "PLANNING", "", ""
			}
			if err := s.saveRun(ctx, s.db, &r); err != nil {
				t.Fatal(err)
			}
			snapshot, err := s.automation.ReadSnapshot(ctx, r.ActorID, r.DocumentID, r.Scope.RevisionID)
			if err != nil {
				t.Fatal(err)
			}
			c, err := s.prepareCall(ctx, r, inputID, snapshot.CommandBaseline, []Message{{Role: "user", Content: "read selection"}})
			if err != nil {
				t.Fatal(err)
			}
			message := toolMessage("cad_get_selection", map[string]any{"limit": 20, "includeEvidence": true})
			c.Status, c.Response = "RECEIVED", &message
			if err = saveCall(ctx, s.db, c); err != nil {
				t.Fatal(err)
			}
			raw := s.executeTool(ctx, r, c, message.ToolCalls[0])
			if raw.IsError || len(raw.Content) <= MaxToolResultBytes {
				t.Fatal("fixture did not exercise recordTool's replacement", len(raw.Content))
			}
			canonical := NormalizeToolResult(raw)
			var process func(*planningSession) error
			if question {
				process = func(session *planningSession) error { return s.processQuestionCall(ctx, r, input, c, session) }
			} else {
				process = func(session *planningSession) error { return s.processCall(ctx, r, c, session) }
			}
			online := &planningSession{messages: []Message{message}}
			if err = process(online); err != nil {
				t.Fatal(err)
			}
			if len(online.messages) != 2 || online.messages[1].Content != string(canonical.Content) {
				t.Fatal("online session retained the raw result")
			}
			var stored []byte
			if err = s.db.QueryRow(ctx, `SELECT payload FROM occccad.assistant_tool_results WHERE call_id=$1`, c.ID).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			var recorded Result
			if json.Unmarshal(stored, &recorded) != nil || !bytes.Equal(recorded.Content, canonical.Content) || !recorded.IsError {
				t.Fatal("ledger differs from online feedback")
			}
			// A different post-recovery candidate must not replace original feedback.
			returned, err := s.recordTool(ctx, r, &c, 0, message.ToolCalls[0], Result{CallID: canonical.CallID, Content: json.RawMessage(`{"different":true}`)})
			if err != nil || !bytes.Equal(returned.Content, canonical.Content) {
				t.Fatal("idempotent recording did not return the durable value", err)
			}
			replayed := &planningSession{messages: []Message{message}}
			// The original captured c has cursor zero, modeling a rebuilt session.
			c.ToolCursor = 0
			if err = process(replayed); err != nil || replayed.messages[1].Content != online.messages[1].Content {
				t.Fatal("replay differs from the online tool message", err)
			}
			latest, err := loadRun(ctx, s.db, r.ID, false)
			if err != nil || latest.ToolCalls != 1 {
				t.Fatal("replay counted an additional tool effect", latest.ToolCalls, err)
			}
			messages, _, err := s.buildContext(ctx, latest, nil)
			if err != nil {
				t.Fatal(err)
			}
			var contextPacket struct {
				RecentFacts []struct {
					Result json.RawMessage `json:"result"`
				} `json:"recentFacts"`
			}
			if json.Unmarshal([]byte(messages[1].Content), &contextPacket) != nil || len(contextPacket.RecentFacts) != 1 {
				t.Fatal("recovery facts missing")
			}
			var summary struct {
				Digest string `json:"digest"`
			}
			if json.Unmarshal(contextPacket.RecentFacts[0].Result, &summary) != nil || summary.Digest != modelcore.ValueDigest(canonical.Content) {
				t.Fatal("recovery summary does not identify canonical normalized feedback")
			}
			trace, err := s.CallTrace(ctx, r.ActorID, r.DocumentID, r.ID, c.ID)
			if err != nil || len(trace.Tools) != 1 || !bytes.Equal(trace.Tools[0].Feedback, canonical.Content) {
				t.Fatal("trace differs from model-visible feedback", err)
			}
			if provider.calls.Load() != 0 {
				t.Fatal("output normalization invoked a model")
			}
			var count int
			if err = s.db.QueryRow(ctx, `SELECT count(*) FROM occccad.assistant_tool_results WHERE call_id=$1`, c.ID).Scan(&count); err != nil || count != 1 {
				t.Fatal(count, err)
			}
		})
	}
}
