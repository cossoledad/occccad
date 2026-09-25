package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/thumbnail"
	"github.com/occccad/occccad/internal/workspace"
	"golang.org/x/sync/errgroup"
)

type handler struct {
	importBudget           *importBudget
	workerID               string
	thumbnailRenderTimeout time.Duration
	database               *database.Pool
	queue                  *jobs.Service
	access                 *access.Service
	artifacts              *artifact.Service
	geometry               *geometry.Client
	workspace              *workspace.Service
}

func main() {
	if err := run(); err != nil {
		slog.Error("job worker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := config.LoadProjectEnv(); err != nil {
		return err
	}
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
	store, localArtifactStore, err := artifact.OpenConfigured(ctx, configuration.DataDirectory)
	if err != nil {
		return err
	}
	artifactService := artifact.NewService(pool, store, localArtifactStore)
	geometryClient, err := geometry.Open(configuration.WorkerAddress, geometry.ArtifactStaging(store, localArtifactStore))
	if err != nil {
		return err
	}
	defer geometryClient.Close()
	hostname, _ := os.Hostname()
	budget := newImportBudget(importConcurrency(os.Getenv("OCCCCAD_IMPORT_CONCURRENCY")))
	concurrency := jobConcurrency(os.Getenv("OCCCCAD_JOB_CONCURRENCY"))
	slog.Info("job workers started", "workers", concurrency, "import_concurrency_max", budget.maximum, "artifact_backend", store.Backend(), "data_directory", localArtifactStore.Root())
	group, groupContext := errgroup.WithContext(ctx)
	for index := 0; index < concurrency; index++ {
		workerID := fmt.Sprintf("%s-%d-%d", hostname, os.Getpid(), index)
		group.Go(func() error {
			h := handler{importBudget: budget, workerID: workerID, thumbnailRenderTimeout: configuration.ThumbnailRenderTimeout, database: pool, queue: jobs.New(pool), artifacts: artifactService, access: access.New(pool), geometry: geometryClient, workspace: workspace.NewWithArtifacts(pool, geometryClient, artifactService)}
			return h.runJobLoop(groupContext)
		})
	}
	return group.Wait()
}

func jobConcurrency(value string) int {
	parsed, err := strconv.Atoi(value)
	if value == "" {
		return 2
	}
	if err != nil || parsed < 1 || parsed > 8 {
		return 2
	}
	return parsed
}

func (h handler) runJobLoop(ctx context.Context) error {
	workerID := h.workerID
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
		jobContext, cancelJob := context.WithCancel(ctx)
		monitorDone := make(chan struct{})
		go h.monitor(jobContext, job.ID, cancelJob, monitorDone)
		err = h.execute(jobContext, job)
		close(monitorDone)
		cancelJob()
		if err != nil {
			finishContext, finishCancel := context.WithTimeout(context.Background(), 3*time.Second)
			cancelRequested, cancelErr := h.queue.CancellationRequested(finishContext, job.ID, workerID)
			if cancelErr == nil && cancelRequested {
				slog.Info("job canceled", "job_id", job.ID, "type", job.Type)
				_ = h.queue.AcknowledgeCanceled(finishContext, job.ID, workerID)
			} else if ctx.Err() == nil {
				slog.Error("execute job", "job_id", job.ID, "type", job.Type, "error", err)
				if job.Type == "EXCHANGE_IMPORT" && !retryableImportError(err) {
					job.MaxAttempts = job.AttemptCount
				}
				_ = h.queue.Fail(finishContext, job, workerID, "PROCESSING_FAILED", err.Error())
			}
			finishCancel()
		}
	}
	return nil
}

func (h handler) monitor(ctx context.Context, jobID string, cancel context.CancelFunc, done <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	heartbeatAt := time.Now().Add(30 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			requested, err := h.queue.CancellationRequested(ctx, jobID, h.workerID)
			if err == nil && requested {
				cancel()
				return
			}
			if time.Now().Before(heartbeatAt) {
				continue
			}
			heartbeatAt = time.Now().Add(30 * time.Second)
			if err := h.queue.Heartbeat(ctx, jobID, h.workerID, 2*time.Minute); err != nil {
				slog.Warn("renew job lease", "job_id", jobID, "error", err)
			}
		}
	}
}

