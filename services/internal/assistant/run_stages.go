package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

type appliedStepFact struct {
	stepID     string
	receipt    workspace.CommandReceipt
	targets    map[string]string
	selections map[string]string
	removed    []string
}

func (s *Service) taskFacts(ctx context.Context, r Run, task automation.Task) ([]appliedStepFact, error) {
	return s.taskReceiptFacts(ctx, r, task, false)
}

func (s *Service) taskReceiptFacts(ctx context.Context, r Run, task automation.Task, includeRecorded bool) ([]appliedStepFact, error) {
	facts := []appliedStepFact{}
	targets := map[string]string{}
	for key, id := range r.Targets {
		targets[key] = id
	}
	r.Targets = targets
	var stageVersion uint64
	if err := s.db.QueryRow(ctx, `SELECT goal_version FROM occccad.assistant_stages WHERE run_id=$1 AND task_id=$2`, r.ID, task.ID).Scan(&stageVersion); err != nil && !errors.Is(err, database.ErrNoRows) {
		return nil, err
	}
	for _, step := range task.Steps {
		if step.Result == nil || step.ReceiptRecorded && !includeRecorded {
			continue
		}
		fact := appliedStepFact{stepID: step.ID, receipt: step.Result.Receipt, targets: map[string]string{}, selections: map[string]string{}}
		snapshot, err := s.automation.ReadSnapshot(ctx, r.ActorID, r.DocumentID, step.Result.Receipt.RevisionID)
		if err != nil {
			return nil, err
		}
		ids := map[string]bool{}
		outputs := map[string]bool{}
		for _, change := range step.Result.Receipt.Changes {
			ids[change.EntityID] = true
			if change.Kind == "DELETE" && (change.SlotID == "entity" || change.SlotID == "body.entity") {
				fact.removed = append(fact.removed, change.EntityID)
			}
		}
		for _, output := range step.Result.Outputs {
			ids[output.ID] = true
			outputs[output.ID] = true
			if output.Kind == "BODY" {
				fact.targets["BODY"] = output.ID
			}
		}
		if output := step.Result.Output; output != nil {
			ids[output.ID] = true
			outputs[output.ID] = true
			if output.Kind == "BODY" {
				fact.targets["BODY"] = output.ID
			}
		}
		if snapshot.DocumentType == "PART" {
			var part workspace.PartModel
			if err = json.Unmarshal(snapshot.Model, &part); err != nil {
				return nil, err
			}
			for _, parameter := range part.Parameters {
				if ids[parameter.ParameterID] && parameter.OwnerFeatureID != "" {
					ids[parameter.OwnerFeatureID] = true
					outputs[parameter.OwnerFeatureID] = true
				}
			}
			for _, feature := range part.Features {
				if !outputs[feature.ID] {
					continue
				}
				for _, requirement := range r.Requirements {
					if requirement.BodyID != "" && feature.BodyID != requirement.BodyID {
						continue
					}
					if s.goalReceiptCandidate(ctx, r, requirement, part, feature, snapshot.RevisionID, stageVersion) {
						key := goalTargetKey(requirement.ID)
						fact.targets[key] = feature.ID
						if feature.Type == "FILLET" {
							if token, e := s.selectionSetForFeature(ctx, r, feature); e == nil {
								fact.selections[key] = token
							}
						}
					}
				}
				role := feature.Type
				if feature.Sketch != nil {
					role = "SKETCH"
				}
				if feature.Type == "PAD" {
					role = "LINEAR_EXTRUDE"
				}
				switch role {
				case "SKETCH", "LINEAR_EXTRUDE", "FILLET", "CHAMFER":
					fact.targets[role] = feature.ID
					if feature.BodyID != "" {
						fact.targets["BODY"] = feature.BodyID
					}
					// Interactive tools do not have a model-authored Action with a
					// selectionSet. Reconstruct their exact formal input collection
					// from the committed feature, never from the live UI selection.
					if (role == "FILLET" || role == "CHAMFER") && step.Handoff != nil {
						if token, e := s.selectionSetForFeature(ctx, r, feature); e == nil {
							fact.selections[role] = token
						} else {
							fact.selections[role+"_EVIDENCE_ERROR"] = e.Error()
						}
					}
				}
			}
		} else if snapshot.DocumentType == "PRODUCT" {
			var product workspace.ProductModel
			if err = json.Unmarshal(snapshot.Model, &product); err != nil {
				return nil, err
			}
			for _, constraint := range product.Constraints {
				if !ids[constraint.ID] {
					continue
				}
				switch constraint.Kind {
				case "FIX":
					fact.targets["FIX_CONSTRAINT"] = constraint.ID
				case "CONCENTRIC":
					fact.targets["AXIS_CONSTRAINT"] = constraint.ID
				case "COINCIDENT", "DISTANCE":
					fact.targets["PLANE_CONSTRAINT"] = constraint.ID
				}
				fact.targets["ASSEMBLY_CONSTRAINT"] = constraint.ID
			}
		}
		if step.Action != nil {
			if args, err := automation.DecodeObject(step.Action.Arguments); err == nil {
				if token, ok := args["selectionSet"].(string); ok && token != "" {
					if _, exists := fact.targets["FILLET"]; exists {
						fact.selections["FILLET"] = token
					}
					if _, exists := fact.targets["CHAMFER"]; exists {
						fact.selections["CHAMFER"] = token
					}
				}
			}
		}
		for key, id := range fact.targets {
			if strings.HasPrefix(key, "GOAL:") {
				r.Targets[key] = id
			}
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

func (s *Service) selectionSetForFeature(ctx context.Context, r Run, feature workspace.Feature) (string, error) {
	if len(feature.Selections) == 0 {
		return "", aiFault("SELECTION_EVIDENCE_MISSING", "已提交特征没有正式输入边选择", false)
	}
	source := feature.Selections[0].SourceVersionID
	ids := []string{}
	seen := map[string]bool{}
	for _, selection := range feature.Selections {
		if source == "" || selection.SourceVersionID != source || selection.Selection.SourceDocumentID != r.DocumentID || selection.Selection.SourceBodyID != feature.BodyID || selection.Selection.ExpectedType != "EDGE" {
			return "", aiFault("SELECTION_SCOPE_MISMATCH", "特征输入选择没有一致的正式Revision/Body范围", false)
		}
		resolved, err := s.automation.ResolveSelectionForSnapshotWithEvidence(ctx, r.ActorID, r.DocumentID, source, selection)
		if err != nil {
			return "", err
		}
		if seen[resolved.TopologyID] {
			return "", aiFault("SELECTION_EVIDENCE_MISMATCH", "已提交特征含重复输入边", false)
		}
		seen[resolved.TopologyID] = true
		ids = append(ids, resolved.TopologyID)
	}
	query := automation.TopologyQuery{RevisionID: source, BodyID: feature.BodyID, Kind: "EDGE", Limit: 1}
	if len(ids) <= 100 {
		query.IDs = ids
	}
	page, err := s.automation.QueryTopology(ctx, r.ActorID, r.DocumentID, query)
	if err != nil {
		return "", err
	}
	if page["matchedCount"] != len(ids) {
		return "", aiFault("SELECTION_RESOURCE", "超过当前显式子集容量，且特征输入不是完整Body边集，不能伪造完整选择", false)
	}
	return page["selectionSet"].(string), nil
}

// checkpointTask records only newly observed exact receipts. The markers live
// in the bounded Task, so the Run's active JSON never grows with total steps.
func (s *Service) checkpointTask(ctx context.Context, r Run, task automation.Task, status, stageStatus string, clearActive, clearRetiring bool, reason string) error {
	return s.checkpointTaskUpdate(ctx, r, task, status, stageStatus, clearActive, clearRetiring, reason, nil)
}

func (s *Service) checkpointTaskUpdate(ctx context.Context, r Run, task automation.Task, status, stageStatus string, clearActive, clearRetiring bool, reason string, update func(database.Tx, *Run, bool) error) error {
	facts, err := s.taskFacts(ctx, r, task)
	if err != nil {
		return err
	}
	_, err = s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		if current.Epoch != r.Epoch {
			return context.Canceled
		}
		wasActive := current.ActiveTaskID == task.ID
		wasRetiring := current.RetiringTaskID == task.ID
		if current.Targets == nil {
			current.Targets = map[string]string{}
		}
		if current.TargetSelections == nil {
			current.TargetSelections = map[string]string{}
		}
		stepIDs := make([]string, 0, len(facts))
		for _, fact := range facts {
			stepIDs = append(stepIDs, fact.stepID)
			current.StepCount++
			if !fact.receipt.NoChange {
				current.AppliedCount++
				current.LatestAppliedRevisionID = fact.receipt.RevisionID
			}
			current.LatestRevisionID = fact.receipt.RevisionID
			forgetRemovedTargets(current.Targets, current.TargetSelections, fact.removed)
			for role, id := range fact.targets {
				// Bind once to the formal output, not the last feature of that type.
				// A later circle/cut must not replace the rectangle/base pad.
				if strings.HasPrefix(role, "GOAL:") && current.Targets[role] != "" && current.Targets[role] != id {
					continue
				}
				current.Targets[role] = id
			}
			for role, token := range fact.selections {
				current.TargetSelections[role] = token
			}
			if err := s.event(ctx, tx, current, "CAD_APPLIED", map[string]any{"taskId": task.ID, "stepId": fact.stepID, "receipt": fact.receipt, "presented": false, "message": s.appliedStepMessage(task, fact.stepID), "goalVersion": current.GoalVersion, "sourceEpoch": task.PlanEpoch}); err != nil {
				return err
			}
		}
		if len(stepIDs) > 0 {
			if _, err := s.automation.RecordRunReceipts(ctx, tx, r.ActorID, r.DocumentID, task.ID, r.ID, task.Version, stepIDs); err != nil {
				return err
			}
		}
		if stageStatus != "" {
			if _, err := tx.Exec(ctx, `UPDATE occccad.assistant_stages SET status=$1 WHERE run_id=$2 AND task_id=$3`, stageStatus, r.ID, task.ID); err != nil {
				return err
			}
		}
		if clearActive && current.ActiveTaskID == task.ID {
			current.ActiveTaskID = ""
		}
		if clearRetiring && current.RetiringTaskID == task.ID {
			current.RetiringTaskID = ""
		}
		if status != "" && (wasActive || wasRetiring) && !terminal(current.Status) && current.Status != "PAUSED" {
			current.Status = status
			current.WaitingReason = reason
		}
		if clearActive && wasActive || clearRetiring && wasRetiring {
			if err := s.event(ctx, tx, current, "STAGE_BOUNDARY", map[string]any{"taskId": task.ID, "status": stageStatus, "goalVersion": current.GoalVersion, "planEpoch": task.PlanEpoch}); err != nil {
				return err
			}
		}
		if update != nil {
			return update(tx, current, wasActive)
		}
		return nil
	})
	if err == nil {
		s.signal()
	}
	return err
}

func (s *Service) stageWait(ctx context.Context, r Run, status, reason string, delay time.Duration, failure *automation.Fault) error {
	_, err := s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		if current.Epoch != r.Epoch {
			return context.Canceled
		}
		if !terminal(current.Status) && current.Status != "PAUSED" {
			current.Status = status
			current.WaitingReason = reason
			current.Failure = failure
		}
		if delay > 0 {
			_, err := tx.Exec(ctx, `UPDATE occccad.assistant_runs SET next_wake=$1 WHERE id=$2`, s.now().Add(delay).UnixMilli(), r.ID)
			return err
		}
		return nil
	})
	return err
}

