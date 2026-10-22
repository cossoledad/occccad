package assistant

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

const MaxDebugServiceBytes = 1 << 20

type QueryDebugInput struct {
	Scope        FrozenScope     `json:"scope"`
	Tool         string          `json:"tool"`
	Arguments    json.RawMessage `json:"arguments"`
	SelectedOnly bool            `json:"selectedOnly"`
}

type QueryDebugSupplement struct {
	Snapshot workspace.ModelSnapshot `json:"snapshot"`
	// These are human diagnostics, not extra facts appended to the model result.
	Page                 map[string]any                 `json:"page,omitempty"`
	SelectionResolutions []automation.ResolvedSelection `json:"selectionResolutions,omitempty"`
}

type QueryDebugResult struct {
	Input             QueryDebugInput      `json:"input"`
	ToolCall          Call                 `json:"toolCall"`
	ServiceResult     *Result              `json:"serviceResult,omitempty"`
	ServiceBytes      int                  `json:"serviceBytes"`
	ServiceLimitBytes int                  `json:"serviceLimitBytes"`
	ServiceOmitted    bool                 `json:"serviceOmitted"`
	ToolResult        Result               `json:"toolResult"`
	ToolBytes         int                  `json:"toolBytes"`
	ToolLimitBytes    int                  `json:"toolLimitBytes"`
	OutputReplaced    bool                 `json:"outputReplaced"`
	Supplement        QueryDebugSupplement `json:"supplement"`
}

