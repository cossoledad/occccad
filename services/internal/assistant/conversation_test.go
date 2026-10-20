package assistant

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
)

func assistantFixture(t *testing.T, base string) (*Service, *workspace.Service, workspace.DocumentView, string) {
	t.Helper()
	db := testsupport.OpenDevelopmentDatabase(t)
	actor := uuid.NewString()
	if _, err := db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name) VALUES($1,$2,'Assistant fixture')`, actor, actor+"@test.invalid"); err != nil {
		t.Fatal(err)
	}
	worker, closeWorker, err := geometry.OpenDevelopmentWorker(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeWorker)
	store, staging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, worker, artifact.NewService(db, store, staging))
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: actor, Type: "PART", Name: "Assistant fixture " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, automation.New(db, domain, access.New(db)), Options{BaseURL: base, Model: "deepseek-flash", KeyFile: filepath.Join(t.TempDir(), "vault", "key")})
	t.Cleanup(s.Close)
	// The synthetic credential uses a test-owned local key file; remove its row
	// before the file disappears. Actual CAD resources use the shared .env store.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.DeleteKey(ctx, actor)
	})
	return s, domain, view, actor
}
func toolMessage(name string, arguments any) Message {
	call := ToolCall{ID: uuid.NewString(), Type: "function"}
	call.Function.Name = name
	call.Function.Arguments = string(marshalJSON(arguments))
	return Message{Role: "assistant", ToolCalls: []ToolCall{call}}
}
func writeCompletion(w http.ResponseWriter, message Message) {
	reason := "stop"
	if len(message.ToolCalls) > 0 {
		reason = "tool_calls"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message, "finish_reason": reason}}, "usage": Usage{100, 50, 150}})
}
func TestAssistantConversationPlansThenNativeModelingAndFollowup(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-assistant-key" {
			t.Error("credential not applied")
		}
		if r.URL.Path == "/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-flash"}]}`))
			return
		}
		var input struct {
			Messages []Message `json:"messages"`
			Tools    []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			t.Error("bad request")
		}
		for _, tool := range input.Tools {
			if tool.Function.Name == "cad_advance_task" {
				t.Error("planner received mutation executor")
			}
		}
		raw := string(marshalJSON(input.Messages))
		if strings.Contains(raw, "synthetic-assistant-key") || strings.Contains(raw, `"model_json"`) {
			t.Error("unbounded/private model input")
		}
		switch calls.Add(1) {
		case 1:
			message := Message{Role: "assistant"}
			for _, op := range []string{"sketch.create", "sketch.rectangle", "part.extrude"} {
				message.ToolCalls = append(message.ToolCalls, toolMessage("cad_describe_operation", map[string]string{"operationId": op}).ToolCalls...)
			}
			writeCompletion(w, message)
		case 2:
			var scope struct {
				Current automation.Context `json:"current"`
			}
			_ = json.Unmarshal([]byte(input.Messages[1].Content), &scope)
			tools := 0
			for _, m := range input.Messages {
				if m.Role == "tool" {
					tools++
					var op automation.Operation
					if json.Unmarshal([]byte(m.Content), &op) != nil || op.InputSchema == nil {
						t.Error("operation contract missing")
					}
				}
			}
			if tools != 3 {
				t.Error("tool call results not retained")
			}
			body := scope.Current.WorkingBodyID
			writeCompletion(w, toolMessage("cad_create_task", map[string]any{"steps": []any{
				map[string]any{"id": "sketch", "operationId": "sketch.create", "version": 1, "arguments": map[string]any{"bodyId": body, "plane": "XY"}},
				map[string]any{"id": "rectangle", "operationId": "sketch.rectangle", "version": 1, "arguments": map[string]any{"width": 80, "height": 50, "unit": "mm"}, "bindings": map[string]string{"sketchId": "sketch"}},
				map[string]any{"id": "pad", "operationId": "part.extrude", "version": 1, "arguments": map[string]any{"bodyId": body, "length": 12, "unit": "mm"}, "bindings": map[string]string{"sketchId": "sketch"}},
			}}))
		case 3:
			if !strings.Contains(raw, "DISPLAYED") || !strings.Contains(raw, "COMPLETED") {
				t.Error("followup lacks actual task checkpoint")
			}
			writeCompletion(w, toolMessage("cad_query_model", map[string]any{"kind": "PARAMETER", "fields": []string{"name", "value", "displayValue", "unit"}, "limit": 20}))
		case 4:
			if !strings.Contains(input.Messages[len(input.Messages)-1].Content, `"displayValue":12`) {
				t.Error("followup lacks real parameter value")
			}
			writeCompletion(w, Message{Role: "assistant", Content: "当前拉伸长度为 12 mm。"})
		default:
			t.Error("unexpected repeated provider call")
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	s, domain, view, actor := assistantFixture(t, server.URL)
	settings, err := s.SaveKey(t.Context(), actor, "synthetic-assistant-key")
	if err != nil || !settings.Configured {
		t.Fatal(settings, err)
	}
	var stored string
	if err = s.db.QueryRow(t.Context(), `SELECT ciphertext FROM occccad.assistant_credentials WHERE actor_id=$1`, actor).Scan(&stored); err != nil || strings.Contains(stored, "synthetic") {
		t.Fatal("plaintext credential", err)
	}
	info, err := os.Stat(s.options.KeyFile)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("key file permissions", err)
	}
	reopened := New(s.db, s.automation, s.options)
	defer reopened.Close()
	if key, e := reopened.credential(t.Context(), actor); e != nil || key != "synthetic-assistant-key" {
		t.Fatal("vault restart", e)
	}
	input := TurnInput{RequestID: uuid.NewString(), Text: "创建一个 80×50 mm 的矩形，拉伸 12 mm", WorkingBodyID: view.Part.ActiveBodyID}
	turn, err := s.Run(t.Context(), actor, view.Document.ID, input)
	if err != nil || turn.Status != "COMPLETED" || turn.TaskID == "" {
		t.Fatalf("plan: %+v %v", turn, err)
	}
	snapshot, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil || snapshot.RevisionID != view.Document.VersionID {
		t.Fatal("planning modified Head", err)
	}
	task, err := s.automation.GetTask(t.Context(), actor, view.Document.ID, turn.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Steps[1].Bindings["sketchId"] != task.Steps[0].ID {
		t.Fatal("local labels not bound to stable IDs")
	}
	replayed, e := s.Run(t.Context(), actor, view.Document.ID, input)
	if e != nil || replayed.ID != turn.ID || calls.Load() != 2 {
		t.Fatal("duplicate provider cost/plan", e)
	}
	for task.Status != "COMPLETED" {
		var pending *automation.Step
		for i := range task.Steps {
			if task.Steps[i].Phase != "DISPLAYED" {
				pending = &task.Steps[i]
				break
			}
		}
		if pending == nil {
			t.Fatal("missing pending step")
		}
		if pending.Phase == "COMMITTED" {
			// Backend tests acknowledge an exact receipt; the live browser test
			// separately proves that this acknowledgment follows a rendered frame.
			task, err = s.automation.Control(t.Context(), actor, view.Document.ID, task.ID, automation.Control{Action: "displayed", ExpectedVersion: task.Version, StepID: pending.ID, RevisionID: pending.Result.Receipt.RevisionID})
		} else {
			task, err = s.automation.Advance(t.Context(), actor, view.Document.ID, task.ID, task.Version)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	final, err := domain.GetDocument(t.Context(), view.Document.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	volume := 0.0
	for _, a := range final.Artifacts {
		volume += a.Volume
	}
	if math.Abs(volume-48000) > 1e-4 {
		t.Fatal("native volume", volume)
	}
	followup, err := s.Run(t.Context(), actor, view.Document.ID, TurnInput{RequestID: uuid.NewString(), Text: "刚才的拉伸长度是多少？"})
	if err != nil || !strings.Contains(followup.Reply, "12 mm") {
		t.Fatal("followup", followup, err)
	}
	other := uuid.NewString()
	_, _ = s.db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name) VALUES($1,$2,'Other fixture')`, other, other+"@test.invalid")
	if _, _, e := s.turn(t.Context(), other, view.Document.ID, turn.ID); e == nil {
		t.Fatal("conversation leaked to another actor")
	}
	if _, e := s.credential(t.Context(), other); e == nil {
		t.Fatal("credential leaked")
	}
	input.Text = "changed request"
	if _, e := s.Run(t.Context(), actor, view.Document.ID, input); e == nil {
		t.Fatal("idempotency key reused")
	}
}

type functionProvider struct {
	complete func(context.Context, []Message) (Completion, error)
}

func TestAssistantPlannerCannotExecuteAndMustReadContracts(t *testing.T) {
	s, domain, view, actor := assistantFixture(t, "http://127.0.0.1")
	var count int
	s.provider = functionProvider{complete: func(ctx context.Context, messages []Message) (Completion, error) {
		count++
		switch count {
		case 1:
			return Completion{Message: toolMessage("cad_advance_task", map[string]any{"taskId": uuid.NewString(), "expectedVersion": 1})}, nil
		case 2:
			if !strings.Contains(messages[len(messages)-1].Content, "TOOL_NOT_ALLOWED") {
				t.Error("executor tool wasn't rejected")
			}
			return Completion{Message: toolMessage("cad_create_task", map[string]any{"steps": []any{map[string]any{"id": "sketch", "operationId": "sketch.create", "version": 1, "arguments": map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"}}}})}, nil
		default:
			if !strings.Contains(messages[len(messages)-1].Content, "OPERATION_NOT_DESCRIBED") {
				t.Error("unchecked operation accepted")
			}
			return Completion{Message: Message{Role: "assistant", Content: "需要先读取操作合同。"}}, nil
		}
	}}
	if _, err := s.SaveKey(t.Context(), actor, "synthetic-restricted-key"); err != nil {
		t.Fatal(err)
	}
	turn, err := s.Run(t.Context(), actor, view.Document.ID, TurnInput{RequestID: uuid.NewString(), Text: "尝试未经校验的工具"})
	if err != nil || turn.TaskID != "" || count != 3 {
		t.Fatal(turn, err)
	}
	snapshot, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil || snapshot.RevisionID != view.Document.VersionID {
		t.Fatal("planner bypassed executor", err)
	}
}

func (p functionProvider) Validate(context.Context, string) error { return nil }
func (p functionProvider) Complete(ctx context.Context, _ string, m []Message, _ []Tool) (Completion, error) {
	return p.complete(ctx, m)
}

func TestAssistantCancellationBudgetAndExpiredRecovery(t *testing.T) {
	s, domain, view, actor := assistantFixture(t, "http://127.0.0.1")
	entered := make(chan struct{})
	s.provider = functionProvider{complete: func(ctx context.Context, _ []Message) (Completion, error) {
		close(entered)
		<-ctx.Done()
		return Completion{}, ctx.Err()
	}}
	if _, err := s.SaveKey(t.Context(), actor, "synthetic-cancel-key"); err != nil {
		t.Fatal(err)
	}
	done := make(chan Turn, 1)
	go func() {
		turn, err := s.Run(t.Context(), actor, view.Document.ID, TurnInput{RequestID: uuid.NewString(), Text: "创建一个矩形"})
		if err != nil {
			t.Error(err)
		}
		done <- turn
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("provider did not start")
	}
	history, err := s.Conversation(t.Context(), actor, view.Document.ID)
	if err != nil || len(history.Turns) != 1 {
		t.Fatal(history, err)
	}
	if _, err = s.Run(t.Context(), actor, view.Document.ID, TurnInput{RequestID: uuid.NewString(), Text: "并发请求"}); err == nil {
		t.Fatal("concurrent charged request allowed")
	}
	if _, err = s.Cancel(t.Context(), actor, view.Document.ID, history.Turns[0].ID); err != nil {
		t.Fatal(err)
	}
	select {
	case turn := <-done:
		if turn.Status != "CANCELLED" {
			t.Fatal(turn)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancel did not stop provider")
	}
	s.provider = functionProvider{complete: func(ctx context.Context, m []Message) (Completion, error) {
		return Completion{Message: toolMessage("cad_context", map[string]any{}), Usage: &Usage{TotalTokens: 40001}}, nil
	}}
	turn, err := s.Run(t.Context(), actor, view.Document.ID, TurnInput{RequestID: uuid.NewString(), Text: "预算测试"})
	if err != nil || turn.Status != "FAILED" || turn.Failure.Code != "AI_TOKEN_BUDGET" {
		t.Fatal(turn, err)
	}
	if _, err = s.db.Exec(t.Context(), `UPDATE occccad.assistant_turns SET status='THINKING',lease_until=0 WHERE id=$1`, turn.ID); err != nil {
		t.Fatal(err)
	}
	history, err = s.Conversation(t.Context(), actor, view.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, past := range history.Turns {
		if past.ID == turn.ID {
			found = past.Status == "INTERRUPTED"
		}
	}
	if !found {
		t.Fatal("expired run did not recover")
	}
	snapshot, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil || snapshot.RevisionID != view.Document.VersionID {
		t.Fatal("failed/cancelled planner changed model", err)
	}
}

func TestAssistantRecoversPlanCreatedBeforeConversationCheckpoint(t *testing.T) {
	s, domain, view, actor := assistantFixture(t, "http://127.0.0.1")
	s.provider = functionProvider{complete: func(context.Context, []Message) (Completion, error) {
		return Completion{Message: Message{Role: "assistant", Content: "请补充尺寸。"}}, nil
	}}
	if _, err := s.SaveKey(t.Context(), actor, "synthetic-recovery-key"); err != nil {
		t.Fatal(err)
	}
	turn, err := s.Run(t.Context(), actor, view.Document.ID, TurnInput{RequestID: uuid.NewString(), Text: "创建矩形"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.automation.CreateTask(t.Context(), actor, view.Document.ID, automation.CreateTask{RequestID: "assistant:" + turn.ID, Goal: turn.Text, ExpectedHead: snapshot.CommandBaseline, Steps: []automation.StepSpec{{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: marshalJSON(map[string]string{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})}}})
	if err != nil {
		t.Fatal(err)
	}
	// Model the crash window: task definition committed, but its ID was not
	// saved into the conversation, and the planner's lease has since expired.
	if _, err = s.db.Exec(t.Context(), `UPDATE occccad.assistant_turns SET status='THINKING',lease_until=0 WHERE id=$1`, turn.ID); err != nil {
		t.Fatal(err)
	}
	history, err := s.Conversation(t.Context(), actor, view.Document.ID)
	if err != nil || len(history.Turns) != 1 || history.Turns[0].TaskID != plan.ID || history.Turns[0].Status != "INTERRUPTED" {
		t.Fatal(history, err)
	}
	recovered, err := s.automation.GetTask(t.Context(), actor, view.Document.ID, plan.ID)
	if err != nil || recovered.Status != "STOPPED" {
		t.Fatal("unassociated plan wasn't stopped", recovered, err)
	}
	if pending, err := s.automation.HasPendingTask(t.Context(), actor, view.Document.ID); err != nil || pending {
		t.Fatal("orphan plan blocks later requests", err)
	}
	after, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil || after.RevisionID != snapshot.RevisionID {
		t.Fatal("recovery wrote geometry", err)
	}
}
