package assistant

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
)

func goalTargetKey(requirement string) string { return "GOAL:" + requirement }

// Source inputs are the authority for requirements, including resumed Runs.
// Recompile in pages instead of retaining an outdated, partial interpretation
// of an earlier user sentence after the requirement compiler gains a check.
func (s *Service) goalRequirements(ctx context.Context, r Run) ([]GoalRequirement, error) {
	result := []GoalRequirement{}
	var after uint64
	for {
		rows, err := s.db.Query(ctx, `SELECT sequence,payload FROM occccad.assistant_inputs WHERE run_id=$1 AND kind<>'QUERY' AND sequence>$2 ORDER BY sequence LIMIT 32`, r.ID, after)
		if err != nil {
			return nil, err
		}
		count := 0
		for rows.Next() {
			var payload []byte
			if err = rows.Scan(&after, &payload); err != nil {
				break
			}
			var turn Turn
			if err = json.Unmarshal(payload, &turn); err != nil {
				break
			}
			if turn.GoalVersion <= r.GoalVersion {
				result = reviseRequirements(result, turn)
			}
			count++
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return nil, err
		}
		if count < 32 {
			return result, nil
		}
	}
}

// A passed base profile cannot be repaired by creating another copy. This
// guard rejects the duplicate before even creating its empty sketch, while
// allowing remaining user operations. Passing the currently supported checks
// alone never auto-finishes an incompletely represented natural-language goal.
func rejectSatisfiedStage(r Run, revision string, steps []automation.StepSpec) error {
	if r.Verification == nil || r.Verification.GoalVersion != r.GoalVersion || r.Verification.RevisionID != revision {
		return nil
	}
	passed := map[string]string{}
	for _, check := range r.Verification.Checks {
		if check.Status == "PASSED" {
			passed[check.ID] = check.TargetID
		}
	}
	for _, requirement := range r.Requirements {
		if requirement.Kind != "RECTANGLE_DIMENSIONS" || passed[requirement.ID] == "" {
			continue
		}
		for _, step := range steps {
			if step.OperationID != "sketch.rectangle" {
				continue
			}
			args, err := automation.DecodeObject(step.Arguments)
			if err != nil {
				continue
			}
			width, _ := args["width"].(float64)
			height, _ := args["height"].(float64)
			if math.Abs(width-requirement.Width) > 1e-6 || math.Abs(height-requirement.Height) > 1e-6 {
				continue
			}
			duplicate := args["sketchId"] == passed[requirement.ID]
			sourceID := step.Bindings["sketchId"]
			for _, binding := range step.TypedBindings {
				if binding.Target == "sketchId" {
					sourceID = binding.SourceStepID
				}
			}
			for _, source := range steps {
				if source.ID != sourceID || source.OperationID != "sketch.create" {
					continue
				}
				sourceArgs, e := automation.DecodeObject(source.Arguments)
				if e == nil && sourceArgs["bodyId"] == r.Scope.WorkingBodyID {
					duplicate = true
				}
			}
			if duplicate {
				return aiFault("AI_GOAL_ALREADY_SATISFIED", "原始矩形已通过当前版本检查，不能为修复其他要求再次创建同尺寸草图。请保留已通过的特征，只处理未完成项；全部意图完成后调用 cad_finish。", false)
			}
		}
	}
	return nil
}

