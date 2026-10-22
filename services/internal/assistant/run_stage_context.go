package assistant

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
)

// The Task's existing idempotency key identifies its unique creating call.
// Recover references from that call's original public response, rather than
// searching for equal Base64 values in unrelated query results.
func (s *Service) stageResultBindings(ctx context.Context, r Run, task automation.Task) (map[string][]QueryResultBinding, error) {
	out := map[string][]QueryResultBinding{}
	var requestID string
	if err := s.db.QueryRow(ctx, `SELECT request_id FROM occccad.automation_tasks WHERE id=$1 AND actor_id=$2 AND document_id=$3`, task.ID, r.ActorID, r.DocumentID).Scan(&requestID); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(requestID, "assistant:") {
		return out, nil
	}
	identity := strings.TrimPrefix(requestID, "assistant:")
	callID, toolID, ok := strings.Cut(identity, ":")
	if !ok {
		return nil, aiFault("AI_TOOL_LEDGER_INVALID", "阶段创建来源无效", false)
	}
	source, err := loadCall(ctx, s.db, callID)
	if err != nil {
		return nil, err
	}
	if source.RunID != r.ID || source.Response == nil {
		return nil, aiFault("AI_TOOL_LEDGER_INVALID", "阶段创建来源与运行不一致", false)
	}
	namespace := uuid.NewSHA1(uuid.MustParse(r.ID), []byte(callID+":"+toolID))
	for _, call := range source.Response.ToolCalls {
		if call.ID != toolID || call.Function.Name != "cad_create_task" {
			continue
		}
		var input struct {
			Steps []plannedStep `json:"steps"`
		}
		if err = json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
			return nil, err
		}
		for _, step := range input.Steps {
			if len(step.ResultBindings) > 0 {
				out[uuid.NewSHA1(namespace, []byte(step.ID)).String()] = step.ResultBindings
			}
		}
		return out, nil
	}
	return nil, aiFault("AI_TOOL_LEDGER_INVALID", "阶段的原始创建工具调用不可用", false)
}
