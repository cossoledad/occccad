package assistant

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
)

func goalTargetKey(requirement string) string { return "GOAL:" + requirement }

func forgetRemovedTargets(targets, selections map[string]string, removed []string) {
	for _, id := range removed {
		for role, target := range targets {
			if target == id {
				delete(targets, role)
				delete(selections, role)
			}
		}
	}
}

// Source inputs are the authority for requirements, including resumed Runs.
// Recompile in pages instead of retaining an outdated, partial interpretation
// of an earlier user sentence after the requirement compiler gains a check.
func (s *Service) goalRequirements(ctx context.Context, r Run) ([]GoalRequirement, error) {
	result := []GoalRequirement{}
	var after uint64
	var latestSourceVersion uint64
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
				if turn.GoalVersion > latestSourceVersion {
					latestSourceVersion = turn.GoalVersion
				}
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
			if latestSourceVersion != r.GoalVersion {
				result = append(result, GoalRequirement{ID: uuid.NewSHA1(uuid.MustParse(r.ID), []byte("missing-goal-source")).String(), Kind: "UNRESOLVED_INTENT", SourceGoalVersion: r.GoalVersion, Coverage: "UNRESOLVED", Unresolved: []string{"GOAL_VERSION_SOURCE_MISSING"}})
			}
			return result, nil
		}
	}
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
			case "RECTANGLE_DIMENSIONS", "EXTRUSION_LENGTH", "ALL_EDGES_FILLET", "SELECTED_EDGES_FILLET", "HOLE_RIM_FILLET", "CENTERED_THROUGH_HOLE", "THROUGH_HOLE":
				if targets[goalTargetKey(requirement.ID)] == "" {
					return true
				}
			}
		}
		return false
	}
	var after uint64
	if !missing() {
		return targets, nil
	}
	for {
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
			r.Targets = targets
			facts, err := s.taskReceiptFacts(ctx, r, task, true)
			if err != nil {
				return nil, err
			}
			if r.TargetSelections == nil {
				r.TargetSelections = map[string]string{}
			}
			for _, fact := range facts {
				for key, token := range fact.selections {
					if strings.HasPrefix(key, "GOAL:") {
						r.TargetSelections[key] = token
					}
				}
				forgetRemovedTargets(targets, nil, fact.removed)
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
	message := checkEvidenceMessage(v.Checks)
	if message == "" {
		return "正在检查实体结果。"
	}
	return message
}

func checkEvidenceMessage(checks []VerificationCheck) string {
	failed, unknown := []string{}, []string{}
	for _, check := range checks {
		if check.Status == "FAILED" {
			failed = append(failed, goalCheckName(check.Name))
		} else if check.Status != "PASSED" {
			unknown = append(unknown, goalCheckName(check.Name))
		}
	}
	parts := []string{}
	if len(failed) > 0 {
		parts = append(parts, "检查发现不一致："+strings.Join(failed, "、"))
	}
	if len(unknown) > 0 {
		parts = append(parts, "证据待核对："+strings.Join(unknown, "、")+"（尚不能判断是否完成）")
	}
	return strings.Join(parts, "；")
}

func goalCheckName(kind string) string {
	names := map[string]string{"RECTANGLE_DIMENSIONS": "矩形尺寸", "EXTRUSION_LENGTH": "基体拉伸", "ALL_EDGES_FILLET": "全部边圆角", "SELECTED_EDGES_FILLET": "选中边圆角", "HOLE_RIM_FILLET": "孔口圆边圆角", "UNSCOPED_FILLET": "未明确目标的圆角", "UNRESOLVED_INTENT": "待明确的建模要求", "CENTERED_THROUGH_HOLE": "中心贯穿孔", "THROUGH_HOLE": "贯穿孔", "NO_FEATURE_TYPE": "取消圆角"}
	if name := names[kind]; name != "" {
		return name
	}
	return kind
}
