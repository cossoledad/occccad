package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/occccad/occccad/internal/database"
)

const jobColumns = `id::text,job_type,state,document_id::text,version_id::text,requested_by_user_id::text,
 input_object_id::text,result_object_id::text,payload,attempt_count,max_attempts,progress,
 error_code,error_message,created_at::text,started_at::text,completed_at::text,cancel_requested_at::text,user_visible`

var ErrNotFound = errors.New("job not found")
var ErrNotCancelable = errors.New("job cannot be canceled in its current state")
var ErrNotRetryable = errors.New("job cannot be retried in its current state")

type Job struct {
	ID                string          `json:"id"`
	Type              string          `json:"type"`
	State             string          `json:"state"`
	DocumentID        *string         `json:"documentId,omitempty"`
	VersionID         *string         `json:"versionId,omitempty"`
	RequestedBy       string          `json:"requestedBy"`
	InputObjectID     *string         `json:"inputObjectId,omitempty"`
	ResultObjectID    *string         `json:"resultObjectId,omitempty"`
	Payload           json.RawMessage `json:"payload"`
	AttemptCount      int             `json:"attemptCount"`
	MaxAttempts       int             `json:"maxAttempts"`
	Progress          int             `json:"progress"`
	ErrorCode         *string         `json:"errorCode,omitempty"`
	ErrorMessage      *string         `json:"errorMessage,omitempty"`
	CreatedAt         string          `json:"createdAt"`
	StartedAt         *string         `json:"startedAt,omitempty"`
	CompletedAt       *string         `json:"completedAt,omitempty"`
	CancelRequestedAt *string         `json:"cancelRequestedAt,omitempty"`
	UserVisible       bool            `json:"userVisible"`
	CanCancel         bool            `json:"canCancel"`
	CanRetry          bool            `json:"canRetry"`
}

type EnqueueRequest struct {
	Type, DocumentID, RequestedBy, InputObjectID, IdempotencyKey string
	VersionID                                                    *string
	Payload                                                      any
	UserVisible                                                  bool
}

type Service struct{ database database.DB }

func New(database database.DB) *Service { return &Service{database: database} }

func (service *Service) Enqueue(ctx context.Context, request EnqueueRequest) (Job, error) {
	payload, err := json.Marshal(request.Payload)
	if err != nil {
		return Job{}, err
	}
	return service.change(ctx, `INSERT INTO occccad.jobs(job_type,document_id,version_id,
 requested_by_user_id,input_object_id,payload,idempotency_key,user_visible)
 VALUES($1,NULLIF($2,'')::uuid,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8)
 ON CONFLICT(job_type,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key
 RETURNING `+jobColumns, "", "", "", request.Type, request.DocumentID, request.VersionID,
		request.RequestedBy, request.InputObjectID, payload, request.IdempotencyKey, request.UserVisible)
}

