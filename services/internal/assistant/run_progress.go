package assistant

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/occccad/occccad/internal/automation"
)

func publicProgress(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 4000 {
		return text
	}
	end := 4000
	for !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "…"
}

// The durable Task retains full actions/receipts. A planning checkpoint needs
// identities and results, not duplicate previews, changes and output wrappers.
func stageContext(task automation.Task) map[string]any {
	steps := []map[string]any{}
	for _, step := range task.Steps {
		item := map[string]any{"id": step.ID, "operationId": step.OperationID, "phase": step.Phase}
		arguments := step.Arguments
		if step.Action != nil {
			arguments = step.Action.Arguments
		}
		if len(arguments) <= 2048 {
			item["arguments"] = arguments
		} else {
			item["argumentsOmitted"] = true
		}
		if len(step.Bindings) > 0 {
			item["bindings"] = step.Bindings
		}
		if len(step.TypedBindings) > 0 {
			item["typedBindings"] = step.TypedBindings
		}
		if step.Failure != nil {
			item["failure"] = step.Failure
		}
		if step.Result != nil {
			result := step.Result
			item["revisionId"] = result.Receipt.RevisionID
			if len(result.Outputs) > 0 {
				item["outputs"] = result.Outputs
			} else if result.Output != nil {
				item["output"] = result.Output
			}
		}
		steps = append(steps, item)
	}
	return map[string]any{"id": task.ID, "status": task.Status, "planEpoch": task.PlanEpoch, "baseline": task.Baseline, "steps": steps}
}

func (s *Service) appliedStepMessage(task automation.Task, id string) string {
	for _, step := range task.Steps {
		if step.ID == id {
			if op, err := s.automation.Catalog.Describe(step.OperationID); err == nil {
				return "已完成" + op.Name
			}
		}
	}
	return "已应用建模步骤"
}

func toolProgress(name string, result Result) string {
	if result.IsError {
		var failure automation.Fault
		if json.Unmarshal(result.Content, &failure) == nil && failure.Message != "" {
			return "正在处理：" + publicProgress(failure.Message)
		}
		return "正在调整操作方案"
	}
	names := map[string]string{
		"cad_context":            "已读取当前模型",
		"cad_query_model":        "已读取模型对象",
		"cad_query_topology":     "已读取实体边面",
		"cad_search_operations":  "已找到相关建模工具",
		"cad_describe_operation": "已确认操作参数",
		"cad_describe_type":      "已读取参数格式",
		"cad_get_selection":      "已读取选中的对象",
		"cad_query_occurrence":   "已读取实例几何",
		"cad_mate_candidates":    "已检查装配候选",
		"cad_verify_assembly":    "已检查装配结果",
		"cad_create_task":        "开始执行建模步骤",
		"cad_get_task":           "已核对步骤结果",
	}
	if name == "cad_verify_goal" {
		var v GoalVerification
		if json.Unmarshal(result.Content, &v) == nil {
			return verificationProgress(&v)
		}
	}
	return names[name]
}