func taskUnknown(task automation.Task) bool {
	for _, step := range task.Steps {
		if step.Phase == "COMMITTING" || step.Phase == "RESULT_UNKNOWN" {
			return true
		}
	}
	return false
}
func taskFailure(task automation.Task) *automation.Fault {
	for _, step := range task.Steps {
		if step.Failure != nil {
			return step.Failure
		}
	}
	return nil
}

func (s *Service) retireStage(ctx context.Context, r Run) error {
	task, err := s.automation.GetTask(ctx, r.ActorID, r.DocumentID, r.RetiringTaskID)
	if err != nil {
		return err
	}
	if task.RunID != r.ID {
		return aiFault("AI_STAGE_MISMATCH", "待结束阶段不属于该运行", false)
	}
	if task.Status != "STOPPED" && task.Status != "COMPLETED" {
		task, err = s.automation.Control(ctx, r.ActorID, r.DocumentID, task.ID, automation.Control{Action: "stop", ExpectedVersion: task.Version, RunID: task.RunID, PlanEpoch: task.PlanEpoch})
		if err != nil {
			var f *automation.Fault
			if errors.As(err, &f) && f.Code == "TASK_VERSION_CONFLICT" {
				s.signal()
				return nil
			}
			return err
		}
	}
	task, err = s.automation.ReconcileTaskReceipts(ctx, r.ActorID, r.DocumentID, task.ID)
	if err != nil {
		var f *automation.Fault
		if errors.As(err, &f) && (f.Code == "AUTOMATION_BUSY" || f.Code == "TASK_VERSION_CONFLICT") {
			return s.stageWait(ctx, r, "WAITING_RETRY", "正在等待原操作边界，已提交结果将先归档", time.Second, f)
		}
		return err
	}
	if taskUnknown(task) {
		return s.stageWait(ctx, r, "WAITING_RETRY", "原提交结果未知，正在按原请求核对回执；后续建模尚未启动", 2*time.Second, taskFailure(task))
	}
	for _, step := range task.Steps {
		if step.Handoff != nil && step.Result == nil && (step.Phase == "UI_UNKNOWN" || step.Phase == "UI_OPENED" || step.Phase == "HANDED_OFF") {
			return s.stageWait(ctx, r, "WAITING_UI", "原交互工具结果需要核对或取消；系统不会重放工具", 2*time.Second, step.Failure)
		}
	}
	return s.checkpointTask(ctx, r, task, "PLANNING", "RETIRED", false, true, "")
}

