package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/assistant"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAutomationDebugHTTPWithoutAssistantOrCredentials(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "debug_query.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	store, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, nil, artifact.NewService(db, store))
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PART", Name: "Read-only debug HTTP"})
	if err != nil {
		t.Fatal(err)
	}
	authorization := access.New(db)
	viewer := uuid.NewString()
	if _, err = db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name) VALUES($1,$2,'Debug viewer')`, viewer, viewer+"@test.invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err = authorization.UpsertGrant(t.Context(), "DOCUMENT", view.Document.ID, access.DefaultUserID, access.ShareRequest{SubjectType: "USER", SubjectID: viewer, Role: access.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	// Register only the read adapter. There is deliberately no Assistant,
	// credentials vault, scheduler or Provider in this HTTP server.
	server := &Server{automation: automation.New(db, domain, authorization)}
	mux := http.NewServeMux()
	server.registerAutomationRoutes(mux)
	input := assistant.QueryDebugInput{Scope: assistant.FrozenScope{DocumentID: view.Document.ID, RevisionID: view.Document.VersionID, WorkingBodyID: view.Part.ActiveBodyID, Complete: true}, Tool: "cad_query_model", Arguments: json.RawMessage(`{"kind":"BODY"}`)}
	post := func(actor string, input assistant.QueryDebugInput) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(input)
		r := httptest.NewRequest(http.MethodPost, "/api/documents/"+view.Document.ID+"/automation/debug-query", bytes.NewReader(raw))
		r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: actor}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, actor := range []string{access.DefaultUserID, viewer} {
		response := post(actor, input)
		var result assistant.QueryDebugResult
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.ToolResult.IsError || result.Supplement.Snapshot.RevisionID != input.Scope.RevisionID || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("readable exact snapshot failed", response.Code, response.Body.String())
		}
	}
	for actor, want := range map[string]int{"": http.StatusUnauthorized, uuid.NewString(): http.StatusForbidden} {
		if response := post(actor, input); response.Code != want {
			t.Fatal("debug bypassed ACL", actor, response.Code, response.Body.String())
		}
	}
	missing := input
	missing.Scope.RevisionID = uuid.NewString()
	if response := post(viewer, missing); response.Code != 404 {
		t.Fatal("missing historical Revision fell back to Head", response.Code, response.Body.String())
	}
	mutation := input
	mutation.Tool = "cad_create_task"
	if response := post(viewer, mutation); response.Code != 400 {
		t.Fatal("mutation tool accepted", response.Code)
	}
	after, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil || after.RevisionID != view.Document.VersionID {
		t.Fatal("debug changed the model", err)
	}
	for _, table := range []string{"assistant_runs", "assistant_model_calls", "automation_tasks"} {
		var count int
		if err = db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.`+table).Scan(&count); err != nil || count != 0 {
			t.Fatal("debug created execution state", table, count, err)
		}
	}
}
