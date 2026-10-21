package assistant

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
	"net/http"
	"path/filepath"
	"testing"
	"time"
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
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message, "finish_reason": reason}}, "usage": Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}})
}
