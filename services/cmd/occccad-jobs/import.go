package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/exchange"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/workspace"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func importConcurrency(raw string) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 4
	}
	return value
}

// Shared by every import in one Jobs process. Limits active work, not cached
// Router processes; the Router separately enforces its global process budget.
type importBudget struct {
	maximum int
	slots   *semaphore.Weighted
}

func newImportBudget(maximum int) *importBudget {
	return &importBudget{maximum: maximum, slots: semaphore.NewWeighted(int64(maximum))}
}
func (b *importBudget) run(ctx context.Context, operation func() error) error {
	if err := b.slots.Acquire(ctx, 1); err != nil {
		return err
	}
	defer b.slots.Release(1)
	return operation()
}
func retryableImportError(err error) bool {
	return !errors.Is(err, workspace.ErrValidation) && status.Code(err) != codes.InvalidArgument
}

// Import phases are explicit and counts are persisted after actual completion.
// Mutex orders progress writes without serializing component computation.
func (h handler) importProgress(ctx context.Context, job jobs.Job, phase string, start, end, total int) func() error {
	var mu sync.Mutex
	completed := 0
	return func() error {
		mu.Lock()
		defer mu.Unlock()
		completed++
		return h.queue.UpdateProgressDetail(ctx, job.ID, h.workerID, start+(end-start)*completed/total, &jobs.ProgressDetail{Phase: phase, Completed: completed, Total: total})
	}
}