func (s *Service) advanceStage(ctx context.Context, r Run) error {
	task, err := s.automation.GetTask(ctx, r.ActorID, r.DocumentID, r.ActiveTaskID)
	if err != nil {
		return err
	}
	if task.RunID != r.ID || task.PlanEpoch != r.Epoch {
		return aiFault("AI_STAGE_MISMATCH", "阶段不属于当前计划代际", false)
	}
	if taskUnknown(task) {
		task, err = s.automation.ReconcileTaskReceipts(ctx, r.ActorID, r.DocumentID, task.ID)
	} else if task.Status == "ACTIVE" {
		ui := false
		for _, step := range task.Steps {
			if step.Phase == "PLANNED" {
				if op, e := s.automation.Catalog.Describe(step.OperationID); e == nil && op.Effect == "UI_COMMAND" {
					ui = true
				}
				break
			}
		}
		if ui {
			return s.stageWait(ctx, r, "WAITING_UI", "这一步需要打开原工具并完成交互", 0, nil)
		}
		task, err = s.automation.Advance(ctx, r.ActorID, r.DocumentID, task.ID, task.Version)
	}
	if err != nil {
		var f *automation.Fault
		if errors.As(err, &f) {
			switch f.Code {
			case "AUTOMATION_BUSY", "TASK_VERSION_CONFLICT":
				return s.stageWait(ctx, r, "WAITING_RETRY", f.Message, time.Second, f)
			case "UI_HANDOFF_REQUIRED":
				return s.stageWait(ctx, r, "WAITING_UI", f.Message, 0, f)
			}
			if repairableStageFault(f) {
				return s.failStage(ctx, r, task, f)
			}
		}
		return err
	}
	if taskUnknown(task) {
		if err = s.checkpointTask(ctx, r, task, "", "", false, false, ""); err != nil {
			return err
		}
		return s.stageWait(ctx, r, "WAITING_RETRY", "提交结果未知，正在恢复原请求回执", 2*time.Second, taskFailure(task))
	}
	switch task.Status {
	case "WAIT_CONFIRM":
		return s.checkpointTask(ctx, r, task, "WAITING_CONFIRMATION", "WAIT_CONFIRM", false, false, "预览已准备好，请确认本次精确操作")
	case "WAIT_UI":
		return s.checkpointTask(ctx, r, task, "WAITING_UI", "WAIT_UI", false, false, "等待原工具完成、取消或回报结果")
	case "COMPLETED":
		if err = s.checkpointTask(ctx, r, task, "VERIFYING", "COMPLETED", true, false, ""); err != nil {
			return err
		}
		current, e := loadRun(ctx, s.db, r.ID, false)
		if e != nil {
			return e
		}
		return s.refreshGoalVerification(ctx, current)
	case "PAUSED", "REPLAN_REQUIRED", "STOPPED":
		failure := taskFailure(task)
		if failure != nil {
			return s.failStage(ctx, r, task, failure)
		}
		return s.checkpointTask(ctx, r, task, "PLANNING", task.Status, true, false, "")
	default:
		return s.checkpointTask(ctx, r, task, "EXECUTING", "EXECUTING", false, false, "")
	}
}
