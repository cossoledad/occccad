package assistant

import (
	"context"
	"strings"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

type RunControl struct {
	Action          string `json:"action"`
	ExpectedVersion uint64 `json:"expectedVersion"`
	Epoch           uint64 `json:"epoch"`
	TokenBudget     *int64 `json:"tokenBudget"`
	Mode            string `json:"mode,omitempty"`
	TaskID          string `json:"taskId,omitempty"`
	StepID          string `json:"stepId,omitempty"`
	TaskVersion     uint64 `json:"taskVersion,omitempty"`
	PreviewID       string `json:"previewId,omitempty"`
	RevisionID      string `json:"revisionId,omitempty"`
	Outcome         string `json:"outcome,omitempty"`
	RequestID       string `json:"requestId,omitempty"`
	HandoffID       string `json:"handoffId,omitempty"`
}

func (s *Service) ControlRun(ctx context.Context, actor, document, id string, input RunControl) (Run, error) {
	r, err := s.GetRun(ctx, actor, document, id)
	if err != nil {
		return r, err
	}
	if input.Epoch != r.Epoch {
		return r, aiFault("AI_EPOCH_CONFLICT", "这项控制属于旧计划，请读取当前任务", false)
	}
	// Exact display acknowledgment is evidence from the caller's real viewport;
	// it can arrive while the scheduler legitimately advances Run.version.
	if input.Action == "presented" {
		if input.RevisionID == "" {
			return r, aiFault("DISPLAY_MISMATCH", "需要实际呈现的精确Revision", false)
		}
		snapshot, e := s.automation.ReadSnapshot(ctx, actor, document, input.RevisionID)
		if e != nil {
			return r, e
		}
		next, e := s.mutateRun(ctx, id, func(tx database.Tx, current *Run) error {
			if current.Epoch != input.Epoch {
				return context.Canceled
			}
			current.PresentedRevisionID = snapshot.RevisionID
			return s.event(ctx, tx, current, "REVISION_PRESENTED", map[string]string{"documentId": document, "revisionId": snapshot.RevisionID})
		})
		return next, e
	}
	if input.Action == "confirm" || input.Action == "handoff_start" || input.Action == "handoff_finish" {
		retiring := input.Action == "handoff_finish" && r.RetiringTaskID == input.TaskID
		if !retiring && (r.ActiveTaskID == "" || r.ActiveTaskID != input.TaskID) {
			return r, aiFault("AI_STAGE_MISMATCH", "该步骤不属于当前计划", false)
		}
		task, e := s.automation.GetTask(ctx, actor, document, input.TaskID)
		if e != nil {
			return r, e
		}
		if input.TaskVersion != task.Version {
			return r, aiFault("TASK_VERSION_CONFLICT", "步骤已变化，请重新读取", true)
		}
		head := task.Baseline
		epoch := task.PlanEpoch
		control := automation.Control{ExpectedVersion: task.Version, StepID: input.StepID, PreviewID: input.PreviewID, ExpectedHead: &head, PlanEpoch: epoch, RunID: r.ID}
		switch input.Action {
		case "confirm":
			control.Action = "confirm"
		case "handoff_start":
			control.Action = "handoff"
		case "handoff_finish":
			switch strings.ToUpper(input.Outcome) {
			case "UI_OPENED":
				control.Action = "ui_opened"
			case "FAILED":
				control.Action = "ui_failed"
			case "UNKNOWN":
				control.Action = "ui_unknown"
			case "CANCELLED":
				control.Action = "ui_cancelled"
			case "COMPLETED":
				control.Action = "ui_complete"
			default:
				return r, aiFault("AI_HANDOFF_INVALID", "界面回报类型无效", false)
			}
			control.HandoffID = input.HandoffID
			control.DomainRequestID = input.RequestID
			for _, step := range task.Steps {
				if step.ID == input.StepID && step.Handoff != nil && control.HandoffID == "" {
					control.HandoffID = step.Handoff.ID
				}
			}
		}
		updatedTask, e := s.automation.Control(ctx, actor, document, task.ID, control)
		if e != nil {
			return r, e
		}
		next, e := s.mutateRun(ctx, id, func(tx database.Tx, current *Run) error {
			if current.Epoch != r.Epoch {
				return context.Canceled
			}
			current.Status = "EXECUTING"
			switch updatedTask.Status {
			case "WAIT_UI":
				current.Status = "WAITING_UI"
			case "WAIT_CONFIRM":
				current.Status = "WAITING_CONFIRMATION"
			case "PAUSED", "STOPPED":
				current.Status = "PAUSED"
			}
			if retiring && current.Status == "EXECUTING" {
				current.Status = "QUEUED"
			}
			current.WaitingReason = ""
			return s.event(ctx, tx, current, "STEP_CONTROL", input)
		})
		if e == nil {
			s.signal()
		}
		return next, e
	}
	if input.ExpectedVersion != r.Version {
		return r, aiFault("AI_RUN_CONFLICT", "运行已变化，请刷新后重试", true)
	}
	if input.Action == "budget" && !validTokenBudget(input.TokenBudget) {
		return r, aiFault("AI_INPUT_INVALID", "累计预算必须为正整数或留空", false)
	}
	if input.Action == "mode" && input.Mode != "AUTOMATIC" && input.Mode != "CONFIRM" {
		return r, aiFault("AI_INPUT_INVALID", "执行模式无效", false)
	}
	var stopTask string
	next, err := s.mutateRun(ctx, id, func(tx database.Tx, current *Run) error {
		if current.Version != input.ExpectedVersion || current.Epoch != input.Epoch {
			return aiFault("AI_RUN_CONFLICT", "运行状态已变化", true)
		}
		switch input.Action {
		case "pause", "stop", "mode":
			if input.Action == "mode" {
				current.Mode = input.Mode
			}
			current.Epoch++
			stopTask = current.ActiveTaskID
			current.RetiringTaskID = stopTask
			current.ActiveTaskID = ""
			current.PendingCallID = ""
			if input.Action == "pause" {
				current.Status = "PAUSED"
				current.WaitingReason = "已暂停，已应用结果保留"
			} else if input.Action == "stop" {
				current.Status = "STOPPED"
				current.WaitingReason = "已停止，已应用结果保留"
				if _, e := tx.Exec(ctx, `UPDATE occccad.assistant_inputs SET status='CANCELLED' WHERE run_id=$1 AND kind='QUERY' AND status IN ('RECEIVED','WAITING_BUDGET')`, id); e != nil {
					return e
				}
			} else {
				current.Status = "QUEUED"
			}
		case "resume":
			if terminal(current.Status) {
				return aiFault("AI_RUN_FINISHED", "已结束的任务不能恢复", false)
			}
			current.Status = "QUEUED"
			current.WaitingReason = ""
			current.Failure = nil
			current.ConsecutiveTransient = 0
		case "budget":
			current.TokenBudget = input.TokenBudget
			current.BudgetVersion++
			if input.TokenBudget == nil || current.Usage.TotalTokens < *input.TokenBudget {
				if _, e := tx.Exec(ctx, `UPDATE occccad.assistant_inputs SET status='RECEIVED',lease_until=0 WHERE run_id=$1 AND kind='QUERY' AND status='WAITING_BUDGET'`, id); e != nil {
					return e
				}
			}
			if current.Status == "WAITING_BUDGET" {
				current.Status = "QUEUED"
				current.WaitingReason = ""
			}
		default:
			return aiFault("AI_CONTROL_INVALID", "未知的运行控制", false)
		}
		if current.Status == "QUEUED" {
			if _, e := tx.Exec(ctx, `UPDATE occccad.assistant_runs SET next_wake=0 WHERE id=$1`, id); e != nil {
				return e
			}
		}
		return s.event(ctx, tx, current, "USER_CONTROL", map[string]any{"action": input.Action, "budgetVersion": current.BudgetVersion, "budget": current.TokenBudget, "source": "USER", "actorId": actor, "mode": current.Mode})
	})
	if err != nil {
		return next, err
	}
	if input.Action == "pause" || input.Action == "stop" || input.Action == "mode" {
		s.cancelWork(id)
	}
	if input.Action == "stop" {
		rows, e := s.db.Query(ctx, `SELECT id::text FROM occccad.assistant_inputs WHERE run_id=$1 AND kind='QUERY' AND status='CANCELLED'`, id)
		if e == nil {
			ids := []string{}
			for rows.Next() {
				var key string
				if rows.Scan(&key) == nil {
					ids = append(ids, key)
				}
			}
			rows.Close()
			for _, key := range ids {
				s.cancelWork("query:" + key)
			}
		}
	}
	if stopTask != "" {
		task, e := s.automation.GetTask(ctx, actor, document, stopTask)
		if e == nil {
			_, _ = s.automation.Control(ctx, actor, document, stopTask, automation.Control{Action: "stop", ExpectedVersion: task.Version, RunID: task.RunID, PlanEpoch: task.PlanEpoch})
		}
	}
	s.signal()
	return next, nil
}

func baselineOf(r Run) workspace.CommandBaseline {
	return workspace.CommandBaseline{RevisionID: r.LatestRevisionID}
}
