package jobs

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
)

func TestProgressHighWaterAndRetryPhasesDatabase(t *testing.T) {
	if os.Getenv("OCCCCAD_TEST_IMPORT_DATABASE") != "1" {
		t.Skip("explicit development database opt-in required")
	}
	if _, err := config.LoadProjectEnv(); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(t.Context(), config.Load().DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	actor, id := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name) VALUES($1,$2,'Progress regression')`, actor, actor+"@progress-test.invalid"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), `DELETE FROM occccad.users WHERE id=$1`, actor)
	if _, err := db.Exec(t.Context(), `INSERT INTO occccad.jobs(id,job_type,requested_by_user_id,idempotency_key,state,lease_owner,lease_expires_at,attempt_count) VALUES($1::uuid,'EXCHANGE_IMPORT',$2,$1::text,'RUNNING','fixture',now()+interval '1 hour',1)`, id, actor); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), `DELETE FROM occccad.jobs WHERE id=$1`, id)
	service := New(db)
	assert := func(percent int, phase string, attempt int) {
		t.Helper()
		job, err := service.Get(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Detail struct {
				Phase   string
				Attempt int
			} `json:"progressDetail"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if job.Progress != percent || payload.Detail.Phase != phase || payload.Detail.Attempt != attempt {
			t.Fatalf("progress=%d phase=%s attempt=%d", job.Progress, payload.Detail.Phase, payload.Detail.Attempt)
		}
	}
	update := func(percent int, phase string) {
		t.Helper()
		if err := service.UpdateProgressDetail(t.Context(), id, "fixture", percent, &ProgressDetail{Phase: phase}); err != nil {
			t.Fatal(err)
		}
	}
	update(70, "CREATING_PARTS")
	update(30, "EVALUATING")
	assert(70, "CREATING_PARTS", 1)
	if _, err := db.Exec(t.Context(), `UPDATE occccad.jobs SET attempt_count=2 WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	update(5, "PREPARING")
	assert(70, "PREPARING", 2)
	update(30, "EVALUATING")
	assert(70, "EVALUATING", 2)
	update(80, "CREATING_PARTS")
	assert(80, "CREATING_PARTS", 2)
	if err := service.UpdateProgress(t.Context(), id, "obsolete-lease-owner", 90); err != ErrNotFound {
		t.Fatalf("unfenced writer: %v", err)
	}
	if _, err := db.Exec(t.Context(), `UPDATE occccad.jobs SET state='FAILED' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	retried, err := service.Retry(t.Context(), id, actor, false)
	if err != nil || retried.Progress != 80 {
		t.Fatalf("manual retry reset progress: %d %v", retried.Progress, err)
	}
}
