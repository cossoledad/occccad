package jobs

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/database"
)

func TestJobLifecycleRealtimeOutboxDatabase(t *testing.T) {
	url := os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires isolated test database")
	}
	db, err := database.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	service := New(db)
	job, err := service.Enqueue(t.Context(), EnqueueRequest{Type: "EXCHANGE_EXPORT", RequestedBy: access.DefaultUserID, IdempotencyKey: uuid.NewString(), Payload: map[string]string{}, UserVisible: true})
	if err != nil {
		t.Fatal(err)
	}
	// Isolate the claimed fixture from unrelated test jobs without deleting data.
	if _, err = db.Exec(t.Context(), `UPDATE occccad.jobs SET priority=30000 WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	events := 0
	expect := func(state string) {
		t.Helper()
		events++
		current, err := service.Get(t.Context(), job.ID)
		if err != nil || current.State != state {
			t.Fatalf("state %s: %+v %v", state, current, err)
		}
		var count int
		if err = db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.outbox_events WHERE aggregate_type='JOB' AND aggregate_id=$1`, job.ID).Scan(&count); err != nil || count != events {
			t.Fatalf("state %s outbox=%d want=%d: %v", state, count, events, err)
		}
	}
	claim := func() {
		t.Helper()
		current, err := service.Claim(t.Context(), "realtime-fixture", time.Minute)
		if err != nil || current.ID != job.ID {
			t.Fatalf("claim: %v %s", err, current.ID)
		}
		job = current
		expect("RUNNING")
	}
	expect("QUEUED")
	claim()
	if err = service.UpdateProgress(t.Context(), job.ID, "realtime-fixture", 20); err != nil {
		t.Fatal(err)
	}
	expect("RUNNING")
	if err = service.Fail(t.Context(), job, "realtime-fixture", "TEST", "test retry"); err != nil {
		t.Fatal(err)
	}
	expect("RETRY_WAIT")
	if _, err = service.RequestCancel(t.Context(), job.ID, access.DefaultUserID, false); err != nil {
		t.Fatal(err)
	}
	expect("CANCELED")
	if _, err = service.Retry(t.Context(), job.ID, access.DefaultUserID, false); err != nil {
		t.Fatal(err)
	}
	expect("QUEUED")
	claim()
	if _, err = service.RequestCancel(t.Context(), job.ID, access.DefaultUserID, false); err != nil {
		t.Fatal(err)
	}
	expect("RUNNING")
	if err = service.AcknowledgeCanceled(t.Context(), job.ID, "realtime-fixture"); err != nil {
		t.Fatal(err)
	}
	expect("CANCELED")
	if _, err = service.Retry(t.Context(), job.ID, access.DefaultUserID, false); err != nil {
		t.Fatal(err)
	}
	expect("QUEUED")
	claim()
	if err = service.Succeed(t.Context(), job.ID, "realtime-fixture", ""); err != nil {
		t.Fatal(err)
	}
	expect("SUCCEEDED")
}
