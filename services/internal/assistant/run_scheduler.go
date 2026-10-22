package assistant

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
)

type planningSession struct {
	mu                sync.Mutex
	epoch             uint64
	generation        string
	messages          []Message
	head              string
	pendingCompletion *Completion
}

func encodeCursor(v any) string { return base64.RawURLEncoding.EncodeToString(marshalJSON(v)) }
func decodeCursor(v string) ([]byte, error) {
	if len(v) > 2048 {
		return nil, aiFault("INVALID_CURSOR", "游标过长", false)
	}
	return base64.RawURLEncoding.DecodeString(v)
}
func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Service) cancelWork(key string) {
	s.runMu.Lock()
	if c := s.runs[key]; c != nil {
		c()
	}
	delete(s.sessions, key)
	s.runMu.Unlock()
}
func (s *Service) Start() {
	s.runMu.Lock()
	if s.cancel != nil || s.db == nil {
		s.runMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	s.runMu.Unlock()
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-s.wake:
			}
			s.dispatch(ctx)
		}
	}()
}
func (s *Service) Close() {
	s.runMu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	for _, cancel := range s.runs {
		cancel()
	}
	s.runMu.Unlock()
	s.wg.Wait()
}
func runnable(status string) bool {
	switch status {
	case "QUEUED", "PLANNING", "EXECUTING", "WAITING_RETRY":
		return true
	}
	return false
}

