package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("job not found")

type Job struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	State          string          `json:"state"`
	DocumentID     *string         `json:"documentId,omitempty"`
	VersionID      *string         `json:"versionId,omitempty"`
	RequestedBy    string          `json:"requestedBy"`
	InputObjectID  *string         `json:"inputObjectId,omitempty"`
	ResultObjectID *string         `json:"resultObjectId,omitempty"`
	Payload        json.RawMessage `json:"payload"`
	AttemptCount   int             `json:"attemptCount"`
	MaxAttempts    int             `json:"maxAttempts"`
	Progress       int             `json:"progress"`
	ErrorCode      *string         `json:"errorCode,omitempty"`
	ErrorMessage   *string         `json:"errorMessage,omitempty"`
	CreatedAt      string          `json:"createdAt"`
	CompletedAt    *string         `json:"completedAt,omitempty"`
}

type EnqueueRequest struct {
	Type, DocumentID, RequestedBy, InputObjectID, IdempotencyKey string
	VersionID                                                    *string
	Payload                                                      any
}

type Service struct{ database *pgxpool.Pool }

func New(database *pgxpool.Pool) *Service { return &Service{database: database} }

func (service *Service) Enqueue(ctx context.Context, request EnqueueRequest) (Job, error) {
	payload, err := json.Marshal(request.Payload)
	if err != nil {
		return Job{}, err
	}
	var input *string
	if request.InputObjectID != "" {
		input = &request.InputObjectID
	}
	row := service.database.QueryRow(ctx, `INSERT INTO occccad.jobs(job_type,document_id,version_id,
		requested_by_user_id,input_object_id,payload,idempotency_key)
		VALUES($1,NULLIF($2,'')::uuid,$3,$4,NULLIF($5,'')::uuid,$6,$7)
		ON CONFLICT(job_type,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key
		RETURNING id::text,job_type,state,document_id::text,version_id::text,requested_by_user_id::text,
		input_object_id::text,result_object_id::text,payload,attempt_count,max_attempts,progress,
		error_code,error_message,created_at::text,completed_at::text`, request.Type, request.DocumentID,
		request.VersionID, request.RequestedBy, input, payload, request.IdempotencyKey)
	return scan(row)
}