// DebugQuery uses the actual Host and output normalizer, without creating a
// Service, credential, Run, model call, or mutation capability. Revision and
// selection scope are explicit; no current-Head fallback is permitted here.
func DebugQuery(ctx context.Context, service *automation.Service, actor, document string, input QueryDebugInput) (QueryDebugResult, error) {
	out := QueryDebugResult{ServiceLimitBytes: MaxDebugServiceBytes, ToolLimitBytes: MaxToolResultBytes}
	original := input
	invalid := func(message string) (QueryDebugResult, error) {
		return out, aiFault("INVALID_DEBUG_QUERY", message, false)
	}
	if input.Scope.DocumentID != document || input.Scope.RevisionID == "" {
		return invalid("调试查询必须固定当前路由的文档与明确 Revision")
	}
	if _, err := uuid.Parse(input.Scope.RevisionID); err != nil {
		return invalid("Revision 必须是正式版本 ID")
	}
	if len(input.Scope.Selections) > 20 {
		return invalid("调试入口单次最多固定 20 个选择；可分页检查整个 Body")
	}
	if input.Scope.EditContextID != "" {
		return invalid("当前调试只支持固定定义快照，不能用普通 Body 数据替代编辑上下文或变体")
	}
	if input.Tool != "cad_query_model" && input.Tool != "cad_query_topology" && input.Tool != "cad_context" {
		return invalid("调试入口只允许模型、拓扑和上下文只读查询")
	}
	snapshot, err := service.ReadSnapshot(ctx, actor, document, input.Scope.RevisionID)
	if err != nil {
		return out, err
	}
	out.Supplement.Snapshot = snapshot
	if body := input.Scope.WorkingBodyID; body != "" {
		var part workspace.PartModel
		if snapshot.DocumentType != "PART" || json.Unmarshal(snapshot.Model, &part) != nil {
			return invalid("Body 范围只适用于有效 Part 快照")
		}
		found := false
		for _, candidate := range part.Bodies {
			found = found || candidate.ID == body
		}
		if !found {
			return invalid("所固定的 Body 不属于这个文档和 Revision")
		}
	}
	checkRange := func(revision, body string) bool {
		return (revision == "" || revision == input.Scope.RevisionID) && (input.Scope.WorkingBodyID == "" || body == "" || body == input.Scope.WorkingBodyID)
	}
	if input.SelectedOnly && (!input.Scope.Complete || len(input.Scope.Selections) == 0 || input.Scope.SelectionCount != len(input.Scope.Selections)) {
		return invalid("请先显式固定完整选区")
	}
	switch input.Tool {
	case "cad_query_model":
		var query automation.Query
		if err = decode(input.Arguments, &query); err != nil {
			return invalid(err.Error())
		}
		if !checkRange(query.RevisionID, query.BodyID) {
			return invalid("参数与固定 Revision/Body 不一致")
		}
		query.RevisionID = input.Scope.RevisionID
		if input.Scope.WorkingBodyID != "" {
			query.BodyID = input.Scope.WorkingBodyID
		}
		if input.SelectedOnly {
			if len(input.Scope.Selections) != 1 {
				return invalid("模型对象查询一次使用一个稳定 ID；多选拓扑请用拓扑查询")
			}
			ref := input.Scope.Selections[0]
			if ref.ID == "" || ref.Selection != nil || ref.SourceDocumentID != document || ref.SourceVersionID != query.RevisionID || ref.BodyID != "" && query.BodyID != "" && ref.BodyID != query.BodyID {
				return invalid("所选对象不属于固定的模型查询范围")
			}
			if query.ID != "" && query.ID != ref.ID {
				return invalid("参数 ID 与固定选区不同")
			}
			query.ID = ref.ID
		}
		input.Arguments = marshalJSON(query)
	case "cad_query_topology":
		var query automation.TopologyQuery
		if err = decode(input.Arguments, &query); err != nil {
			return invalid(err.Error())
		}
		if !checkRange(query.RevisionID, query.BodyID) {
			return invalid("参数与固定 Revision/Body 不一致")
		}
		query.RevisionID = input.Scope.RevisionID
		if input.Scope.WorkingBodyID != "" {
			query.BodyID = input.Scope.WorkingBodyID
		}
		if query.BodyID == "" {
			return invalid("拓扑查询必须固定 Body")
		}
		if input.SelectedOnly {
			if len(query.IDs) > 0 {
				return invalid("固定选区与手写 ids 不能同时指定")
			}
			for _, ref := range input.Scope.Selections {
				if ref.Selection == nil || ref.SourceDocumentID != document || ref.SourceVersionID != query.RevisionID || ref.Selection.SourceBodyID != query.BodyID || string(ref.Selection.ExpectedType) != query.Kind {
					return invalid("拓扑选区必须属于固定文档、Revision、Body 和拓扑类型")
				}
				resolved, e := service.ResolveSelectionForSnapshotWithEvidence(ctx, actor, document, query.RevisionID, workspace.FeatureSelection{Selection: *ref.Selection, SourceVersionID: ref.SourceVersionID, SourceFeatureID: ref.FeatureID})
				if e != nil {
					return out, e
				}
				query.IDs = append(query.IDs, resolved.TopologyID)
				out.Supplement.SelectionResolutions = append(out.Supplement.SelectionResolutions, resolved)
			}
		}
		input.Arguments = marshalJSON(query)
	case "cad_context":
		var args struct{}
		if err = decode(input.Arguments, &args); err != nil {
			return invalid(err.Error())
		}
		if input.SelectedOnly {
			return invalid("上下文查询不支持选区过滤")
		}
	}
	host := NewHost(service, actor, document, false)
	host.revision = input.Scope.RevisionID
	out.Input = original
	out.ToolCall = Call{ID: "debug-query", Name: input.Tool, Arguments: input.Arguments}
	raw := host.call(ctx, out.ToolCall)
	out.ServiceBytes = len(raw.Content)
	if out.ServiceBytes > MaxDebugServiceBytes {
		out.ServiceOmitted = true
	} else {
		out.ServiceResult = &raw
	}
	out.ToolResult = NormalizeToolResult(raw)
	out.ToolBytes = len(out.ToolResult.Content)
	out.OutputReplaced = raw.IsError != out.ToolResult.IsError || !bytes.Equal(raw.Content, out.ToolResult.Content)
	var page map[string]any
	if json.Unmarshal(raw.Content, &page) == nil {
		out.Supplement.Page = map[string]any{}
		for _, key := range []string{"scope", "filter", "scopeCount", "candidateCount", "unknownCount", "matchComplete", "matchedCount", "returnedCount", "hasMore", "nextCursor", "complete", "selectionSetCount", "selectionSetComplete", "pageComplete"} {
			if value, ok := page[key]; ok {
				out.Supplement.Page[key] = value
			}
		}
		if items, ok := page["items"].([]any); ok {
			out.Supplement.Page["returnedCount"] = len(items)
		}
	}
	return out, nil
}