// Dispatch reads before writers and reserve one slot for interactive reads.
// Each actor has at most two in-flight requests; quotas bound concurrency only.
func (s *Service) dispatch(ctx context.Context) {
	vault, e := s.vaultScope()
	if e != nil {
		return
	}
	s.reconcileAbandonedCalls(ctx, vault)
	queries := []struct {
		query    string
		question bool
	}{
		{`SELECT id,actor_id FROM (SELECT i.id::text AS id,r.actor_id::text AS actor_id,r.updated_at,i.sequence,ROW_NUMBER() OVER (PARTITION BY r.actor_id ORDER BY r.updated_at,i.sequence) AS actor_rank FROM occccad.assistant_inputs i JOIN occccad.assistant_runs r ON r.id=i.run_id WHERE r.payload->>'credentialScope'=$2 AND i.kind='QUERY' AND i.status='RECEIVED' AND i.lease_until<=$1) candidates WHERE actor_rank<=2 ORDER BY updated_at,sequence,id LIMIT 32`, true},
		{`SELECT id,actor_id FROM (SELECT id::text AS id,actor_id::text AS actor_id,updated_at,ROW_NUMBER() OVER (PARTITION BY actor_id ORDER BY updated_at,id) AS actor_rank FROM occccad.assistant_runs WHERE payload->>'credentialScope'=$2 AND (status IN ('QUEUED','PLANNING','EXECUTING','WAITING_RETRY') OR COALESCE(payload->>'retiringTaskId','')<>'') AND next_wake<=$1 AND lease_until<=$1) candidates WHERE actor_rank<=2 ORDER BY updated_at,id LIMIT 32`, false},
	}
	for _, lane := range queries {
		rows, err := s.db.Query(ctx, lane.query, s.now().UnixMilli(), vault)
		if err != nil {
			continue
		}
		type work struct{ id, actor string }
		items := []work{}
		for rows.Next() {
			var v work
			if rows.Scan(&v.id, &v.actor) == nil {
				items = append(items, v)
			}
		}
		rowErr := rows.Err()
		rows.Close()
		if rowErr != nil {
			continue
		}
		for _, item := range items {
			s.launch(ctx, item.id, item.actor, lane.question)
		}
	}
}
func (s *Service) launch(parent context.Context, id, actor string, question bool) bool {
	key := id
	if question {
		key = "query:" + id
	}
	s.runMu.Lock()
	actorActive, actorWriters, writers := 0, 0, 0
	for k, a := range s.runActors {
		if a == actor {
			actorActive++
			if !strings.HasPrefix(k, "query:") {
				actorWriters++
			}
		}
		if !strings.HasPrefix(k, "query:") {
			writers++
		}
	}
	if len(s.runs) >= 4 || s.runs[key] != nil || actorActive >= 2 || (!question && (writers >= 3 || actorWriters >= 1)) {
		s.runMu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(parent)
	s.runs[key] = cancel
	s.runActors[key] = actor
	s.wg.Add(1)
	s.runMu.Unlock()
	go func() {
		defer s.wg.Done()
		defer cancel()
		defer func() { s.runMu.Lock(); delete(s.runs, key); delete(s.runActors, key); s.runMu.Unlock(); s.signal() }()
		if question {
			_ = s.stepQuestion(ctx, id)
		} else {
			_ = s.stepRun(ctx, id)
		}
	}()
	return true
}

// Old epochs and stopped runs may have lost their pending-call pointer before
// a process died. Reconcile only expired leases; never claim an in-flight call.
func (s *Service) reconcileAbandonedCalls(ctx context.Context, vault string) {
	rows, e := s.db.Query(ctx, `SELECT c.id::text,c.run_id::text,c.input_id FROM occccad.assistant_model_calls c JOIN occccad.assistant_runs r ON r.id=c.run_id LEFT JOIN occccad.assistant_inputs i ON i.id::text=NULLIF(c.input_id,'') WHERE r.payload->>'credentialScope'=$2 AND c.status='SENT' AND r.lease_until<=$1 AND (c.input_id='' OR COALESCE(i.lease_until,0)<=$1) ORDER BY c.started_at,c.id LIMIT 16`, s.now().UnixMilli(), vault)
	if e != nil {
		return
	}
	type pending struct{ id, run, input string }
	items := []pending{}
	for rows.Next() {
		var v pending
		if rows.Scan(&v.id, &v.run, &v.input) == nil {
			items = append(items, v)
		}
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return
	}
	for _, v := range items {
		key := v.run
		if v.input != "" {
			key = "query:" + v.input
		}
		s.runMu.Lock()
		busy := s.runs[key] != nil
		s.runMu.Unlock()
		if !busy {
			_ = s.recoverUnknownCall(ctx, v.id)
		}
	}
}
func (s *Service) claimRun(ctx context.Context, id string) (Run, string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Run{}, "", err
	}
	defer tx.Rollback(ctx)
	r, err := loadRun(ctx, tx, id, true)
	if err != nil {
		return r, "", err
	}
	var lease, wake int64
	if err = tx.QueryRow(ctx, `SELECT lease_until,next_wake FROM occccad.assistant_runs WHERE id=$1`, id).Scan(&lease, &wake); err != nil {
		return r, "", err
	}
	if (!runnable(r.Status) && r.RetiringTaskID == "") || lease > s.now().UnixMilli() || wake > s.now().UnixMilli() {
		return r, "", aiFault("AI_LEASE_BUSY", "运行正在其他调度器中处理或等待恢复", true)
	}
	token := uuid.NewString()
	if _, err = tx.Exec(ctx, `UPDATE occccad.assistant_runs SET lease_token=$1,lease_until=$2 WHERE id=$3`, token, s.now().Add(45*time.Second).UnixMilli(), id); err != nil {
		return r, "", err
	}
	return r, token, tx.Commit(ctx)
}
func (s *Service) renew(ctx context.Context, cancel context.CancelFunc, r Run, token string) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				result, err := s.db.Exec(ctx, `UPDATE occccad.assistant_runs SET lease_until=$1 WHERE id=$2 AND epoch=$3 AND lease_token=$4`, s.now().Add(45*time.Second).UnixMilli(), r.ID, r.Epoch, token)
				if err != nil || result.RowsAffected() != 1 {
					cancel()
					return
				}
			}
		}
	}()
	return func() { close(stop); <-done }
}

