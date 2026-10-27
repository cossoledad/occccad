//go:build integration

package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/database"
)

type finishFaultDB struct {
	database.DB
	reject bool
}
type finishFaultTx struct {
	database.Tx
	owner *finishFaultDB
}

func (d *finishFaultDB) Begin(ctx context.Context) (database.Tx, error) {
	tx, err := d.DB.Begin(ctx)
	return &finishFaultTx{Tx: tx, owner: d}, err
}
func (t *finishFaultTx) Exec(ctx context.Context, query string, args ...any) (database.Result, error) {
	if t.owner.reject && strings.Contains(query, "INSERT INTO occccad.assistant_tool_results") {
		t.owner.reject = false
		return database.Result{}, errors.New("test-owned interruption before finish tool record")
	}
	return t.Tx.Exec(ctx, query, args...)
}

func TestAssistantCurrentRequestFinishAndLedgerAreAtomic(t *testing.T) {
	f := newRequestFixture(t, "可以，保留")
	message := toolMessage("cad_finish", map[string]any{"summary": "好的，保留。"})
	c := controlledToolCall(t, f.s, f.r, message)
	f.s.db = &finishFaultDB{DB: f.s.db, reject: true}
	tool := message.ToolCalls[0]
	result := f.s.executeTool(t.Context(), f.r, c, tool)
	if _, err := f.s.recordTool(t.Context(), f.r, &c, 0, tool, result); err == nil {
		t.Fatal("fault injection did not fire")
	}
	f.refresh(t)
	if f.r.Status == "COMPLETED" || f.r.Reply != "" {
		t.Fatal("tool-record rollback left a completed request", f.r)
	}
	stored, err := loadCall(t.Context(), f.s.db, c.ID)
	if err != nil || stored.Status != "RECEIVED" || stored.ToolCursor != 0 {
		t.Fatal("finish rollback lost response recovery", stored, err)
	}
	var records int
	if err = f.s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.assistant_tool_results WHERE call_id=$1`, c.ID).Scan(&records); err != nil || records != 0 {
		t.Fatal(records, err)
	}
	// Recover using the original durable response; no provider or CAD action.
	result, err = f.s.recordTool(t.Context(), f.r, &stored, 0, tool, result)
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	f.refresh(t)
	if f.r.Status != "COMPLETED" || f.r.Reply != "好的，保留。" || f.r.RequestAppliedCount != 0 || stored.Status != "APPLIED" || stored.ToolCursor != 1 {
		t.Fatal("finish recovery was incomplete", f.r, stored)
	}
}

func TestAssistantPartChecksAreOptionalAndSnapshotScoped(t *testing.T) {
	s, _, provider, r, _ := questionFixture(t)
	host := NewHost(s.automation, r.ActorID, r.DocumentID, false)
	host.revision = r.Scope.RevisionID
	input := map[string]any{"revisionId": r.Scope.RevisionID, "checks": []map[string]any{{"id": "body", "kind": "ENTITY_EXISTS", "bodyId": r.Scope.WorkingBodyID}}}
	result := host.Call(t.Context(), Call{ID: "check", Name: "cad_verify_part", Arguments: marshalJSON(input)})
	if result.IsError || !bytes.Contains(result.Content, []byte(`"passed":true`)) || bytes.Contains(result.Content, []byte(`"model"`)) {
		t.Fatal("check failed or expanded the full model", string(result.Content))
	}
	for _, revision := range []string{"", "different-snapshot"} {
		input["revisionId"] = revision
		result = host.Call(t.Context(), Call{ID: "invalid", Name: "cad_verify_part", Arguments: marshalJSON(input)})
		if !result.IsError {
			t.Fatal("check escaped the pinned snapshot", revision)
		}
	}
	current, err := s.GetRun(t.Context(), r.ActorID, r.DocumentID, r.ID)
	if err != nil || current.Version != r.Version || current.Status != r.Status || provider.calls.Load() != 0 {
		t.Fatal("optional check changed Run or called a model", current, err)
	}
	var packet map[string]json.RawMessage
	if err := json.Unmarshal(marshalJSON(current), &packet); err != nil {
		t.Fatal(err)
	}
	if packet["requirements"] != nil || packet["verification"] != nil {
		t.Fatal("Run still owns a natural-language completion contract")
	}
}
