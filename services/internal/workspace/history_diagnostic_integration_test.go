//go:build integration

package workspace

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/debugartifact"
	"github.com/occccad/occccad/internal/testsupport"
)

func TestHistoryDiagnosticRetainsFailureTimeRevision(t *testing.T) {
	db := testsupport.OpenDevelopmentDatabase(t)
	configuration := testsupport.Environment(t)
	store, local, err := artifact.OpenConfigured(t.Context(), configuration.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithArtifacts(db, nil, artifact.NewService(db, store, local))
	archive, err := debugartifact.NewStore(filepath.Join(configuration.LogDirectory, "debug", "cad-diagnostics"), 50, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	service.SetDiagnosticArtifactStore(archive)
	view, err := service.CreateDocument(t.Context(), CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PART", Name: "History diagnostic " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	_, failure := service.ApplyCommand(t.Context(), view.Document.ID, CommandRequest{ActorID: access.DefaultUserID, Type: "UNDO", RequestID: uuid.NewString()})
	var diagnostic interface{ DiagnosticID() string }
	if failure == nil || !errors.As(failure, &diagnostic) {
		t.Fatal("history failure has no retained diagnostic", failure)
	}
	identity := strings.Split(diagnostic.DiagnosticID(), "/")
	before, err := service.ReadOperationDiagnostic(t.Context(), identity[0], identity[1])
	if err != nil {
		t.Fatal(err)
	}
	var frozen OperationDiagnostic
	if err = json.Unmarshal(before, &frozen); err != nil {
		t.Fatal(err)
	}
	if frozen.Mode != "HISTORY" || frozen.BaseRevisionID != view.Document.VersionID || len(frozen.BaseModel) == 0 {
		t.Fatal("history failure lacks its immutable baseline")
	}
	updated, err := service.ApplyCommand(t.Context(), view.Document.ID, CommandRequest{ActorID: access.DefaultUserID, Type: "CREATE_PARAMETER", RequestID: uuid.NewString(), Name: "later", Value: 3, Unit: "mm"})
	if err != nil || updated.Document.VersionID == frozen.BaseRevisionID {
		t.Fatal("failed to advance the document", err)
	}
	after, err := service.ReadOperationDiagnostic(t.Context(), identity[0], identity[1])
	if err != nil || string(before) != string(after) {
		t.Fatal("later Head changed the history diagnostic", err)
	}
	t.Log("CAD_DIAGNOSTIC", diagnostic.DiagnosticID())
}
