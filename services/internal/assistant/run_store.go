package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
)

func loadRun(ctx context.Context, db database.Executor, id string, lock bool) (Run, error) {
	var r Run
	var raw []byte
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	err := db.QueryRow(ctx, `SELECT actor_id::text,document_id::text,access_mode,version,epoch,status,payload FROM occccad.assistant_runs WHERE id=$1`+suffix, id).Scan(&r.ActorID, &r.DocumentID, &r.AccessMode, &r.Version, &r.Epoch, &r.Status, &raw)
	if errors.Is(err, database.ErrNoRows) {
		return r, aiFault("AI_RUN_NOT_FOUND", "运行不存在或没有访问权限", false)
	}
	if err != nil {
		return r, err
	}
	actor, document, mode, version, epoch, status := r.ActorID, r.DocumentID, r.AccessMode, r.Version, r.Epoch, r.Status
	if err = json.Unmarshal(raw, &r); err != nil {
		return r, err
	}
	r.ActorID, r.DocumentID, r.AccessMode, r.Version, r.Epoch, r.Status = actor, document, mode, version, epoch, status
	return r, nil
}
func (s *Service) GetRun(ctx context.Context, actor, document, id string) (Run, error) {
	if _, err := s.automation.Context(ctx, actor, document); err != nil {
		return Run{}, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return Run{}, aiFault("AI_RUN_NOT_FOUND", "运行不存在", false)
	}
	r, err := loadRun(ctx, s.db, id, false)
	if err != nil {
		return r, err
	}
	if r.ActorID != actor || r.DocumentID != document {
		return Run{}, aiFault("AI_RUN_NOT_FOUND", "运行不存在或没有访问权限", false)
	}
	return r, nil
}
func (s *Service) saveRun(ctx context.Context, db database.Executor, r *Run) error {
	previous := r.Version
	r.Version++
	r.UpdatedAt = timestamp(s.now())
	result, err := db.Exec(ctx, `UPDATE occccad.assistant_runs SET version=$1,epoch=$2,status=$3,payload=$4,updated_at=$5,access_mode=$8 WHERE id=$6 AND version=$7`, r.Version, r.Epoch, r.Status, marshalJSON(r), r.UpdatedAt, r.ID, previous, r.AccessMode)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return aiFault("AI_RUN_CONFLICT", "运行状态已变化，请重新读取", true)
	}
	return nil
}
func (s *Service) event(ctx context.Context, db database.Executor, r *Run, kind string, data any) error {
	r.EventSequence++
	e := RunEvent{Sequence: r.EventSequence, Epoch: r.Epoch, Type: kind, At: timestamp(s.now())}
	if data != nil {
		e.Data = marshalJSON(data)
	}
	_, err := db.Exec(ctx, `INSERT INTO occccad.assistant_run_events(run_id,sequence,epoch,payload) VALUES($1,$2,$3,$4)`, r.ID, e.Sequence, e.Epoch, marshalJSON(e))
	return err
}
func (s *Service) mutateRun(ctx context.Context, id string, fn func(database.Tx, *Run) error) (Run, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	r, err := loadRun(ctx, tx, id, true)
	if err != nil {
		return r, err
	}
	if err = fn(tx, &r); err != nil {
		return r, err
	}
	if err = s.saveRun(ctx, tx, &r); err != nil {
		return r, err
	}
	return r, tx.Commit(ctx)
}

