package workspace

import (
	"os"
	"testing"

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

// Read-only comparison against an explicitly supplied development database.
func TestHistoryCapabilityBatchMatchesSinglePostgres(t *testing.T) {
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
	service := New(pool, nil)
	var actor string
	if err = pool.QueryRow(t.Context(), `SELECT id::text FROM occccad.users WHERE platform_role='ADMIN' LIMIT 1`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	page, err := service.ListDocuments(t.Context(), DocumentListOptions{ActorID: actor, AllFolders: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Documents) == 0 {
		t.Fatal("expected at least one development document for read-only verification")
	}
	// Cross the batching boundary using immutable identities without creating data.
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
