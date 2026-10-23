package assistant

import (
	"context"

	"github.com/occccad/occccad/internal/database"
)

// TurnID is frozen when a call is created. InputID remains exclusively the
// QUERY lease/control identity. Never infer attribution from epochs or totals.
func updateTurnUsage(ctx context.Context, tx database.Tx, c CallRecord, update func(*TurnUsage)) error {
	if c.TurnID == "" {
		return nil
	} // Explicitly unassigned historical call.
	t, _, err := loadInput(ctx, tx, c.TurnID)
	if err != nil {
		return err
	}
	if t.RunID != c.RunID {
		return aiFault("AI_TURN_MISMATCH", "调用轮次不属于当前会话", false)
	}
	if t.Usage == nil {
		t.Usage = &TurnUsage{}
	}
	update(t.Usage)
	return saveInput(ctx, tx, t)
}

func addReportedUsage(target *RunUsage, usage *Usage) {
	target.ReportedCalls++
	target.PromptTokens += int64(usage.PromptTokens)
	target.CompletionTokens += int64(usage.CompletionTokens)
	target.TotalTokens += int64(usage.TotalTokens)
	if usage.CacheHitTokens != nil {
		target.CacheHitTokens += *usage.CacheHitTokens
	}
	if usage.ReasoningTokens != nil {
		target.ReasoningTokens += *usage.ReasoningTokens
	}
}

// The existing Task idempotency key records the unique creating model call.
// Historical/unlinked tasks remain unassigned; plan epochs are not attribution.
func taskTurnID(ctx context.Context, db database.Executor, r Run, taskID string) (string, error) {
	var id string
	err := db.QueryRow(ctx, `SELECT COALESCE(c.payload->>'turnId','') FROM occccad.automation_tasks t LEFT JOIN occccad.assistant_model_calls c ON c.run_id=$2 AND t.request_id LIKE ('assistant:' || c.id::text || ':%') WHERE t.id=$1 AND t.actor_id=$3 AND t.document_id=$4`, taskID, r.ID, r.ActorID, r.DocumentID).Scan(&id)
	return id, err
}
