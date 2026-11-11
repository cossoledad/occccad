package api

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/exchange"
	"github.com/occccad/occccad/internal/jobs"
)

func TestOnlineExchangeCompletionIsNotStarvedByOfflineOutbox(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	online, offline := uuid.NewString(), uuid.NewString()
	for _, id := range []string{online, offline} {
		if _, err = db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name) VALUES($1,$2,'Outbox regression')`, id, id+"@test.invalid"); err != nil {
			t.Fatal(err)
		}
	}
	queue := jobs.New(db)
	for n := 0; n < 101; n++ {
		if _, err = queue.Enqueue(t.Context(), jobs.EnqueueRequest{Type: "EXCHANGE_IMPORT", RequestedBy: offline, IdempotencyKey: fmt.Sprintf("offline-%d", n), Payload: map[string]string{"fileName": "offline.step"}}); err != nil {
			t.Fatal(err)
		}
	}
	job, err := queue.Enqueue(t.Context(), jobs.EnqueueRequest{Type: "EXCHANGE_IMPORT", RequestedBy: online, IdempotencyKey: "online", Payload: map[string]string{"fileName": "empty.step"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(t.Context(), `UPDATE occccad.jobs SET state='RUNNING',lease_owner='owner' WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = queue.SucceedImport(t.Context(), job.ID, "owner", "", &exchange.Report{SchemaVersion: 1, Warnings: []exchange.Warning{{Code: "EXCHANGE_EMPTY_PART", Message: "已创建空 Part 文档"}}}); err != nil {
		t.Fatal(err)
	}
	hub := newRealtimeHub()
	t.Cleanup(hub.close)
	client := &realtimeClient{hub: hub, actor: access.User{ID: online}, send: make(chan []byte, 256), done: make(chan struct{}), subscriptions: map[string]string{}, acknowledged: map[string]uint64{}}
	hub.add(client)
	server := &Server{database: db, jobs: queue, realtime: hub}
	if err = server.publishJobRealtimeBatch(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-client.send:
		var event struct {
			Payload struct {
				Job jobs.Job `json:"job"`
			} `json:"payload"`
		}
		if err = json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		if event.Payload.Job.ID != job.ID || event.Payload.Job.State != "SUCCEEDED" || len(event.Payload.Job.Warnings) != 1 || len(event.Payload.Job.Payload) != 0 && string(event.Payload.Job.Payload) != "null" {
			t.Fatalf("completion lost state/warning or sent full payload: %s", data)
		}
	default:
		t.Fatal("online completion starved behind more than one offline batch")
	}
	var pending int
	if err = db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.outbox_events e JOIN occccad.jobs j ON j.id=e.aggregate_id WHERE j.requested_by_user_id=$1 AND e.published_at IS NULL`, offline).Scan(&pending); err != nil || pending != 101 {
		t.Fatalf("offline events were discarded: %d %v", pending, err)
	}
	reconnected := &realtimeClient{hub: hub, actor: access.User{ID: offline}, send: make(chan []byte, 256), done: make(chan struct{}), subscriptions: map[string]string{}, acknowledged: map[string]uint64{}}
	hub.add(reconnected)
	for range 2 {
		if err = server.publishJobRealtimeBatch(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if len(reconnected.send) != 101 {
		t.Fatal("reconnected user did not receive retained events", len(reconnected.send))
	}
}
