package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
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
func TestXdeImportSharedDefinitionsAndRoundTrip(t *testing.T) {
	sourcePath, binary := os.Getenv("OCCCCAD_TEST_EXCHANGE_STEP"), os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	testURL := os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if binary == "" || (testURL == "" && os.Getenv("OCCCCAD_TEST_IMPORT_DATABASE") != "1") {
		t.Skip("requires Worker and test database or explicit development opt-in")
	}
	if testURL == "" {
		if _, err := config.LoadProjectEnv(); err != nil {
			t.Fatal(err)
		}
		testURL = config.Load().DatabaseURL
	}
	if sourcePath == "" {
		t.Setenv("OCCCCAD_ARTIFACT_BACKEND", "LOCAL")
	}
	db, err := database.Open(t.Context(), testURL)
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
	generated := sourcePath == ""
	var source artifact.Object
	if generated {
		evaluation, err := client.EvaluatePartFromArtifact(t.Context(), runID+"/fixture", runID+"/fixture", []geometry.RectangularPad{{Width: 20, Height: 10, Length: 5, Plane: "XY"}}, geometry.ArtifactReference{}, artifact.StagingKey(runID, "fixture.brep"), artifact.StagingKey(runID, "fixture.mesh.glb"))
		if err != nil {
			t.Fatal(err)
		}
		object, err := artifacts.Adopt(t.Context(), artifact.KindBREP, "application/vnd.opencascade.brep", evaluation.BrepArtifact.ObjectKey)
		if err != nil {
			t.Fatal(err)
		}
		ref := &workerv1.ArtifactReference{Backend: object.Backend, ObjectKey: object.Key, Sha256: object.SHA256, SizeBytes: uint64(object.Size), ContentType: object.ContentType}
		occurrence := func(id, target string, x float64) *workerv1.ExchangeOccurrence {
			return &workerv1.ExchangeOccurrence{Id: id, Name: id, DefinitionId: target, Translation: &workerv1.Vec3{X: x}, Rotation: &workerv1.Quaternion{W: 1}}
		}
		sub := &workerv1.ExchangeDefinition{Id: "sub", Name: "Sub Product", Kind: "PRODUCT"}
		for i := 0; i < 100; i++ {
			child := occurrence(fmt.Sprintf("Pin.%d", i), "pin", float64(i)*30)
			if i >= 98 {
				child.Name = "Repeated Pin"
			}
			sub.Children = append(sub.Children, child)
		}
		graph := &geometry.ExchangeGraph{Definitions: []*workerv1.ExchangeDefinition{{Id: "pin", Name: "Shared Pin", Kind: "PART", Brep: ref}, {Id: "other", Name: "Identical But Distinct", Kind: "PART", Brep: ref}, sub, {Id: "root", Name: "Root Product", Kind: "PRODUCT", Children: []*workerv1.ExchangeOccurrence{occurrence("Sub.1", "sub", 0), occurrence("Sub.2", "sub", 100), occurrence("Other.1", "other", 200)}}}, Roots: []*workerv1.ExchangeOccurrence{occurrence("root", "root", 0)}}
		exported, err := client.ExportExchange(t.Context(), runID+"/fixture-export", "STEP", artifact.StagingKey(runID, "fixture.step"), graph)
		if err != nil {
			t.Fatal(err)
		}
		source, err = artifacts.Adopt(t.Context(), artifact.KindExchangeSource, exported.ContentType, exported.ObjectKey)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		file, err := os.Open(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		source, err = artifacts.Put(t.Context(), artifact.KindExchangeSource, "model/step", file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
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
	if product.Product == nil {
		t.Fatal("missing root Product")
	}
	graph, err := service.ExchangeExportGraph(t.Context(), product.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	expected := len(graph.Definitions)
	if generated {
		if expected != 4 || len(product.Product.Instances) != 3 || len(product.ResolvedInstances) != 201 {
			t.Fatalf("graph lost sharing: defs=%d root=%d leaves=%d", expected, len(product.Product.Instances), len(product.ResolvedInstances))
		}
		if product.Product.Instances[0].ReferencedDocumentID != product.Product.Instances[1].ReferencedDocumentID {
			t.Fatal("Sub Product duplicated")
		}
		var count int
		if err := db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.documents WHERE folder_id=$1 AND deleted_at IS NULL`, folder.ID).Scan(&count); err != nil || count != 4 {
			t.Fatalf("unique documents=%d err=%v", count, err)
		}
	}
	var docs []string
	rows, err := db.Query(t.Context(), `SELECT id::text FROM occccad.documents WHERE folder_id=$1 AND document_type='PART' AND deleted_at IS NULL`, folder.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		docs = append(docs, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for _, id := range docs {
		part, err := service.GetDocument(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if part.Artifact == nil || !part.Artifact.Naming.CanBind {
			t.Fatal("missing import naming")
		}
		if _, err := service.BindPersistentSelection(t.Context(), id, workspace.BindPersistentSelectionRequest{SourceVersionID: part.Document.VersionID, GeometryKey: part.Artifact.GeometryKey, Kind: "FACE", LocalID: 1}); err != nil {
			t.Fatal(err)
		}
	}
	exported, err := client.ExportExchange(t.Context(), runID+"/roundtrip", "STEP", artifact.StagingKey(runID, "roundtrip.step"), graph)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip, err := client.InspectExchange(t.Context(), runID+"/roundtrip-inspect", "STEP", exported)
	if err != nil {
		t.Fatal(err)
	}
	if len(roundtrip.Definitions) != expected {
		t.Fatalf("roundtrip definitions %d != %d", len(roundtrip.Definitions), expected)
	}
	if generated {
		parts, products, occurrences := 0, 0, 0
		for _, def := range roundtrip.Definitions {
			if def.Name == "Sub Product" {
				duplicates := 0
				for _, child := range def.Children {
					if child.Name == "Repeated Pin" {
						duplicates++
					}
				}
				if duplicates != 2 {
					t.Fatal("roundtrip lost original duplicate source names")
				}
			}
			if def.Kind == "PART" {
				parts++
			} else {
				products++
			}
			occurrences += len(def.Children)
		}
		if parts != 2 || products != 2 || occurrences != 103 {
			t.Fatalf("roundtrip graph %d/%d/%d", parts, products, occurrences)
		}
	}
	if generated {
		// Undo/Redo must replay the stored typed occurrence payload, including
		// duplicate source display names and explicit local poses.
		subID := product.Product.Instances[0].ReferencedDocumentID
		before, err := service.GetDocument(t.Context(), subID)
		if err != nil {
			t.Fatal(err)
		}
		undone, err := service.ApplyCommand(t.Context(), subID, workspace.CommandRequest{Type: "UNDO", ActorID: actor, RequestID: runID + "/undo"})
		if err != nil || len(undone.Product.Instances) != 0 {
			t.Fatalf("import Undo: %v", err)
		}
		redone, err := service.ApplyCommand(t.Context(), subID, workspace.CommandRequest{Type: "REDO", ActorID: actor, RequestID: runID + "/redo"})
		if err != nil || len(redone.Product.Instances) != 100 {
			t.Fatalf("import Redo: %v", err)
		}
		originals := map[string]workspace.ProductInstance{}
		replayed := map[string]workspace.ProductInstance{}
		for _, instance := range before.Product.Instances {
			originals[instance.ID] = instance
		}
		for _, instance := range redone.Product.Instances {
			replayed[instance.ID] = instance
		}
		a, _ := json.Marshal(originals)
		b, _ := json.Marshal(replayed)
		if string(a) != string(b) {
			t.Fatalf("import replay changed occurrence data: before=%s after=%s", a, b)
		}
	}
	if generated {
		// Re-import the actual exported STEP through the same Jobs coordinator.
		object, err := artifacts.Adopt(t.Context(), artifact.KindExchangeSource, exported.ContentType, exported.ObjectKey)
		if err != nil {
			t.Fatal(err)
		}
		nextPayload, _ := json.Marshal(map[string]string{"fileName": "roundtrip.step", "folderId": folder.ID, "format": "STEP", "requestId": runID + "/reimport"})
		var nextID string
		if err := db.QueryRow(t.Context(), `INSERT INTO occccad.jobs(job_type,requested_by_user_id,input_object_id,payload,idempotency_key,user_visible,state,lease_owner,lease_expires_at,attempt_count) VALUES('EXCHANGE_IMPORT',$1,$2,$3,$4,true,'RUNNING',$5,now()+interval '1 hour',1) RETURNING id::text`, actor, object.ID, nextPayload, runID+"/reimport", owner).Scan(&nextID); err != nil {
			t.Fatal(err)
		}
		next, err := queue.Get(t.Context(), nextID)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.execute(t.Context(), next); err != nil {
			t.Fatal(err)
		}
		next, err = queue.Get(t.Context(), nextID)
		if err != nil || next.DocumentID == nil {
			t.Fatalf("reimport: %v", err)
		}
		restored, err := service.GetDocument(t.Context(), *next.DocumentID)
		if err != nil {
			t.Fatal(err)
		}
		if len(restored.ResolvedInstances) != 201 || len(restored.Product.Instances) != 3 || restored.Product.Instances[0].ReferencedDocumentID != restored.Product.Instances[1].ReferencedDocumentID {
			t.Fatal("reimport lost shared hierarchy")
		}
		var total int
		if err := db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.documents WHERE folder_id=$1 AND deleted_at IS NULL`, folder.ID).Scan(&total); err != nil || total != 8 {
			t.Fatalf("roundtrip duplicated definitions %d %v", total, err)
		}
		for i, original := range product.ResolvedInstances {
			actual := restored.ResolvedInstances[i]
			for j := 0; j < 3; j++ {
				if math.Abs(actual.Translation[j]-original.Translation[j]) > 1e-6 {
					t.Fatal("roundtrip changed occurrence placement")
				}
			}
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
	t.Logf("%s: %d unique definitions, hierarchical Product, all FACE bindings valid, monotonic progress; elapsed=%s", filepath.Base(sourcePath), expected, time.Since(started).Round(time.Second))
}
