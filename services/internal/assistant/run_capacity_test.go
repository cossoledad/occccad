//go:build integration

package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type capacityProvider struct {
	complete func(context.Context, []Message, Generation) (Completion, error)
}

func (*capacityProvider) Validate(context.Context, string) error { return nil }
func (*capacityProvider) Models(context.Context, string) ([]Model, error) {
	return []Model{{ID: "deepseek-flash"}}, nil
}
func (p *capacityProvider) Complete(ctx context.Context, _ string, m []Message, _ []Tool, g ...Generation) (Completion, error) {
	return p.complete(ctx, m, g[0])
}

// Only this test-owned reader supplies synthetic parameters. Production CAD
// correctness is checked separately through native stages/Worker and browser.
type capacityWorkspace struct {
	*workspace.Service
	parameters []modelcore.ParameterDefinition
}

func (w capacityWorkspace) ReadModelSnapshot(ctx context.Context, doc, revision string) (workspace.ModelSnapshot, error) {
	snapshot, e := w.Service.ReadModelSnapshot(ctx, doc, revision)
	if e != nil {
		return snapshot, e
	}
	var part workspace.PartModel
	if e = json.Unmarshal(snapshot.Model, &part); e != nil {
		return snapshot, e
	}
	part.Parameters = w.parameters
	snapshot.Model = marshalJSON(part)
	return snapshot, nil
}

