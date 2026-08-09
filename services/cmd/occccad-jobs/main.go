package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/workspace"
)

type handler struct {
	workerID  string
	database  *pgxpool.Pool
	queue     *jobs.Service
	artifacts *artifact.Service
	workspace *workspace.Service
}

func main() {
	if err := run(); err != nil {
		slog.Error("job worker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configuration := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := database.Open(ctx, configuration.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		return err
	}
	store, err := artifact.NewLocalStore(configuration.DataDirectory)
	if err != nil {
		return err
	}
	geometryClient, err := geometry.Open(configuration.WorkerAddress)
	if err != nil {
		return err
	}
	defer geometryClient.Close()
	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	artifactService := artifact.NewService(pool, store)
	h := handler{workerID: workerID, database: pool, queue: jobs.New(pool), artifacts: artifactService,
		workspace: workspace.NewWithArtifacts(pool, geometryClient, artifactService)}
	slog.Info("job worker started", "worker_id", workerID, "artifact_backend", "LOCAL", "data_directory", store.Root())
	for ctx.Err() == nil {
		job, err := h.queue.Claim(ctx, workerID, 2*time.Minute)
		if errors.Is(err, pgx.ErrNoRows) {
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
			continue
		}
		if err != nil {
			slog.Error("claim job", "error", err)
			continue
		}
		heartbeatDone := make(chan struct{})
		go h.heartbeat(ctx, job.ID, heartbeatDone)
		err = h.execute(ctx, job)
		close(heartbeatDone)
		if err != nil {
			slog.Error("execute job", "job_id", job.ID, "type", job.Type, "error", err)
			_ = h.queue.Fail(ctx, job, workerID, "PROCESSING_FAILED", err.Error())
		}
	}
	return nil
}

func (h handler) heartbeat(ctx context.Context, jobID string, done <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			if err := h.queue.Heartbeat(ctx, jobID, h.workerID, 2*time.Minute); err != nil {
				slog.Warn("renew job lease", "job_id", jobID, "error", err)
			}
		}
	}
}

func (h handler) execute(ctx context.Context, job jobs.Job) error {
	if job.DocumentID == nil {
		return errors.New("job has no document")
	}
	var payload struct {
		FileName  string `json:"fileName"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	current, err := h.workspace.GetDocument(ctx, *job.DocumentID)
	if err != nil {
		return err
	}
	if job.VersionID != nil && current.Document.VersionID != *job.VersionID {
		return errors.New("document head changed after the job was submitted")
	}
	switch job.Type {
	case "STEP_IMPORT":
		if job.InputObjectID == nil {
			return errors.New("STEP import has no source object")
		}
		_, reader, err := h.artifacts.Open(ctx, *job.InputObjectID)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		view, err := h.workspace.ImportStep(ctx, *job.DocumentID, payload.RequestID, payload.FileName, data)
		if err != nil {
			return err
		}
		if err := h.enqueuePreview(ctx, job.RequestedBy, view); err != nil {
			slog.Warn("enqueue imported document preview", "job_id", job.ID, "error", err)
		}
		return h.queue.Succeed(ctx, job.ID, h.workerID, "")
	case "STEP_EXPORT":
		data, _, err := h.workspace.ExportStep(ctx, *job.DocumentID, payload.RequestID)
		if err != nil {
			return err
		}
		object, err := h.artifacts.Put(ctx, artifact.KindStepExport, "application/step", bytes.NewReader(data))
		if err != nil {
			return err
		}
		return h.queue.Succeed(ctx, job.ID, h.workerID, object.ID)
	case "THUMBNAIL_RENDER":
		var preview struct {
			PreviewIdentity string `json:"previewIdentity"`
			RendererVersion string `json:"rendererVersion"`
			Name            string `json:"name"`
			BBox            struct {
				Min [3]float64 `json:"min"`
				Max [3]float64 `json:"max"`
			} `json:"bbox"`
		}
		if err := json.Unmarshal(job.Payload, &preview); err != nil {
			return err
		}
		dx := preview.BBox.Max[0] - preview.BBox.Min[0]
		dy := preview.BBox.Max[1] - preview.BBox.Min[1]
		dz := preview.BBox.Max[2] - preview.BBox.Min[2]
		caption := fmt.Sprintf("%.1f × %.1f × %.1f mm", dx, dy, dz)
		svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="320" height="200" viewBox="0 0 320 200"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#f5f9fc"/><stop offset="1" stop-color="#dbe9f3"/></linearGradient></defs><rect width="320" height="200" fill="url(#g)"/><path d="M160 38l70 39-70 40-70-40z" fill="#84b9dc" stroke="#286e9c" stroke-width="2"/><path d="M90 77v48l70 39v-47z" fill="#5e9fc9" stroke="#286e9c" stroke-width="2"/><path d="M230 77v48l-70 39v-47z" fill="#3e83b2" stroke="#286e9c" stroke-width="2"/><text x="16" y="178" font-family="system-ui,sans-serif" font-size="12" fill="#29485d">%s</text><text x="304" y="178" text-anchor="end" font-family="system-ui,sans-serif" font-size="10" fill="#688093">%s</text></svg>`, html.EscapeString(preview.Name), html.EscapeString(caption))
		object, err := h.artifacts.Put(ctx, artifact.KindThumbnail, "image/svg+xml", bytes.NewReader([]byte(svg)))
		if err != nil {
			return err
		}
		_, err = h.database.Exec(ctx, `INSERT INTO occccad.document_previews(
			document_id,version_id,preview_identity,renderer_version,object_id,state)
			VALUES($1,$2,$3,$4,$5,'READY') ON CONFLICT(document_id,preview_identity)
			DO UPDATE SET object_id=EXCLUDED.object_id,state='READY',error_message=NULL,updated_at=now()`,
			*job.DocumentID, job.VersionID, preview.PreviewIdentity, preview.RendererVersion, object.ID)
		if err != nil {
			return err
		}
		return h.queue.Succeed(ctx, job.ID, h.workerID, object.ID)
	default:
		return fmt.Errorf("unsupported job type %s", job.Type)
	}
}

func (h handler) enqueuePreview(ctx context.Context, requestedBy string, view workspace.DocumentView) error {
	if view.Artifact == nil {
		return nil
	}
	digest := sha256.Sum256([]byte(view.Document.ID + ":" + view.Document.VersionID + ":preview-v1"))
	identity := hex.EncodeToString(digest[:])
	_, err := h.queue.Enqueue(ctx, jobs.EnqueueRequest{Type: "THUMBNAIL_RENDER",
		DocumentID: view.Document.ID, VersionID: &view.Document.VersionID, RequestedBy: requestedBy,
		IdempotencyKey: identity, Payload: map[string]any{"previewIdentity": identity,
			"rendererVersion": "svg-v1", "name": view.Document.Name, "bbox": view.Artifact.BBox}})
	return err
}
