package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/database"
)

const (
	questionLeaseDuration = 45 * time.Second
	questionHeartbeat     = 10 * time.Second
	questionRetryDelay    = 5 * time.Second
)

type questionLeaseKey struct{}
type questionLease struct{ inputID, token string }

// Call preparation, dispatch and tool recording may use this fence inside
// their existing short transaction. A question follows its own immutable input
// and lease, independently of the writer's plan epoch.
func (s *Service) requireQuestionLease(ctx context.Context, db database.Executor, inputID string) error {
	lease, ok := ctx.Value(questionLeaseKey{}).(questionLease)
	if !ok {
		return nil
	}
	if lease.inputID != inputID || lease.token == "" {
		return context.Canceled
	}
	var status, token string
	var until int64
	if err := db.QueryRow(ctx, `SELECT status,lease_token,lease_until FROM occccad.assistant_inputs WHERE id=$1 AND kind='QUERY'`, inputID).Scan(&status, &token, &until); err != nil {
		return err
	}
	if status != "RECEIVED" || token != lease.token || until <= s.now().UnixMilli() {
		return context.Canceled
	}
	return nil
}

func (s *Service) renewQuestionOnce(ctx context.Context, id, token string) error {
	now := s.now()
	result, err := s.db.Exec(ctx, `UPDATE occccad.assistant_inputs SET lease_until=$1 WHERE id=$2 AND kind='QUERY' AND status='RECEIVED' AND lease_token=$3 AND lease_until>$4`, now.Add(questionLeaseDuration).UnixMilli(), id, token, now.UnixMilli())
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return context.Canceled
	}
	return nil
}

func (s *Service) renewQuestion(ctx context.Context, cancel context.CancelFunc, id, token string, period time.Duration) func() {
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(period)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				if err := s.renewQuestionOnce(ctx, id, token); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	return func() { close(stop); <-done }
}

// The response/pending call and writer state are retained. Raising the same
// Run's user budget can requeue this input and consume the saved response.
func (s *Service) waitQuestionBudget(ctx context.Context, tx database.Tx, current *Run, inputID string) error {
	if err := s.requireQuestionLease(ctx, tx, inputID); err != nil {
		return err
	}
	t, _, err := loadInput(ctx, tx, inputID)
	if err != nil {
		return err
	}
	if t.RunID != current.ID || t.Kind != "QUERY" || t.Status != "RECEIVED" {
		return context.Canceled
	}
	t.Status = "WAITING_BUDGET"
	if err = saveInput(ctx, tx, t); err != nil {
		return err
	}
	return s.event(ctx, tx, current, "QUESTION_BUDGET_WAIT", map[string]any{"inputId": inputID, "reported": current.Usage.TotalTokens, "budget": current.TokenBudget})
}

