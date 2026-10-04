package jobs

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/database"
)

func TestLocalModeJobLifecycleAndAtomicOutbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.db")
	db, e := database.Open(t.Context(), "sqlite:"+path)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(t.Context(), db); e != nil {
		t.Fatal(e)
	}
	actor := "00000000-0000-7000-8000-000000000001"
	service := New(db)
	ctx := t.Context()
	request := EnqueueRequest{Type: "EXCHANGE_EXPORT", RequestedBy: actor, IdempotencyKey: "first", Payload: map[string]any{"format": "STEP"}, UserVisible: true}
	job, e := service.Enqueue(ctx, request)
	if e != nil {
		t.Fatal(e)
	}
	duplicate, e := service.Enqueue(ctx, request)
	if e != nil || duplicate.ID != job.ID {
		t.Fatalf("idempotency: %v", e)
	}
	// Separate pools model API and two Jobs processes sharing the file.
	other, e := database.Open(ctx, "sqlite:"+path)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for i, s := range []*Service{service, New(other)} {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.Claim(ctx, []string{"a", "b"}[i], time.Minute); out <- e }()
	}
	wg.Wait()
	close(out)
	claimed, empty := 0, 0
	for e := range out {
		if e == nil {
			claimed++
		} else if errors.Is(e, database.ErrNoRows) {
			empty++
		} else {
			t.Fatal(e)
		}
	}
	if claimed != 1 || empty != 1 {
		t.Fatalf("double claim: %d/%d", claimed, empty)
	}
	var owner string
	if e = db.QueryRow(ctx, `SELECT lease_owner FROM occccad.jobs WHERE id=$1`, job.ID).Scan(&owner); e != nil {
		t.Fatal(e)
	}
	if e = service.UpdateProgressDetail(ctx, job.ID, owner, 70, &ProgressDetail{Phase: "WRITING"}); e != nil {
		t.Fatal(e)
	}
	if e = service.UpdateProgressDetail(ctx, job.ID, owner, 20, &ProgressDetail{Phase: "OLD"}); e != nil {
		t.Fatal(e)
	}
	current, e := service.Get(ctx, job.ID)
	if e != nil || current.Progress != 70 {
		t.Fatalf("progress: %+v %v", current, e)
	}
	if e = service.Heartbeat(ctx, job.ID, "stale", time.Minute); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	canceled, e := service.RequestCancel(ctx, job.ID, actor, false)
	if e != nil || canceled.CancelRequestedAt == nil {
		t.Fatalf("cancel: %v", e)
	}
	if e = service.Succeed(ctx, job.ID, owner, ""); !errors.Is(e, ErrNotFound) {
		t.Fatal("canceled job completed", e)
	}
	if e = service.AcknowledgeCanceled(ctx, job.ID, owner); e != nil {
		t.Fatal(e)
	}
	if _, e = service.Retry(ctx, job.ID, actor, false); e != nil {
		t.Fatal(e)
	}
	current, e = service.Claim(ctx, "retry", time.Minute)
	if e != nil || current.AttemptCount != 2 {
		t.Fatal(e)
	}
	if e = service.UpdateProgressDetail(ctx, job.ID, "retry", 5, &ProgressDetail{Phase: "PREPARE"}); e != nil {
		t.Fatal(e)
	}
	if e = service.Succeed(ctx, job.ID, "retry", ""); e != nil {
		t.Fatal(e)
	}
	current, e = service.Get(ctx, job.ID)
	if e != nil || current.State != "SUCCEEDED" || current.Progress != 100 {
		t.Fatalf("final: %+v %v", current, e)
	}
	// Deliberately break the last write: no job may survive a failed notification.
	if _, e = db.Exec(ctx, `CREATE TRIGGER reject_outbox BEFORE INSERT ON outbox_events BEGIN SELECT RAISE(ABORT,'injected outbox failure'); END`); e != nil {
		t.Fatal(e)
	}
	request.IdempotencyKey = "rollback"
	if _, e = service.Enqueue(ctx, request); e == nil {
		t.Fatal("expected injected failure")
	}
	var n int
	if e = db.QueryRow(ctx, `SELECT count(*) FROM occccad.jobs WHERE idempotency_key='rollback'`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("partial commit: %d %v", n, e)
	}
}

func TestLocalModeLeaseReclaimAndCancellation(t *testing.T) {
	db, e := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "jobs.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(t.Context(), db); e != nil {
		t.Fatal(e)
	}
	s := New(db)
	job, e := s.Enqueue(t.Context(), EnqueueRequest{Type: "EXCHANGE_EXPORT", RequestedBy: "00000000-0000-7000-8000-000000000001", IdempotencyKey: "lease", Payload: map[string]any{}})
	if e != nil {
		t.Fatal(e)
	}
	job, e = s.Claim(t.Context(), "old", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(t.Context(), `UPDATE occccad.jobs SET lease_expires_at=$2 WHERE id=$1`, job.ID, time.Now().Add(-time.Second)); e != nil {
		t.Fatal(e)
	}
	reclaimed, e := s.Claim(t.Context(), "new", time.Minute)
	if e != nil || reclaimed.AttemptCount != 2 {
		t.Fatal(e)
	}
	if e = s.Succeed(t.Context(), job.ID, "old", ""); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if e = s.Fail(t.Context(), reclaimed, "new", "FAIL", "retry me"); e != nil {
		t.Fatal(e)
	}
	current, e := s.Get(t.Context(), job.ID)
	if e != nil || current.State != "RETRY_WAIT" {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e = s.Claim(ctx, "canceled", time.Minute); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
