package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/control"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/geometryrpc"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
)

// Opt-in integration writes only its own user's test folder, then trashes it.
// It uses the real import handler, PostgreSQL, storage, Router and C++ workers.
func TestMultiSolidImportCreatesNamedPartsAndProduct(t *testing.T) {
	sourcePath, binary := os.Getenv("OCCCCAD_TEST_EXCHANGE_STEP"), os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if sourcePath == "" || binary == "" || os.Getenv("OCCCCAD_TEST_IMPORT_DATABASE") != "1" {
		t.Skip("requires STEP, Worker and explicit development database opt-in")
	}
	if _, err := config.LoadProjectEnv(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCCCCAD_IMPORT_CONCURRENCY", "4")
	c := config.Load()
	db, err := database.Open(t.Context(), c.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	runID := uuid.NewString()
	var actor string
	if err := db.QueryRow(t.Context(), `INSERT INTO occccad.users(email,display_name) VALUES($1,'Import regression') RETURNING id::text`, runID+"@import-test.invalid").Scan(&actor); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), `UPDATE occccad.users SET status='DISABLED' WHERE id=$1`, actor)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	root := t.TempDir()
	// Large resident capacity must still scale busy native workers; capacity=1
	// masks the historical single-process pileup and health-probe failure.
	pool := control.NewGeometryPool(t.Context(), control.GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: root, LogDirectory: t.TempDir(), MinimumWorkers: 1, MaximumWorkers: 4, GeometryCapacity: 100})
	defer pool.Close()
	if err := pool.Start(); err != nil {
		t.Fatal(err)
	}
	routerListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(geometryrpc.ServerOptions()...)
	defer server.Stop()
	workerv1.RegisterGeometryWorkerServer(server, pool)
	go server.Serve(routerListener)
	store, local, err := artifact.OpenConfigured(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	client, err := geometry.Open(routerListener.Addr().String(), geometry.ArtifactStaging(store, local))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	artifacts := artifact.NewService(db, store, local)
	service := workspace.NewWithArtifacts(db, client, artifacts)
	folder, err := service.CreateFolder(t.Context(), workspace.CreateFolderRequest{Name: "Import regression " + runID, ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := service.DeleteFolder(context.Background(), folder.ID); err != nil {
			t.Error(err)
		}
	}()
	defer db.Exec(context.Background(), `UPDATE occccad.jobs SET state='CANCELED',cancel_requested_at=now(),completed_at=now() WHERE requested_by_user_id=$1 AND state IN ('QUEUED','RETRY_WAIT','RUNNING')`, actor)
	file, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	source, err := artifacts.Put(t.Context(), artifact.KindExchangeSource, "application/step", file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"fileName": "regression-" + runID[:8] + ".step", "folderId": folder.ID, "format": "STEP", "requestId": runID})
	owner := "import-test-" + runID
	var jobID string
	// Claim only this fixture. Never consume another user's queued job.
	if err := db.QueryRow(t.Context(), `INSERT INTO occccad.jobs(job_type,requested_by_user_id,input_object_id,payload,idempotency_key,user_visible,state,lease_owner,lease_expires_at,attempt_count) VALUES('EXCHANGE_IMPORT',$1,$2,$3,$4,true,'RUNNING',$5,now()+interval '1 hour',1) RETURNING id::text`, actor, source.ID, payload, runID, owner).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	queue := jobs.New(db)
	job, err := queue.Get(t.Context(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	h := handler{importBudget: newImportBudget(importConcurrency(os.Getenv("OCCCCAD_IMPORT_CONCURRENCY"))), workerID: owner, database: db, queue: queue, artifacts: artifacts, access: access.New(db), geometry: client, workspace: service}
	poll, stop := context.WithCancel(t.Context())
	var monitor sync.WaitGroup
	monitor.Add(1)
	go func() {
		defer monitor.Done()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		last, printed := 0, -1
		for {
			select {
			case <-poll.Done():
				return
			case <-ticker.C:
				current, err := queue.Get(poll, jobID)
				if err != nil {
					continue
				}
				if current.Progress < last {
					t.Errorf("progress regressed %d -> %d", last, current.Progress)
				}
				last = current.Progress
				if current.Progress/10 != printed {
					printed = current.Progress / 10
					t.Logf("import progress=%d%%", current.Progress)
				}
			}
		}
	}()
	started := time.Now()
	err = h.execute(t.Context(), job)
	stop()
	monitor.Wait()
	if err != nil {
		t.Fatal(err)
	}
	finished, err := queue.Get(t.Context(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != "SUCCEEDED" || finished.Progress != 100 || finished.DocumentID == nil {
		t.Fatalf("incomplete job: %+v", finished)
	}
	product, err := service.GetDocument(t.Context(), *finished.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	expected := 114
	if value := os.Getenv("OCCCCAD_TEST_EXPECTED_SOLIDS"); value != "" {
		expected, _ = strconv.Atoi(value)
	}
	if product.Product == nil || len(product.Product.Instances) != expected {
		t.Fatalf("expected Product with %d parts", expected)
	}
	for index, instance := range product.Product.Instances {
		if instance.ReferenceMode != "PINNED" {
			t.Fatal("import snapshot must be pinned")
		}
		part, err := service.GetDocument(t.Context(), instance.ReferencedDocumentID)
		if err != nil {
			t.Fatal(err)
		}
		if part.Artifact == nil || !part.Artifact.Naming.CanBind || part.Artifact.Topology["solids"] != float64(1) {
			t.Fatalf("part %d lacks valid single-Solid naming", index)
		}
		if len(part.Part.Features) != 1 || part.Part.Features[0].ImportDefinitionID == "" {
			t.Fatal("missing frozen import definition")
		}
		if _, err := service.BindPersistentSelection(t.Context(), part.Document.ID, workspace.BindPersistentSelectionRequest{SourceVersionID: part.Document.VersionID, GeometryKey: part.Artifact.GeometryKey, Kind: "FACE", LocalID: 1}); err != nil {
			t.Fatalf("part %d face binding: %v", index, err)
		}
	}
	// Retry/older phase progress cannot rewind the displayed high-water mark.
	if _, err := db.Exec(t.Context(), `UPDATE occccad.jobs SET state='RUNNING',lease_owner=$2,progress=70,attempt_count=2 WHERE id=$1`, jobID, owner); err != nil {
		t.Fatal(err)
	}
	if err := queue.UpdateProgressDetail(t.Context(), jobID, owner, 30, &jobs.ProgressDetail{Phase: "EVALUATING", Completed: 1, Total: expected}); err != nil {
		t.Fatal(err)
	}
	retained, err := queue.Get(t.Context(), jobID)
	if err != nil || retained.Progress != 70 {
		t.Fatalf("retry progress: %d %v", retained.Progress, err)
	}
	if err := queue.SucceedImport(t.Context(), jobID, owner, *finished.DocumentID); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d editable named Parts + Product, all FACE bindings valid, monotonic progress; elapsed=%s", filepath.Base(sourcePath), expected, time.Since(started).Round(time.Second))
}
