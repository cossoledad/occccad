package assistant

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

// These failures establish a rejected plan/input, rather than an uncertain
// effect or a decision that belongs to the user. A repair gets a new stage;
// the original command and its baseline are never changed or replayed here.
func repairableStageFault(f *automation.Fault) bool {
	if f == nil {
		return false
	}
	switch f.Code {
	case "INVALID_BINDING", "INVALID_ARGUMENTS", "INVALID_ACTION", "VALIDATION_FAILED",
		"OPERATION_CATALOG_CHANGED", "OPERATION_VERSION_UNSUPPORTED", "OPERATION_UNSUPPORTED",
		"WORKSPACE_HEAD_CONFLICT", "FEATURE_EDIT_STALE", "TOPOLOGY_SELECTION_STALE", "TOPOLOGY_SCOPE_MISMATCH",
		"INVALID_SELECTION", "SELECTION_NOT_FOUND", "SELECTION_AMBIGUOUS", "SELECTION_TYPE_MISMATCH",
		"SELECTION_CONTRACT_MISMATCH", "INPUT_ERROR":
		return true
	}
	return false
}

// stageFailureIntent omits task, step, request and preview identities. Binding
// selectors and actual source outputs remain: changing a role/ID/reference is
// a different repair strategy, while renaming local step labels is not.
func stageFailureIntent(task automation.Task) any {
	for _, step := range task.Steps {
		if step.Result != nil {
			continue
		}
		arguments := step.Arguments
		if step.Action != nil {
			arguments = step.Action.Arguments
		}
		args, err := automation.DecodeObject(arguments)
		if err != nil {
			args = map[string]any{"invalidArguments": string(arguments)}
		}
		bindings := append([]automation.InputBinding{}, step.TypedBindings...)
		for target, source := range step.Bindings {
			bindings = append(bindings, automation.InputBinding{Target: target, SourceStepID: source, Representation: "ENTITY_ID"})
		}
		references := make([]map[string]any, 0, len(bindings))
		for _, binding := range bindings {
			ref := map[string]any{"target": binding.Target, "outputId": binding.OutputID, "outputRole": binding.OutputRole, "representation": binding.Representation}
			for _, source := range task.Steps {
				if source.ID != binding.SourceStepID {
					continue
				}
				ref["sourceOperation"] = source.OperationID
				ref["sourceVersion"] = source.Version
				if source.Result != nil {
					outputs := append([]automation.TypedOutput{}, source.Result.TypedOutputs...)
					if len(outputs) == 0 && source.Result.Output != nil {
						outputs = append(outputs, automation.TypedOutput{Role: "PRIMARY", Entity: *source.Result.Output})
					}
					sort.Slice(outputs, func(i, j int) bool { return string(marshalJSON(outputs[i])) < string(marshalJSON(outputs[j])) })
					ref["outputs"] = outputs
				}
				break
			}
			references = append(references, ref)
		}
		sort.Slice(references, func(i, j int) bool { return string(marshalJSON(references[i])) < string(marshalJSON(references[j])) })
		return map[string]any{"operationId": step.OperationID, "version": step.Version, "arguments": args, "bindings": references}
	}
	return nil
}

