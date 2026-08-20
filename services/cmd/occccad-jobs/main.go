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
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/exchange"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/thumbnail"
	"github.com/occccad/occccad/internal/workspace"
	"golang.org/x/sync/errgroup"
)

type handler struct {
	workerID  string
	database  *pgxpool.Pool
	queue     *jobs.Service
	access    *access.Service
	artifacts *artifact.Service
	geometry  *geometry.Client
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
		access: access.New(pool), geometry: geometryClient,
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
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	switch job.Type {
	case "EXCHANGE_IMPORT":
		if job.InputObjectID == nil {
			return errors.New("exchange import has no source object")
		}
		if payload.FolderID != "" {
			if _, err := h.access.RequireFolder(ctx, payload.FolderID, job.RequestedBy, access.RoleEditor); err != nil {
				return fmt.Errorf("import destination access changed: %w", err)
			}
		}
		source, err := h.artifacts.Get(ctx, *job.InputObjectID)
		if err != nil {
			return err
		}
		if err := h.queue.UpdateProgress(ctx, job.ID, h.workerID, 15); err != nil {
			return err
		}
		format := strings.ToUpper(payload.Format)
		reference := geometry.ArtifactReference{Backend: source.Backend, ObjectKey: source.Key,
			SHA256: source.SHA256, Size: source.Size, ContentType: source.ContentType}
		inspection, err := h.geometry.InspectExchange(ctx, payload.RequestID+"/inspect", format, reference)
		if err != nil {
			return err
		}
		if len(inspection.Components) == 0 {
			return errors.New("exchange source contains no importable components")
		}
		if err := h.queue.UpdateProgress(ctx, job.ID, h.workerID, 30); err != nil {
			return err
		}
		type imported struct {
			name, key  string
			evaluation *workerv1.EvaluatePartResponse
		}
		results := make([]imported, len(inspection.Components))
		group, groupContext := errgroup.WithContext(ctx)
		group.SetLimit(8)
		for index, component := range inspection.Components {
			index, component := index, component
			group.Go(func() error {
				digest := sha256.Sum256([]byte("exchange-v1\x00" + source.SHA256 + fmt.Sprintf("/%d", component.SourceIndex)))
				key := "sha256:" + hex.EncodeToString(digest[:])
				prefix := fmt.Sprintf("%s/component-%d", job.ID, component.SourceIndex)
				evaluation, err := h.geometry.ImportExchange(groupContext, payload.RequestID+fmt.Sprintf("/component/%d", component.SourceIndex),
					key, format, reference, component.SourceIndex,
					artifact.StagingKey(prefix, "shape.brep"), artifact.StagingKey(prefix, "mesh.glb"))
				if err != nil {
					return err
				}
				results[index] = imported{name: component.Name, key: key, evaluation: evaluation}
				return nil
			})
		}
		if err := group.Wait(); err != nil {
			return err
		}
		if err := h.queue.UpdateProgress(ctx, job.ID, h.workerID, 70); err != nil {
			return err
		}
		baseName := exchange.ImportedDocumentName(payload.FileName)
		parts := make([]workspace.DocumentView, 0, len(results))
		for index, result := range results {
			if err := h.queue.UpdateProgress(ctx, job.ID, h.workerID, 70+(index*20)/len(results)); err != nil {
				return err
			}
			name := baseName
			if len(results) > 1 {
				name = fmt.Sprintf("%s - %s", baseName, result.name)
			}
			view, err := h.workspace.CommitImportedPart(ctx, job.RequestedBy, payload.FolderID,
				payload.RequestID+fmt.Sprintf("/part/%d", index), name, payload.FileName, format, result.key, result.evaluation)
			if err != nil {
				return err
			}
			parts = append(parts, view)
			if err := h.enqueuePreview(ctx, job.RequestedBy, view); err != nil {
				slog.Warn("enqueue imported document preview", "job_id", job.ID, "error", err)
			}
		}
		root := parts[0]
		if inspection.DocumentType == "PRODUCT" {
			root, err = h.workspace.CommitImportedProduct(ctx, job.RequestedBy, payload.FolderID,
				payload.RequestID+"/product", baseName, parts)
			if err != nil {
				return err
			}
			if err := h.enqueuePreview(ctx, job.RequestedBy, root); err != nil {
				slog.Warn("enqueue imported Product preview", "job_id", job.ID, "error", err)
			}
		}
		if err := h.queue.UpdateProgress(ctx, job.ID, h.workerID, 95); err != nil {
			return err
		}
		return h.queue.SucceedImport(ctx, job.ID, h.workerID, root.Document.ID)
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
		_, _, sourceComponents, err := h.workspace.ExchangeExportComponents(ctx, *job.DocumentID)
		if err != nil {
			return err
		}
		if err := h.queue.UpdateProgress(ctx, job.ID, h.workerID, 35); err != nil {
			return err
		}
		components := make([]geometry.ExchangeComponent, 0, len(sourceComponents))
		for _, component := range sourceComponents {
			components = append(components, geometry.ExchangeComponent{Name: component.Name,
				BRep: component.BRep, Translation: component.Translation})
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
		svg, err := thumbnail.Render(current)
		if err != nil {
			return err
		}
		object, err := h.artifacts.Put(ctx, artifact.KindThumbnail, "image/svg+xml", bytes.NewReader(svg))
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
	digest := sha256.Sum256([]byte(view.Document.ID + ":" + view.Document.VersionID + ":preview-v2"))
	identity := hex.EncodeToString(digest[:])
	_, err := h.queue.Enqueue(ctx, jobs.EnqueueRequest{Type: "THUMBNAIL_RENDER",
		DocumentID: view.Document.ID, VersionID: &view.Document.VersionID, RequestedBy: requestedBy,
		IdempotencyKey: identity, Payload: map[string]any{"previewIdentity": identity,
			"rendererVersion": "svg-v2"}})
	return err
}
