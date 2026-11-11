//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/control"
	"github.com/occccad/occccad/internal/exchange"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/geometryrpc"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
)

type exchangeFixture struct {
	h             handler
	actor, folder string
}

func newExchangeFixture(t *testing.T) exchangeFixture {
	t.Helper()
	environment := testsupport.Environment(t)
	db := testsupport.OpenDevelopmentDatabase(t)
	id := uuid.NewString()
	var actor string
	if err := db.QueryRow(t.Context(), `INSERT INTO occccad.users(email,display_name) VALUES($1,'Exchange regression') RETURNING id::text`, id+"@exchange-test.invalid").Scan(&actor); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(context.Background(), `UPDATE occccad.users SET status='DISABLED' WHERE id=$1`, actor) })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	pool := control.NewGeometryPool(t.Context(), control.GeometryPoolConfig{WorkerBinary: os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN"), WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: environment.DataDirectory, LogDirectory: environment.LogDirectory, MinimumWorkers: 1, MaximumWorkers: 4, GeometryCapacity: 100})
	if err := pool.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(geometryrpc.ServerOptions()...)
	workerv1.RegisterGeometryWorkerServer(server, pool)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	store, local, err := artifact.OpenConfigured(t.Context(), environment.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	client, err := geometry.Open(listener.Addr().String(), geometry.ArtifactStaging(store, local))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	artifacts := artifact.NewService(db, store, local)
	service := workspace.NewWithArtifacts(db, client, artifacts)
	folder, err := service.CreateFolder(t.Context(), workspace.CreateFolderRequest{Name: "Exchange regression " + id, ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.DeleteFolder(context.Background(), folder.ID); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(func() {
		db.Exec(context.Background(), `UPDATE occccad.jobs SET state='CANCELED',cancel_requested_at=now(),completed_at=now() WHERE requested_by_user_id=$1 AND state IN ('QUEUED','RETRY_WAIT','RUNNING')`, actor)
	})
	return exchangeFixture{handler{importBudget: newImportBudget(4), workerID: "exchange-" + id, database: db, queue: jobs.New(db), artifacts: artifacts, access: access.New(db), geometry: client, workspace: service}, actor, folder.ID}
}

func (f exchangeFixture) importFile(t *testing.T, name string, data []byte) (jobs.Job, error) {
	t.Helper()
	source, err := f.h.artifacts.Put(t.Context(), artifact.KindExchangeSource, "model/step", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	payload, _ := json.Marshal(map[string]string{"fileName": name, "format": "STEP", "folderId": f.folder, "requestId": id})
	var jobID string
	if err := f.h.database.QueryRow(t.Context(), `INSERT INTO occccad.jobs(job_type,requested_by_user_id,input_object_id,payload,idempotency_key,user_visible,state,lease_owner,lease_expires_at,attempt_count) VALUES('EXCHANGE_IMPORT',$1,$2,$3,$4,true,'RUNNING',$5,now()+interval '1 hour',1) RETURNING id::text`, f.actor, source.ID, payload, id, f.h.workerID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	job, err := f.h.queue.Get(t.Context(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.h.execute(t.Context(), job); err != nil {
		return job, err
	}
	return f.h.queue.Get(t.Context(), jobID)
}

const metadataOnlySTEP = `ISO-10303-21;
HEADER;
FILE_DESCRIPTION(('metadata-only import regression'),'2;1');
FILE_NAME('metadata.step','2026-11-11T00:00:00',(''),(''),'occccad','occccad','');
FILE_SCHEMA(('CONFIG_CONTROL_DESIGN'));
ENDSEC;
DATA;
#1=APPLICATION_CONTEXT('configuration controlled 3d designs of mechanical parts and assemblies');
#2=APPLICATION_PROTOCOL_DEFINITION('international standard','config_control_design',1994,#1);
#3=PRODUCT_CONTEXT('',#1,'mechanical');
#4=PRODUCT('metadata','Metadata only','',(#3));
#5=PRODUCT_DEFINITION_FORMATION('','',#4);
#6=PRODUCT_DEFINITION_CONTEXT('part definition',#1,'design');
#7=PRODUCT_DEFINITION('design','',#5,#6);
ENDSEC;
END-ISO-10303-21;
`

func TestReadableUnsupportedContentCreatesEmptyPartAndWarnings(t *testing.T) {
	f := newExchangeFixture(t)
	job, err := f.importFile(t, "metadata.step", []byte(metadataOnlySTEP))
	if err != nil {
		t.Fatal(err)
	}
	if job.State != "SUCCEEDED" || job.DocumentID == nil {
		t.Fatalf("no successful document: %+v", job)
	}
	view, err := f.h.workspace.GetDocument(t.Context(), *job.DocumentID)
	if err != nil || view.Part == nil || len(view.Part.Features) != 0 {
		t.Fatal("empty Part was not created", err)
	}
	var payload struct {
		Exchange exchange.Report `json:"exchange"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Exchange.EmptyParts != 1 || len(payload.Exchange.Warnings) == 0 || payload.Exchange.SourceSHA256 == "" || payload.Exchange.KernelVersion == "" {
		t.Fatalf("completion lost warning/provenance: %s", job.Payload)
	}
	// Re-read through a fresh service, as the message center does after reload.
	stored, err := jobs.New(f.h.database).Get(t.Context(), job.ID)
	if err != nil || !bytes.Equal(stored.Payload, job.Payload) {
		t.Fatal("warnings were not durable", err)
	}
	if _, err := f.importFile(t, "broken.step", []byte("not a STEP file")); err == nil {
		t.Fatal("corrupt file was accepted")
	}
	// Supplemental curves need not be Part geometry (graphical PMI uses them).
	// Their presence cannot turn a readable metadata-only file into rejection.
	curves := strings.Replace(metadataOnlySTEP, "ENDSEC;\nEND-ISO-10303-21;", `#8=CARTESIAN_POINT('',(0.,0.,0.));
#9=CARTESIAN_POINT('',(1.,0.,0.));
#10=POLYLINE('',(#8,#9));
#11=GEOMETRIC_CURVE_SET('',(#10));
ENDSEC;
END-ISO-10303-21;`, 1)
	supplemental, err := f.importFile(t, "supplemental-only.step", []byte(curves))
	if err != nil || supplemental.State != "SUCCEEDED" || supplemental.DocumentID == nil || len(supplemental.Warnings) == 0 {
		t.Fatal("supplemental content prevented document creation", err)
	}
}

func TestNistSTEPImportCorpus(t *testing.T) {
	root, err := config.ProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(os.Getenv("OCCCCAD_TEST_RESOURCES_DIR"), "nist-step")
	manifest, err := os.ReadFile(filepath.Join(root, "tests/exchange-corpus/manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Packages []struct {
			ID    string
			Files []struct {
				Path, SHA256 string
				Bytes        int
				Tessellated  bool
				SolidCount   int
			}
		}
	}
	if err := json.Unmarshal(manifest, &corpus); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); err != nil {
		t.Skip("NOT_APPLICABLE: NIST corpus is not prepared; see tests/exchange-corpus/README.md")
	}
	f := newExchangeFixture(t)
	var evidence []map[string]any
	defer func() {
		data, _ := json.MarshalIndent(evidence, "", "  ")
		os.WriteFile(filepath.Join(directory, "import-results.json"), append(data, '\n'), 0600)
	}()
	for _, pkg := range corpus.Packages {
		for _, entry := range pkg.Files {
			t.Run(pkg.ID+"/"+filepath.Base(entry.Path), func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(directory, entry.SHA256+".stp"))
				if err != nil {
					t.Fatal(err)
				}
				digest := sha256.Sum256(data)
				if len(data) != entry.Bytes || hex.EncodeToString(digest[:]) != entry.SHA256 {
					t.Fatal("corpus integrity mismatch")
				}
				started := time.Now()
				job, err := f.importFile(t, filepath.Base(entry.Path), data)
				row := map[string]any{"package": pkg.ID, "file": entry.Path, "sha256": entry.SHA256, "wallMs": float64(time.Since(started).Microseconds()) / 1000}
				evidence = append(evidence, row)
				if err != nil {
					row["error"] = err.Error()
					t.Fatal(err)
				}
				row["state"] = job.State
				row["payload"] = json.RawMessage(job.Payload)
				if job.State != "SUCCEEDED" || job.DocumentID == nil {
					t.Fatal("no document")
				}
				view, err := workspace.NewWithArtifacts(f.h.database, f.h.geometry, f.h.artifacts).GetDocument(t.Context(), *job.DocumentID)
				if err != nil {
					t.Fatal(err)
				}
				if entry.SolidCount > 0 && view.Part != nil && len(view.Part.Features) == 0 {
					t.Fatal("supported solid silently became an empty Part")
				}
				// The source BREP is retained as input and has no business Naming.
				// Check the accepted Feature/Body outputs, not those source artifacts.
				if view.Part != nil {
					for _, body := range view.Part.Bodies {
						if body.GeometryKey != "" && view.Artifacts[body.GeometryKey].BRepBytes > 0 && !view.Artifacts[body.GeometryKey].Naming.CanBind {
							t.Fatal("Body lost stable selection")
						}
					}
					for _, feature := range view.Part.Features {
						if feature.Result != nil && feature.Result.GeometryKey != "" && !view.Artifacts[feature.Result.GeometryKey].Naming.CanBind {
							t.Fatal("independent result lost stable selection")
						}
					}
				}
			})
		}
	}
}
