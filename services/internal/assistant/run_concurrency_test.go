package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

type concurrencyProvider struct {
	calls    atomic.Int32
	complete func(context.Context, int, []Message, []Tool) (Completion, error)
}

func (*concurrencyProvider) Validate(context.Context, string) error { return nil }
func (*concurrencyProvider) Models(context.Context, string) ([]Model, error) {
	return []Model{{ID: "deepseek-flash"}}, nil
}
func (p *concurrencyProvider) Complete(ctx context.Context, _ string, messages []Message, tools []Tool, _ ...Generation) (Completion, error) {
	return p.complete(ctx, int(p.calls.Add(1)), messages, tools)
}
func concurrencyMessage(name string, args any) Message {
	call := ToolCall{ID: uuid.NewString(), Type: "function"}
	call.Function.Name = name
	call.Function.Arguments = string(marshalJSON(args))
	return Message{Role: "assistant", ToolCalls: []ToolCall{call}}
}
func concurrencyCompletion(message Message) Completion {
	return Completion{Message: message, Usage: &Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}}
}
func concurrencyFixture(t *testing.T, p *concurrencyProvider) (*Service, *workspace.Service, workspace.DocumentView, string) {
	t.Helper()
	s, domain, view, actor := assistantFixture(t, "http://127.0.0.1:1")
	s.provider = p
	if _, err := s.SaveKey(t.Context(), actor, "synthetic-concurrency-key"); err != nil {
		t.Fatal(err)
	}
	return s, domain, view, actor
}

