package api

import (
	"encoding/json"
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

func TestRealtimeAssemblyFailureRetainsDiagnostic(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "interaction.db"))
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
	store, err := debugartifact.NewStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	domain.SetDiagnosticArtifactStore(store)
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PRODUCT", Name: "interaction diagnostic"})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{workspace: domain, access: access.New(db)}
	client := &realtimeClient{actor: access.User{ID: access.DefaultUserID}, hub: newRealtimeHub(), send: make(chan []byte, 1), done: make(chan struct{})}
	envelope := newRealtimeEnvelope("request", "assembly.interaction.begin.v1", "", nil, map[string]string{"documentId": view.Document.ID, "instanceId": "missing", "baseRevisionId": view.Document.VersionID}, nil)
	client.startAssemblyOperation(server, t.Context(), envelope)
	select {
	case data := <-client.send:
		var response realtimeEnvelope
		if err = json.Unmarshal(data, &response); err != nil {
			t.Fatal(err)
		}
		if response.CorrelationID != envelope.ID || response.Error == nil || response.Error.Code != "VALIDATION_FAILED" || response.Error.DiagnosticID == "" || response.Error.Phase == "" {
			t.Fatalf("interaction error lost its domain diagnostic: %+v", response)
		}
		ids := strings.Split(response.Error.DiagnosticID, "/")
		record, err := domain.ReadOperationDiagnostic(t.Context(), ids[0], ids[1])
		if err != nil {
			t.Fatal(err)
		}
		var diagnostic workspace.OperationDiagnostic
		if err = json.Unmarshal(record, &diagnostic); err != nil {
			t.Fatal(err)
		}
		if diagnostic.InteractionBegin == nil || diagnostic.InteractionBegin.InstanceID != "missing" || diagnostic.InteractionBegin.BaseRevisionID != view.Document.VersionID {
			t.Fatal("missing original Begin input")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("interaction error response timed out")
	}
	head, err := domain.GetDocument(t.Context(), view.Document.ID, access.DefaultUserID)
	if err != nil || head.Document.VersionID != view.Document.VersionID {
		t.Fatal("failed interaction changed Head", err)
	}
}
