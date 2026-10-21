package automation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type StepSpec struct {
	ID          string          `json:"id"`
	OperationID string          `json:"operationId"`
	Version     int             `json:"version"`
	Arguments   json.RawMessage `json:"arguments"`
	// A binding is an explicit reference to a preceding step's output, never
	// a tree position, display name or an arbitrary expression.
	Bindings      map[string]string `json:"bindings,omitempty"`
	TypedBindings []InputBinding    `json:"typedBindings,omitempty"`
}
type Presentation struct {
	Status     string `json:"status"` // NOT_PRESENTED | PRESENTED | FAILED
	RevisionID string `json:"revisionId,omitempty"`
	At         string `json:"at,omitempty"`
}
type Confirmation struct {
	RunID        string                    `json:"runId,omitempty"`
	PlanEpoch    uint64                    `json:"planEpoch"`
	TaskVersion  uint64                    `json:"taskVersion"`
	StepID       string                    `json:"stepId"`
	PreviewID    string                    `json:"previewId"`
	ExpectedHead workspace.CommandBaseline `json:"expectedHead"`
	IntentDigest string                    `json:"intentDigest"`
}
type Step struct {
	StepSpec
	Phase           string                    `json:"phase"`
	Action          *ActionRequest            `json:"action,omitempty"`
	Preview         *workspace.CommandPreview `json:"preview,omitempty"`
	Result          *ActionResult             `json:"result,omitempty"`
	Failure         *Fault                    `json:"failure,omitempty"`
	Presentation    Presentation              `json:"presentation"`
	Confirmation    *Confirmation             `json:"confirmation,omitempty"`
	Handoff         *HandoffExpectation       `json:"handoff,omitempty"`
	ReceiptRecorded bool                      `json:"receiptRecorded,omitempty"`
}
type Event struct {
	Sequence uint64 `json:"sequence"`
	StepID   string `json:"stepId,omitempty"`
	Type     string `json:"type"`
	At       string `json:"at"`
}
type Task struct {
	ID              string                    `json:"id"`
	DocumentID      string                    `json:"documentId"`
	Version         uint64                    `json:"version"`
	Goal            string                    `json:"goal"`
	Status          string                    `json:"status"`
	Baseline        workspace.CommandBaseline `json:"baseline"`
	CatalogDigest   string                    `json:"catalogDigest"`
	Steps           []Step                    `json:"steps"`
	Events          []Event                   `json:"events"`
	EventSequence   uint64                    `json:"eventSequence"`
	EventsTruncated bool                      `json:"eventsTruncated"`
	ExecutionMode   string                    `json:"executionMode"`
	RunID           string                    `json:"runId,omitempty"`
	PlanEpoch       uint64                    `json:"planEpoch,omitempty"`
}
type CreateTask struct {
	RequestID     string                    `json:"requestId"`
	Goal          string                    `json:"goal"`
	ExpectedHead  workspace.CommandBaseline `json:"expectedHead"`
	Steps         []StepSpec                `json:"steps"`
	ExecutionMode string                    `json:"executionMode,omitempty"`
	RunID         string                    `json:"runId,omitempty"`
	PlanEpoch     uint64                    `json:"planEpoch,omitempty"`
}

func (task *Task) event(stepID, kind string) {
	task.EventSequence++
	task.Events = append(task.Events, Event{task.EventSequence, stepID, kind, time.Now().UTC().Format(time.RFC3339Nano)})
	if len(task.Events) > 192 {
		task.Events = task.Events[len(task.Events)-192:]
		task.EventsTruncated = true
	}
}
func (task *Task) pending() *Step {
	for i := range task.Steps {
		if !stepApplied(task.Steps[i]) {
			return &task.Steps[i]
		}
	}
	return nil
}
func stepApplied(step Step) bool {
	return step.Phase == "APPLIED" || step.Phase == "DISPLAYED" || step.Phase == "SUPERSEDED" || step.Phase == "UI_COMPLETED"
}

