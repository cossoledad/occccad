package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/debugartifact"
	"github.com/occccad/occccad/internal/workspace"
)

func TestOperationDiagnosticDownloadPermissionsAndSnapshot(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "diagnostic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	local, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, nil, artifact.NewService(db, local))
	store, e := debugartifact.NewStore(t.TempDir(), 50, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	domain.SetDiagnosticArtifactStore(store)
	v, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PART", Name: "Diagnostic permissions"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = domain.PreviewCommand(t.Context(), v.Document.ID, workspace.CommandRequest{ActorID: access.DefaultUserID, RequestID: "bad-input", Type: "CREATE_PARAMETER", Name: "bad", Value: 1, Unit: "unknown-unit"})
	if err == nil {
		t.Fatal("expected input failure")
	}
	failure := realtimeDomainError(err)
	if failure.DiagnosticID == "" {
		t.Fatal("diagnostic not transported", err)
	}
	var marked interface{ DiagnosticID() string }
	if !errors.As(err, &marked) {
		t.Fatal(err)
	}
	id := strings.Split(failure.DiagnosticID, "/")[1]
	server := &Server{database: db, workspace: domain, access: access.New(db)}
	get := func(user, doc, diag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/diagnostic", nil)
		r.SetPathValue("documentID", doc)
		r.SetPathValue("diagnosticID", diag)
		r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: user}))
		w := httptest.NewRecorder()
		server.downloadOperationDiagnostic(w, r)
		return w
	}
	got := get(access.DefaultUserID, v.Document.ID, id)
	if got.Code != 200 || !strings.Contains(got.Body.String(), v.Document.VersionID) || got.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(got.Code, got.Body.String())
	}
	if got = get("00000000-0000-7000-8000-000000000099", v.Document.ID, id); got.Code != 403 {
		t.Fatal("unauthorized diagnostic", got.Code)
	}
	other, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PART", Name: "Unrelated"})
	if err != nil {
		t.Fatal(err)
	}
	if got = get(access.DefaultUserID, other.Document.ID, id); got.Code != 404 {
		t.Fatal("cross document evidence", got.Code)
	}
}
