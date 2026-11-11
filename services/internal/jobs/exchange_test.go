package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/exchange"
)

func TestImportWarningsCommitWithSuccessAndOutbox(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "exchange.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	s := New(db)
	job, err := s.Enqueue(t.Context(), EnqueueRequest{Type: "EXCHANGE_IMPORT", RequestedBy: "00000000-0000-7000-8000-000000000001", IdempotencyKey: "warning", Payload: map[string]any{"fileName": "metadata.step"}, UserVisible: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(t.Context(), "owner", time.Minute); err != nil {
		t.Fatal(err)
	}
	report := &exchange.Report{SchemaVersion: 1, EmptyParts: 1, Warnings: []exchange.Warning{{Code: "EXCHANGE_EMPTY_PART", Message: "已创建空 Part 文档", Count: 1}}}
	if err := s.SucceedImport(t.Context(), job.ID, "stale", "", report); !errors.Is(err, ErrNotFound) {
		t.Fatal("stale lease completed", err)
	}
	if _, err := db.Exec(t.Context(), `CREATE TRIGGER reject_exchange_notification BEFORE INSERT ON outbox_events BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.SucceedImport(t.Context(), job.ID, "owner", "", report); err == nil {
		t.Fatal("completion survived a failed notification")
	}
	stored, err := s.Get(t.Context(), job.ID)
	if err != nil || stored.State != "RUNNING" || bytes.Contains(stored.Payload, []byte("EXCHANGE_EMPTY_PART")) {
		t.Fatal("warning/state did not roll back", err)
	}
	if _, err := db.Exec(t.Context(), `DROP TRIGGER reject_exchange_notification`); err != nil {
		t.Fatal(err)
	}
	if err := s.SucceedImport(t.Context(), job.ID, "owner", "", report); err != nil {
		t.Fatal(err)
	}
	stored, err = New(db).Get(context.Background(), job.ID)
	if err != nil || stored.State != "SUCCEEDED" {
		t.Fatal(err)
	}
	var payload struct {
		FileName string          `json:"fileName"`
		Exchange exchange.Report `json:"exchange"`
	}
	if err := json.Unmarshal(stored.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.FileName != "metadata.step" || payload.Exchange.EmptyParts != 1 || len(payload.Exchange.Warnings) != 1 {
		t.Fatalf("completion report lost: %s", stored.Payload)
	}
}
