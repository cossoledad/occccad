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
	if page.Documents[0].Health == nil || page.Documents[0].Health.Severity != HealthNormal {
		t.Fatal("ordinary empty Part health", page.Documents[0].Health)
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

func TestFailedRevisionPropagatesErrorToRepeatedInstances(t *testing.T) {
	ctx := t.Context()
	db, err := database.Open(ctx, "sqlite:"+filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	store, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := NewWithArtifacts(db, nil, artifact.NewService(db, store))
	actor := access.DefaultUserID
	part, err := s.CreateDocument(ctx, CreateDocumentRequest{ActorID: actor, Type: "PART", Name: "Part"})
	if err != nil {
		t.Fatal(err)
	}
	instances := []ProductInstance{{ID: "part-a", Name: "A", ReferenceMode: "PINNED", ReferencedDocumentID: part.Document.ID, ReferencedVersionID: part.Document.VersionID}, {ID: "part-b", Name: "B", ReferenceMode: "PINNED", ReferencedDocumentID: part.Document.ID, ReferencedVersionID: part.Document.VersionID}}
	product, err := s.CommitImportedProduct(ctx, actor, "", "fixture", "Assembly", instances)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE occccad.document_versions SET state='FAILED' WHERE id=$1`, part.Document.VersionID); err != nil {
		t.Fatal(err)
	}
	product, err = s.GetDocument(ctx, product.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	if product.Document.Health.Severity != HealthError || product.StructureTree.Health.Severity != HealthError {
		t.Fatal(product.Document.Health)
	}
	for _, i := range product.Product.Instances {
		if i.Health.Severity != HealthError {
			t.Fatal(i.Health)
		}
	}
}
