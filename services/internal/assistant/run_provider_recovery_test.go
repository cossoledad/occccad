package assistant

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

func TestAssistantBudgetToolResponseRebuildsValidThinkingTranscript(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "deepseek-flash"}}})
			return
		}
		var body struct {
			Messages []Message `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		number := calls.Add(1)
		if number == 1 {
			message := toolMessage("cad_describe_operation", map[string]any{"operationId": "part.fillet"})
			message.ReasoningContent = "private online reasoning"
			writeCompletion(w, message)
			return
		}
		for i, message := range body.Messages {
			if len(message.ToolCalls) > 0 && (i+1 == len(body.Messages) || body.Messages[i+1].Role != "tool") {
				http.Error(w, "unanswered tool calls", 400)
				return
			}
			if message.ReasoningContent != "" {
				t.Error("restored request replayed old private reasoning")
			}
		}
		writeCompletion(w, Message{Role: "assistant", Content: "已经读取正式圆角操作合同。"})
	}))
	defer provider.Close()
	s, _, view, actor := assistantFixture(t, provider.URL)
	if _, e := s.SaveKey(t.Context(), actor, "synthetic-budget-transcript-key"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SavePreferences(t.Context(), actor, Generation{Model: "deepseek-flash", Thinking: "high"}); e != nil {
		t.Fatal(e)
	}
	limit := int64(50)
	run, e := s.CreateRun(t.Context(), actor, view.Document.ID, RunInput{RequestID: uuid.NewString(), Text: "查看圆角操作的参数有哪些？", TokenBudget: &limit})
	if e != nil {
		t.Fatal(e)
	}
	run, e = s.Step(t.Context(), actor, view.Document.ID, run.ID)
	if e != nil || run.Status != "WAITING_BUDGET" {
		t.Fatalf("expected paid response checkpoint: %s %v", run.Status, e)
	}
	raised := int64(1000)
	run, e = s.ControlRun(t.Context(), actor, view.Document.ID, run.ID, RunControl{Action: "budget", ExpectedVersion: run.Version, Epoch: run.Epoch, TokenBudget: &raised})
	if e != nil {
		t.Fatal(e)
	}
	run, e = s.Step(t.Context(), actor, view.Document.ID, run.ID)
	if e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 1 {
		t.Fatal("saved response sent again")
	}
	run, e = s.Step(t.Context(), actor, view.Document.ID, run.ID)
	if e != nil || run.Status != "COMPLETED" || calls.Load() != 2 || run.Usage.TotalTokens != 300 || run.Usage.UnknownAttempts != 0 {
		t.Fatalf("invalid resumed transcript: %+v calls=%d err=%v", run, calls.Load(), e)
	}
}
