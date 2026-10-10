package jobs

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/database"
)

func TestMotionJobLeaseAttemptCancelAndSavedResult(t *testing.T) {
	db, e := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "motion.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(t.Context(), db); e != nil {
		t.Fatal(e)
	}
	s := New(db)
	actor := "00000000-0000-7000-8000-000000000001"
	j, e := s.Enqueue(t.Context(), EnqueueRequest{Type: "MOTION_STUDY", RequestedBy: actor, IdempotencyKey: "motion-fixture", UserVisible: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, err := s.DeleteMotionResult(t.Context(), j.ID, actor, false); !errors.Is(err, ErrNotDismissible) {
		t.Fatal("running job disappeared", err)
	}
	j, e = s.Claim(t.Context(), "worker", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	var resultID string
	if e = db.QueryRow(t.Context(), `INSERT INTO occccad.artifact_objects(kind,sha256,storage_backend,object_key,content_type,size_bytes) VALUES('MOTION_RUN',$1,'LOCAL','motion-result','application/json',2) RETURNING id::text`, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").Scan(&resultID); e != nil {
		t.Fatal(e)
	}
	if e = s.FinishMotion(t.Context(), j.ID, "worker", j.AttemptCount+1, resultID); !errors.Is(e, ErrNotFound) {
		t.Fatalf("late attempt accepted: %v", e)
	}
	oldAttempt := j.AttemptCount
	if _, e = db.Exec(t.Context(), `UPDATE occccad.jobs SET lease_expires_at='2000-01-01T00:00:00Z' WHERE id=$1`, j.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.FinishMotion(t.Context(), j.ID, "worker", oldAttempt, resultID); !errors.Is(e, ErrNotFound) {
		t.Fatal("expired lease saved a result", e)
	}
	j, e = s.Claim(t.Context(), "replacement", time.Minute)
	if e != nil || j.AttemptCount != oldAttempt+1 {
		t.Fatal("lease takeover", j, e)
	}
	if e = s.FinishMotion(t.Context(), j.ID, "worker", oldAttempt, resultID); !errors.Is(e, ErrNotFound) {
		t.Fatal("old owner overwrote takeover", e)
	}
	if _, e = s.RequestCancel(t.Context(), j.ID, actor, false); e != nil {
		t.Fatal(e)
	}
	if e = s.FinishMotion(t.Context(), j.ID, "replacement", j.AttemptCount, resultID); e != nil {
		t.Fatal(e)
	}
	result, e := s.Get(t.Context(), j.ID)
	if e != nil || result.State != "CANCELED" || result.ResultObjectID == nil || *result.ResultObjectID != resultID {
		t.Fatal(result, e)
	}
	var state string
	if e = db.QueryRow(t.Context(), `SELECT result FROM occccad.job_attempts WHERE job_id=$1 AND attempt=$2`, j.ID, j.AttemptCount).Scan(&state); e != nil || state != "CANCELED" {
		t.Fatal(state, e)
	}
	if e = s.FinishMotion(t.Context(), j.ID, "replacement", j.AttemptCount, resultID); !errors.Is(e, ErrNotFound) {
		t.Fatal("late duplicate result accepted", e)
	}
	if result.CanRetry {
		t.Fatal("a saved run must be rerun as a new job")
	}
	if _, e = s.Retry(t.Context(), j.ID, actor, false); !errors.Is(e, ErrNotRetryable) {
		t.Fatal("retry overwrites saved run identity", e)
	}
}

func TestMotionResultDeletePreservesHistory(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "results.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	s := New(db)
	actor := "00000000-0000-7000-8000-000000000001"
	j, err := s.Enqueue(t.Context(), EnqueueRequest{Type: "MOTION_STUDY", RequestedBy: actor, IdempotencyKey: "delete-run", UserVisible: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RequestCancel(t.Context(), j.ID, actor, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteMotionResult(t.Context(), j.ID, "00000000-0000-7000-8000-000000000002", false); !errors.Is(err, ErrNotDismissible) {
		t.Fatal("foreign run deleted", err)
	}
	for i := 0; i < 2; i++ {
		if _, err = s.DeleteMotionResult(t.Context(), j.ID, actor, false); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListForUser(t.Context(), actor, 100)
	if err != nil || len(list) != 0 {
		t.Fatal(list, err)
	}
	retained, err := s.Get(t.Context(), j.ID)
	if err != nil || retained.State != "CANCELED" || retained.UserVisible {
		t.Fatal("audit record lost", retained, err)
	}
}