func (s *Service) CreateTask(ctx context.Context, actor, document string, input CreateTask) (Task, error) {
	if err := s.authorize(ctx, actor, document, access.RoleEditor); err != nil {
		return Task{}, err
	}
	if len(input.RequestID) < 1 || len(input.RequestID) > 160 || len(input.Goal) < 1 || len(input.Goal) > 2048 || len(input.Steps) < 1 || len(input.Steps) > 32 || len(marshal(input)) > 64<<10 {
		return Task{}, fault("INVALID_TASK", "task requires a requestId, goal and 1..32 bounded steps")
	}
	if input.ExecutionMode == "" {
		input.ExecutionMode = "CONFIRM"
	}
	if input.ExecutionMode != "AUTOMATIC" && input.ExecutionMode != "CONFIRM" {
		return Task{}, fault("INVALID_EXECUTION_MODE", "executionMode must be AUTOMATIC or CONFIRM")
	}
	if input.RunID != "" {
		if _, err := uuid.Parse(input.RunID); err != nil || input.PlanEpoch == 0 {
			return Task{}, fault("INVALID_TASK", "Run tasks require a stable Run ID and a positive plan epoch")
		}
	}
	digest := modelcore.ValueDigest(marshal(input))
	var priorID, priorDigest string
	err := s.db.QueryRow(ctx, `SELECT id::text,request_digest FROM occccad.automation_tasks WHERE document_id=$1 AND actor_id=$2 AND request_id=$3`, document, actor, input.RequestID).Scan(&priorID, &priorDigest)
	if err == nil {
		if digest != priorDigest {
			return Task{}, fault("IDEMPOTENCY_KEY_REUSED", "task requestId has different intent")
		}
		return s.GetTask(ctx, actor, document, priorID)
	}
	if !errors.Is(err, database.ErrNoRows) {
		return Task{}, err
	}
	snapshot, err := s.workspace.ReadModelSnapshot(ctx, document, "")
	if err != nil {
		return Task{}, err
	}
	if snapshot.DocumentType != "PART" && snapshot.DocumentType != "PRODUCT" {
		return Task{}, fault("DOCUMENT_TYPE_UNSUPPORTED", "unsupported document definition")
	}
	if snapshot.CommandBaseline != input.ExpectedHead {
		return Task{}, fault("WORKSPACE_HEAD_CONFLICT", "task baseline changed")
	}
	known := map[string]bool{}
	task := Task{ID: uuid.NewString(), DocumentID: document, Version: 1, Goal: input.Goal, Status: "ACTIVE", Baseline: input.ExpectedHead, CatalogDigest: s.Catalog.Digest, Steps: []Step{}, Events: []Event{}, ExecutionMode: input.ExecutionMode, RunID: input.RunID, PlanEpoch: input.PlanEpoch}
	for _, spec := range input.Steps {
		if _, err := uuid.Parse(spec.ID); err != nil || known[spec.ID] {
			return Task{}, fault("INVALID_TASK", "steps require unique stable UUIDs")
		}
		op, err := s.Catalog.Describe(spec.OperationID)
		if err != nil {
			return Task{}, err
		}
		if op.DocumentType != "ANY" && op.DocumentType != snapshot.DocumentType {
			return Task{}, fault("DOCUMENT_TYPE_UNSUPPORTED", "operation does not match this document")
		}
		if op.Handoff != "" {
			return Task{}, fault("UI_HANDOFF_REQUIRED", op.Handoff)
		}
		if op.Effect == "UI_COMMAND" && len(input.Steps) != 1 {
			return Task{}, fault("UI_HANDOFF_REQUIRED", "Interactive workbench commands require a separate single-step handoff")
		}
		if op.Version != spec.Version {
			return Task{}, fault("OPERATION_VERSION_UNSUPPORTED", "step operation version changed")
		}
		args, err := DecodeObject(spec.Arguments)
		if err != nil {
			return Task{}, err
		}
		if err = validateFields(op, args, true); err != nil {
			return Task{}, err
		}
		for key, source := range spec.Bindings {
			if !known[source] {
				return Task{}, fault("INVALID_BINDING", "binding must reference a preceding step")
			}
			if _, exists := args[key]; exists {
				return Task{}, fault("INVALID_BINDING", "bound argument also has a literal")
			}
			valid := false
			for _, f := range op.Fields {
				valid = valid || f.Name == key && bindingKind(key) != ""
			}
			if !valid {
				return Task{}, fault("INVALID_BINDING", "only declared entity inputs can bind an output")
			}
		}
		boundFields := map[string]bool{}
		for key := range spec.Bindings {
			boundFields[key] = true
		}
		for _, binding := range spec.TypedBindings {
			if boundFields[binding.Target] {
				return Task{}, fault("INVALID_BINDING", "duplicate typed input binding")
			}
			container, _, _ := bindingSlot(binding.Target)
			for existing := range boundFields {
				priorContainer, _, _ := bindingSlot(existing)
				if container != "" && existing == container || priorContainer != "" && binding.Target == priorContainer {
					return Task{}, fault("INVALID_BINDING", "whole-object and nested bindings cannot overlap")
				}
			}
			if err := validateInputBinding(op, args, binding, known); err != nil {
				return Task{}, err
			}
			boundFields[binding.Target] = true
		}
		for _, field := range op.Fields {
			_, literal := args[field.Name]
			bound := boundFields[field.Name]
			for _, binding := range spec.TypedBindings {
				container, _, _ := bindingSlot(binding.Target)
				bound = bound || container == field.Name
			}
			if field.Required && !literal && !bound {
				return Task{}, fault("INVALID_ARGUMENTS", "missing task argument: "+field.Name)
			}
		}
		known[spec.ID] = true
		task.Steps = append(task.Steps, Step{StepSpec: spec, Phase: "PLANNED", Presentation: Presentation{Status: "NOT_PRESENTED"}})
	}
	task.event("", "TASK_CREATED")
	_, err = s.db.Exec(ctx, `INSERT INTO occccad.automation_tasks(id,document_id,workspace_id,actor_id,request_id,request_digest,version,payload) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, task.ID, document, task.Baseline.WorkspaceID, actor, input.RequestID, digest, task.Version, marshal(task))
	if database.IsUniqueViolation(err) {
		return s.CreateTask(ctx, actor, document, input)
	}
	return task, err
}
func (s *Service) GetTask(ctx context.Context, actor, document, id string) (Task, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return Task{}, err
	}
	return loadTask(ctx, s.db, actor, document, id)
}

// FindTaskByRequest recovers an unknown task-definition outcome by its original
// idempotency key, including a crash before the caller saved the task ID.
func (s *Service) FindTaskByRequest(ctx context.Context, actor, document, requestID string) (Task, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return Task{}, err
	}
	var id string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM occccad.automation_tasks WHERE actor_id=$1 AND document_id=$2 AND request_id=$3`, actor, document, requestID).Scan(&id); err != nil {
		return Task{}, err
	}
	return loadTask(ctx, s.db, actor, document, id)
}
func loadTask(ctx context.Context, db database.Executor, actor, document, id string) (Task, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Task{}, fault("TASK_NOT_FOUND", "task is unavailable")
	}
	var raw []byte
	var version uint64
	err := db.QueryRow(ctx, `SELECT payload,version FROM occccad.automation_tasks WHERE id=$1 AND actor_id=$2 AND document_id=$3`, id, actor, document).Scan(&raw, &version)
	if errors.Is(err, database.ErrNoRows) {
		return Task{}, fault("TASK_NOT_FOUND", "task is unavailable")
	}
	if err != nil {
		return Task{}, err
	}
	var task Task
	if err = json.Unmarshal(raw, &task); err != nil {
		return task, err
	}
	task.Version = version
	if task.ExecutionMode == "" {
		task.ExecutionMode = "CONFIRM"
	}
	return task, nil
}
func saveTask(ctx context.Context, db database.Executor, task *Task, actor string) error {
	previous := task.Version
	task.Version++
	result, err := db.Exec(ctx, `UPDATE occccad.automation_tasks SET version=$1,payload=$2,updated_at=now() WHERE id=$3 AND actor_id=$4 AND version=$5`, task.Version, marshal(task), task.ID, actor, previous)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fault("TASK_VERSION_CONFLICT", "task changed; reload before proceeding")
	}
	return nil
}

