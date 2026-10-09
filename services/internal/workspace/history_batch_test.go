package workspace

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/database"
)

func TestHistoryCapabilityBatchEmptyDoesNotQuery(t *testing.T) {
	service := &Service{}
	if err := service.populateHistoryCapabilities(t.Context(), nil, "actor"); err != nil {
		t.Fatal(err)
	}
	if err := service.populateHistoryCapabilities(t.Context(), []DocumentSummary{{ID: "document"}}, ""); err != nil {
		t.Fatal(err)
	}
}

// Compare batched and individual queries using an isolated test document.
func TestHistoryCapabilityBatchMatchesSingleDatabase(t *testing.T) {
	url := os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OCCCCAD_TEST_DATABASE_URL is not set")
	}
	t.Setenv("OCCCCAD_DB_CONCURRENCY", "1")
	t.Setenv("OCCCCAD_DB_BACKGROUND_CONCURRENCY", "1")
	pool, err := database.Open(t.Context(), url)
	if err != nil {
		t.Fatal("cannot connect test database")
	}
	defer pool.Close()
	if err := database.Migrate(t.Context(), pool); err != nil {
		t.Fatal(err)
	}
	service := New(pool, nil)
	var actor string
	if err = pool.QueryRow(t.Context(), `SELECT id::text FROM occccad.users WHERE platform_role='ADMIN' LIMIT 1`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	documentID := uuid.NewString()
	name := "history-batch-" + documentID
	if _, err := pool.Exec(t.Context(), `INSERT INTO occccad.documents(id,document_type,name,owner_user_id) VALUES($1,'PRODUCT',$2,$3)`, documentID, name, actor); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE occccad.documents SET head_version_id=NULL WHERE id=$1`, documentID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM occccad.document_versions WHERE document_id=$1`, documentID)
		if _, err := pool.Exec(context.Background(), `DELETE FROM occccad.documents WHERE id=$1`, documentID); err != nil {
			t.Error(err)
		}
	}()
	revisionID := uuid.NewString()
	if _, err := pool.Exec(t.Context(), `INSERT INTO occccad.document_versions(id,document_id,sequence,model_json,state,model_hash) VALUES($1,$2,1,'{"instances":[]}','READY','initial')`, revisionID, documentID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE occccad.documents SET head_version_id=$1 WHERE id=$2`, revisionID, documentID); err != nil {
		t.Fatal(err)
	}
	page, err := service.ListDocuments(t.Context(), DocumentListOptions{ActorID: actor, AllFolders: true, Query: name, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Documents) == 0 {
		t.Fatal("expected the isolated test document")
	}
	// Cross the batching boundary using immutable identities.
	documents := make([]DocumentSummary, 130)
	for i := range documents {
		documents[i] = page.Documents[i%len(page.Documents)]
	}
	if err = service.populateHistoryCapabilities(t.Context(), documents, actor); err != nil {
		t.Fatal(err)
	}
	for _, document := range page.Documents {
		undo, redo, err := service.historyCapabilities(t.Context(), document.ID, actor)
		if err != nil {
			t.Fatal(err)
		}
		for _, batched := range documents {
			if batched.ID == document.ID && (batched.CanUndo != undo || batched.CanRedo != redo) {
				t.Fatal("batch changed history capabilities")
			}
		}
	}
	if pool.Snapshot().Active != 0 {
		t.Fatal("list retained a database admission slot")
	}
}
