package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func loadCall(ctx context.Context, db database.Executor, id string) (CallRecord, error) {
	var c CallRecord
	var raw []byte
	var status string
	err := db.QueryRow(ctx, `SELECT payload,status FROM occccad.assistant_model_calls WHERE id=$1`, id).Scan(&raw, &status)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(raw, &c)
	c.Status = status
	return c, err
}
func saveCall(ctx context.Context, db database.Executor, c CallRecord) error {
	_, err := db.Exec(ctx, `UPDATE occccad.assistant_model_calls SET status=$1,payload=$2,finished_at=$3 WHERE id=$4`, c.Status, marshalJSON(c), c.FinishedAt, c.ID)
	return err
}

func (s *Service) prepareCall(ctx context.Context, r Run, inputID string, baseline workspace.CommandBaseline, messages []Message) (CallRecord, error) {
	settings, err := s.Settings(ctx, r.ActorID)
	if err != nil {
		return CallRecord{}, err
	}
	generation := Generation{Provider: settings.Provider, ProviderOrigin: settings.ProviderOrigin, Model: settings.Model, Thinking: settings.Thinking, Version: settings.Version, Limits: s.requestLimits()}
	generation.MaxTokens = generation.Limits.MaxOutputTokens
	if generation.Thinking != "off" {
		generation.MaxTokens = generation.Limits.MaxThinkingTokens
	}
	c := CallRecord{ID: uuid.NewString(), RunID: r.ID, InputID: inputID, LogicalID: uuid.NewString(), Attempt: 1, Epoch: r.Epoch, Status: "PREPARED", Provider: settings.Provider, Generation: generation, Baseline: baseline, RequestDigest: modelcore.ValueDigest(publicMessages(messages)), UsageState: "NOT_SENT", StartedAt: timestamp(s.now())}
	_, err = s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		if inputID == "" && (current.Epoch != r.Epoch || current.Status == "STOPPED" || current.Status == "PAUSED") {
			return context.Canceled
		}
		if inputID != "" {
			if e := s.requireQuestionLease(ctx, tx, inputID); e != nil {
				return e
			}
		}
		if current.TokenBudget != nil && current.Usage.TotalTokens >= *current.TokenBudget {
			if inputID != "" {
				return s.waitQuestionBudget(ctx, tx, current, inputID)
			}
			current.Status = "WAITING_BUDGET"
			current.WaitingReason = "已达到你设置的累计预算；提高或清空预算后可继续同一任务"
			return s.event(ctx, tx, current, "BUDGET_WAIT", map[string]any{"reported": current.Usage.TotalTokens, "budget": current.TokenBudget})
		}
		if current.TokenBudget != nil {
			remaining := *current.TokenBudget - current.Usage.TotalTokens
			if remaining > 0 && remaining < int64(c.Generation.MaxTokens) {
				c.Generation.MaxTokens = int(remaining)
			}
		}
		if inputID == "" {
			if current.LatestRevisionID != baseline.RevisionID {
				current.Verification = nil
			}
			current.LatestRevisionID = baseline.RevisionID
			current.PendingCallID = c.ID
		} else {
			updated, e := tx.Exec(ctx, `UPDATE occccad.assistant_inputs SET pending_call_id=$1 WHERE id=$2 AND run_id=$3 AND status='RECEIVED'`, c.ID, inputID, r.ID)
			if e != nil {
				return e
			}
			if updated.RowsAffected() != 1 {
				return context.Canceled
			}
		}
		_, e := tx.Exec(ctx, `INSERT INTO occccad.assistant_model_calls(id,run_id,input_id,logical_id,attempt,epoch,status,payload,started_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, c.ID, c.RunID, c.InputID, c.LogicalID, c.Attempt, c.Epoch, c.Status, marshalJSON(c), c.StartedAt)
		return e
	})
	if err != nil {
		return c, err
	}
	var exists int
	if err = s.db.QueryRow(ctx, `SELECT count(*) FROM occccad.assistant_model_calls WHERE id=$1`, c.ID).Scan(&exists); err != nil {
		return c, err
	}
	if exists == 0 {
		return c, aiFault("AI_BUDGET_WAIT", "等待调整累计预算", false)
	}
	return c, nil
}
func publicMessages(messages []Message) []byte {
	// Private reasoning is not even part of the persisted request digest input.
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		out = append(out, map[string]any{"role": m.Role, "content": m.Content, "toolCalls": m.ToolCalls, "toolCallId": m.ToolCallID})
	}
	return marshalJSON(out)
}
func (s *Service) markSent(ctx context.Context, c *CallRecord) error {
	_, err := s.mutateRun(ctx, c.RunID, func(tx database.Tx, r *Run) error {
		if c.InputID == "" && (r.Epoch != c.Epoch || r.Status == "STOPPED" || r.Status == "PAUSED") {
			return context.Canceled
		}
		stored, e := loadCall(ctx, tx, c.ID)
		if e != nil {
			return e
		}
		if stored.Status != "PREPARED" {
			return aiFault("AI_CALL_STATE", "调用已经开始，不能重复发送", false)
		}
		if c.InputID != "" {
			if e = s.requireQuestionLease(ctx, tx, c.InputID); e != nil {
				return e
			}
			var status string
			if e = tx.QueryRow(ctx, `SELECT status FROM occccad.assistant_inputs WHERE id=$1`, c.InputID).Scan(&status); e != nil {
				return e
			}
			if status != "RECEIVED" {
				return context.Canceled
			}
		}
		c.Status = "SENT"
		c.UsageState = "PENDING"
		if e = saveCall(ctx, tx, *c); e != nil {
			return e
		}
		r.ModelCalls++
		return s.event(ctx, tx, r, "MODEL_CALL_SENT", map[string]any{"id": c.ID, "logicalId": c.LogicalID, "attempt": c.Attempt, "generation": c.Generation})
	})
	return err
}
func usageUnknown(c CallRecord) bool {
	return c.Usage == nil && c.Status != "PREPARED" && c.Status != "SENT"
}
func (s *Service) receiveCall(ctx context.Context, c *CallRecord, completion Completion, callErr error) error {
	next, err := s.mutateRun(ctx, c.RunID, func(tx database.Tx, r *Run) error {
		stored, e := loadCall(ctx, tx, c.ID)
		if e != nil {
			return e
		}
		if stored.Status != "SENT" && stored.Status != "UNKNOWN" {
			*c = stored
			return nil
		}
		wasUnknown := usageUnknown(stored)
		stored.FinishedAt = timestamp(s.now())
		stored.ProviderRequestID = completion.ProviderRequestID
		stored.Usage = completion.Usage
		stored.Status = "RECEIVED"
		message := completion.Message
		message.ReasoningContent = ""
		if callErr == nil {
			stored.Response = &message
		} else {
			stored.Status = "UNKNOWN"
			stored.Failure = asFault(callErr)
		}
		if stored.Usage == nil {
			stored.UsageState = "UNKNOWN"
			if !wasUnknown {
				r.Usage.UnknownAttempts++
			}
		} else {
			stored.UsageState = "REPORTED"
			if wasUnknown {
				r.Usage.UnknownAttempts--
			}
			r.Usage.ReportedCalls++
			r.Usage.PromptTokens += int64(stored.Usage.PromptTokens)
			r.Usage.CompletionTokens += int64(stored.Usage.CompletionTokens)
			r.Usage.TotalTokens += int64(stored.Usage.TotalTokens)
			if stored.Usage.CacheHitTokens != nil {
				r.Usage.CacheHitTokens += *stored.Usage.CacheHitTokens
			}
			if stored.Usage.ReasoningTokens != nil {
				r.Usage.ReasoningTokens += *stored.Usage.ReasoningTokens
			}
		}
		if e = saveCall(ctx, tx, stored); e != nil {
			return e
		}
		*c = stored
		// Only the provider's public answer is conversation material. Private
		// reasoning_content stays out of the journal and UI, including recovery.
		if callErr == nil && r.Epoch == stored.Epoch && len(message.ToolCalls) > 0 && strings.TrimSpace(message.Content) != "" {
			if e = s.event(ctx, tx, r, "ASSISTANT_MESSAGE", map[string]any{"callId": stored.ID, "inputId": stored.InputID, "goalVersion": r.GoalVersion, "message": publicProgress(message.Content)}); e != nil {
				return e
			}
		}
		return s.event(ctx, tx, r, "MODEL_CALL_RECORDED", map[string]any{"id": stored.ID, "status": stored.Status, "usageState": stored.UsageState, "usage": stored.Usage, "epoch": stored.Epoch, "stale": r.Epoch != stored.Epoch})
	})
	_ = next
	return err
}
func (s *Service) recoverUnknownCall(ctx context.Context, id string) error {
	c, err := loadCall(ctx, s.db, id)
	if err != nil {
		return err
	}
	if c.Status == "SENT" {
		return s.receiveCall(ctx, &c, Completion{}, aiFault("AI_CALL_OUTCOME_UNKNOWN", "连接中断前的模型请求是否处理或计费未知；后续调用将单独记账", true))
	}
	return nil
}
func asFault(err error) *automation.Fault {
	if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrUnauthorized) {
		return &automation.Fault{Code: "AI_ACCESS_DENIED", Message: "当前账号已无该操作所需权限；进度保留，等待权限或目标调整", Retryable: false}
	}
	if errors.Is(err, access.ErrNotFound) || errors.Is(err, workspace.ErrNotFound) {
		return &automation.Fault{Code: "AI_REFERENCE_UNAVAILABLE", Message: "目标或所需引用不可用，需重新选择或补充访问条件", Retryable: false}
	}
	var f *automation.Fault
	if errors.As(err, &f) {
		return f
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &automation.Fault{Code: "AI_REQUEST_TIMEOUT", Message: "本次模型请求超时，已保留进度和未知用量记录", Retryable: true}
	}
	if errors.Is(err, context.Canceled) {
		return &automation.Fault{Code: "AI_REQUEST_CANCELLED", Message: "本次请求已取消，供应商处理及计费情况可能未知", Retryable: true}
	}
	return &automation.Fault{Code: "AI_EXECUTION_FAILURE", Message: "执行暂时中断，进度已保存", Retryable: true}
}
func (s *Service) Calls(ctx context.Context, actor, document, id, cursor string, limit int) (map[string]any, error) {
	if _, e := s.GetRun(ctx, actor, document, id); e != nil {
		return nil, e
	}
	limit = boundedLimit(limit)
	// Stable time+UUID cursor; no offset scan over a growing ledger.
	at, key := "", ""
	if cursor != "" {
		var e error
		var raw []byte
		raw, e = decodeCursor(cursor)
		if e != nil {
			return nil, e
		}
		var v struct{ At, ID string }
		if json.Unmarshal(raw, &v) != nil {
			return nil, aiFault("INVALID_CURSOR", "调用游标无效", false)
		}
		at, key = v.At, v.ID
	}
	rows, e := s.db.Query(ctx, `SELECT payload FROM occccad.assistant_model_calls WHERE run_id=$1 AND ($2='' OR started_at>$2 OR (started_at=$2 AND id::text>$3)) ORDER BY started_at,id LIMIT $4`, id, at, key, limit+1)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []CallRecord{}
	for rows.Next() {
		var raw []byte
		var c CallRecord
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &c); e != nil {
			return nil, e
		}
		items = append(items, c)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	next := ""
	if len(items) > 0 {
		last := items[len(items)-1]
		next = encodeCursor(map[string]string{"At": last.StartedAt, "ID": last.ID})
	}
	return map[string]any{"items": items, "hasMore": more, "nextCursor": next}, nil
}
