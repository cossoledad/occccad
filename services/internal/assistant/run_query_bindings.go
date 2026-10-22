package assistant

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

// A ledger coordinate, not a new entity identity or global short-ID registry.
type QueryResultReference struct {
	CallID string `json:"callId"`
	ToolID string `json:"toolId"`
	Slot   string `json:"slot"`
}
type QueryResultBinding struct {
	Target string               `json:"target"`
	Source QueryResultReference `json:"source"`
}
type plannedStep struct {
	automation.StepSpec
	ResultBindings []QueryResultBinding `json:"resultBindings,omitempty"`
}

func (s *Service) bindQueryResult(ctx context.Context, r Run, baseline workspace.CommandBaseline, step *plannedStep) error {
	if len(step.ResultBindings) == 0 {
		return nil
	}
	invalid := func(message string) error { return aiFault("QUERY_RESULT_REF_INVALID", message, false) }
	if len(step.ResultBindings) != 1 || (step.OperationID != "part.fillet" && step.OperationID != "part.chamfer") {
		return invalid("此操作没有声明查询集合输入槽位")
	}
	binding := step.ResultBindings[0]
	ref := binding.Source
	if binding.Target != "selectionSet" || ref.Slot != "selectionSet" || len(ref.ToolID) == 0 || len(ref.ToolID) > 160 {
		return invalid("只允许明确的 selectionSet 输入与输出槽位")
	}
	if _, err := uuid.Parse(ref.CallID); err != nil {
		return invalid("查询账本 callId 无效")
	}
	args, err := automation.DecodeObject(step.Arguments)
	if err != nil {
		return err
	}
	if _, exists := args[binding.Target]; exists {
		return invalid("同一槽位不能同时传值和引用")
	}
	if _, exists := step.Bindings[binding.Target]; exists {
		return invalid("查询引用与步骤绑定不能覆盖同一槽位")
	}
	for _, b := range step.TypedBindings {
		if b.Target == binding.Target {
			return invalid("查询引用与步骤绑定不能覆盖同一槽位")
		}
	}
	var runID, name string
	var payload []byte
	if err = s.db.QueryRow(ctx, `SELECT c.run_id::text,t.name,t.payload FROM occccad.assistant_tool_results t JOIN occccad.assistant_model_calls c ON c.id=t.call_id WHERE t.call_id=$1 AND t.tool_id=$2`, ref.CallID, ref.ToolID).Scan(&runID, &name, &payload); err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return invalid("查询引用不存在")
		}
		return err
	}
	if runID != r.ID {
		return aiFault("QUERY_RESULT_REF_SCOPE", "查询引用必须来自当前运行", false)
	}
	var result Result
	if name != "cad_query_topology" || json.Unmarshal(payload, &result) != nil || result.CallID != ref.ToolID || result.IsError || len(result.Content) > MaxToolResultBytes || !json.Valid(result.Content) {
		return invalid("查询来源不是成功的已记录拓扑工具结果")
	}
	var page struct {
		Scope                struct{ DocumentID, RevisionID, BodyID, Kind, ManifestDigest string }
		SelectionSet         string `json:"selectionSet"`
		SelectionSetCount    int    `json:"selectionSetCount"`
		SelectionSetComplete bool   `json:"selectionSetComplete"`
	}
	if json.Unmarshal(result.Content, &page) != nil || page.SelectionSet == "" {
		return invalid("原查询没有 selectionSet 输出")
	}
	raw, e := base64.RawURLEncoding.DecodeString(page.SelectionSet)
	var set struct {
		Document, Revision, Body, Kind, Digest string
		Count                                  int
		Complete                               bool
		MatchDigest                            string
	}
	if e != nil || len(raw) > 12<<10 || !json.Valid(raw) || json.Unmarshal(raw, &set) != nil || set.Count < 0 || set.Count != page.SelectionSetCount || set.Complete != page.SelectionSetComplete || set.Digest != page.Scope.ManifestDigest || set.MatchDigest == "" || set.Document != page.Scope.DocumentID || set.Revision != page.Scope.RevisionID || set.Body != page.Scope.BodyID || set.Kind != page.Scope.Kind {
		return invalid("原集合数据损坏或与查询来源不一致")
	}
	if page.Scope.DocumentID != r.DocumentID || page.Scope.Kind != "EDGE" || args["bodyId"] != page.Scope.BodyID || r.Scope.WorkingBodyID != "" && r.Scope.WorkingBodyID != page.Scope.BodyID {
		return aiFault("QUERY_RESULT_REF_SCOPE", "查询集合与命令的文档、Body 或类型不一致", false)
	}
	if page.Scope.RevisionID != baseline.RevisionID {
		return aiFault("QUERY_RESULT_REF_STALE", "查询集合版本已过期，请查询命令基线的拓扑", false)
	}
	// Recheck current actor's access to the immutable source; never substitute Head.
	if _, err = s.automation.ReadSnapshot(ctx, r.ActorID, r.DocumentID, page.Scope.RevisionID); err != nil {
		return err
	}
	args[binding.Target] = page.SelectionSet
	step.Arguments = marshalJSON(args)
	return nil
}

func queryResultReference(callID, toolID, name string, result Result) *QueryResultReference {
	if name != "cad_query_topology" || result.IsError {
		return nil
	}
	var page struct {
		SelectionSet string `json:"selectionSet"`
	}
	if json.Unmarshal(result.Content, &page) != nil || page.SelectionSet == "" {
		return nil
	}
	return &QueryResultReference{CallID: callID, ToolID: toolID, Slot: "selectionSet"}
}