// Step is the same leased production scheduler used by Start, exposed for
// controlled clocks/fault injection; it has no total loop or deadline limit.
func (s *Service) Step(ctx context.Context, actor, document, id string) (Run, error) {
	if _, err := s.GetRun(ctx, actor, document, id); err != nil {
		return Run{}, err
	}
	err := s.stepRun(ctx, id)
	r, e := s.GetRun(ctx, actor, document, id)
	if err == nil {
		err = e
	}
	return r, err
}
func (s *Service) stepRun(parent context.Context, id string) (resultErr error) {
	r, token, err := s.claimRun(parent, id)
	if err != nil {
		return err
	}
	defer func() {
		if resultErr != nil && !errors.Is(resultErr, context.Canceled) {
			finish, stop := detached(parent)
			defer stop()
			latest, e := loadRun(finish, s.db, id, false)
			if e == nil && latest.Epoch == r.Epoch && runnable(latest.Status) {
				_ = s.deferRun(finish, latest, asFault(resultErr))
			}
		}
	}()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stopRenew := s.renew(ctx, cancel, r, token)
	defer stopRenew()
	defer func() {
		finish, stop := detached(parent)
		defer stop()
		_, _ = s.db.Exec(finish, `UPDATE occccad.assistant_runs SET lease_token='',lease_until=0 WHERE id=$1 AND lease_token=$2`, id, token)
	}()
	if r.RetiringTaskID != "" {
		return s.retireStage(ctx, r)
	}
	if r.ActiveTaskID != "" {
		return s.advanceStage(ctx, r)
	}
	if r.PendingCallID != "" {
		c, e := loadCall(ctx, s.db, r.PendingCallID)
		if e != nil {
			return e
		}
		if c.Status == "SENT" {
			if e = s.recoverUnknownCall(ctx, c.ID); e != nil {
				return e
			}
			return s.deferRun(ctx, r, asFault(errors.New("provider outcome unknown")))
		}
		if c.Status == "RECEIVED" {
			// The durable public response has no private reasoning. An earlier
			// online session may still end in unanswered tool calls (e.g. budget
			// pause). Rebuild its next request from facts, never reuse that tail.
			s.runMu.Lock()
			delete(s.sessions, r.ID)
			s.runMu.Unlock()
			return s.processCall(ctx, r, c, nil)
		}
		if c.Status == "PREPARED" { // No outbound attempt was committed; safe to build a new intent.
			c.Status = "NOT_SENT"
			_ = saveCall(ctx, s.db, c)
		}
		_, e = s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
			if current.Epoch != r.Epoch {
				return context.Canceled
			}
			current.PendingCallID = ""
			current.Status = "PLANNING"
			return nil
		})
		return e
	}
	return s.planCall(ctx, r, nil)
}
func (s *Service) deferRun(ctx context.Context, r Run, f *automation.Fault) error {
	_, err := s.mutateRun(ctx, r.ID, func(tx database.Tx, current *Run) error {
		if current.Epoch != r.Epoch || terminal(current.Status) || current.Status == "PAUSED" {
			return context.Canceled
		}
		current.PendingCallID = ""
		current.Failure = f
		digest := modelcore.ValueDigest(marshalJSON(map[string]any{"head": current.LatestRevisionID, "goalVersion": current.GoalVersion, "code": f.Code, "message": f.Message}))
		if f.Retryable {
			current.Status = "WAITING_RETRY"
			current.ConsecutiveTransient++
			delay := time.Second << min(current.ConsecutiveTransient, 8)
			if _, e := tx.Exec(ctx, `UPDATE occccad.assistant_runs SET next_wake=$1 WHERE id=$2`, s.now().Add(delay).UnixMilli(), r.ID); e != nil {
				return e
			}
			current.WaitingReason = "临时连接或限流，进度已保存，等待退避后继续"
		} else {
			current.Status = "WAITING_INPUT"
			current.WaitingReason = f.Message
		}
		current.LastFailureDigest = digest
		return s.event(ctx, tx, current, "RUN_WAIT", map[string]any{"status": current.Status, "reason": current.WaitingReason})
	})
	return err
}
