//go:build integration

package assistant

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
)

// These are test-owned fault-injection records and a synthetic definition;
// no shared CAD data, production credential or native geometry is required.
type questionObservedDB struct {
	database.DB
	transactions atomic.Int64
}
type questionObservedTx struct {
	database.Tx
	parent *questionObservedDB
	ended  atomic.Bool
}

func (d *questionObservedDB) Begin(ctx context.Context) (database.Tx, error) {
	tx, e := d.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	d.transactions.Add(1)
	return &questionObservedTx{Tx: tx, parent: d}, nil
}
func (t *questionObservedTx) Commit(ctx context.Context) error {
	e := t.Tx.Commit(ctx)
	if t.ended.CompareAndSwap(false, true) {
		t.parent.transactions.Add(-1)
	}
	return e
}
func (t *questionObservedTx) Rollback(ctx context.Context) error {
	e := t.Tx.Rollback(ctx)
	if t.ended.CompareAndSwap(false, true) {
		t.parent.transactions.Add(-1)
	}
	return e
}

type questionProvider struct {
	calls    atomic.Int64
	complete func(context.Context) (Completion, error)
}

func (*questionProvider) Validate(context.Context, string) error { return nil }
func (*questionProvider) Models(context.Context, string) ([]Model, error) {
	return []Model{{ID: "deepseek-flash"}}, nil
}
func (p *questionProvider) Complete(ctx context.Context, _ string, _ []Message, _ []Tool, _ ...Generation) (Completion, error) {
	p.calls.Add(1)
	if p.complete != nil {
		return p.complete(ctx)
	}
	return Completion{}, errors.New("unexpected provider call")
}

