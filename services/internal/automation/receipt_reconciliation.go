package automation

import (
	"context"
	"errors"
	"time"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

// ReconcileTaskReceipts reads original command identities only. It is safe for
// stopped/superseded plans, and never prepares, commits, or replays an action.
func (s *Service) ReconcileTaskReceipts(ctx context.Context, actor, document, id string) (Task, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return Task{}, err
	}
	task, err := loadTask(ctx, s.db, actor, document, id)
	if err != nil {
		return task, err
	}
	originalVersion := task.Version
	var leaseUntil int64
	if err = s.db.QueryRow(ctx, `SELECT lease_until FROM occccad.automation_tasks WHERE id=$1 AND actor_id=$2 AND document_id=$3`, id, actor, document).Scan(&leaseUntil); err != nil {
		return task, err
	}
	if leaseUntil > time.Now().UnixMilli() {
		return task, &Fault{Code: "AUTOMATION_BUSY", Message: "原操作仍在执行边界内，等待其回执归档后再继续。", Retryable: true}
	}
	changed := false
	for i := range task.Steps {
		step := &task.Steps[i]
		if step.Handoff != nil && step.Result == nil {
			input := Control{Action: "ui_complete", StepID: step.ID, HandoffID: step.Handoff.ID, DomainRequestID: step.Handoff.DomainRequestID}
			if _, findErr := s.workspace.FindCommandReceipt(ctx, document, actor, step.Handoff.DomainRequestID); errors.Is(findErr, workspace.ErrNotFound) {
				continue
			} else if findErr != nil {
				return task, findErr
			}
			result, err := s.validateHandoffReceipt(ctx, actor, document, task, input)
			if err != nil {
				return task, err
			}
			if err := s.controlHandoff(&task, input, &result); err != nil {
				return task, err
			}
			changed = true
			continue
		}
		if step.Action == nil || step.Result != nil {
			continue
		}
		result, recoveryErr := s.recoverAction(ctx, actor, document, *step.Action)
		if errors.Is(recoveryErr, workspace.ErrNotFound) {
			if step.Phase == "COMMITTING" {
				step.Phase = "RESULT_UNKNOWN"
				step.Failure = &Fault{Code: "COMMIT_RESULT_UNKNOWN", Message: "原提交尚无确定回执；保留原请求身份，不能以新请求重放。", Retryable: true}
				if task.Status == "ACTIVE" {
					task.Status = "PAUSED"
				}
				task.event(step.ID, "COMMIT_RESULT_UNKNOWN")
				changed = true
			}
			continue
		}
		if recoveryErr != nil {
			return task, recoveryErr
		}
		step.Result = &result
		step.Phase = "APPLIED"
		step.Failure, step.Preview, step.Confirmation = nil, nil, nil
		if result.Receipt.Sequence >= task.Baseline.Sequence {
			task.Baseline = result.Receipt.CommandBaseline
		}
		task.event(step.ID, "RECEIPT_RECOVERED")
		changed = true
	}
	if !changed {
		return task, nil
	}
	if task.Status == "ACTIVE" && task.pending() == nil {
		task.Status = "COMPLETED"
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return task, err
	}
	defer tx.Rollback(ctx)
	var currentVersion uint64
	if err = tx.QueryRow(ctx, `SELECT version,lease_until FROM occccad.automation_tasks WHERE id=$1 AND actor_id=$2 AND document_id=$3 FOR UPDATE`, id, actor, document).Scan(&currentVersion, &leaseUntil); err != nil {
		return task, err
	}
	if leaseUntil > time.Now().UnixMilli() {
		return task, &Fault{Code: "AUTOMATION_BUSY", Message: "original operation crossed a new in-flight boundary", Retryable: true}
	}
	if currentVersion != originalVersion {
		return task, fault("TASK_VERSION_CONFLICT", "task changed while recovering exact receipts")
	}
	if err = saveTask(ctx, tx, &task, actor); err != nil {
		return task, err
	}
	if _, err = tx.Exec(ctx, `UPDATE occccad.automation_tasks SET lease_token='',lease_until=0 WHERE id=$1`, id); err != nil {
		return task, err
	}
	return task, tx.Commit(ctx)
}
