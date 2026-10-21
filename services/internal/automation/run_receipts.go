package automation

import (
	"context"

	"github.com/occccad/occccad/internal/database"
)

// RecordRunReceipts joins the authorized Run's existing short transaction.
// These bounded per-stage markers and the Run counters commit atomically.
// No independent ACL/Workspace/database-pool acquisition happens inside it.
func (s *Service) RecordRunReceipts(ctx context.Context, db database.Executor, actor, document, taskID, runID string, expected uint64, stepIDs []string) (Task, error) {
	task, err := loadTask(ctx, db, actor, document, taskID)
	if err != nil {
		return task, err
	}
	if task.RunID != runID || task.Version != expected {
		return task, fault("TASK_VERSION_CONFLICT", "Run receipt checkpoint changed")
	}
	wanted := map[string]bool{}
	for _, id := range stepIDs {
		if wanted[id] {
			return task, fault("INVALID_RECEIPT_CHECKPOINT", "duplicate step identity")
		}
		wanted[id] = true
	}
	for i := range task.Steps {
		step := &task.Steps[i]
		if !wanted[step.ID] {
			continue
		}
		if step.Result == nil || step.ReceiptRecorded {
			return task, fault("INVALID_RECEIPT_CHECKPOINT", "step lacks a new authoritative receipt")
		}
		step.ReceiptRecorded = true
		delete(wanted, step.ID)
	}
	if len(wanted) > 0 {
		return task, fault("INVALID_RECEIPT_CHECKPOINT", "step does not belong to this bounded stage")
	}
	if len(stepIDs) == 0 {
		return task, nil
	}
	return task, saveTask(ctx, db, &task, actor)
}
