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
	Bindings map[string]string `json:"bindings,omitempty"`
}
type Step struct {
	StepSpec
	Phase   string                    `json:"phase"`
	Action  *ActionRequest            `json:"action,omitempty"`
	Preview *workspace.CommandPreview `json:"preview,omitempty"`
	Result  *ActionResult             `json:"result,omitempty"`
	Failure *Fault                    `json:"failure,omitempty"`
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
}
type CreateTask struct {
	RequestID    string                    `json:"requestId"`
	Goal         string                    `json:"goal"`
	ExpectedHead workspace.CommandBaseline `json:"expectedHead"`
	Steps        []StepSpec                `json:"steps"`
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
		if task.Steps[i].Phase != "DISPLAYED" && task.Steps[i].Phase != "SUPERSEDED" {
			return &task.Steps[i]
		}
	}
	return nil
}

func (s *Service) CreateTask(ctx context.Context, actor, document string, input CreateTask) (Task, error) {
	if err := s.authorize(ctx, actor, document, access.RoleEditor); err != nil {
		return Task{}, err
	}
	if len(input.RequestID) < 1 || len(input.RequestID) > 160 || len(input.Goal) < 1 || len(input.Goal) > 2048 || len(input.Steps) < 1 || len(input.Steps) > 32 || len(marshal(input)) > 64<<10 {
		return Task{}, fault("INVALID_TASK", "task requires a requestId, goal and 1..32 bounded steps")
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
	if snapshot.DocumentType != "PART" {
		return Task{}, fault("DOCUMENT_TYPE_UNSUPPORTED", "tasks currently target standalone Part definitions")
	}
	if snapshot.CommandBaseline != input.ExpectedHead {
		return Task{}, fault("WORKSPACE_HEAD_CONFLICT", "task baseline changed")
	}
	known := map[string]bool{}
	task := Task{ID: uuid.NewString(), DocumentID: document, Version: 1, Goal: input.Goal, Status: "ACTIVE", Baseline: input.ExpectedHead, CatalogDigest: s.Catalog.Digest, Steps: []Step{}, Events: []Event{}}
	for _, spec := range input.Steps {
		if _, err := uuid.Parse(spec.ID); err != nil || known[spec.ID] {
			return Task{}, fault("INVALID_TASK", "steps require unique stable UUIDs")
		}
		op, err := s.Catalog.Describe(spec.OperationID)
		if err != nil {
			return Task{}, err
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
				valid = valid || f.Name == key && (key == "sketchId" || key == "parameterId")
			}
			if !valid {
				return Task{}, fault("INVALID_BINDING", "only declared entity inputs can bind an output")
			}
		}
		for _, field := range op.Fields {
			_, literal := args[field.Name]
			_, bound := spec.Bindings[field.Name]
			if field.Required && !literal && !bound {
				return Task{}, fault("INVALID_ARGUMENTS", "missing task argument: "+field.Name)
			}
		}
		known[spec.ID] = true
		task.Steps = append(task.Steps, Step{StepSpec: spec, Phase: "PLANNED"})
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
	if task.CatalogDigest != s.Catalog.Digest {
		return task, "", fault("OPERATION_CATALOG_CHANGED", "reload operation contracts before creating a new plan")
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
		return task, "", fault("TASK_FINISHED", "all steps have been displayed")
	}
	if step.Phase == "COMMITTED" {
		return task, "", fault("DISPLAY_REQUIRED", "display the committed result before advancing")
	}
	if step.Action == nil {
		args, err := DecodeObject(step.Arguments)
		if err != nil {
			return task, "", err
		}
		for field, source := range step.Bindings {
			for _, prior := range task.Steps {
				if prior.ID == source && prior.Result != nil && prior.Result.Output != nil {
					requiredKind := "SKETCH"
					if field == "parameterId" {
						requiredKind = "PARAMETER"
					}
					if prior.Result.Output.Kind != requiredKind {
						return task, "", fault("INVALID_BINDING", "step output has incompatible entity kind")
					}
					args[field] = prior.Result.Output.ID
				}
			}
		}
		step.Action = &ActionRequest{OperationID: step.OperationID, Version: step.Version, Arguments: marshal(args), ExpectedHead: task.Baseline, RequestID: "automation:" + task.ID + ":" + step.ID}
	}
	if step.Phase == "PREVIEW_READY" {
		step.Phase = "COMMITTING"
	} else {
		step.Phase = "PREPARING"
		step.Preview = nil
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
	task, token, err := s.claim(ctx, actor, document, id, expected)
	if err != nil {
		return task, err
	}
	step := task.pending()
	stepID := step.ID
	action := *step.Action
	// The request may have committed immediately before a previous process died.
	result, recoveryErr := s.recoverAction(ctx, actor, document, action)
	var preview workspace.CommandPreview
	if recoveryErr == nil {
		step.Result = &result
		step.Phase = "COMMITTED"
		task.Baseline = result.Receipt.CommandBaseline
	} else if !errors.Is(recoveryErr, workspace.ErrNotFound) {
		err = recoveryErr
	} else if step.Phase == "COMMITTING" {
		result, err = s.Commit(ctx, actor, document, action)
		if err == nil {
			step.Result = &result
			step.Preview = nil
			step.Phase = "COMMITTED"
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
		step.Failure = classify(err)
		step.Phase = "FAILED"
		step.Preview = nil
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
	if current.Status == "STOPPED" && step.Preview != nil {
		s.workspace.DiscardPreview(document, actor, step.Preview.PreviewID)
		for i := range current.Steps {
			if current.Steps[i].ID == stepID {
				current.Steps[i].Preview = nil
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
	if err != nil && current.Status == "ACTIVE" {
		current.Status = "PAUSED"
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
}

func (s *Service) Control(ctx context.Context, actor, document, id string, input Control) (Task, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Task{}, fault("TASK_NOT_FOUND", "task is unavailable")
	}
	if err := s.authorize(ctx, actor, document, access.RoleEditor); err != nil {
		return Task{}, err
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
	switch input.Action {
	case "pause":
		if task.Status != "ACTIVE" {
			return task, fault("TASK_NOT_ACTIVE", "task cannot be paused")
		}
		task.Status = "PAUSED"
	case "stop":
		if task.Status == "COMPLETED" || task.Status == "STOPPED" {
			return task, nil
		}
		task.Status = "STOPPED"
		if step := task.pending(); step != nil && step.Preview != nil {
			s.workspace.DiscardPreview(document, actor, step.Preview.PreviewID)
			step.Preview = nil
			if step.Action != nil {
				step.Action.PreviewID = ""
			}
		}
	case "resume":
		if task.Status != "PAUSED" {
			return task, fault("TASK_NOT_PAUSED", "task cannot be resumed")
		}
		if leaseUntil > time.Now().UnixMilli() {
			return task, &Fault{Code: "AUTOMATION_BUSY", Message: "current operation must finish before resuming", Retryable: true}
		}
		if input.ExpectedHead != nil {
			var head workspace.CommandBaseline
			if err = tx.QueryRow(ctx, `SELECT id::text,head_revision_id::text,head_sequence FROM occccad.workspaces WHERE id=$1 AND document_id=$2`, task.Baseline.WorkspaceID, document).Scan(&head.WorkspaceID, &head.RevisionID, &head.Sequence); err != nil {
				return task, err
			}
			if head != *input.ExpectedHead {
				return task, fault("WORKSPACE_HEAD_CONFLICT", "requested resume baseline is no longer current")
			}
			if step := task.pending(); step != nil {
				if step.Phase == "COMMITTED" && step.Result != nil && step.Result.Receipt.RevisionID != head.RevisionID {
					step.Phase = "SUPERSEDED"
					task.event(step.ID, "COMMITTED_REVISION_SUPERSEDED")
				}
				if step.Action != nil && step.Result == nil {
					step.Action.ExpectedHead = head
				}
			}
			task.Baseline = head
			task.event("", "BASELINE_ACCEPTED")
		}
		task.Status = "ACTIVE"
		if step := task.pending(); step != nil && step.Phase == "PREVIEW_READY" {
			if step.Preview != nil {
				s.workspace.DiscardPreview(document, actor, step.Preview.PreviewID)
			}
			step.Preview = nil
			step.Action.PreviewID = ""
			step.Phase = "PLANNED"
		}
		if task.pending() == nil {
			task.Status = "COMPLETED"
		}
	case "displayed":
		step := task.pending()
		if step == nil || step.ID != input.StepID || step.Phase != "COMMITTED" || step.Result == nil || step.Result.Receipt.RevisionID != input.RevisionID {
			return task, fault("DISPLAY_MISMATCH", "display acknowledgment must identify the current committed step and revision")
		}
		step.Phase = "DISPLAYED"
		task.event(step.ID, "DISPLAYED")
		if task.pending() == nil && task.Status != "STOPPED" {
			task.Status = "COMPLETED"
		}
	default:
		return task, fault("INVALID_CONTROL", "supported actions: pause, resume, stop, displayed")
	}
	task.event(input.StepID, "TASK_"+task.Status)
	if err = saveTask(ctx, tx, &task, actor); err != nil {
		return task, err
	}
	return task, tx.Commit(ctx)
}
