package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/assistant"
	"github.com/occccad/occccad/internal/authn"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
)

// The provider is controlled; HTTP/auth/CSRF, account vault, migration, durable
// scheduler, usage ledger and workspace reads use their production paths.
func TestAssistantHTTPDurableRunsAndAccountSettings(t *testing.T) {
	var calls atomic.Int32
	var block, modelsUnavailable, writeProbe atomic.Bool
	entered, release := make(chan struct{}, 1), make(chan struct{})
	defer close(release)
	var writerBlock atomic.Bool
	writerEntered, writerRelease := make(chan struct{}, 1), make(chan struct{})
	defer close(writerRelease)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		credential := r.Header.Get("Authorization")
		if credential != "Bearer synthetic-http-key" && credential != "Bearer synthetic-viewer-key" {
			t.Error("unexpected provider credential")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/models" {
			if modelsUnavailable.Load() {
				w.WriteHeader(503)
				return
			}
			_, _ = io.WriteString(w, `{"data":[{"id":"deepseek-flash","effort":{"supported_levels":["low","high"]}},{"id":"deepseek-v4-pro","effort":{"supported_levels":["low","high","max"]}}]}`)
			return
		}
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(404)
			return
		}
		calls.Add(1)
		if block.Load() {
			select {
			case entered <- struct{}{}:
			default:
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		var input struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if writerBlock.Load() {
			text := ""
			for _, message := range input.Messages {
				if message.Role == "user" {
					text = message.Content
				}
			}
			if strings.Contains(text, "改成") {
				select {
				case writerEntered <- struct{}{}:
				default:
				}
				select {
				case <-writerRelease:
				case <-r.Context().Done():
					return
				}
			}
		}
		if credential == "Bearer synthetic-viewer-key" {
			for _, tool := range input.Tools {
				if tool.Function.Name == "cad_create_task" {
					t.Error("write tool was offered to a read-only query")
				}
			}
			if writeProbe.Swap(false) {
				_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"forbidden-write","type":"function","function":{"name":"cad_create_task","arguments":"{\"steps\":[{\"id\":\"bad\",\"operationId\":\"sketch.create\",\"version\":1,\"arguments\":{\"plane\":\"XY\"}}]}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":123,"completion_tokens":7,"total_tokens":130}}`)
				return
			}
		}
		_, _ = io.WriteString(w, `{"id":"controlled-response","choices":[{"message":{"role":"assistant","content":"当前模型参数可以查询；本次没有修改模型。","reasoning_content":"private-api-test-reasoning"},"finish_reason":"stop"}],"usage":{"prompt_tokens":123,"completion_tokens":7,"total_tokens":130,"prompt_cache_hit_tokens":20,"completion_tokens_details":{"reasoning_tokens":5}}}`)
	}))
	defer provider.Close()

	db := testsupport.OpenDevelopmentDatabase(t)
	authentication := authn.New(db, time.Hour)
	register := func(name string) authn.Session {
		t.Helper()
		email := uuid.NewString() + "@test.invalid"
		if _, err := authentication.Register(t.Context(), authn.RegisterRequest{Email: email, DisplayName: name, Password: "Fixture-Password-1234"}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(t.Context(), `UPDATE occccad.users SET status='ACTIVE' WHERE email=$1`, email); err != nil {
			t.Fatal(err)
		}
		session, err := authentication.Login(t.Context(), email, "Fixture-Password-1234", "assistant-http-test", "local")
		if err != nil {
			t.Fatal(err)
		}
		return session
	}
	owner, viewer := register("Assistant HTTP owner"), register("Assistant HTTP viewer")
	store, staging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, nil, artifact.NewService(db, store, staging))
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: owner.User.ID, Type: "PART", Name: "Assistant durable HTTP " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	authorization := access.New(db)
	if _, err = authorization.UpsertGrant(t.Context(), "DOCUMENT", view.Document.ID, owner.User.ID, access.ShareRequest{SubjectType: "USER", SubjectID: viewer.User.ID, Role: access.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	server := New(db, nil, domain, authorization, authentication, nil, jobs.New(db), false, nil)
	server.assistant = assistant.New(db, server.automation, assistant.Options{BaseURL: provider.URL, Model: "deepseek-flash", KeyFile: filepath.Join(t.TempDir(), "assistant.key")})
	defer func() {
		server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.assistant.DeleteKey(ctx, owner.User.ID)
		_ = server.assistant.DeleteKey(ctx, viewer.User.ID)
	}()
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	call := func(session *authn.Session, method, path string, body any, csrf bool) (int, []byte) {
		t.Helper()
		raw := []byte{}
		if body != nil {
			raw, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		request, e := http.NewRequestWithContext(t.Context(), method, httpServer.URL+path, strings.NewReader(string(raw)))
		if e != nil {
			t.Fatal(e)
		}
		request.Header.Set("Content-Type", "application/json")
		if session != nil {
			request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
			if csrf {
				request.Header.Set("X-CSRF-Token", session.CSRFToken)
				request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: session.CSRFToken})
			}
		}
		response, e := client.Do(request)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, e = io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(raw), "synthetic-http-key") || strings.Contains(string(raw), "synthetic-viewer-key") || strings.Contains(string(raw), "private-api-test-reasoning") {
			t.Fatal("HTTP response leaked credentials or private reasoning")
		}
		return response.StatusCode, raw
	}
	settings := func(session *authn.Session) assistant.Settings {
		t.Helper()
		status, raw := call(session, "GET", "/api/assistant/settings", nil, false)
		var v assistant.Settings
		if status != 200 || json.Unmarshal(raw, &v) != nil {
			t.Fatal("settings", status, string(raw))
		}
		return v
	}
	mustRun := func(status int, raw []byte, want int) assistant.Run {
		t.Helper()
		var r assistant.Run
		if status != want || json.Unmarshal(raw, &r) != nil || r.ID == "" {
			t.Fatal("run response", status, string(raw))
		}
		return r
	}
	path := "/api/documents/" + view.Document.ID + "/assistant/runs"
	readRun := func(session *authn.Session, id string) assistant.Run {
		t.Helper()
		status, raw := call(session, "GET", path+"/"+id, nil, false)
		return mustRun(status, raw, 200)
	}
	awaitStatus := func(session *authn.Session, id, want string) assistant.Run {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			r := readRun(session, id)
			if r.Status == want {
				return r
			}
			if time.Now().After(deadline) {
				t.Fatalf("waiting for %s: status=%s failure=%v reason=%s", want, r.Status, r.Failure, r.WaitingReason)
			}
			time.Sleep(15 * time.Millisecond)
		}
	}

	if status, _ := call(nil, "GET", "/api/assistant/settings", nil, false); status != 401 {
		t.Fatal("unauthenticated settings", status)
	}
	if settings(&owner).Configured || settings(&viewer).Configured {
		t.Fatal("new account inherited another account credential")
	}
	if status, _ := call(&owner, "PUT", "/api/assistant/settings", map[string]any{"apiKey": "synthetic-http-key"}, false); status != 403 {
		t.Fatal("key write missing CSRF", status)
	}
	if status, raw := call(&owner, "PUT", "/api/assistant/settings", map[string]any{"apiKey": "synthetic-http-key"}, true); status != 200 {
		t.Fatal("save key", status, string(raw))
	}
	initialSettings := settings(&owner)
	if !initialSettings.Configured || initialSettings.TokenBudget != nil || initialSettings.Version < 1 {
		t.Fatal("default must be an explicit nullable cumulative budget", initialSettings)
	}
	if settings(&viewer).Configured {
		t.Fatal("credential escaped account isolation")
	}
	if status, raw := call(&owner, "GET", "/api/assistant/models", nil, false); status != 200 || !strings.Contains(string(raw), "deepseek-v4-pro") {
		t.Fatal("model discovery", status, string(raw))
	}
	preferences := map[string]any{"model": "deepseek-v4-pro", "thinking": "high"}
	if status, _ := call(&owner, "PUT", "/api/assistant/settings", preferences, false); status != 403 {
		t.Fatal("preferences missing CSRF", status)
	}
	if status, raw := call(&owner, "PUT", "/api/assistant/settings", preferences, true); status != 200 {
		t.Fatal("preferences", status, string(raw))
	}
	for _, bad := range []map[string]any{{"model": "invented"}, {"thinking": "medium"}, {"tokenBudget": 0}, {"tokenBudget": -1}, {"tokenBudget": 1.5}, {"tokenBudget": "unlimited"}, {"budget": "high"}} {
		if status, _ := call(&owner, "PUT", "/api/assistant/settings", bad, true); status != 400 {
			t.Fatal("invalid/retired preference accepted", bad, status)
		}
	}
	modelsUnavailable.Store(true)
	if status, raw := call(&owner, "PUT", "/api/assistant/settings", map[string]any{"tokenBudget": 2500}, true); status != 200 {
		t.Fatal("local budget depended on model discovery", status, string(raw))
	}
	afterBudget := settings(&owner)
	if afterBudget.TokenBudget == nil || *afterBudget.TokenBudget != 2500 || afterBudget.Model != "deepseek-v4-pro" || afterBudget.Thinking != "high" || afterBudget.Version <= initialSettings.Version {
		t.Fatal("preference patch lost other settings", afterBudget)
	}
	if status, _ := call(&owner, "PUT", "/api/assistant/settings", map[string]any{"tokenBudget": nil}, true); status != 200 {
		t.Fatal("clear optional budget", status)
	}
	modelsUnavailable.Store(false)
	if settings(&owner).TokenBudget != nil {
		t.Fatal("cleared budget retained a hidden cap")
	}

	input := assistant.RunInput{RequestID: uuid.NewString(), Text: "当前参数有哪些？"}
	if status, _ := call(&owner, "POST", path, input, false); status != 403 {
		t.Fatal("Run creation missing CSRF", status)
	}
	block.Store(true)
	status, raw := call(&owner, "POST", path, input, true)
	run := mustRun(status, raw, http.StatusAccepted)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("background scheduler did not call the provider")
	}
	if run.Status != "QUEUED" || run.TokenBudget != nil {
		t.Fatal("submission waited for provider or introduced a cap", run.Status)
	}
	status, raw = call(&owner, "POST", path, input, true)
	replay := mustRun(status, raw, http.StatusAccepted)
	if replay.ID != run.ID || calls.Load() != 1 {
		t.Fatal("replayed creation called provider again", calls.Load())
	}
	changed := input
	changed.Text = "其他内容"
	if status, _ = call(&owner, "POST", path, changed, true); status < 400 {
		t.Fatal("idempotency identity accepted a changed goal")
	}
	block.Store(false)
	release <- struct{}{}
	complete := awaitStatus(&owner, run.ID, "COMPLETED")
	if complete.Usage.TotalTokens != 130 || complete.ModelCalls != 1 || complete.PresentedRevisionID != "" || complete.AppliedCount != 0 {
		t.Fatal("durable usage/read/presentation separation", complete.Usage, complete.ModelCalls, complete.PresentedRevisionID)
	}
	status, raw = call(&owner, "POST", path, input, true)
	if mustRun(status, raw, 202).ID != run.ID || calls.Load() != 1 {
		t.Fatal("terminal replay changed Run or charged again")
	}
	if status, raw = call(&viewer, "GET", path+"/"+run.ID, nil, false); status < 400 || !strings.Contains(string(raw), "AI_RUN_NOT_FOUND") {
		t.Fatal("another actor read a private Run", status, string(raw))
	}
	if status, _ = call(&owner, "POST", path+"/"+run.ID+"/control", map[string]any{"action": "pause", "expectedVersion": complete.Version, "epoch": complete.Epoch}, false); status != 403 {
		t.Fatal("control missing CSRF", status)
	}
	if status, raw = call(&owner, "POST", path+"/"+run.ID+"/control", map[string]any{"action": "confirm", "expectedVersion": complete.Version, "epoch": complete.Epoch - 1, "taskId": uuid.NewString(), "stepId": uuid.NewString(), "previewId": uuid.NewString()}, true); status < 400 || !strings.Contains(string(raw), "AI_EPOCH_CONFLICT") {
		t.Fatal("old page confirmation accepted", status, string(raw))
	}
	if status, raw = call(&owner, "GET", path+"/"+run.ID+"/calls", nil, false); status != 200 || !strings.Contains(string(raw), "deepseek-v4-pro") || !strings.Contains(string(raw), `"total_tokens":130`) {
		t.Fatal("durable model-call ledger", status, string(raw))
	}
	for _, format := range []string{"json", "markdown"} {
		for _, suffix := range []string{"", "&turnId=" + complete.CurrentTurnID} {
			status, raw = call(&owner, "GET", path+"/"+run.ID+"/export?format="+format+suffix, nil, false)
			if status != 200 || !strings.Contains(string(raw), "occccad.assistant-export.v1") || !strings.Contains(string(raw), "MODEL_REQUEST_CAPTURED") || !strings.Contains(string(raw), "当前参数有哪些") || !strings.Contains(string(raw), "recordCount") {
				t.Fatal("complete HTTP export", status, string(raw))
			}
		}
	}
	if status, _ = call(&viewer, "GET", path+"/"+run.ID+"/export?format=json", nil, false); status < 400 {
		t.Fatal("export shared a private Run")
	}
	if status, _ = call(&owner, "GET", path+"/"+run.ID+"/export?format=csv", nil, false); status != 400 {
		t.Fatal("invalid export format")
	}
	message := assistant.MessageInput{RequestID: uuid.NewString(), Text: "当前任务状态是什么？"}
	if status, _ = call(&owner, "POST", path+"/"+run.ID+"/messages", message, false); status != 403 {
		t.Fatal("message missing CSRF", status)
	}
	if status, raw = call(&owner, "POST", path+"/"+run.ID+"/messages", message, true); status != 202 {
		t.Fatal("completed Run cannot answer a new question", status, string(raw))
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		status, raw = call(&owner, "GET", path+"/"+run.ID+"/messages?limit=1", nil, false)
		var page assistant.Page[assistant.Turn]
		if status != 200 || json.Unmarshal(raw, &page) != nil {
			t.Fatal("message page", status, string(raw))
		}
		if len(page.Items) == 1 && page.Items[0].Status == "ANSWERED" {
			if !page.HasEarlier {
				t.Fatal("history page lost earlier messages")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("query did not complete", string(raw))
		}
		time.Sleep(15 * time.Millisecond)
	}
	beforeReplay := calls.Load()
	if status, _ = call(&owner, "POST", path+"/"+run.ID+"/messages", message, true); status != 202 || calls.Load() != beforeReplay {
		t.Fatal("message transport replay charged another call")
	}
	if status, raw = call(&owner, "GET", path+"/"+run.ID+"/events?limit=1", nil, false); status != 200 || !strings.Contains(string(raw), `"hasEarlier":true`) {
		t.Fatal("event pagination", status, string(raw))
	}
	if status, _ = call(&owner, "POST", path, assistant.RunInput{RequestID: uuid.NewString(), Text: "当前参数有哪些？", WorkingBodyID: uuid.NewString()}, true); status != 400 {
		t.Fatal("forged Body accepted", status)
	}

	// A budget is opt-in and pausing preserves the received response/call ledger.
	limit := int64(100)
	budgetInput := assistant.RunInput{RequestID: uuid.NewString(), Text: "当前参数有哪些？", TokenBudget: &limit}
	status, raw = call(&owner, "POST", path, budgetInput, true)
	limited := mustRun(status, raw, 202)
	limited = awaitStatus(&owner, limited.ID, "WAITING_BUDGET")
	budgetCalls := calls.Load()
	if status, raw = call(&owner, "POST", path+"/"+limited.ID+"/control", map[string]any{"action": "budget", "expectedVersion": limited.Version, "epoch": limited.Epoch, "tokenBudget": 1000}, true); status != 200 {
		t.Fatal("raise active Run budget", status, string(raw))
	}
	resumed := awaitStatus(&owner, limited.ID, "COMPLETED")
	if resumed.ID != limited.ID || resumed.Usage.TotalTokens != 130 || calls.Load() != budgetCalls {
		t.Fatal("budget adjustment replayed a paid response", resumed.ID, resumed.Usage, calls.Load(), budgetCalls)
	}
	if settings(&owner).TokenBudget != nil {
		t.Fatal("active Run adjustment changed the account default")
	}

	if status, raw = call(&viewer, "PUT", "/api/assistant/settings", map[string]any{"apiKey": "synthetic-viewer-key"}, true); status != 200 {
		t.Fatal("viewer own credential", status, string(raw))
	}
	writeProbe.Store(true)
	status, raw = call(&viewer, "POST", path, assistant.RunInput{RequestID: uuid.NewString(), Text: "当前参数有哪些？"}, true)
	readOnly := mustRun(status, raw, 202)
	readOnly = awaitStatus(&viewer, readOnly.ID, "COMPLETED")
	if readOnly.AppliedCount != 0 || readOnly.LatestAppliedRevisionID != "" {
		t.Fatal("read-only provider bypassed write boundary")
	}
	snapshot, err := domain.GetDocument(t.Context(), view.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Document.VersionID != view.Document.VersionID {
		t.Fatal("read-only query changed the model revision")
	}

	// Ordinary messages, including questions, are interpreted by the model.
	writerBlock.Store(true)
	status, raw = call(&owner, "POST", path, assistant.RunInput{RequestID: uuid.NewString(), Text: "厚度改成20mm"}, true)
	writer := mustRun(status, raw, 202)
	select {
	case <-writerEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not reach the controlled provider boundary")
	}
	var accepted struct {
		Run  assistant.Run  `json:"run"`
		Turn assistant.Turn `json:"turn"`
	}
	status, raw = call(&owner, "POST", path+"/"+writer.ID+"/messages", assistant.MessageInput{RequestID: uuid.NewString(), Text: "当前圆角半径是多少？"}, true)
	if status != 202 || json.Unmarshal(raw, &accepted) != nil || accepted.Turn.Kind != "STEER" || accepted.Run.GoalVersion != writer.GoalVersion+1 || accepted.Run.Epoch <= writer.Epoch {
		t.Fatal("keywords still classified a writable message", status, string(raw))
	}
	status, raw = call(&owner, "POST", path+"/"+writer.ID+"/messages", assistant.MessageInput{RequestID: uuid.NewString(), Text: "可以把厚度改成20mm吗"}, true)
	if status != 202 || json.Unmarshal(raw, &accepted) != nil || accepted.Turn.Kind != "STEER" || accepted.Run.GoalVersion != writer.GoalVersion+2 || accepted.Run.Epoch <= writer.Epoch {
		t.Fatal("a polite change request was treated as read-only", status, string(raw))
	}
	if status, raw = call(&owner, "POST", path+"/"+writer.ID+"/control", map[string]any{"action": "confirm", "expectedVersion": accepted.Run.Version, "epoch": writer.Epoch, "taskId": uuid.NewString(), "stepId": uuid.NewString(), "previewId": uuid.NewString()}, true); status < 400 || !strings.Contains(string(raw), "AI_EPOCH_CONFLICT") {
		t.Fatal("a pre-revision confirmation crossed the new goal boundary", status, string(raw))
	}
	for attempt := 0; attempt < 3; attempt++ {
		latest := readRun(&owner, writer.ID)
		status, raw = call(&owner, "POST", path+"/"+writer.ID+"/control", map[string]any{"action": "stop", "expectedVersion": latest.Version, "epoch": latest.Epoch}, true)
		if status == 200 {
			break
		}
		if !strings.Contains(string(raw), "AI_RUN_CONFLICT") || attempt == 2 {
			t.Fatal("stop controlled writer", status, string(raw))
		}
		time.Sleep(15 * time.Millisecond)
	}
	writerBlock.Store(false)
	if status, raw = call(&owner, "DELETE", "/api/assistant/settings", nil, true); status != 200 {
		t.Fatal("delete credential", status, string(raw))
	}
	if settings(&owner).Configured || !settings(&viewer).Configured {
		t.Fatal("key deletion affected another account")
	}
	if status, raw = call(&owner, "POST", path, assistant.RunInput{RequestID: uuid.NewString(), Text: "当前参数有哪些？"}, true); status != 400 || !strings.Contains(string(raw), "AI_KEY_REQUIRED") {
		t.Fatal("removed key still starts new model work", status, string(raw))
	}
}