func validText(request, text string) error {
	if _, err := uuid.Parse(request); err != nil || len(strings.TrimSpace(text)) < 1 || len(text) > 8000 {
		return aiFault("AI_INPUT_INVALID", "请输入有效需求，并将本条消息控制在约2000字以内", false)
	}
	return nil
}
func (s *Service) CreateRun(ctx context.Context, actor, document string, input RunInput) (Run, error) {
	input.Text = strings.TrimSpace(input.Text)
	if err := validText(input.RequestID, input.Text); err != nil {
		return Run{}, err
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(actor+":"+document+":"+input.RequestID)).String()
	digest := modelcore.ValueDigest(marshalJSON(input))
	var priorDigest string
	if err := s.db.QueryRow(ctx, `SELECT request_digest FROM occccad.assistant_runs WHERE id=$1`, id).Scan(&priorDigest); err == nil {
		if priorDigest != digest {
			return Run{}, aiFault("IDEMPOTENCY_KEY_REUSED", "该请求编号已用于其他内容", false)
		}
		return s.GetRun(ctx, actor, document, id)
	} else if !errors.Is(err, database.ErrNoRows) {
		return Run{}, err
	}
	settings, err := s.Settings(ctx, actor)
	if err != nil {
		return Run{}, err
	}
	if !settings.Configured {
		return Run{}, aiFault("AI_KEY_REQUIRED", "请先连接 DeepSeek", false)
	}
	scope, writable, err := s.freezeScope(ctx, actor, document, input.Scope, input.WorkingBodyID)
	if err != nil {
		return Run{}, err
	}
	mode := input.Mode
	if mode == "" {
		mode = "AUTOMATIC"
	}
	if mode != "AUTOMATIC" && mode != "CONFIRM" {
		return Run{}, aiFault("AI_INPUT_INVALID", "执行模式无效", false)
	}
	if !validTokenBudget(input.TokenBudget) {
		return Run{}, aiFault("AI_INPUT_INVALID", "累计Token预算须为正数或留空", false)
	}
	budget := input.TokenBudget
	if budget == nil {
		budget = settings.TokenBudget
	}
	accessMode := "WRITE"
	kind := "GOAL"
	if !writable || isQuestion(input.Text) {
		accessMode = "READ"
		kind = "QUERY"
	}
	vault, err := s.vaultScope()
	if err != nil {
		return Run{}, err
	}
	r := Run{ID: id, CredentialScope: vault, ActorID: actor, AccessMode: accessMode, DocumentID: document, Version: 1, Epoch: 1, GoalVersion: 1, Goal: input.Text, Status: "QUEUED", Mode: mode, Scope: scope, TokenBudget: budget, BudgetVersion: 1, CreatedAt: timestamp(s.now()), UpdatedAt: timestamp(s.now()), LatestRevisionID: scope.RevisionID}
	turn := Turn{ID: uuid.NewSHA1(uuid.MustParse(id), []byte(input.RequestID)).String(), RunID: id, Sequence: 1, RequestID: input.RequestID, Text: input.Text, Kind: kind, Status: "APPLIED", Scope: scope, GoalVersion: 1, CreatedAt: r.CreatedAt}
	r.MessageSequence = 1
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO occccad.assistant_runs(id,actor_id,document_id,request_id,request_digest,access_mode,version,epoch,status,payload,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, r.ID, actor, document, input.RequestID, digest, accessMode, r.Version, r.Epoch, r.Status, marshalJSON(r), r.CreatedAt, r.UpdatedAt)
	if database.IsUniqueViolation(err) {
		return Run{}, aiFault("AI_DOCUMENT_BUSY", "该文档已有持续任务，请向现有任务补充需求或停止后再开始", true)
	}
	if err != nil {
		return r, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.assistant_inputs(id,run_id,sequence,request_id,request_digest,kind,status,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, turn.ID, r.ID, turn.Sequence, turn.RequestID, modelcore.ValueDigest(marshalJSON(MessageInput{RequestID: input.RequestID, Text: input.Text, Scope: &scope})), kind, turn.Status, marshalJSON(turn)); err != nil {
		return r, err
	}
	if err = s.event(ctx, tx, &r, "RUN_CREATED", map[string]any{"mode": mode, "budget": budget, "sourceInputId": turn.ID}); err != nil {
		return r, err
	}
	// The event sequence is part of the same initial durable checkpoint.
	if _, err = tx.Exec(ctx, `UPDATE occccad.assistant_runs SET payload=$1 WHERE id=$2`, marshalJSON(r), r.ID); err != nil {
		return r, err
	}
	if err = tx.Commit(ctx); err == nil {
		s.signal()
	}
	return r, err
}

func (s *Service) AddMessage(ctx context.Context, actor, document, id string, input MessageInput) (Run, Turn, error) {
	input.Text = strings.TrimSpace(input.Text)
	if err := validText(input.RequestID, input.Text); err != nil {
		return Run{}, Turn{}, err
	}
	current, err := s.GetRun(ctx, actor, document, id)
	if err != nil {
		return current, Turn{}, err
	}
	digest := modelcore.ValueDigest(marshalJSON(input))
	var raw []byte
	var oldDigest string
	if err = s.db.QueryRow(ctx, `SELECT payload,request_digest FROM occccad.assistant_inputs WHERE run_id=$1 AND request_id=$2`, id, input.RequestID).Scan(&raw, &oldDigest); err == nil {
		if digest != oldDigest {
			return current, Turn{}, aiFault("IDEMPOTENCY_KEY_REUSED", "消息编号已用于其他内容", false)
		}
		var t Turn
		err = json.Unmarshal(raw, &t)
		return current, t, err
	} else if !errors.Is(err, database.ErrNoRows) {
		return current, Turn{}, err
	}
	scope := current.Scope
	if input.Scope == nil && current.AccessMode == "WRITE" {
		scope.RevisionID = ""
	}
	// Recheck ACL and the frozen definition even when the caller omits a new scope.
	scope, writable, err := s.freezeScope(ctx, actor, document, &scope, "")
	if err != nil {
		return current, Turn{}, err
	}
	if input.Scope != nil {
		scope, writable, err = s.freezeScope(ctx, actor, document, input.Scope, "")
		if err != nil {
			return current, Turn{}, err
		}
	}
	question := isQuestion(input.Text) || !writable
	var turn Turn
	next, err := s.mutateRun(ctx, id, func(tx database.Tx, r *Run) error {
		if r.ActorID != actor || r.DocumentID != document {
			return aiFault("AI_RUN_NOT_FOUND", "运行不可访问", false)
		}
		if r.Status == "STOPPED" && !question {
			return aiFault("AI_RUN_FINISHED", "任务已结束，请发起新的目标", false)
		}
		r.MessageSequence++
		turn = Turn{ID: uuid.NewSHA1(uuid.MustParse(id), []byte(input.RequestID)).String(), RunID: id, Sequence: r.MessageSequence, RequestID: input.RequestID, Text: input.Text, Kind: "QUERY", Status: "RECEIVED", Scope: scope, GoalVersion: r.GoalVersion, CreatedAt: timestamp(s.now())}
		if !question {
			r.Epoch++
			r.GoalVersion++
			turn.GoalVersion = r.GoalVersion
			turn.Kind = "STEER"
			turn.Status = "WAITING_BOUNDARY"
			r.AccessMode = "WRITE"
			r.RequestAppliedCount = 0
			r.LatestRevisionID = scope.RevisionID
			r.Goal = input.Text
			r.Reply = ""
			r.Scope = scope
			r.Failure = nil
			r.WaitingReason = "已收到消息，正在等待当前操作边界"
			if r.ActiveTaskID != "" {
				r.RetiringTaskID = r.ActiveTaskID
			}
			r.ActiveTaskID = ""
			r.PendingCallID = ""
			r.LastFailureDigest = ""
			r.LastStageFailureDigest = ""
			r.ConsecutiveTransient = 0
			r.Status = "QUEUED"
			if _, e := tx.Exec(ctx, `UPDATE occccad.assistant_runs SET next_wake=0 WHERE id=$1`, r.ID); e != nil {
				return e
			}
		}
		_, e := tx.Exec(ctx, `INSERT INTO occccad.assistant_inputs(id,run_id,sequence,request_id,request_digest,kind,status,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, turn.ID, id, turn.Sequence, turn.RequestID, digest, turn.Kind, turn.Status, marshalJSON(turn))
		if e != nil {
			return e
		}
		return s.event(ctx, tx, r, "INPUT_RECEIVED", turn)
	})
	if err == nil {
		if !question {
			s.cancelWork(id)
		}
		s.signal()
	}
	return next, turn, err
}

func boundedLimit(limit int) int {
	if limit < 1 {
		return 30
	}
	if limit > 100 {
		return 100
	}
	return limit
}
func pagePayload[T any](ctx context.Context, db database.Executor, table, run string, after, before uint64, limit int) (Page[T], error) {
	p := Page[T]{Items: []T{}}
	limit = boundedLimit(limit)
	direction := "DESC"
	condition := ""
	bound := uint64(0)
	if after > 0 {
		direction = "ASC"
		condition = " AND sequence>$2"
		bound = after
	} else if before > 0 {
		condition = " AND sequence<$2"
		bound = before
	}
	var query string
	switch table {
	case "assistant_inputs":
		query = `SELECT sequence,payload FROM occccad.assistant_inputs WHERE run_id=$1`
	case "assistant_run_events":
		query = `SELECT sequence,payload FROM occccad.assistant_run_events WHERE run_id=$1 AND COALESCE(payload->>'type','')<>'MODEL_REQUEST_CAPTURED'`
	default:
		return p, aiFault("AI_PAGE_INVALID", "未知的分页集合", false)
	}
	if condition == "" {
		condition = " AND sequence>$2"
	}
	query += condition + " ORDER BY sequence " + direction + " LIMIT $3"

	rows, err := db.Query(ctx, query, run, bound, limit+1)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	type item struct {
		sequence uint64
		value    T
	}
	all := []item{}
	for rows.Next() {
		var seq uint64
		var raw []byte
		var v T
		if err = rows.Scan(&seq, &raw); err != nil {
			return p, err
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return p, err
		}
		all = append(all, item{seq, v})
	}
	if err = rows.Err(); err != nil {
		return p, err
	}
	if len(all) > limit {
		if direction == "DESC" {
			p.HasEarlier = true
		} else {
			p.HasMore = true
		}
		all = all[:limit]
	}
	sort.Slice(all, func(i, j int) bool { return all[i].sequence < all[j].sequence })
	for _, v := range all {
		p.Items = append(p.Items, v.value)
		p.NextAfter = v.sequence
	}
	return p, nil
}
func (s *Service) Messages(ctx context.Context, actor, document, id string, after, before uint64, limit int) (Page[Turn], error) {
	if _, e := s.GetRun(ctx, actor, document, id); e != nil {
		return Page[Turn]{}, e
	}
	p, err := pagePayload[Turn](ctx, s.db, "assistant_inputs", id, after, before, limit)
	if err != nil || len(p.Items) == 0 {
		return p, err
	}
	// Bulk stop/budget changes update the indexed state without rewriting every
	// historical JSON payload. The status column is authoritative on read.
	rows, err := s.db.Query(ctx, `SELECT sequence,status FROM occccad.assistant_inputs WHERE run_id=$1 AND sequence>=$2 AND sequence<=$3`, id, p.Items[0].Sequence, p.Items[len(p.Items)-1].Sequence)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	statuses := map[uint64]string{}
	for rows.Next() {
		var sequence uint64
		var status string
		if err = rows.Scan(&sequence, &status); err != nil {
			return p, err
		}
		statuses[sequence] = status
	}
	if err = rows.Err(); err != nil {
		return p, err
	}
	for i := range p.Items {
		p.Items[i].Status = statuses[p.Items[i].Sequence]
	}
	return p, nil
}
func (s *Service) Events(ctx context.Context, actor, document, id string, after, before uint64, limit int) (Page[RunEvent], error) {
	if _, e := s.GetRun(ctx, actor, document, id); e != nil {
		return Page[RunEvent]{}, e
	}
	return pagePayload[RunEvent](ctx, s.db, "assistant_run_events", id, after, before, limit)
}
func (s *Service) Stages(ctx context.Context, actor, document, id string, after, before uint64, limit int) (Page[Stage], error) {
	if _, e := s.GetRun(ctx, actor, document, id); e != nil {
		return Page[Stage]{}, e
	}
	limit = boundedLimit(limit)
	direction := "DESC"
	condition := "sequence>0"
	bound := uint64(0)
	if after > 0 {
		direction = "ASC"
		condition = "sequence>$2"
		bound = after
	} else if before > 0 {
		condition = "sequence<$2"
		bound = before
	}
	if condition == "sequence>0" {
		condition = "sequence>$2"
	}
	query := `SELECT sequence,epoch,goal_version,task_id::text,status FROM occccad.assistant_stages WHERE run_id=$1`
	query += " AND " + condition + " ORDER BY sequence " + direction + " LIMIT $3"
	rows, e := s.db.Query(ctx, query, id, bound, limit+1)
	p := Page[Stage]{Items: []Stage{}}
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		var st Stage
		if e = rows.Scan(&st.Sequence, &st.Epoch, &st.GoalVersion, &st.TaskID, &st.Status); e != nil {
			return p, e
		}
		p.Items = append(p.Items, st)
	}
	if e = rows.Err(); e != nil {
		return p, e
	}
	if len(p.Items) > limit {
		if direction == "DESC" {
			p.HasEarlier = true
		} else {
			p.HasMore = true
		}
		p.Items = p.Items[:limit]
	}
	sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Sequence < p.Items[j].Sequence })
	for _, st := range p.Items {
		p.NextAfter = st.Sequence
	}
	return p, nil
}
func (s *Service) ListRuns(ctx context.Context, actor, document, cursor string, limit int) (map[string]any, error) {
	if _, e := s.automation.Context(ctx, actor, document); e != nil {
		return nil, e
	}
	limit = boundedLimit(limit)
	var position struct{ At, ID string }
	if cursor != "" {
		raw, e := decodeCursor(cursor)
		if e != nil || json.Unmarshal(raw, &position) != nil || position.At == "" || position.ID == "" {
			return nil, aiFault("INVALID_CURSOR", "运行游标无效", false)
		}
	}
	rows, e := s.db.Query(ctx, `SELECT id::text FROM occccad.assistant_runs WHERE actor_id=$1 AND document_id=$2 AND ($3='' OR created_at<$3 OR (created_at=$3 AND id::text<$4)) ORDER BY created_at DESC,id DESC LIMIT $5`, actor, document, position.At, position.ID, limit+1)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	more := len(ids) > limit
	if more {
		ids = ids[:limit]
	}
	items := []Run{}
	next := ""
	for _, id := range ids {
		r, e := loadRun(ctx, s.db, id, false)
		if e != nil {
			return nil, e
		}
		items = append(items, r)
		next = encodeCursor(map[string]string{"At": r.CreatedAt, "ID": id})
	}
	return map[string]any{"items": items, "nextCursor": next, "hasMore": more}, nil
}