func (service *Service) Get(ctx context.Context, id string) (Job, error) {
	result, err := scan(service.database.QueryRow(ctx, `SELECT id::text,job_type,state,document_id::text,
		version_id::text,requested_by_user_id::text,input_object_id::text,result_object_id::text,payload,
		attempt_count,max_attempts,progress,error_code,error_message,created_at::text,started_at::text,
		completed_at::text,cancel_requested_at::text,user_visible
		FROM occccad.jobs WHERE id=$1`, id))
	if errors.Is(err, database.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return result, err
}

func (service *Service) ListForUser(ctx context.Context, userID string, limit int) ([]Job, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	rows, err := service.database.Query(ctx, `SELECT id::text,job_type,state,document_id::text,
		version_id::text,requested_by_user_id::text,input_object_id::text,result_object_id::text,payload,
		attempt_count,max_attempts,progress,error_code,error_message,created_at::text,started_at::text,
		completed_at::text,cancel_requested_at::text,user_visible
		FROM occccad.jobs WHERE requested_by_user_id=$1 AND user_visible ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Job{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) Claim(ctx context.Context, workerID string, lease time.Duration) (Job, error) {
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM occccad.jobs WHERE
 (state IN ('QUEUED','RETRY_WAIT') AND available_at<=now()) OR
 (state='RUNNING' AND lease_expires_at<now())
 ORDER BY priority DESC,created_at LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id); err != nil {
		return Job{}, err
	}
	job, err := scan(tx.QueryRow(ctx, `UPDATE occccad.jobs SET state='RUNNING',lease_owner=$1,lease_expires_at=now()+$2::interval,
 heartbeat_at=now(),started_at=COALESCE(started_at,now()),attempt_count=attempt_count+1
 WHERE id=$3 RETURNING `+jobColumns, workerID, lease.String(), id))
	if err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.job_attempts(job_id,attempt,worker_id) VALUES($1,$2,$3)`, job.ID, job.AttemptCount, workerID); err != nil {
		return Job{}, err
	}
	if err = notify(ctx, tx, job); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (service *Service) Succeed(ctx context.Context, jobID, workerID, resultID string) error {
	_, err := service.change(ctx, `UPDATE occccad.jobs SET state='SUCCEEDED',progress=100,result_object_id=NULLIF($3,'')::uuid,
 completed_at=now(),lease_owner=NULL,lease_expires_at=NULL WHERE id=$1 AND state='RUNNING'
 AND lease_owner=$2 AND cancel_requested_at IS NULL RETURNING `+jobColumns, "SUCCEEDED", "", "", jobID, workerID, resultID)
	if errors.Is(err, database.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (service *Service) SucceedImport(ctx context.Context, jobID, workerID, resultID string) error {
	_, err := service.change(ctx, `UPDATE occccad.jobs SET state='SUCCEEDED',progress=100,document_id=NULLIF($3,'')::uuid,
 completed_at=now(),lease_owner=NULL,lease_expires_at=NULL WHERE id=$1 AND state='RUNNING'
 AND lease_owner=$2 AND cancel_requested_at IS NULL RETURNING `+jobColumns, "SUCCEEDED", "", "", jobID, workerID, resultID)
	if errors.Is(err, database.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (service *Service) Heartbeat(ctx context.Context, jobID, workerID string, lease time.Duration) error {
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	command, err := service.database.Exec(ctx, `UPDATE occccad.jobs SET heartbeat_at=now(),lease_expires_at=now()+$3::interval
 WHERE id=$1 AND state='RUNNING' AND lease_owner=$2`, jobID, workerID, lease.String())
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type ProgressDetail struct {
	Phase     string `json:"phase"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
}

func (service *Service) UpdateProgress(ctx context.Context, jobID, workerID string, progress int) error {
	return service.UpdateProgressDetail(ctx, jobID, workerID, progress, nil)
}

// Keep the high-water mark across retry/lease reclamation, and never let an
// older phase replace details belonging to a more advanced phase in that attempt.
// A new attempt reports its actual phase while the overall percentage stays put.
func (service *Service) UpdateProgressDetail(ctx context.Context, jobID, workerID string, progress int, detail *ProgressDetail) error {
	progress = max(0, min(99, progress))
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	job, err := scan(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM occccad.jobs
 WHERE id=$1 AND state='RUNNING' AND lease_owner=$2 AND cancel_requested_at IS NULL FOR UPDATE`, jobID, workerID))
	if errors.Is(err, database.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	payload := map[string]json.RawMessage{}
	if err = json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	if payload == nil {
		payload = map[string]json.RawMessage{}
	}
	var previous struct {
		Attempt       *int
		PhaseProgress *int
	}
	if raw := payload["progressDetail"]; raw != nil {
		if err = json.Unmarshal(raw, &previous); err != nil {
			return err
		}
	}
	priorAttempt, priorProgress := -1, job.Progress
	if previous.Attempt != nil {
		priorAttempt = *previous.Attempt
	}
	if previous.PhaseProgress != nil {
		priorProgress = *previous.PhaseProgress
	}
	if detail != nil && (priorAttempt < job.AttemptCount || progress >= priorProgress) {
		value := struct {
			*ProgressDetail
			Attempt       int
			PhaseProgress int
		}{detail, job.AttemptCount, progress}
		payload["progressDetail"], err = json.Marshal(value)
		if err != nil {
			return err
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE occccad.jobs SET progress=$2,payload=$3 WHERE id=$1`, job.ID, max(job.Progress, progress), encoded); err != nil {
		return err
	}
	if err = notify(ctx, tx, job); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (service *Service) CancellationRequested(ctx context.Context, jobID, workerID string) (bool, error) {
	var requested bool
	err := service.database.QueryRow(ctx, `SELECT cancel_requested_at IS NOT NULL
		FROM occccad.jobs WHERE id=$1 AND state='RUNNING' AND lease_owner=$2`, jobID, workerID).Scan(&requested)
	if errors.Is(err, database.ErrNoRows) {
		return false, ErrNotFound
	}
	return requested, err
}

// RequestCancel immediately finishes unclaimed work and asks the current lease
// owner to cooperatively stop running work. A running attempt remains RUNNING
// until its worker acknowledges cancellation, so it cannot be claimed twice.
func (service *Service) RequestCancel(ctx context.Context, jobID, userID string, isAdmin bool) (Job, error) {
	job, err := service.change(ctx, `UPDATE occccad.jobs SET
 state=CASE WHEN state IN ('QUEUED','RETRY_WAIT') THEN 'CANCELED' ELSE state END,
 cancel_requested_at=COALESCE(cancel_requested_at,now()),
 completed_at=CASE WHEN state IN ('QUEUED','RETRY_WAIT') THEN now() ELSE completed_at END
 WHERE id=$1 AND ($3 OR requested_by_user_id=$2)
 AND (state IN ('QUEUED','RETRY_WAIT') OR (state='RUNNING' AND (job_type<>'EXCHANGE_IMPORT' OR progress<70)))
 AND cancel_requested_at IS NULL RETURNING `+jobColumns, "", "", "", jobID, userID, isAdmin)
	if errors.Is(err, database.ErrNoRows) {
		if _, getErr := service.Get(ctx, jobID); getErr != nil {
			return Job{}, getErr
		}
		return Job{}, ErrNotCancelable
	}
	return job, err
}

func (service *Service) AcknowledgeCanceled(ctx context.Context, jobID, workerID string) error {
	_, err := service.change(ctx, `UPDATE occccad.jobs SET state='CANCELED',completed_at=now(),lease_owner=NULL,lease_expires_at=NULL
 WHERE id=$1 AND state='RUNNING' AND lease_owner=$2 AND cancel_requested_at IS NOT NULL RETURNING `+jobColumns,
		"CANCELED", "", "", jobID, workerID)
	if errors.Is(err, database.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (service *Service) Retry(ctx context.Context, jobID, userID string, isAdmin bool) (Job, error) {
	job, err := service.change(ctx, `UPDATE occccad.jobs SET state='QUEUED',available_at=now(),completed_at=NULL,
 cancel_requested_at=NULL,error_code=NULL,error_message=NULL,max_attempts=GREATEST(max_attempts,attempt_count+3)
 WHERE id=$1 AND job_type<>'MOTION_STUDY' AND ($3 OR requested_by_user_id=$2) AND state IN ('FAILED','CANCELED') RETURNING `+jobColumns,
		"", "", "", jobID, userID, isAdmin)
	if errors.Is(err, database.ErrNoRows) {
		if _, getErr := service.Get(ctx, jobID); getErr != nil {
			return Job{}, getErr
		}
		return Job{}, ErrNotRetryable
	}
	return job, err
}

func (service *Service) Fail(ctx context.Context, job Job, workerID, code, message string) error {
	state := "FAILED"
	delay := time.Duration(0)
	if job.AttemptCount < job.MaxAttempts {
		state = "RETRY_WAIT"
		delay = time.Duration(job.AttemptCount) * 5 * time.Second
	}
	_, err := service.change(ctx, `UPDATE occccad.jobs SET state=$3,error_code=$4,error_message=$5,available_at=now()+$6::interval,
 completed_at=CASE WHEN $3='FAILED' THEN now() END,lease_owner=NULL,lease_expires_at=NULL
 WHERE id=$1 AND state='RUNNING' AND lease_owner=$2 AND cancel_requested_at IS NULL RETURNING `+jobColumns,
		"FAILED", code, message, job.ID, workerID, state, code, message, delay.String())
	if errors.Is(err, database.ErrNoRows) {
		return nil
	}
	return err
}

func scan(row database.Row) (Job, error) {
	var result Job
	err := row.Scan(&result.ID, &result.Type, &result.State, &result.DocumentID, &result.VersionID,
		&result.RequestedBy, &result.InputObjectID, &result.ResultObjectID, &result.Payload,
		&result.AttemptCount, &result.MaxAttempts, &result.Progress, &result.ErrorCode,
		&result.ErrorMessage, &result.CreatedAt, &result.StartedAt, &result.CompletedAt, &result.CancelRequestedAt, &result.UserVisible)
	populateCapabilities(&result)
	return result, err
}

func populateCapabilities(result *Job) {
	result.CanCancel = (result.State == "QUEUED" || result.State == "RETRY_WAIT" ||
		(result.State == "RUNNING" && (result.Type != "EXCHANGE_IMPORT" || result.Progress < 70))) && result.CancelRequestedAt == nil
	result.CanRetry = result.Type != "MOTION_STUDY" && (result.State == "FAILED" || result.State == "CANCELED")
}

// change commits the state, attempt and notification in one transaction on every
// backend. A failure in any step rolls back the entire transition.
func (service *Service) change(ctx context.Context, query, attemptResult, code, message string, args ...any) (Job, error) {
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	job, err := scan(tx.QueryRow(ctx, query, args...))
	if err != nil {
		return Job{}, err
	}
	if attemptResult == "TERMINAL_STATE" {
		attemptResult = job.State
	}
	if attemptResult != "" {
		if _, err = tx.Exec(ctx, `UPDATE occccad.job_attempts SET completed_at=now(),result=$3,error_code=NULLIF($4,''),error_message=NULLIF($5,'')
  WHERE job_id=$1 AND attempt=$2`, job.ID, job.AttemptCount, attemptResult, code, message); err != nil {
			return Job{}, err
		}
	}
	if err = notify(ctx, tx, job); err != nil {
		return Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	return job, nil
}
func notify(ctx context.Context, tx database.Tx, job Job) error {
	payload, err := json.Marshal(map[string]string{"jobId": job.ID, "state": job.State})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO occccad.outbox_events(aggregate_type,aggregate_id,event_type,schema_version,payload)
 VALUES('JOB',$1,'job.state.changed',1,$2)`, job.ID, payload)
	return err
}

// Completion belongs to an exact lease attempt; partial frames remain readable
// after cancellation. Never changes Product Head or its poses.
func (service *Service) FinishMotion(ctx context.Context, jobID, workerID string, attempt int, resultID string) error {
	_, err := service.change(ctx, `UPDATE occccad.jobs SET state=CASE WHEN cancel_requested_at IS NULL THEN 'SUCCEEDED' ELSE 'CANCELED' END,
 progress=100,result_object_id=$4,completed_at=now(),lease_owner=NULL,lease_expires_at=NULL
 WHERE id=$1 AND job_type='MOTION_STUDY' AND state='RUNNING' AND lease_owner=$2 AND attempt_count=$3
 AND lease_expires_at>now() RETURNING `+jobColumns, "TERMINAL_STATE", "", "", jobID, workerID, attempt, resultID)
	if errors.Is(err, database.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