func (h handler) execute(ctx context.Context, job jobs.Job) error {
	if err := h.queue.UpdateProgress(ctx, job.ID, h.workerID, 5); err != nil {
		return err
	}
	var payload struct {
		FileName  string `json:"fileName"`
		FolderID  string `json:"folderId"`
		Format    string `json:"format"`
		RequestID string `json:"requestId"`
		ReleaseID string `json:"releaseId"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	switch job.Type {
	case "EXCHANGE_IMPORT":
		return h.executeImport(ctx, job, payload.FileName, payload.FolderID, payload.Format, payload.RequestID)
	case "EXCHANGE_EXPORT":
		if job.DocumentID == nil {
			return errors.New("exchange export has no document")
		}
		if _, err := h.access.RequireDocument(ctx, *job.DocumentID, job.RequestedBy, access.RoleViewer); err != nil {
			return fmt.Errorf("export document access changed: %w", err)
		}
		current, err := h.workspace.GetDocument(ctx, *job.DocumentID)
		if err != nil {
			return err
		}
		if job.VersionID != nil && current.Document.VersionID != *job.VersionID {
			return errors.New("document head changed after the export job was submitted")
		}
		var sourceComponents []workspace.ExchangeExportComponent
		if payload.ReleaseID != "" {
			_, _, sourceComponents, err = h.workspace.ExchangeReleaseExportComponents(ctx, *job.DocumentID, payload.ReleaseID)
		} else {
			_, _, sourceComponents, err = h.workspace.ExchangeExportComponents(ctx, *job.DocumentID)
		}
		if err != nil {
			return err
		}
		if err := h.queue.UpdateProgress(ctx, job.ID, h.workerID, 35); err != nil {
			return err
		}
		components := make([]geometry.ExchangeComponent, 0, len(sourceComponents))
		for _, component := range sourceComponents {
			components = append(components, geometry.ExchangeComponent{Name: component.Name,
				BRep: component.BRep, Translation: component.Translation, Rotation: component.Rotation})
		}
		outputKey := artifact.StagingKey(job.ID, "export."+strings.ToLower(payload.Format))
		result, err := h.geometry.ExportExchange(ctx, payload.RequestID, strings.ToUpper(payload.Format), outputKey, components)
		if err != nil {
			return err
		}
		if err := h.queue.UpdateProgress(ctx, job.ID, h.workerID, 85); err != nil {
			return err
		}
		object, err := h.artifacts.Adopt(ctx, artifact.KindExchangeExport, result.ContentType, result.ObjectKey)
		if err != nil {
			return err
		}
		if err := h.queue.UpdateProgress(ctx, job.ID, h.workerID, 95); err != nil {
			return err
		}
		return h.queue.Succeed(ctx, job.ID, h.workerID, object.ID)
	case "THUMBNAIL_RENDER":
		if job.DocumentID == nil {
			return errors.New("thumbnail job has no document")
		}
		current, err := h.workspace.GetDocument(ctx, *job.DocumentID)
		if err != nil {
			return err
		}
		if job.VersionID != nil && current.Document.VersionID != *job.VersionID {
			return h.queue.Succeed(ctx, job.ID, h.workerID, "")
		}
		var preview struct {
			PreviewIdentity string `json:"previewIdentity"`
			RendererVersion string `json:"rendererVersion"`
		}
		if err := json.Unmarshal(job.Payload, &preview); err != nil {
			return err
		}
		if preview.RendererVersion != thumbnail.RendererVersion {
			return h.queue.Succeed(ctx, job.ID, h.workerID, "")
		}
		if err := h.workspace.HydrateDisplay(ctx, &current); err != nil {
			return err
		}
		payload, usedDefault, err := thumbnail.RenderContext(ctx, current, h.thumbnailRenderTimeout)
		if err != nil {
			return err
		}
		if usedDefault {
			slog.Warn("thumbnail render exceeded time or scene budget; storing default", "job_id", job.ID,
				"document_id", *job.DocumentID, "timeout", h.thumbnailRenderTimeout)
		}
		object, err := h.artifacts.Put(ctx, artifact.KindThumbnail, thumbnail.ContentType, bytes.NewReader(payload))
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
	digest := sha256.Sum256([]byte(view.Document.ID + ":" + view.Document.VersionID + ":" + thumbnail.RendererVersion))
	identity := hex.EncodeToString(digest[:])
	_, err := h.queue.Enqueue(ctx, jobs.EnqueueRequest{Type: "THUMBNAIL_RENDER",
		DocumentID: view.Document.ID, VersionID: &view.Document.VersionID, RequestedBy: requestedBy,
		IdempotencyKey: identity, Payload: map[string]any{"previewIdentity": identity,
			"rendererVersion": thumbnail.RendererVersion}})
	return err
}