func loadInput(ctx context.Context, db database.Executor, id string) (Turn, string, error) {
	var t Turn
	var raw []byte
	var pending, status string
	err := db.QueryRow(ctx, `SELECT payload,status,pending_call_id FROM occccad.assistant_inputs WHERE id=$1`, id).Scan(&raw, &status, &pending)
	if err != nil {
		return t, pending, err
	}
	err = json.Unmarshal(raw, &t)
	t.Status = status
	return t, pending, err
}
func saveInput(ctx context.Context, db database.Executor, t Turn) error {
	_, err := db.Exec(ctx, `UPDATE occccad.assistant_inputs SET payload=$1,status=$2 WHERE id=$3 AND run_id=$4`, marshalJSON(t), t.Status, t.ID, t.RunID)
	return err
}
func (s *Service) stepQuestion(parent context.Context, id string) (resultErr error) {
	ctx := parent
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var lease int64
	if err = tx.QueryRow(ctx, `SELECT lease_until FROM occccad.assistant_inputs WHERE id=$1 FOR UPDATE`, id).Scan(&lease); err != nil {
		return err
	}
	t, pending, err := loadInput(ctx, tx, id)
	if err != nil {
		return err
	}
	if t.Kind != "QUERY" || t.Status != "RECEIVED" || lease > s.now().UnixMilli() {
		return nil
	}
	token := uuid.NewString()
	if _, err = tx.Exec(ctx, `UPDATE occccad.assistant_inputs SET lease_token=$1,lease_until=$2 WHERE id=$3`, token, s.now().Add(questionLeaseDuration).UnixMilli(), id); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.WithValue(parent, questionLeaseKey{}, questionLease{inputID: id, token: token}))
	stopRenew := s.renewQuestion(ctx, cancel, id, token, questionHeartbeat)
	defer func() {
		cancel()
		stopRenew()
		finish, stop := detached(ctx)
		defer stop()
		if resultErr != nil && !errors.Is(resultErr, context.Canceled) {
			if latest, e := loadRun(finish, s.db, t.RunID, false); e == nil {
				_ = s.failQuestion(finish, latest, t, resultErr)
			}
		}
		if latest, _, e := loadInput(finish, s.db, id); e == nil && (latest.Status == "ANSWERED" || latest.Status == "FAILED" || latest.Status == "CANCELLED") {
			s.runMu.Lock()
			delete(s.sessions, "query:"+id)
			s.runMu.Unlock()
		}
		// Retry backoff is installed by clearing this token in failQuestion;
		// cleanup must never erase that delay or another scheduler's lease.
		_, _ = s.db.Exec(finish, `UPDATE occccad.assistant_inputs SET lease_token='',lease_until=0 WHERE id=$1 AND lease_token=$2`, id, token)
	}()
	r, err := loadRun(ctx, s.db, t.RunID, false)
	if err != nil {
		return err
	}
	if pending != "" {
		c, e := loadCall(ctx, s.db, pending)
		if e != nil {
			return e
		}
		if c.RunID != r.ID || c.InputID != t.ID {
			return s.failQuestion(ctx, r, t, aiFault("AI_CALL_REFERENCE", "问题的调用引用不属于该输入", false))
		}
		if c.Status == "RECEIVED" {
			s.runMu.Lock()
			delete(s.sessions, "query:"+t.ID)
			s.runMu.Unlock()
			return s.processQuestionCall(ctx, r, t, c, nil)
		}
		if c.Status == "SENT" {
			if e = s.recoverUnknownCall(ctx, c.ID); e != nil {
				return e
			}
			c, e = loadCall(ctx, s.db, c.ID)
			if e != nil {
				return e
			}
		}
		if c.Status == "UNKNOWN" {
			if c.Failure != nil {
				return s.failQuestion(ctx, r, t, c.Failure)
			}
			return s.failQuestion(ctx, r, t, aiFault("AI_CALL_OUTCOME_UNKNOWN", "上一条模型请求的处理和计费未知；等待退避后以新请求继续", true))
		}
		if c.Status == "PREPARED" {
			c.Status = "NOT_SENT"
			if e = saveCall(ctx, s.db, c); e != nil {
				return e
			}
		}
		if err = s.requireQuestionLease(ctx, s.db, id); err != nil {
			return err
		}
		if _, e = s.db.Exec(ctx, `UPDATE occccad.assistant_inputs SET pending_call_id='' WHERE id=$1 AND lease_token=$2 AND status='RECEIVED'`, id, token); e != nil {
			return e
		}
		return nil
	}
	if err = s.requireQuestionLease(ctx, s.db, id); err != nil {
		return err
	}
	return s.planCall(ctx, r, &t)
}
func (s *Service) failQuestion(ctx context.Context, r Run, t Turn, err error) error {
	f := asFault(err)
	if f.Code == "AI_BUDGET_WAIT" {
		return nil
	}
	_, e := s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		latest, pending, e := loadInput(ctx, tx, t.ID)
		if e != nil {
			return e
		}
		if latest.RunID != r.ID || latest.Kind != "QUERY" || latest.Status != "RECEIVED" {
			return nil
		}
		if e = s.requireQuestionLease(ctx, tx, t.ID); e != nil {
			return e
		}
		// A new question has its own immutable scope and does not revise/stop a writer.
		if f.Retryable {
			if pending != "" {
				call, e := loadCall(ctx, tx, pending)
				if e != nil {
					return e
				}
				if call.Status == "UNKNOWN" || call.Status == "APPLIED" || call.Status == "NOT_SENT" {
					pending = ""
				}
			}
			wake := s.now().Add(questionRetryDelay).UnixMilli()
			if _, e = tx.Exec(ctx, `UPDATE occccad.assistant_inputs SET pending_call_id=$1,lease_token='',lease_until=$2 WHERE id=$3 AND run_id=$4 AND status='RECEIVED'`, pending, wake, t.ID, r.ID); e != nil {
				return e
			}
			return s.event(ctx, tx, current, "QUESTION_RETRY", map[string]any{"inputId": t.ID, "failure": f, "nextAttemptAt": wake})
		}
		latest.Status = "FAILED"
		latest.Reply = f.Message
		if e := saveInput(ctx, tx, latest); e != nil {
			return e
		}
		return s.event(ctx, tx, current, "QUESTION_FAILED", map[string]any{"inputId": t.ID, "failure": f})
	})
	return e
}
func (s *Service) processQuestionCall(ctx context.Context, r Run, t Turn, c CallRecord, session *planningSession) error {
	budgetReady := func() (bool, error) {
		if err := s.requireQuestionLease(ctx, s.db, t.ID); err != nil {
			return false, err
		}
		fresh, err := loadRun(ctx, s.db, r.ID, false)
		if err != nil {
			return false, err
		}
		r = fresh
		if fresh.TokenBudget != nil && fresh.Usage.TotalTokens >= *fresh.TokenBudget {
			_, err = s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
				if current.TokenBudget == nil || current.Usage.TotalTokens < *current.TokenBudget {
					return nil
				}
				return s.waitQuestionBudget(ctx, tx, current, t.ID)
			})
			return false, err
		}
		return true, nil
	}
	if ready, err := budgetReady(); err != nil || !ready {
		return err
	}
	if c.Response == nil {
		return s.failQuestion(ctx, r, t, errors.New("missing public response"))
	}
	if len(c.Response.ToolCalls) == 0 {
		_, err := s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
			if e := s.requireQuestionLease(ctx, tx, t.ID); e != nil {
				return e
			}
			latest, _, e := loadInput(ctx, tx, t.ID)
			if e != nil {
				return e
			}
			if latest.Status != "RECEIVED" {
				return nil
			}
			if current.TokenBudget != nil && current.Usage.TotalTokens >= *current.TokenBudget {
				return s.waitQuestionBudget(ctx, tx, current, t.ID)
			}
			latest.Status = "ANSWERED"
			latest.Reply = c.Response.Content
			if e = saveInput(ctx, tx, latest); e != nil {
				return e
			}
			c.Status = "APPLIED"
			if e = saveCall(ctx, tx, c); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE occccad.assistant_inputs SET pending_call_id='' WHERE id=$1`, t.ID); e != nil {
				return e
			}
			return s.event(ctx, tx, current, "QUESTION_ANSWERED", map[string]any{"inputId": t.ID, "scopeRevision": t.Scope.RevisionID, "reply": latest.Reply})
		})
		return err
	}
	reader := r
	reader.AccessMode = "READ"
	reader.Scope = t.Scope
	for i := c.ToolCursor; i < len(c.Response.ToolCalls); i++ {
		if ready, err := budgetReady(); err != nil || !ready {
			return err
		}
		current, _, err := loadInput(ctx, s.db, t.ID)
		if err != nil {
			return err
		}
		if current.Status != "RECEIVED" {
			return context.Canceled
		}
		call := c.Response.ToolCalls[i]
		var result Result
		var stored []byte
		lookupErr := s.db.QueryRow(ctx, `SELECT payload FROM occccad.assistant_tool_results WHERE call_id=$1 AND tool_id=$2`, c.ID, call.ID).Scan(&stored)
		if lookupErr == nil {
			if err = json.Unmarshal(stored, &result); err != nil {
				return err
			}
		} else if !errors.Is(lookupErr, database.ErrNoRows) {
			return lookupErr
		} else {
			switch call.Function.Name {
			case "cad_create_task", "cad_advance_task", "cad_finish", "cad_wait_for_input":
				result = Result{CallID: call.ID, IsError: true, Content: json.RawMessage(`{"code":"READ_ONLY","message":"This question cannot create or advance CAD actions."}`)}
			default:
				result = s.executeTool(ctx, reader, c, call)
			}
		}
		if err = s.requireQuestionLease(ctx, s.db, t.ID); err != nil {
			return err
		}
		if err = s.recordTool(ctx, r, &c, i, call, result); err != nil {
			return err
		}
		if session != nil {
			session.messages = append(session.messages, Message{Role: "tool", ToolCallID: call.ID, Content: string(result.Content)})
		}
	}
	_, err := s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		if err := s.requireQuestionLease(ctx, tx, t.ID); err != nil {
			return err
		}
		latest, _, err := loadInput(ctx, tx, t.ID)
		if err != nil {
			return err
		}
		if latest.Status != "RECEIVED" {
			return context.Canceled
		}
		c.Status = "APPLIED"
		if err = saveCall(ctx, tx, c); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE occccad.assistant_inputs SET pending_call_id='' WHERE id=$1 AND run_id=$2`, t.ID, r.ID)
		return err
	})
	return err
}