func (s *Service) failStage(ctx context.Context, r Run, task automation.Task, failure *automation.Fault) error {
	if task.Status != "STOPPED" && task.Status != "COMPLETED" {
		var err error
		task, err = s.automation.Control(ctx, r.ActorID, r.DocumentID, task.ID, automation.Control{Action: "stop", ExpectedVersion: task.Version, RunID: task.RunID, PlanEpoch: task.PlanEpoch})
		if err != nil {
			var f *automation.Fault
			if errors.As(err, &f) && f.Code == "TASK_VERSION_CONFLICT" {
				return s.stageWait(ctx, r, "WAITING_RETRY", "阶段状态已变化，先核对原操作结果", time.Second, f)
			}
			return err
		}
	}
	// Even claim rejection may follow earlier commits that have not reached
	// the Run checkpoint. Only exact original receipts can release this stage.
	task, err := s.automation.ReconcileTaskReceipts(ctx, r.ActorID, r.DocumentID, task.ID)
	if err != nil {
		var f *automation.Fault
		if errors.As(err, &f) && (f.Code == "AUTOMATION_BUSY" || f.Code == "TASK_VERSION_CONFLICT") {
			return s.stageWait(ctx, r, "WAITING_RETRY", "正在核对旧阶段的真实回执，尚未开始修复", time.Second, f)
		}
		return err
	}
	if taskUnknown(task) {
		if err = s.checkpointTask(ctx, r, task, "", "", false, false, ""); err != nil {
			return err
		}
		return s.stageWait(ctx, r, "WAITING_RETRY", "原提交结果未知，先按原请求恢复回执", 2*time.Second, taskFailure(task))
	}
	for _, step := range task.Steps {
		if step.Handoff != nil && step.Result == nil && (step.Phase == "HANDED_OFF" || step.Phase == "UI_OPENED" || step.Phase == "UI_UNKNOWN") {
			return s.stageWait(ctx, r, "WAITING_UI", "原交互结果仍需核对；系统不会重放工具", 0, step.Failure)
		}
	}
	canRepair := repairableStageFault(failure)
	err = s.checkpointTaskUpdate(ctx, r, task, "", "FAILED", true, false, "", func(tx database.Tx, current *Run, wasActive bool) error {
		if !wasActive {
			return nil // A replay of this same stage is not a second failed repair.
		}
		var head workspace.CommandBaseline
		if err := tx.QueryRow(ctx, `SELECT id::text,head_revision_id::text,head_sequence FROM occccad.workspaces WHERE id=$1 AND document_id=$2`, task.Baseline.WorkspaceID, r.DocumentID).Scan(&head.WorkspaceID, &head.RevisionID, &head.Sequence); err != nil {
			return err
		}
		current.LatestRevisionID = head.RevisionID
		digest := modelcore.ValueDigest(marshalJSON(map[string]any{"head": head, "goalVersion": current.GoalVersion, "catalog": s.automation.Catalog.Digest, "intent": stageFailureIntent(task), "diagnostic": map[string]any{"code": failure.Code, "message": failure.Message, "phase": failure.Phase}}))
		if !terminal(current.Status) && current.Status != "PAUSED" {
			current.Failure = failure
			// Known rejected geometry is settled evidence, not an uncertain
			// commit. Let the agent either repair it or end with an honest reply.
			current.Status = "PLANNING"
			current.WaitingReason = "操作未应用，正在根据诊断处理本次请求"
			if failure.Code == "FORBIDDEN" {
				current.Status = "WAITING_INPUT"
				current.WaitingReason = failure.Message
			}
			if canRepair {
				if current.LastStageFailureDigest == digest {
					if err := s.event(ctx, tx, current, "REPEATED_DIAGNOSTIC", map[string]string{"source": "CAD_STAGE", "digest": digest, "taskId": task.ID}); err != nil {
						return err
					}
				}
				current.LastStageFailureDigest = digest
			}
		}
		message := "操作未应用，已保留诊断。"
		if canRepair {
			message = "上一步未应用，正在读取最新结果并调整操作。"
		}
		return s.event(ctx, tx, current, "STAGE_DIAGNOSTIC", map[string]any{"taskId": task.ID, "failure": failure, "inputSnapshot": task.Baseline, "steps": task.Steps, "repairable": canRepair, "semanticDigest": digest, "message": message, "goalVersion": current.GoalVersion})
	})
	if err == nil {
		// A rejected preview/binding may leave the Head unchanged. Invalidate
		// the old language working set so its next request includes this durable
		// diagnosis and the exact applied prefix, even on that same Head.
		s.runMu.Lock()
		delete(s.sessions, r.ID)
		s.runMu.Unlock()
	}
	return err
}
