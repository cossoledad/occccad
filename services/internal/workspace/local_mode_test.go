package workspace

import (
	"path/filepath"
	"testing"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
)

func TestLocalModeDocumentHistoryAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.db")
	db, e := database.Open(t.Context(), "sqlite:"+path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(t.Context(), db); e != nil {
		t.Fatal(e)
	}
	actor := access.DefaultUserID
	ctx := t.Context()
	store, e := artifact.NewLocalStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	s := NewWithArtifacts(db, nil, artifact.NewService(db, store))
	folder, e := s.CreateFolder(ctx, CreateFolderRequest{Name: "工程", ActorID: actor})
	if e != nil {
		t.Fatal(e)
	}
	part, e := s.CreateDocument(ctx, CreateDocumentRequest{RequestID: "create-local-part", Type: "PART", Name: "Local Part", ActorID: actor, FolderID: &folder.ID})
	if e != nil {
		t.Fatal(e)
	}
	initial := part.Document.VersionID
	page, e := s.ListDocuments(ctx, DocumentListOptions{ActorID: actor, AllFolders: true})
	if e != nil || page.Total != 1 {
		t.Fatalf("document listing: %+v %v", page, e)
	}
	folders, e := s.ListFolders(ctx, "", actor, false)
	if e != nil || len(folders) != 1 || folders[0].Permission != "OWNER" {
		t.Fatalf("folder permissions: %+v %v", folders, e)
	}
	request := CommandRequest{ActorID: actor, RequestID: "local-parameter", Type: "CREATE_PARAMETER", Name: "Length", Value: 25, Unit: "mm"}
	part, e = s.ApplyCommand(ctx, part.Document.ID, request)
	if e != nil {
		t.Fatal(e)
	}
	head := part.Document.VersionID
	again, e := s.ApplyCommand(ctx, part.Document.ID, request)
	if e != nil || again.Document.VersionID != head {
		t.Fatalf("replay: %v", e)
	}
	if part.Part == nil || len(part.Part.Parameters) != 1 || head == initial {
		t.Fatalf("parameter commit: %+v", part.Part)
	}
	undo, e := s.ApplyCommand(ctx, part.Document.ID, CommandRequest{ActorID: actor, RequestID: "local-undo", Type: "UNDO"})
	if e != nil {
		t.Fatal(e)
	}
	if len(undo.Part.Parameters) != 0 || undo.Document.VersionID == initial {
		t.Fatal("undo must append a revision")
	}
	redo, e := s.ApplyCommand(ctx, part.Document.ID, CommandRequest{ActorID: actor, RequestID: "local-redo", Type: "REDO"})
	if e != nil {
		t.Fatal(e)
	}
	if len(redo.Part.Parameters) != 1 {
		t.Fatal("redo lost parameter")
	}
	var revisions, outbox int
	if e = db.QueryRow(ctx, `SELECT count(*) FROM occccad.document_versions WHERE document_id=$1`, part.Document.ID).Scan(&revisions); e != nil || revisions != 4 {
		t.Fatalf("revision history %d %v", revisions, e)
	}
	if e = db.QueryRow(ctx, `SELECT count(*) FROM occccad.outbox_events WHERE aggregate_type='WORKSPACE'`).Scan(&outbox); e != nil || outbox != 4 {
		t.Fatalf("outbox history %d %v", outbox, e)
	}
	// Use a second connection to prove persisted state rather than service caches.
	other, e := database.Open(ctx, "sqlite:"+path)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	cold := NewWithArtifacts(other, nil, artifact.NewService(other, store))
	loaded, e := cold.GetDocument(ctx, part.Document.ID, actor)
	if e != nil || loaded.Document.VersionID != redo.Document.VersionID || len(loaded.Part.Parameters) != 1 {
		t.Fatalf("cold read: %v", e)
	}
}