// Reconstruct bindings only from this Run's exact command receipts when a
// checkpoint has no requirement binding yet. History is read in bounded pages;
// it never becomes the model's context or an ever-growing Run output list.
func (s *Service) goalTargets(ctx context.Context, r Run) (map[string]string, error) {
	targets := map[string]string{}
	for key, id := range r.Targets {
		targets[key] = id
	}
	missing := func() bool {
		for _, requirement := range r.Requirements {
			switch requirement.Kind {
			case "RECTANGLE_DIMENSIONS", "EXTRUSION_LENGTH", "ALL_EDGES_FILLET", "SELECTED_EDGES_FILLET", "CENTERED_THROUGH_HOLE", "THROUGH_HOLE":
				if targets[goalTargetKey(requirement.ID)] == "" {
					return true
				}
			}
		}
		return false
	}
	var after uint64
	for missing() {
		rows, err := s.db.Query(ctx, `SELECT s.sequence,t.payload FROM occccad.assistant_stages s JOIN occccad.automation_tasks t ON t.id=s.task_id WHERE s.run_id=$1 AND s.sequence>$2 ORDER BY s.sequence LIMIT 16`, r.ID, after)
		if err != nil {
			return nil, err
		}
		tasks := []automation.Task{}
		for rows.Next() {
			var raw []byte
			if err = rows.Scan(&after, &raw); err != nil {
				break
			}
			var task automation.Task
			if err = json.Unmarshal(raw, &task); err != nil {
				break
			}
			tasks = append(tasks, task)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return nil, err
		}
		if len(tasks) == 0 {
			break
		}
		for _, task := range tasks {
			facts, err := s.taskReceiptFacts(ctx, r, task, true)
			if err != nil {
				return nil, err
			}
			for _, fact := range facts {
				for key, id := range fact.targets {
					if strings.HasPrefix(key, "GOAL:") && targets[key] == "" {
						targets[key] = id
					}
				}
			}
		}
	}
	return targets, nil
}

func (s *Service) refreshGoalVerification(ctx context.Context, r Run) error {
	v, err := s.verifyGoal(ctx, r)
	if err != nil {
		return err
	}
	return s.recordGoalVerification(ctx, r, v)
}

func (s *Service) recordGoalVerification(ctx context.Context, r Run, v *GoalVerification) error {
	_, err := s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		if current.Epoch != r.Epoch || current.ActiveTaskID != "" || current.RetiringTaskID != "" || terminal(current.Status) || current.Status == "PAUSED" {
			return context.Canceled
		}
		current.Verification = v
		current.Requirements = v.requirements
		current.LatestRevisionID = v.RevisionID
		if v.Status == "VERIFIED" {
			current.Failure = nil
			current.WaitingReason = ""
		}
		if current.Status == "VERIFYING" {
			current.Status = "PLANNING"
		}
		if current.Targets == nil {
			current.Targets = map[string]string{}
		}
		for _, check := range v.Checks {
			if check.TargetID != "" && current.Targets[goalTargetKey(check.ID)] == "" {
				current.Targets[goalTargetKey(check.ID)] = check.TargetID
			}
		}
		return s.event(ctx, tx, current, "GOAL_CHECKED", map[string]any{"status": v.Status, "goalVersion": v.GoalVersion, "revisionId": v.RevisionID, "message": verificationProgress(v)})
	})
	return err
}

func verificationProgress(v *GoalVerification) string {
	if v.Status == "VERIFIED" {
		return "目标检查已通过，正在收尾。"
	}
	missing := []string{}
	for _, check := range v.Checks {
		if check.Status != "PASSED" && check.ID != "required-body" && check.ID != "required-solid" {
			missing = append(missing, goalCheckName(check.Name))
		}
	}
	if len(missing) == 0 {
		return "正在检查实体结果。"
	}
	return "还需完成：" + strings.Join(missing, "、") + "。"
}

func goalCheckName(kind string) string {
	names := map[string]string{"RECTANGLE_DIMENSIONS": "矩形尺寸", "EXTRUSION_LENGTH": "基体拉伸", "ALL_EDGES_FILLET": "全部边圆角", "SELECTED_EDGES_FILLET": "选中边圆角", "CENTERED_THROUGH_HOLE": "中心贯穿孔", "THROUGH_HOLE": "贯穿孔", "NO_FEATURE_TYPE": "取消圆角"}
	if name := names[kind]; name != "" {
		return name
	}
	return kind
}