// HasPendingTask checks the whole journal, rather than treating the most recent
// page as a complete set of unfinished work.
func (s *Service) HasPendingTask(ctx context.Context, actor, document string) (bool, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return false, err
	}
	var count int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM occccad.automation_tasks WHERE actor_id=$1 AND document_id=$2 AND payload->>'status' IN ('ACTIVE','PAUSED','WAIT_CONFIRM','WAIT_UI','REPLAN_REQUIRED')`, actor, document).Scan(&count)
	return count > 0, err
}

func (s *Service) ListTasks(ctx context.Context, actor, document string) ([]Task, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT payload,version FROM occccad.automation_tasks WHERE document_id=$1 AND actor_id=$2 ORDER BY created_at DESC,id DESC LIMIT 10`, document, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		var raw []byte
		var version uint64
		if err = rows.Scan(&raw, &version); err != nil {
			return nil, err
		}
		var task Task
		if err = json.Unmarshal(raw, &task); err != nil {
			return nil, err
		}
		task.Version = version
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// claim serializes automation evaluations per Workspace using a short lock.
// The database transaction is released before invoking an evaluator or LLM.
func (s *Service) claim(ctx context.Context, actor, document, id string, expected uint64) (Task, string, error) {
	if err := s.authorize(ctx, actor, document, access.RoleEditor); err != nil {
		return Task{}, "", err
	}
	task, err := loadTask(ctx, s.db, actor, document, id)
	if err != nil {
		return task, "", err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return task, "", err
	}
	defer tx.Rollback(ctx)
	var workspaceID string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM occccad.workspaces WHERE id=$1 FOR UPDATE`, task.Baseline.WorkspaceID).Scan(&workspaceID); err != nil {
		return task, "", err
	}
	task, err = loadTask(ctx, tx, actor, document, id)
	if err != nil {
		return task, "", err
	}
	if expected != task.Version {
		return task, "", fault("TASK_VERSION_CONFLICT", "task changed; reload before proceeding")
	}
	if task.Status != "ACTIVE" {
		return task, "", fault("TASK_NOT_ACTIVE", "resume the task before proceeding")
	}
	var busy int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM occccad.automation_tasks WHERE workspace_id=$1 AND lease_until>$2`, workspaceID, time.Now().UnixMilli()).Scan(&busy); err != nil {
		return task, "", err
	}
	if busy > 0 {
		return task, "", &Fault{Code: "AUTOMATION_BUSY", Message: "a workspace operation is still running", Retryable: true}
	}
	step := task.pending()
	if step == nil {
		return task, "", fault("TASK_FINISHED", "all steps have authoritative results")
	}
	if step.Phase == "COMMITTING" || step.Phase == "RESULT_UNKNOWN" {
		return task, "", &Fault{Code: "COMMIT_RESULT_UNKNOWN", Message: "recover the original command receipt before issuing another action", Retryable: true}
	}
	// Binding/contract rejection happens before an execution lease exists. Keep
	// its diagnosis durable too, so a restart cannot leave an active plan that
	// repeats the same rejected claim without exposing it to the coordinator.
	rejectPlan := func(planErr error) (Task, string, error) {
		invalidateCandidate(s, document, actor, &task)
		step.Phase, step.Failure = "FAILED", classify(planErr)
		task.Status = "PAUSED"
		task.event(step.ID, "PLAN_REJECTED")
		if err := saveTask(ctx, tx, &task, actor); err != nil {
			return task, "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return task, "", err
		}
		return task, "", planErr
	}
	if task.CatalogDigest != s.Catalog.Digest {
		return rejectPlan(fault("OPERATION_CATALOG_CHANGED", "reload operation contracts before creating a new plan"))
	}
	if op, _ := s.Catalog.Describe(step.OperationID); op.Effect == "UI_COMMAND" {
		return task, "", fault("UI_HANDOFF_REQUIRED", "Execute this step through the workbench handoff")
	}
	if step.Action == nil {
		args, err := DecodeObject(step.Arguments)
		if err != nil {
			return rejectPlan(err)
		}
		if err := bindInputs(&task, step, args); err != nil {
			return rejectPlan(err)
		}
		step.Action = &ActionRequest{OperationID: step.OperationID, Version: step.Version, Arguments: marshal(args), ExpectedHead: task.Baseline, RequestID: "automation:" + task.ID + ":" + step.ID}
	}
	if step.Phase == "PREVIEW_READY" {
		if task.ExecutionMode == "CONFIRM" && !confirmationMatches(task, *step) {
			return task, "", fault("CONFIRMATION_REQUIRED", "confirm this exact preview before committing")
		}
		step.Phase = "COMMITTING"
	} else {
		step.Phase = "PREPARING"
		step.Preview = nil
		step.Confirmation = nil
	}
	step.Failure = nil
	task.event(step.ID, step.Phase)
	if err = saveTask(ctx, tx, &task, actor); err != nil {
		return task, "", err
	}
	token := uuid.NewString()
	_, err = tx.Exec(ctx, `UPDATE occccad.automation_tasks SET lease_token=$1,lease_until=$2 WHERE id=$3`, token, time.Now().Add(130*time.Second).UnixMilli(), id)
	if err != nil {
		return task, "", err
	}
	return task, token, tx.Commit(ctx)
}