func questionFixture(t *testing.T) (*Service, *questionObservedDB, *questionProvider, Run, Turn) {
	t.Helper()
	db, e := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "question_fault_test.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	if e = database.Migrate(t.Context(), db); e != nil {
		t.Fatal(e)
	}
	actor, document, revision, body := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, e = db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name) VALUES($1,$2,'Question fault fixture')`, actor, actor+"@test.invalid"); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(t.Context(), `INSERT INTO occccad.documents(id,name,document_type,owner_user_id) VALUES($1,'Question fault fixture','PART',$2)`, document, actor); e != nil {
		t.Fatal(e)
	}
	snapshot := workspace.ModelSnapshot{CommandBaseline: workspace.CommandBaseline{WorkspaceID: uuid.NewString(), RevisionID: revision, Sequence: 1}, DocumentID: document, DocumentType: "PART", Model: marshalJSON(workspace.PartModel{Units: "mm", ActiveBodyID: body, Bodies: []workspace.PartBody{{ID: body, Name: "Body"}}})}
	ws := &syntheticStageWorkspace{head: snapshot, versions: map[string]workspace.ModelSnapshot{revision: snapshot}}
	tracked := &questionObservedDB{DB: db}
	s := New(tracked, automation.New(tracked, ws, access.New(tracked)), Options{KeyFile: filepath.Join(t.TempDir(), "question-vault.key")})
	t.Cleanup(s.Close)
	provider := &questionProvider{}
	s.provider = provider
	now := timestamp(s.now())
	r := Run{ID: uuid.NewString(), ActorID: actor, DocumentID: document, AccessMode: "WRITE", Version: 1, Epoch: 1, GoalVersion: 1, Goal: "Keep building the original model", Status: "EXECUTING", ActiveTaskID: "writer-task", PendingCallID: "writer-call", LatestRevisionID: revision, Mode: "AUTOMATIC", Scope: FrozenScope{RootDocumentID: document, DocumentID: document, RevisionID: revision, WorkingBodyID: body, Selections: []SelectedReference{}, Complete: true}, CreatedAt: now, UpdatedAt: now}
	input := Turn{ID: uuid.NewString(), RunID: r.ID, Sequence: 1, RequestID: uuid.NewString(), Text: "当前模型是什么尺寸？", Kind: "QUERY", Status: "RECEIVED", Scope: r.Scope, GoalVersion: 1, CreatedAt: now}
	r.CurrentTurnID = input.ID
	input.Usage = &TurnUsage{}
	if _, e = db.Exec(t.Context(), `INSERT INTO occccad.assistant_runs(id,actor_id,document_id,request_id,request_digest,access_mode,version,epoch,status,payload,created_at,updated_at) VALUES($1,$2,$3,$4,'question-fixture','WRITE',1,1,$5,$6,$7,$7)`, r.ID, actor, document, uuid.NewString(), r.Status, marshalJSON(r), now); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(t.Context(), `INSERT INTO occccad.assistant_inputs(id,run_id,sequence,request_id,request_digest,kind,status,payload) VALUES($1,$2,1,$3,'question-fixture','QUERY','RECEIVED',$4)`, input.ID, r.ID, input.RequestID, marshalJSON(input)); e != nil {
		t.Fatal(e)
	}
	return s, tracked, provider, r, input
}
func saveQuestionFixtureCall(t *testing.T, s *Service, r Run, input Turn, c CallRecord) {
	t.Helper()
	c.RunID, c.InputID, c.TurnID = r.ID, input.ID, input.ID
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if c.LogicalID == "" {
		c.LogicalID = uuid.NewString()
	}
	if c.StartedAt == "" {
		c.StartedAt = timestamp(s.now())
	}
	if _, e := s.db.Exec(t.Context(), `INSERT INTO occccad.assistant_model_calls(id,run_id,input_id,logical_id,attempt,epoch,status,payload,started_at) VALUES($1,$2,$3,$4,1,$5,$6,$7,$8)`, c.ID, r.ID, input.ID, c.LogicalID, r.Epoch, c.Status, marshalJSON(c), c.StartedAt); e != nil {
		t.Fatal(e)
	}
	if _, e := s.db.Exec(t.Context(), `UPDATE occccad.assistant_inputs SET pending_call_id=$1 WHERE id=$2`, c.ID, input.ID); e != nil {
		t.Fatal(e)
	}
}

func TestAssistantQuestionRetryPreservesBackoffAndWriter(t *testing.T) {
	s, _, provider, r, input := questionFixture(t)
	now := s.now()
	s.options.Now = func() time.Time { return now }
	saveQuestionFixtureCall(t, s, r, input, CallRecord{ID: uuid.NewString(), Status: "UNKNOWN", Failure: &automation.Fault{Code: "AI_RATE_LIMITED", Message: "try later", Retryable: true}})
	if e := s.stepQuestion(t.Context(), input.ID); e != nil {
		t.Fatal(e)
	}
	var token, pending string
	var until int64
	if e := s.db.QueryRow(t.Context(), `SELECT lease_token,lease_until,pending_call_id FROM occccad.assistant_inputs WHERE id=$1`, input.ID).Scan(&token, &until, &pending); e != nil {
		t.Fatal(e)
	}
	if token != "" || until != now.Add(questionRetryDelay).UnixMilli() || pending != "" {
		t.Fatal("deferred cleanup erased retry backoff or retained unknown outbound attempt", token, until, pending)
	}
	if e := s.stepQuestion(t.Context(), input.ID); e != nil {
		t.Fatal(e)
	}
	if provider.calls.Load() != 0 {
		t.Fatal("retry backoff was bypassed")
	}
	fresh, e := loadRun(t.Context(), s.db, r.ID, false)
	if e != nil || fresh.Status != r.Status || fresh.Epoch != r.Epoch || fresh.ActiveTaskID != r.ActiveTaskID || fresh.PendingCallID != r.PendingCallID {
		t.Fatal("question failure changed writer progress", fresh, e)
	}
}

func TestAssistantQuestionSavedResponseResumesAfterBudgetIncrease(t *testing.T) {
	s, _, provider, r, input := questionFixture(t)
	budget := int64(100)
	r.TokenBudget = &budget
	r.Usage.TotalTokens = 150
	r.ModelCalls = 1
	if e := s.saveRun(t.Context(), s.db, &r); e != nil {
		t.Fatal(e)
	}
	call := CallRecord{ID: uuid.NewString(), Status: "RECEIVED", Response: &Message{Role: "assistant", Content: "尺寸来自这条问题发送时冻结的模型版本。"}}
	saveQuestionFixtureCall(t, s, r, input, call)
	s.sessions["query:"+input.ID] = &planningSession{}
	if e := s.stepQuestion(t.Context(), input.ID); e != nil {
		t.Fatal(e)
	}
	waiting, pending, e := loadInput(t.Context(), s.db, input.ID)
	if e != nil || waiting.Status != "WAITING_BUDGET" || pending != call.ID {
		t.Fatal("budget did not retain the saved response", waiting, pending, e)
	}
	saved, e := loadCall(t.Context(), s.db, call.ID)
	if e != nil || saved.Status != "RECEIVED" || saved.Response == nil || saved.Response.Content != call.Response.Content {
		t.Fatal("budget wait discarded the paid durable response", e)
	}
	current, e := loadRun(t.Context(), s.db, r.ID, false)
	if e != nil || current.Status != r.Status || current.ActiveTaskID != r.ActiveTaskID || current.PendingCallID != r.PendingCallID {
		t.Fatal("question budget stopped writer", current, e)
	}
	larger := int64(1000)
	current, e = s.ControlRun(t.Context(), r.ActorID, r.DocumentID, r.ID, RunControl{Action: "budget", ExpectedVersion: current.Version, Epoch: current.Epoch, TokenBudget: &larger})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.stepQuestion(t.Context(), input.ID); e != nil {
		t.Fatal(e)
	}
	answered, pending, e := loadInput(t.Context(), s.db, input.ID)
	if e != nil || answered.Status != "ANSWERED" || answered.Reply != call.Response.Content || pending != "" {
		t.Fatal("saved response was not resumed", answered, pending, e)
	}
	if provider.calls.Load() != 0 {
		t.Fatal("resuming an already paid response invoked the provider again")
	}
	if s.sessions["query:"+input.ID] != nil {
		t.Fatal("answered question retained an unbounded in-memory session")
	}
	current, e = loadRun(t.Context(), s.db, r.ID, false)
	if e != nil || current.Status != r.Status || current.ModelCalls != 1 || current.ActiveTaskID != r.ActiveTaskID || current.PendingCallID != r.PendingCallID {
		t.Fatal("answering changed writer state or paid call count", current, e)
	}
}

func TestAssistantQuestionFailedInputReleasesSession(t *testing.T) {
	s, _, _, r, input := questionFixture(t)
	s.sessions["query:"+input.ID] = &planningSession{}
	saveQuestionFixtureCall(t, s, r, input, CallRecord{ID: uuid.NewString(), Status: "UNKNOWN", Failure: &automation.Fault{Code: "AI_AUTHENTICATION", Message: "fixture authentication rejected"}})
	if err := s.stepQuestion(t.Context(), input.ID); err != nil {
		t.Fatal(err)
	}
	failed, _, err := loadInput(t.Context(), s.db, input.ID)
	if err != nil || failed.Status != "FAILED" || s.sessions["query:"+input.ID] != nil {
		t.Fatal("failed question did not release its in-memory session", failed, err)
	}
}

func TestAssistantQuestionPrepareBudgetDoesNotPauseWriter(t *testing.T) {
	s, _, _, r, input := questionFixture(t)
	budget := int64(10)
	r.TokenBudget = &budget
	r.Usage.TotalTokens = 10
	if e := s.saveRun(t.Context(), s.db, &r); e != nil {
		t.Fatal(e)
	}
	_, e := s.prepareCall(t.Context(), r, input.ID, workspace.CommandBaseline{RevisionID: r.LatestRevisionID}, []Message{{Role: "user", Content: input.Text}})
	if asFault(e).Code != "AI_BUDGET_WAIT" {
		t.Fatal("missing budget gate", e)
	}
	current, e := loadRun(t.Context(), s.db, r.ID, false)
	if e != nil || current.Status != r.Status || current.ActiveTaskID != r.ActiveTaskID || current.PendingCallID != r.PendingCallID {
		t.Fatal("question call preparation changed writer state", current, e)
	}
	question, _, e := loadInput(t.Context(), s.db, input.ID)
	if e != nil || question.Status != "WAITING_BUDGET" {
		t.Fatal("question was not independently paused for budget", question, e)
	}
}

func TestAssistantQuestionHeartbeatRenewsAndCancelsLostLease(t *testing.T) {
	s, _, _, _, input := questionFixture(t)
	var clock atomic.Int64
	clock.Store(s.now().UnixMilli())
	s.options.Now = func() time.Time { return time.UnixMilli(clock.Load()) }
	token := uuid.NewString()
	if _, e := s.db.Exec(t.Context(), `UPDATE occccad.assistant_inputs SET lease_token=$1,lease_until=$2 WHERE id=$3`, token, s.now().Add(questionLeaseDuration).UnixMilli(), input.ID); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), questionLeaseKey{}, questionLease{inputID: input.ID, token: token}))
	defer cancel()
	stop := s.renewQuestion(ctx, cancel, input.ID, token, 5*time.Millisecond)
	defer stop()
	clock.Add((20 * time.Second).Milliseconds())
	deadline := time.Now().Add(time.Second)
	var until int64
	for time.Now().Before(deadline) {
		if e := s.db.QueryRow(t.Context(), `SELECT lease_until FROM occccad.assistant_inputs WHERE id=$1`, input.ID).Scan(&until); e != nil {
			t.Fatal(e)
		}
		if until == s.now().Add(questionLeaseDuration).UnixMilli() {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if until != s.now().Add(questionLeaseDuration).UnixMilli() {
		t.Fatal("heartbeat did not extend the independent 45-second question lease", until)
	}
	if _, e := s.db.Exec(t.Context(), `UPDATE occccad.assistant_inputs SET lease_token=$1 WHERE id=$2`, uuid.NewString(), input.ID); e != nil {
		t.Fatal(e)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("lost question lease did not cancel pending I/O")
	}
	if !errors.Is(s.requireQuestionLease(t.Context(), s.db, input.ID), nil) {
		t.Fatal("unleased direct helper context should not be fenced")
	}
}

func TestAssistantQuestionProviderWaitHoldsNoTransactionAndSurvivesWriterPause(t *testing.T) {
	s, tracked, provider, r, input := questionFixture(t)
	if _, e := s.SaveKey(t.Context(), r.ActorID, "sk-question-owned-fixture-key"); e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	provider.complete = func(ctx context.Context) (Completion, error) {
		if tracked.transactions.Load() != 0 {
			return Completion{}, errors.New("provider wait holds a DB transaction")
		}
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return Completion{}, ctx.Err()
		}
		return Completion{Message: Message{Role: "assistant", Content: "Frozen model dimensions"}, Usage: &Usage{PromptTokens: 8, CompletionTokens: 2, TotalTokens: 10}}, nil
	}
	done := make(chan error, 1)
	go func() { done <- s.stepQuestion(t.Context(), input.ID) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}
	if tracked.transactions.Load() != 0 {
		t.Fatal("provider wait serialized reads behind a DB transaction")
	}
	var token string
	var until int64
	if e := s.db.QueryRow(t.Context(), `SELECT lease_token,lease_until FROM occccad.assistant_inputs WHERE id=$1`, input.ID).Scan(&token, &until); e != nil {
		t.Fatal(e)
	}
	if token == "" || until < s.now().Add(44*time.Second).UnixMilli() || until > s.now().Add(46*time.Second).UnixMilli() {
		t.Fatal("question did not claim a short independent lease", token, until)
	}
	if e := s.renewQuestionOnce(t.Context(), input.ID, token); e != nil {
		t.Fatal("could not renew while provider was waiting", e)
	}
	paused, e := s.mutateRun(t.Context(), r.ID, func(tx database.Tx, current *Run) error { current.Status = "PAUSED"; current.Epoch++; return nil })
	if e != nil {
		t.Fatal(e)
	}
	close(release)
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("question did not finish independently of paused writer")
	}
	answer, _, e := loadInput(t.Context(), s.db, input.ID)
	if e != nil || answer.Status != "ANSWERED" || answer.Reply != "Frozen model dimensions" {
		t.Fatal("writer epoch change cancelled independent question", answer, e)
	}
	latest, e := loadRun(t.Context(), s.db, r.ID, false)
	if e != nil || latest.Status != "PAUSED" || latest.Epoch != paused.Epoch || latest.ActiveTaskID != r.ActiveTaskID {
		t.Fatal("question completion resumed/revised writer", latest, e)
	}
}