func (h handler) executeImport(ctx context.Context, job jobs.Job, fileName, folderID, format, requestID string) error {
	if job.InputObjectID == nil {
		return errors.New("exchange import has no source object")
	}
	if folderID != "" {
		if _, err := h.access.RequireFolder(ctx, folderID, job.RequestedBy, access.RoleEditor); err != nil {
			return fmt.Errorf("import destination access changed: %w", err)
		}
	}
	source, err := h.artifacts.Get(ctx, *job.InputObjectID)
	if err != nil {
		return err
	}
	format = strings.ToUpper(format)
	phaseStarted := time.Now()
	if err := h.queue.UpdateProgressDetail(ctx, job.ID, h.workerID, 5, &jobs.ProgressDetail{Phase: "PREPARING"}); err != nil {
		return err
	}
	ref := geometry.ArtifactReference{Backend: source.Backend, ObjectKey: source.Key, SHA256: source.SHA256, Size: source.Size, ContentType: source.ContentType}
	budget := h.importBudget
	if budget == nil {
		return errors.New("import scheduler is not configured")
	}
	if err := budget.slots.Acquire(ctx, 1); err != nil {
		return err
	}
	inspection, err := h.geometry.InspectExchange(ctx, requestID+"/prepare", format, ref, artifact.StagingKey(job.ID, "components"))
	budget.slots.Release(1)
	if err != nil {
		return err
	}
	count := len(inspection.Components)
	slog.Info("exchange preparation completed", "job_id", job.ID, "components", count, "concurrency_limit", min(count, budget.maximum), "duration_ms", time.Since(phaseStarted).Milliseconds())
	phaseStarted = time.Now()
	if count == 0 {
		return errors.New("exchange source contains no importable solids")
	}
	if err := h.queue.UpdateProgressDetail(ctx, job.ID, h.workerID, 15, &jobs.ProgressDetail{Phase: "EVALUATING", Total: count}); err != nil {
		return err
	}
	type imported struct {
		key        string
		evaluation *workerv1.EvaluatePartResponse
	}
	results := make([]imported, count)
	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(min(count, budget.maximum))
	evaluated := h.importProgress(groupContext, job, "EVALUATING", 15, 65, count)
	for index, component := range inspection.Components {
		group.Go(func() error {
			return budget.run(groupContext, func() error {
				if err := groupContext.Err(); err != nil {
					return err
				}
				if component.PreparedBRep.ObjectKey == "" {
					return errors.New("worker did not prepare single-Solid import snapshots")
				}
				snapshot, err := h.artifacts.Adopt(groupContext, artifact.KindBREP, "application/vnd.opencascade.brep", component.PreparedBRep.ObjectKey)
				if err != nil {
					return err
				}
				if snapshot.SHA256 != component.PreparedBRep.SHA256 || snapshot.Size != component.PreparedBRep.Size {
					return errors.New("prepared component integrity mismatch")
				}
				input := geometry.ArtifactReference{Backend: snapshot.Backend, ObjectKey: snapshot.Key, SHA256: snapshot.SHA256, Size: snapshot.Size, ContentType: snapshot.ContentType}
				digest := sha256.Sum256([]byte("exchange-solids-heal-1um-v2\x00" + source.SHA256 + fmt.Sprintf("/%d", component.SourceIndex)))
				key := "sha256:" + hex.EncodeToString(digest[:])
				prefix := fmt.Sprintf("%s/component-%d", job.ID, component.SourceIndex)
				evaluation, err := h.geometry.ImportExchange(groupContext, requestID+fmt.Sprintf("/component/%d", component.SourceIndex), key, "BREP", input, 1, artifact.StagingKey(prefix, "shape.brep"), artifact.StagingKey(prefix, "mesh.glb"))
				if err != nil {
					return fmt.Errorf("component %d (%s): %w", component.SourceIndex, component.Name, err)
				}
				if evaluation.GetTopology().GetSolidCount() != 1 {
					return fmt.Errorf("%w: prepared component is not a single Solid", workspace.ErrValidation)
				}
				results[index] = imported{key, evaluation}
				return evaluated()
			})
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	slog.Info("exchange component evaluation completed", "job_id", job.ID, "components", count, "concurrency_limit", min(count, budget.maximum), "duration_ms", time.Since(phaseStarted).Milliseconds())
	phaseStarted = time.Now()
	// Preserve the existing cancellation boundary before any document is created.
	if err := h.queue.UpdateProgressDetail(ctx, job.ID, h.workerID, 70, &jobs.ProgressDetail{Phase: "CREATING_PARTS", Total: count}); err != nil {
		return err
	}
	parts := make([]workspace.DocumentView, count)
	baseName := exchange.ImportedDocumentName(fileName)
	group, groupContext = errgroup.WithContext(ctx)
	group.SetLimit(min(count, budget.maximum))
	committed := h.importProgress(groupContext, job, "CREATING_PARTS", 70, 94, count)
	for index, result := range results {
		group.Go(func() error {
			return budget.run(groupContext, func() error {
				if err := groupContext.Err(); err != nil {
					return err
				}
				name := baseName
				if count > 1 {
					name = fmt.Sprintf("%s - %s", baseName, inspection.Components[index].Name)
				}
				view, err := h.workspace.CommitImportedPart(groupContext, job.RequestedBy, folderID, requestID+fmt.Sprintf("/part/%d", index), name, fileName, format, result.key, result.evaluation, &workspace.ImportSource{ObjectID: source.ID, SHA256: source.SHA256, Format: format, ComponentIndex: inspection.Components[index].SourceIndex})
				if err != nil {
					return err
				}
				// Product construction needs identity, not another copy of each mesh.
				parts[index] = workspace.DocumentView{Document: view.Document}
				results[index].evaluation = nil
				if err := h.enqueuePreview(groupContext, job.RequestedBy, view); err != nil {
					slog.Warn("enqueue imported document preview", "job_id", job.ID, "error", err)
				}
				return committed()
			})
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	slog.Info("exchange Part creation completed", "job_id", job.ID, "components", count, "concurrency_limit", min(count, budget.maximum), "duration_ms", time.Since(phaseStarted).Milliseconds())
	phaseStarted = time.Now()
	root := parts[0]
	if count > 1 {
		if err := h.queue.UpdateProgressDetail(ctx, job.ID, h.workerID, 95, &jobs.ProgressDetail{Phase: "ASSEMBLING", Completed: count, Total: count}); err != nil {
			return err
		}
		root, err = h.workspace.CommitImportedProduct(ctx, job.RequestedBy, folderID, requestID+"/product", baseName, parts)
		if err != nil {
			return err
		}
		if err := h.enqueuePreview(ctx, job.RequestedBy, root); err != nil {
			slog.Warn("enqueue imported Product preview", "job_id", job.ID, "error", err)
		}
	}
	slog.Info("exchange assembly completed", "job_id", job.ID, "components", count, "concurrency_limit", min(count, budget.maximum), "duration_ms", time.Since(phaseStarted).Milliseconds())
	return h.queue.SucceedImport(ctx, job.ID, h.workerID, root.Document.ID)
}