func TestAssistantRunUnboundedCallsSyntheticMillionsAndRecovery(t *testing.T) {
	s, domain, view, actor := assistantFixture(t, "http://127.0.0.1:1")
	now := time.Now().UTC()
	s.options.Now = func() time.Time { return now }
	s.options.Limits = defaultRequestLimits()
	s.options.Limits.MaxContextBytes = 24 << 10
	params := []modelcore.ParameterDefinition{}
	for i := 0; i < 120; i++ {
		q, _ := modelcore.NewQuantity(float64(i+1), "mm")
		params = append(params, modelcore.ParameterDefinition{ParameterID: uuid.NewString(), Key: fmt.Sprintf("parameter_%03d", i), DisplayUnit: "mm", ValueType: modelcore.ValueQuantity, EvaluatedValue: &q})
	}
	s.automation = automation.New(s.db, capacityWorkspace{domain, params}, access.New(s.db))
	const private = "PRIVATE_CHAIN_OF_THOUGHT_TEST_SENTINEL"
	goal := "当前参数有哪些？核对80×50 mm与拉伸12 mm，保持原对象不变；轴孔装配方向SAME且保留一个旋转自由度。"
	calls, rebuilds := 0, 0
	provider := &capacityProvider{complete: func(ctx context.Context, m []Message, g Generation) (Completion, error) {
		calls++
		if len(m) == 3 {
			rebuilds++
		}
		if len(marshalJSON(m)) > s.requestLimits().MaxContextBytes {
			t.Fatal("unbounded working set")
		}
		if len(m) < 3 || !strings.Contains(m[1].Content, "80") || !strings.Contains(m[1].Content, "SAME") || !strings.Contains(m[1].Content, "保持原对象不变") {
			t.Fatal("essential source intent disappeared")
		}
		if g.MaxTokens != 8192 {
			t.Fatalf("actual configuration missing: %+v", g)
		}
		cache, reasoning := int64(30000), int64(8000)
		c := Completion{ProviderRequestID: fmt.Sprintf("controlled-%d", calls), Usage: &Usage{PromptTokens: 40000, CompletionTokens: 10000, TotalTokens: 50000, CacheHitTokens: &cache, ReasoningTokens: &reasoning}}
		if calls <= 40 {
			for j := 0; j < 3; j++ {
				q := toolMessage("cad_query_model", map[string]any{"kind": "PARAMETER", "id": params[(calls-1)*3+j].ParameterID, "fields": []string{"value", "unit"}, "limit": 1})
				c.Message.ToolCalls = append(c.Message.ToolCalls, q.ToolCalls[0])
			}
			c.Message.Role = "assistant"
			c.Message.ReasoningContent = private + strings.Repeat("private working memory ", 80)
		} else {
			c.Message = Message{Role: "assistant", Content: "已按只读快照查询120个参数，未修改模型。"}
		}
		return c, nil
	}}
	s.provider = provider
	if _, e := s.SaveKey(t.Context(), actor, "controlled-not-a-real-provider-key"); e != nil {
		t.Fatal(e)
	}
	r, e := s.CreateRun(t.Context(), actor, view.Document.ID, RunInput{RequestID: uuid.NewString(), Text: goal})
	if e != nil {
		t.Fatal(e)
	}
	if r.AccessMode != "WRITE" || r.TokenBudget != nil {
		t.Fatal("default budget or read scope incorrect")
	}
	step := func() {
		t.Helper()
		var err error
		r, err = s.Step(t.Context(), actor, r.DocumentID, r.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	for calls < 18 {
		step()
	}
	// Fault after the public response and usage were durably stored, before tools.
	messages, current, e := s.buildContext(t.Context(), r, nil)
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.prepareCall(t.Context(), r, "", current.Snapshot.CommandBaseline, messages)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.markSent(t.Context(), &c); e != nil {
		t.Fatal(e)
	}
	completion, e := provider.Complete(t.Context(), "", messages, planningTools(), c.Generation)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.receiveCall(t.Context(), &c, completion, nil); e != nil {
		t.Fatal(e)
	}
	s.Close()
	reopened := New(s.db, s.automation, s.options)
	reopened.provider = provider
	t.Cleanup(reopened.Close)
	s = reopened
	before := calls
	step()
	if calls != before {
		t.Fatal("persisted response charged again")
	}
	if e = s.receiveCall(t.Context(), &c, completion, nil); e != nil {
		t.Fatal(e)
	}
	for calls < 26 {
		step()
	}
	// Fault while a request could have been sent: no fabricated zero usage.
	messages, current, e = s.buildContext(t.Context(), r, nil)
	if e != nil {
		t.Fatal(e)
	}
	unknown, e := s.prepareCall(t.Context(), r, "", current.Snapshot.CommandBaseline, messages)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.markSent(t.Context(), &unknown); e != nil {
		t.Fatal(e)
	}
	s.Close()
	reopened = New(s.db, s.automation, s.options)
	reopened.provider = provider
	t.Cleanup(reopened.Close)
	s = reopened
	step()
	now = now.Add(3 * time.Hour)
	for r.Status != "COMPLETED" && calls < 45 {
		step()
	}
	if r.Status != "COMPLETED" || calls != 41 || r.ModelCalls != 42 || r.ToolCalls != 120 || r.Usage.TotalTokens != 2050000 || r.Usage.PromptTokens != 1640000 || r.Usage.CompletionTokens != 410000 || r.Usage.CacheHitTokens != 1230000 || r.Usage.ReasoningTokens != 328000 || r.Usage.UnknownAttempts != 1 || r.Usage.ReportedCalls != 41 || rebuilds < 3 {
		t.Fatalf("capacity/recovery mismatch run=%+v calls=%d rebuilds=%d", r, calls, rebuilds)
	}
	// All 120 public tools read distinct immutable IDs; no meaningless iterations.
	var results [][]byte
	rows, e := s.db.Query(t.Context(), `SELECT payload FROM occccad.assistant_tool_results WHERE call_id IN (SELECT id FROM occccad.assistant_model_calls WHERE run_id=$1)`, r.ID)
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var raw []byte
		if e = rows.Scan(&raw); e != nil {
			t.Fatal(e)
		}
		results = append(results, raw)
	}
	rows.Close()
	for _, raw := range results {
		var v Result
		if json.Unmarshal(raw, &v) != nil || v.IsError || !strings.Contains(string(v.Content), `"matchedCount":1`) {
			t.Fatalf("query did not progress: %s", raw)
		}
	}
	// Ledger pagination uses time+ID even where controlled clock timestamps tie.
	seen := map[string]bool{}
	cursor := ""
	for {
		page, e := s.Calls(t.Context(), actor, r.DocumentID, r.ID, cursor, 7)
		if e != nil {
			t.Fatal(e)
		}
		items := page["items"].([]CallRecord)
		for _, v := range items {
			if seen[v.ID] {
				t.Fatal("duplicate ledger page")
			}
			seen[v.ID] = true
			if strings.Contains(string(marshalJSON(v)), private) {
				t.Fatal("private reasoning persisted")
			}
		}
		if !page["hasMore"].(bool) {
			break
		}
		cursor = page["nextCursor"].(string)
	}
	if len(seen) != 42 {
		t.Fatalf("ledger missing rows: %d", len(seen))
	}
	tail, e := s.Events(t.Context(), actor, r.DocumentID, r.ID, 0, 0, 5)
	if e != nil || !tail.HasEarlier || len(tail.Items) != 5 {
		t.Fatalf("event tail: %+v %v", tail, e)
	}
	older, e := s.Events(t.Context(), actor, r.DocumentID, r.ID, 0, tail.Items[0].Sequence, 5)
	if e != nil || len(older.Items) != 5 || older.Items[4].Sequence >= tail.Items[0].Sequence {
		t.Fatal("event reverse pagination")
	}
	delta, e := s.Events(t.Context(), actor, r.DocumentID, r.ID, older.Items[4].Sequence, 0, 5)
	if e != nil || len(delta.Items) != 5 || delta.Items[0].Sequence != older.Items[4].Sequence+1 {
		t.Fatal("event incremental pagination")
	}
	if _, e = s.Calls(t.Context(), uuid.NewString(), r.DocumentID, r.ID, "", 10); e == nil {
		t.Fatal("cross-actor ledger exposure")
	}
	t.Logf("CONTROLLED_SYNTHETIC_USAGE run=%s calls=%d tools=%d total=%d cacheSubset=%d reasoningSubset=%d unknownAttempts=%d contextRebuilds=%d elapsedClock=3h; no paid provider calls", r.ID, r.ModelCalls, r.ToolCalls, r.Usage.TotalTokens, r.Usage.CacheHitTokens, r.Usage.ReasoningTokens, r.Usage.UnknownAttempts, rebuilds)
}