func TestAssistantRunLateProviderResponseCannotReviveSupersededGoal(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	var body string
	p := &concurrencyProvider{}
	p.complete = func(_ context.Context, number int, _ []Message, _ []Tool) (Completion, error) {
		if number == 1 {
			return concurrencyCompletion(concurrencyMessage("cad_describe_operation", map[string]any{"operationId": "sketch.create"})), nil
		}
		if number != 2 {
			t.Errorf("unexpected provider request %d", number)
			return Completion{}, errors.New("unexpected test request")
		}
		close(entered)
		// Deliberately ignore cancellation: a real provider may have processed
		// and billed a request even after our connection was interrupted.
		<-release
		message := concurrencyMessage("cad_create_task", map[string]any{"steps": []any{map[string]any{"id": "old-sketch", "operationId": "sketch.create", "version": 1, "arguments": map[string]any{"bodyId": body, "plane": "XY"}}}})
		message.ReasoningContent = "synthetic private late reasoning must stay out of persisted facts"
		return concurrencyCompletion(message), nil
	}
	s, domain, view, actor := concurrencyFixture(t, p)
	body = view.Part.ActiveBodyID
	r, err := s.CreateRun(t.Context(), actor, view.Document.ID, RunInput{RequestID: uuid.NewString(), Text: "在 XY 平面建立80×50 mm矩形，拉伸12 mm，再对全部边做R2圆角"})
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Step(t.Context(), actor, view.Document.ID, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		run Run
		err error
	}
	done := make(chan outcome, 1)
	go func() { next, e := s.Step(t.Context(), actor, view.Document.ID, r.ID); done <- outcome{next, e} }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("provider did not enter controlled request")
	}
	revised, turn, err := s.AddMessage(t.Context(), actor, view.Document.ID, r.ID, MessageInput{RequestID: uuid.NewString(), Text: "厚度改成20mm，后面不做圆角"})
	if err != nil || turn.Kind != "STEER" || revised.Epoch != r.Epoch+1 || revised.GoalVersion != 2 {
		t.Fatal("steering was not accepted while the request was in flight", revised, turn, err)
	}
	once.Do(func() { close(release) })
	select {
	case result := <-done:
		if result.err != nil && !errors.Is(result.err, context.Canceled) {
			t.Fatal(result.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("late provider request did not finish")
	}
	current, err := s.GetRun(t.Context(), actor, view.Document.ID, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Epoch != revised.Epoch || current.GoalVersion != 2 || current.ActiveTaskID != "" || current.StageCount != 0 || current.Reply != "" || current.Usage.ReportedCalls != 2 || current.Usage.TotalTokens != 300 {
		t.Fatal("late response changed current goal or its real synthetic usage disappeared", current)
	}
	has20, noFillet := false, false
	for _, requirement := range current.Requirements {
		if requirement.Kind == "EXTRUSION_LENGTH" && requirement.Length == 20 {
			has20 = true
		}
		if requirement.Kind == "NO_FILLET" {
			noFillet = true
		}
		if requirement.Kind == "ALL_EDGES_FILLET" || requirement.Kind == "SELECTED_EDGES_FILLET" {
			t.Fatal("old fillet requirement survived explicit cancellation")
		}
	}
	if !has20 || !noFillet {
		t.Fatal("revised user requirements were not preserved", current.Requirements)
	}
	actual, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil || actual.RevisionID != initial.RevisionID {
		t.Fatal("late old response performed CAD mutations", actual, err)
	}
	var stages, tasks, leaked int
	if err = s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.assistant_stages WHERE run_id=$1`, r.ID).Scan(&stages); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.automation_tasks WHERE payload->>'runId'=$1`, r.ID).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.assistant_model_calls WHERE run_id=$1 AND CAST(payload AS text) LIKE '%synthetic private late reasoning%'`, r.ID).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if stages != 0 || tasks != 0 || leaked != 0 {
		t.Fatal("late plan or private reasoning was persisted as executable facts", stages, tasks, leaked)
	}
}

func TestAssistantRunQuestionAnswersWhileWriterWaitsAndRejectsWrites(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	var questionCalls atomic.Int32
	p := &concurrencyProvider{}
	p.complete = func(_ context.Context, _ int, messages []Message, tools []Tool) (Completion, error) {
		user := ""
		for _, message := range messages {
			if message.Role == "user" {
				user = message.Content
			}
		}
		if user == "当前参数有哪些？" {
			for _, tool := range tools {
				if tool.Name == "cad_create_task" || tool.Name == "cad_advance_task" {
					t.Error("read-only question advertised a write executor")
				}
			}
			if questionCalls.Add(1) == 1 {
				message := Message{Role: "assistant"}
				for i := 0; i < 2; i++ {
					message.ToolCalls = append(message.ToolCalls, concurrencyMessage("cad_create_task", map[string]any{"steps": []any{}}).ToolCalls...)
				}
				message.ToolCalls = append(message.ToolCalls, concurrencyMessage("cad_query_model", map[string]any{"kind": "PARAMETER", "fields": []string{"name", "value", "unit"}, "limit": 10}).ToolCalls...)
				return concurrencyCompletion(message), nil
			}
			return concurrencyCompletion(Message{Role: "assistant", Content: "只读查询已完成，当前尚未建立拉伸特征。"}), nil
		}
		close(entered)
		<-release
		return concurrencyCompletion(Message{Role: "assistant", Content: "请补充后续建模要求。"}), nil
	}
	s, domain, view, actor := concurrencyFixture(t, p)
	r, err := s.CreateRun(t.Context(), actor, view.Document.ID, RunInput{RequestID: uuid.NewString(), Text: "创建80×50mm矩形并拉伸12mm"})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, e := s.Step(t.Context(), actor, view.Document.ID, r.ID); done <- e }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("writer did not enter provider request")
	}
	current, question, err := s.AddMessage(t.Context(), actor, view.Document.ID, r.ID, MessageInput{RequestID: uuid.NewString(), Text: "当前参数有哪些？"})
	if err != nil || question.Kind != "QUERY" || current.Epoch != r.Epoch {
		t.Fatal("read-only question revised the active writer", current, question, err)
	}
	if err = s.stepQuestion(t.Context(), question.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.stepQuestion(t.Context(), question.ID); err != nil {
		t.Fatal(err)
	}
	answered, _, err := loadInput(t.Context(), s.db, question.ID)
	if err != nil || answered.Status != "ANSWERED" || !strings.Contains(answered.Reply, "只读查询") {
		t.Fatal("question waited behind the writer", answered, err)
	}
	current, err = s.GetRun(t.Context(), actor, view.Document.ID, r.ID)
	if err != nil || current.Epoch != r.Epoch || current.GoalVersion != r.GoalVersion || current.Status == "BLOCKED" || current.Status == "COMPLETED" || current.ActiveTaskID != "" || current.PendingCallID == "" {
		t.Fatal("question tool failures changed the writer lifecycle", current, err)
	}
	var rejected int
	if err = s.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.assistant_tool_results t JOIN occccad.assistant_model_calls c ON c.id=t.call_id WHERE c.input_id=$1 AND t.name='cad_create_task' AND CAST(t.payload AS text) LIKE '%READ_ONLY%'`, question.ID).Scan(&rejected); err != nil || rejected != 2 {
		t.Fatal("server did not reject every attempted write from the read-only question", rejected, err)
	}
	actual, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil || actual.RevisionID != initial.RevisionID {
		t.Fatal("question mutated CAD", actual, err)
	}
	once.Do(func() { close(release) })
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("writer did not finish")
	}
}