func (s *Service) Advance(ctx context.Context, actor, document, id string, expected uint64) (Task, error) {
	ctx, cancelOperation := context.WithTimeout(ctx, 110*time.Second)
	defer cancelOperation()
	observed, err := s.GetTask(ctx, actor, document, id)
	if err != nil {
		return observed, err
	}
	if observed.Version != expected {
		return observed, fault("TASK_VERSION_CONFLICT", "task changed; reload before advancing")
	}
	if step := observed.pending(); step != nil && (step.Phase == "COMMITTING" || step.Phase == "RESULT_UNKNOWN") {
		return s.ReconcileTaskReceipts(ctx, actor, document, id)
	}
	task, token, err := s.claim(ctx, actor, document, id, expected)
	if err != nil {
		return task, err
	}
	step := task.pending()
	stepID := step.ID
	action := *step.Action
	commitIssued := step.Phase == "COMMITTING"
	// The request may have committed immediately before a previous process died.
	result, recoveryErr := s.recoverAction(ctx, actor, document, action)
	var preview workspace.CommandPreview
	if recoveryErr == nil {
		step.Result = &result
		step.Phase = "APPLIED"
		step.Preview, step.Confirmation = nil, nil
		task.Baseline = result.Receipt.CommandBaseline
	} else if !errors.Is(recoveryErr, workspace.ErrNotFound) {
		err = recoveryErr
	} else if step.Phase == "COMMITTING" {
		result, err = s.Commit(ctx, actor, document, action)
		if err == nil {
			step.Result = &result
			step.Preview = nil
			step.Confirmation = nil
			step.Phase = "APPLIED"
			task.Baseline = result.Receipt.CommandBaseline
		}
	} else {
		preview, err = s.Prepare(ctx, actor, document, action)
		if err == nil {
			action.PreviewID = preview.PreviewID
			step.Action = &action
			step.Preview = &preview
			step.Phase = "PREVIEW_READY"
		}
	}
	if err != nil {
		// A lost response is not proof of non-commit. Preserve the original
		// request identity and reconcile its exact receipt before classifying it.
		recovery, cancelRecovery := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		if recovered, recoverErr := s.recoverAction(recovery, actor, document, action); recoverErr == nil {
			step.Result = &recovered
			step.Phase = "APPLIED"
			step.Preview, step.Confirmation = nil, nil
			task.Baseline = recovered.Receipt.CommandBaseline
			err = nil
		}
		cancelRecovery()
	}
	if err != nil {
		step.Failure = classify(err)
		step.Phase = "FAILED"
		if commitIssued && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			step.Phase = "RESULT_UNKNOWN"
			step.Failure = &Fault{Code: "COMMIT_RESULT_UNKNOWN", Message: "提交结果尚未确定，必须按原请求恢复领域回执后继续。", Retryable: true}
		}
		step.Preview = nil
		step.Confirmation = nil
		if step.Failure.Code == "PREVIEW_EXPIRED_OR_MISMATCHED" {
			// Candidates are process-local and expire. Recreate the candidate,
			// then require fresh confirmation of that new candidate in CONFIRM.
			step.Phase = "PLANNED"
			step.Action.PreviewID = ""
		}
	}
	// Finishing survives the initiating connection closing. CAS/receipt recovery
	// decides model truth; task bookkeeping is bounded and may be retried.
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	tx, finishErr := s.db.Begin(finish)
	if finishErr != nil {
		return task, finishErr
	}
	defer tx.Rollback(finish)
	var actual string
	if finishErr = tx.QueryRow(finish, `SELECT lease_token FROM occccad.automation_tasks WHERE id=$1 FOR UPDATE`, id).Scan(&actual); finishErr != nil {
		return task, finishErr
	}
	if actual != token {
		return task, fault("TASK_LEASE_LOST", "reload to recover the exact command receipt")
	}
	current, finishErr := loadTask(finish, tx, actor, document, id)
	if finishErr != nil {
		return task, finishErr
	}
	for i := range current.Steps {
		if current.Steps[i].ID == stepID {
			current.Steps[i] = *step
		}
	}
	if current.Status != "ACTIVE" && step.Preview != nil {
		s.workspace.DiscardPreview(document, actor, step.Preview.PreviewID)
		for i := range current.Steps {
			if current.Steps[i].ID == stepID {
				current.Steps[i].Preview = nil
				current.Steps[i].Confirmation = nil
				current.Steps[i].Phase = "PLANNED"
				current.Steps[i].Action.PreviewID = ""
			}
		}
	}
	if step.Result != nil {
		if current.Baseline.Sequence > step.Result.Receipt.Sequence {
			for i := range current.Steps {
				if current.Steps[i].ID == stepID {
					current.Steps[i].Phase = "SUPERSEDED"
				}
			}
		} else {
			current.Baseline = task.Baseline
		}
	}
	if err != nil && (step.Phase == "FAILED" || step.Phase == "RESULT_UNKNOWN") && current.Status == "ACTIVE" {
		current.Status = "PAUSED"
	}
	if current.Status == "ACTIVE" {
		if current.pending() == nil {
			current.Status = "COMPLETED"
		} else if current.ExecutionMode == "CONFIRM" && current.pending().Phase == "PREVIEW_READY" {
			current.Status = "WAIT_CONFIRM"
		}
	}
	for _, stored := range current.Steps {
		if stored.ID == stepID {
			current.event(stepID, stored.Phase)
			break
		}
	}
	if finishErr = saveTask(finish, tx, &current, actor); finishErr != nil {
		return current, finishErr
	}
	if _, finishErr = tx.Exec(finish, `UPDATE occccad.automation_tasks SET lease_token='',lease_until=0 WHERE id=$1 AND lease_token=$2`, id, token); finishErr != nil {
		return current, finishErr
	}
	return current, tx.Commit(finish)
}

