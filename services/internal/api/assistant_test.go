package api

import (
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

func TestAssistantHTTPKeyAndConversationAuthorization(t *testing.T) {
	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-http-key" {
			t.Error("wrong provider credential")
		}
		if r.URL.Path == "/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-flash"}]}`))
			return
		}
		providerCalls.Add(1)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"请告诉我矩形的宽度、高度和拉伸长度。"},"finish_reason":"stop"}]}`))
	}))
	defer provider.Close()
	db := testsupport.OpenDevelopmentDatabase(t)
	authentication := authn.New(db, time.Hour)
	email := uuid.NewString() + "@test.invalid"
	_, err := authentication.Register(t.Context(), authn.RegisterRequest{Email: email, DisplayName: "Assistant HTTP fixture", Password: "Fixture-Password-1234"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(t.Context(), `UPDATE occccad.users SET status='ACTIVE' WHERE email=$1`, email); err != nil {
		t.Fatal(err)
	}
	session, err := authentication.Login(t.Context(), email, "Fixture-Password-1234", "assistant-test", "local")
	if err != nil {
		t.Fatal(err)
	}
	store, staging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, nil, artifact.NewService(db, store, staging))
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: session.User.ID, Type: "PART", Name: "Assistant HTTP"})
	if err != nil {
		t.Fatal(err)
	}
	server := New(db, nil, domain, access.New(db), authentication, nil, jobs.New(db), false, nil)
	defer server.Close()
	server.assistant = assistant.New(db, server.automation, assistant.Options{BaseURL: provider.URL, Model: "deepseek-flash", KeyFile: filepath.Join(t.TempDir(), "assistant.key")})
	defer server.assistant.DeleteKey(t.Context(), session.User.ID)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	call := func(method, path, body string, csrf bool) (int, []byte) {
		t.Helper()
		r, _ := http.NewRequest(method, httpServer.URL+path, strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.Token})
		r.Header.Set("Content-Type", "application/json")
		if csrf {
			r.Header.Set("X-CSRF-Token", session.CSRFToken)
			r.AddCookie(&http.Cookie{Name: csrfCookieName, Value: session.CSRFToken})
		}
		response, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		return response.StatusCode, raw
	}
	if status, _ := call("PUT", "/api/assistant/settings", `{"apiKey":"synthetic-http-key"}`, false); status != 403 {
		t.Fatal("key write missing CSRF gate", status)
	}
	if status, raw := call("PUT", "/api/assistant/settings", `{"apiKey":"synthetic-http-key"}`, true); status != 200 || strings.Contains(string(raw), "synthetic") {
		t.Fatal("save key", status, string(raw))
	}
	if status, raw := call("GET", "/api/assistant/settings", "", false); status != 200 || !strings.Contains(string(raw), `"configured":true`) || strings.Contains(string(raw), "synthetic") {
		t.Fatal("key echo/settings", status, string(raw))
	}
	path := "/api/documents/" + view.Document.ID + "/assistant/turns"
	input := assistant.TurnInput{RequestID: uuid.NewString(), Text: "帮我建一个矩形零件"}
	raw, _ := json.Marshal(input)
	status, reply := call("POST", path, string(raw), true)
	var turn assistant.Turn
	if status != 200 || json.Unmarshal(reply, &turn) != nil || turn.Status != "COMPLETED" || turn.TaskID != "" {
		t.Fatal("clarification", status, string(reply))
	}
	if status, _ = call("POST", path, string(raw), true); status != 200 || providerCalls.Load() != 1 {
		t.Fatal("duplicate HTTP charged provider again", status, providerCalls.Load())
	}
	if status, reply = call("GET", path, "", false); status != 200 || !strings.Contains(string(reply), "矩形") {
		t.Fatal("restore conversation", status, string(reply))
	}
	input.RequestID = uuid.NewString()
	input.WorkingBodyID = uuid.NewString()
	raw, _ = json.Marshal(input)
	if status, _ = call("POST", path, string(raw), true); status != 400 || providerCalls.Load() != 1 {
		t.Fatal("forged Body reached model", status, providerCalls.Load())
	}
	if status, _ = call("DELETE", "/api/assistant/settings", "", true); status != 200 {
		t.Fatal("remove key", status)
	}
	input.WorkingBodyID = ""
	raw, _ = json.Marshal(input)
	if status, reply = call("POST", path, string(raw), true); status != 400 || !strings.Contains(string(reply), "AI_KEY_REQUIRED") {
		t.Fatal("removed key still usable", status, string(reply))
	}
}