func TestAssistantRunIdenticalFailedIntentBlocksOnFactsNotElapsedCalls(t *testing.T) {
	p := &concurrencyProvider{complete: func(context.Context, int, []Message, []Tool) (Completion, error) {
		return concurrencyCompletion(concurrencyMessage("cad_create_task", map[string]any{"steps": []any{}})), nil
	}}
	s, _, view, actor := concurrencyFixture(t, p)
	r, err := s.CreateRun(t.Context(), actor, view.Document.ID, RunInput{RequestID: uuid.NewString(), Text: "创建一个零件"})
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Step(t.Context(), actor, view.Document.ID, r.ID)
	if err != nil || r.Status == "BLOCKED" {
		t.Fatal(r, err)
	}
	r, err = s.Step(t.Context(), actor, view.Document.ID, r.ID)
	if err != nil || r.Status != "BLOCKED" || r.StageCount != 0 || r.AppliedCount != 0 || r.LastFailureDigest == "" || !strings.Contains(r.WaitingReason, "同一事实") {
		t.Fatal("same invalid intent repeated without factual progress", r, err)
	}
}

func TestAssistantRunProviderRateLimitBackoffUsesClockAndPreservesUnknownAttempt(t *testing.T) {
	p := &concurrencyProvider{}
	p.complete = func(_ context.Context, call int, _ []Message, _ []Tool) (Completion, error) {
		if call == 1 {
			return Completion{}, &automation.Fault{Code: "AI_RATE_LIMIT", Message: "synthetic provider 429", Retryable: true}
		}
		return concurrencyCompletion(Message{Role: "assistant", Content: "需要指定目标形状。"}), nil
	}
	s, _, view, actor := concurrencyFixture(t, p)
	clock := time.Now()
	s.options.Now = func() time.Time { return clock }
	r, err := s.CreateRun(t.Context(), actor, view.Document.ID, RunInput{RequestID: uuid.NewString(), Text: "创建一个零件"})
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Step(t.Context(), actor, view.Document.ID, r.ID)
	if err != nil || r.Status != "WAITING_RETRY" || p.calls.Load() != 1 || r.Usage.UnknownAttempts != 1 {
		t.Fatal("transient provider limit was not recoverable/unknown", r, err)
	}
	var wake int64
	if err = s.db.QueryRow(t.Context(), `SELECT next_wake FROM occccad.assistant_runs WHERE id=$1`, r.ID).Scan(&wake); err != nil || wake <= clock.UnixMilli() {
		t.Fatal("retry has no time-based backoff", wake, err)
	}
	_, _ = s.Step(t.Context(), actor, view.Document.ID, r.ID)
	if p.calls.Load() != 1 {
		t.Fatal("provider was called before backoff elapsed")
	}
	clock = time.UnixMilli(wake).Add(time.Millisecond)
	r, err = s.Step(t.Context(), actor, view.Document.ID, r.ID)
	if err != nil || p.calls.Load() != 2 || r.ModelCalls != 2 || r.Usage.UnknownAttempts != 1 || r.Usage.ReportedCalls != 1 || r.Usage.TotalTokens != 150 {
		t.Fatal("retry did not continue the same Run or usage was fabricated", r, err)
	}
	rows, err := s.db.Query(t.Context(), `SELECT payload FROM occccad.assistant_model_calls WHERE run_id=$1 ORDER BY started_at,id`, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	unknown, reported := 0, 0
	for rows.Next() {
		var raw []byte
		var call CallRecord
		if err = rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &call); err != nil {
			t.Fatal(err)
		}
		if call.UsageState == "UNKNOWN" {
			unknown++
		}
		if call.UsageState == "REPORTED" {
			reported++
		}
	}
	if unknown != 1 || reported != 1 {
		t.Fatal("different actual provider attempts were merged", unknown, reported)
	}
}