type Control struct {
	ExpectedHead    *workspace.CommandBaseline `json:"expectedHead,omitempty"`
	Action          string                     `json:"action"`
	ExpectedVersion uint64                     `json:"expectedVersion"`
	StepID          string                     `json:"stepId,omitempty"`
	RevisionID      string                     `json:"revisionId,omitempty"`
	PreviewID       string                     `json:"previewId,omitempty"`
	RunID           string                     `json:"runId,omitempty"`
	PlanEpoch       uint64                     `json:"planEpoch,omitempty"`
	DomainRequestID string                     `json:"domainRequestId,omitempty"`
	HandoffID       string                     `json:"handoffId,omitempty"`
}

func (s *Service) Control(ctx context.Context, actor, document, id string, input Control) (Task, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Task{}, fault("TASK_NOT_FOUND", "task is unavailable")
	}
	if err := s.authorize(ctx, actor, document, access.RoleEditor); err != nil {
		return Task{}, err
	}
	var handoffResult *ActionResult
	if input.Action == "ui_complete" {
		observed, err := loadTask(ctx, s.db, actor, document, id)
		if err != nil {
			return observed, err
		}
		if observed.Version != input.ExpectedVersion {
			return observed, fault("TASK_VERSION_CONFLICT", "task changed before receipt verification")
		}
		result, err := s.validateHandoffReceipt(ctx, actor, document, observed, input)
		if err != nil {
			return observed, err
		}
		handoffResult = &result
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	var exists string
	var leaseUntil int64
	if err = tx.QueryRow(ctx, `SELECT id::text,lease_until FROM occccad.automation_tasks WHERE id=$1 AND actor_id=$2 AND document_id=$3 FOR UPDATE`, id, actor, document).Scan(&exists, &leaseUntil); err != nil {
		return Task{}, fault("TASK_NOT_FOUND", "task is unavailable")
	}
	task, err := loadTask(ctx, tx, actor, document, id)
	if err != nil {
		return task, err
	}
	if input.ExpectedVersion != task.Version {
		return task, fault("TASK_VERSION_CONFLICT", "task changed; reload before controlling it")
	}
	if input.RunID != task.RunID || input.PlanEpoch != task.PlanEpoch {
		return task, fault("PLAN_EPOCH_CONFLICT", "control belongs to another Run or superseded plan epoch")
	}
	switch input.Action {
	case "confirm":
		step := task.pending()
		if task.Status != "WAIT_CONFIRM" || task.ExecutionMode != "CONFIRM" || step == nil || step.ID != input.StepID || step.Phase != "PREVIEW_READY" || step.Action == nil || step.Preview == nil || input.PreviewID == "" || input.PreviewID != step.Preview.PreviewID || input.ExpectedHead == nil || *input.ExpectedHead != step.Action.ExpectedHead {
			return task, fault("CONFIRMATION_MISMATCH", "confirmation must identify this exact task, epoch, step, baseline and preview")
		}
		head, err := currentTaskHead(ctx, tx, task)
		if err != nil {
			return task, err
		}
		if head != *input.ExpectedHead {
			return task, fault("WORKSPACE_HEAD_CONFLICT", "preview input changed; replan before confirming")
		}
		step.Confirmation = &Confirmation{RunID: task.RunID, PlanEpoch: task.PlanEpoch, TaskVersion: task.Version, StepID: step.ID, PreviewID: input.PreviewID, ExpectedHead: *input.ExpectedHead, IntentDigest: actionIntent(*step.Action)}
		task.Status = "ACTIVE"
		task.event(step.ID, "PREVIEW_CONFIRMED")
	case "ui_opened", "ui_failed", "ui_complete", "ui_cancelled", "ui_unknown":
		if err := s.controlHandoff(&task, input, handoffResult); err != nil {
			return task, err
		}
	case "handoff":
		step := task.pending()
		if step == nil || step.ID != input.StepID || step.Phase != "PLANNED" || task.Status != "ACTIVE" || task.CatalogDigest != s.Catalog.Digest {
			return task, fault("UI_HANDOFF_INVALID", "handoff requires the active planned step")
		}
		op, e := s.Catalog.Describe(step.OperationID)
		if e != nil || op.Effect != "UI_COMMAND" {
			return task, fault("UI_HANDOFF_INVALID", "step is not a workbench command")
		}
		var head workspace.CommandBaseline
		if err = tx.QueryRow(ctx, `SELECT id::text,head_revision_id::text,head_sequence FROM occccad.workspaces WHERE id=$1 AND document_id=$2`, task.Baseline.WorkspaceID, document).Scan(&head.WorkspaceID, &head.RevisionID, &head.Sequence); err != nil {
			return task, err
		}
		if head != task.Baseline {
			return task, fault("WORKSPACE_HEAD_CONFLICT", "handoff scope changed")
		}
		step.Handoff = &HandoffExpectation{ID: uuid.NewString(), RunID: task.RunID, PlanEpoch: task.PlanEpoch, UICommand: op.UICommand, Baseline: task.Baseline, DomainRequestID: "automation-handoff:" + task.ID + ":" + step.ID}
		step.Phase = "HANDED_OFF"
		task.Status = "WAIT_UI"
		task.event(step.ID, "HANDED_OFF")
	case "pause":
		if task.Status != "ACTIVE" && task.Status != "WAIT_CONFIRM" && task.Status != "WAIT_UI" {
			return task, fault("TASK_NOT_ACTIVE", "task cannot be paused")
		}
		task.Status = "PAUSED"
		invalidateCandidate(s, document, actor, &task)
	case "stop":
		if task.Status == "COMPLETED" || task.Status == "STOPPED" {
			return task, nil
		}
		task.Status = "STOPPED"
		invalidateCandidate(s, document, actor, &task)
	case "resume":
		if task.Status != "PAUSED" {
			return task, fault("TASK_NOT_PAUSED", "task cannot be resumed")
		}
		if leaseUntil > time.Now().UnixMilli() {
			return task, &Fault{Code: "AUTOMATION_BUSY", Message: "current operation must finish before resuming", Retryable: true}
		}
		if step := task.pending(); step != nil && (step.Phase == "COMMITTING" || step.Phase == "RESULT_UNKNOWN") {
			return task, &Fault{Code: "COMMIT_RESULT_UNKNOWN", Message: "recover the original in-flight command receipt before resuming", Retryable: true}
		}
		head, headErr := currentTaskHead(ctx, tx, task)
		if headErr != nil {
			return task, headErr
		}
		if input.ExpectedHead != nil && *input.ExpectedHead != head {
			return task, fault("WORKSPACE_HEAD_CONFLICT", "requested baseline is no longer current")
		}
		if head != task.Baseline {
			// Changing only ExpectedHead would silently approve stale arguments or
			// topology. The Run must read facts and create a new bounded stage.
			invalidateCandidate(s, document, actor, &task)
			task.Status = "REPLAN_REQUIRED"
			task.event("", "WORKSPACE_CHANGED_REPLAN_REQUIRED")
		} else {
			task.Status = "ACTIVE"
			invalidateCandidate(s, document, actor, &task)
			if step := task.pending(); step != nil && step.Handoff != nil {
				task.Status = "WAIT_UI"
			}
		}
		if task.pending() == nil {
			task.Status = "COMPLETED"
		}
	case "displayed", "presentation_failed":
		var step *Step
		for i := range task.Steps {
			if task.Steps[i].ID == input.StepID {
				step = &task.Steps[i]
				break
			}
		}
		if step == nil || step.Result == nil || step.Result.Receipt.RevisionID != input.RevisionID {
			return task, fault("DISPLAY_MISMATCH", "presentation acknowledgment must identify an applied step and its exact revision")
		}
		status := "PRESENTED"
		if input.Action == "presentation_failed" {
			status = "FAILED"
		}
		step.Presentation = Presentation{Status: status, RevisionID: input.RevisionID, At: time.Now().UTC().Format(time.RFC3339Nano)}
		task.event(step.ID, "RESULT_"+status)
	default:
		return task, fault("INVALID_CONTROL", "supported actions: pause, resume, stop, confirm, displayed, presentation_failed, handoff, ui_opened, ui_failed, ui_complete, ui_cancelled, ui_unknown")
	}
	task.event(input.StepID, "TASK_"+task.Status)
	if err = saveTask(ctx, tx, &task, actor); err != nil {
		return task, err
	}
	return task, tx.Commit(ctx)
}