func (service *Service) Get(ctx context.Context, id string) (Job, error) {
	result, err := scan(service.database.QueryRow(ctx, `SELECT id::text,job_type,state,document_id::text,
		version_id::text,requested_by_user_id::text,input_object_id::text,result_object_id::text,payload,
		attempt_count,max_attempts,progress,error_code,error_message,created_at::text,completed_at::text
		FROM occccad.jobs WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
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
		attempt_count,max_attempts,progress,error_code,error_message,created_at::text,completed_at::text
		FROM occccad.jobs WHERE requested_by_user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
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
	transaction, err := service.database.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer func() { _ = transaction.Rollback(ctx) }()
	row := transaction.QueryRow(ctx, `WITH candidate AS (
		SELECT id FROM occccad.jobs WHERE
			(state IN ('QUEUED','RETRY_WAIT') AND available_at<=now()) OR
			(state='RUNNING' AND lease_expires_at<now())
		ORDER BY priority DESC,created_at FOR UPDATE SKIP LOCKED LIMIT 1
	), claimed AS (
		UPDATE occccad.jobs j SET state='RUNNING',lease_owner=$1,lease_expires_at=now()+$2::interval,
			heartbeat_at=now(),started_at=COALESCE(started_at,now()),attempt_count=attempt_count+1
		FROM candidate WHERE j.id=candidate.id
		RETURNING j.*
	)
	SELECT id::text,job_type,state,document_id::text,version_id::text,requested_by_user_id::text,
		input_object_id::text,result_object_id::text,payload,attempt_count,max_attempts,progress,
		error_code,error_message,created_at::text,completed_at::text FROM claimed`, workerID, lease.String())
	job, err := scan(row)
	if err != nil {
		return Job{}, err
	}
	_, err = transaction.Exec(ctx, `INSERT INTO occccad.job_attempts(job_id,attempt,worker_id)
		VALUES($1,$2,$3)`, job.ID, job.AttemptCount, workerID)
	if err != nil {
		return Job{}, err
	}
	if err := transaction.Commit(ctx); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (service *Service) Succeed(ctx context.Context, jobID, workerID, resultObjectID string) error {
	var updated int
	err := service.database.QueryRow(ctx, `WITH finished AS (
		UPDATE occccad.jobs SET state='SUCCEEDED',progress=100,result_object_id=NULLIF($3,'')::uuid,
			completed_at=now(),lease_owner=NULL,lease_expires_at=NULL WHERE id=$1 AND state='RUNNING' AND lease_owner=$2
		RETURNING id,job_type,attempt_count), attempt AS (
		UPDATE occccad.job_attempts a SET completed_at=now(),result='SUCCEEDED'
		FROM finished WHERE a.job_id=finished.id AND a.attempt=finished.attempt_count RETURNING a.job_id), notified AS (
		INSERT INTO occccad.outbox_events(aggregate_type,aggregate_id,event_type,schema_version,payload)
		SELECT 'JOB',id,'job.state.changed',1,jsonb_build_object('jobId',id,'state','SUCCEEDED')
		FROM finished WHERE job_type IN ('EXCHANGE_IMPORT','EXCHANGE_EXPORT') RETURNING id)
	SELECT count(*) FROM finished`, jobID, workerID, resultObjectID).Scan(&updated)
	if err != nil {
		return err
	}
	if updated == 0 {
		return ErrNotFound
	}
	return nil
}

func (service *Service) SucceedImport(ctx context.Context, jobID, workerID, documentID string) error {
	var updated int
	err := service.database.QueryRow(ctx, `WITH finished AS (
		UPDATE occccad.jobs SET state='SUCCEEDED',progress=100,document_id=$3,
			completed_at=now(),lease_owner=NULL,lease_expires_at=NULL
			WHERE id=$1 AND state='RUNNING' AND lease_owner=$2 RETURNING id,attempt_count), attempt AS (
		UPDATE occccad.job_attempts a SET completed_at=now(),result='SUCCEEDED'
		FROM finished WHERE a.job_id=finished.id AND a.attempt=finished.attempt_count RETURNING a.job_id), notified AS (
		INSERT INTO occccad.outbox_events(aggregate_type,aggregate_id,event_type,schema_version,payload)
		SELECT 'JOB',id,'job.state.changed',1,jsonb_build_object('jobId',id,'state','SUCCEEDED')
		FROM finished RETURNING id)
	SELECT count(*) FROM finished`, jobID, workerID, documentID).Scan(&updated)
	if err != nil {
		return err
	}
	if updated == 0 {
		return ErrNotFound
	}
	return nil
}

func (service *Service) Heartbeat(ctx context.Context, jobID, workerID string, lease time.Duration) error {
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	command, err := service.database.Exec(ctx, `UPDATE occccad.jobs SET heartbeat_at=now(),
		lease_expires_at=now()+$3::interval WHERE id=$1 AND state='RUNNING' AND lease_owner=$2`,
		jobID, workerID, lease.String())
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (service *Service) Fail(ctx context.Context, job Job, workerID, code, message string) error {
	state := "FAILED"
	delay := "0 seconds"
	if job.AttemptCount < job.MaxAttempts {
		state, delay = "RETRY_WAIT", (time.Duration(job.AttemptCount) * 5 * time.Second).String()
	}
	_, err := service.database.Exec(ctx, `WITH finished AS (
		UPDATE occccad.jobs SET state=$3,error_code=$4,error_message=$5,available_at=now()+$6::interval,
			completed_at=CASE WHEN $3='FAILED' THEN now() END,lease_owner=NULL,lease_expires_at=NULL
		WHERE id=$1 AND lease_owner=$2 RETURNING id,job_type,attempt_count,state), attempt AS (
		UPDATE occccad.job_attempts a SET completed_at=now(),result='FAILED',error_code=$4,error_message=$5
		FROM finished WHERE a.job_id=finished.id AND a.attempt=finished.attempt_count RETURNING a.job_id)
	INSERT INTO occccad.outbox_events(aggregate_type,aggregate_id,event_type,schema_version,payload)
	SELECT 'JOB',id,'job.state.changed',1,jsonb_build_object('jobId',id,'state',state)
	FROM finished WHERE state='FAILED' AND job_type IN ('EXCHANGE_IMPORT','EXCHANGE_EXPORT')`,
		job.ID, workerID, state, code, message, delay)
	return err
}

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Job, error) {
	var result Job
	err := row.Scan(&result.ID, &result.Type, &result.State, &result.DocumentID, &result.VersionID,
		&result.RequestedBy, &result.InputObjectID, &result.ResultObjectID, &result.Payload,
		&result.AttemptCount, &result.MaxAttempts, &result.Progress, &result.ErrorCode,
		&result.ErrorMessage, &result.CreatedAt, &result.CompletedAt)
	return result, err
}
