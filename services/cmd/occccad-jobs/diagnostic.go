package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/jobs"
)

type jobDiagnosticInput struct {
	Type          string          `json:"jobType"`
	DocumentID    *string         `json:"documentId,omitempty"`
	VersionID     *string         `json:"versionId,omitempty"`
	InputObjectID *string         `json:"inputObjectId,omitempty"`
	RequestedBy   string          `json:"requestedBy"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

func (h handler) retainJobFailure(ctx context.Context, job jobs.Job, failure error) string {
	if h.diagnostics == nil || errors.Is(failure, context.Canceled) {
		return ""
	}
	input := jobDiagnosticInput{Type: job.Type, DocumentID: job.DocumentID, VersionID: job.VersionID,
		InputObjectID: job.InputObjectID, RequestedBy: job.RequestedBy, Payload: job.Payload}
	encoded, err := json.Marshal(input)
	if err != nil {
		slog.WarnContext(ctx, "cannot encode job diagnostic input", "job_id", job.ID)
		return ""
	}
	hash := sha256.Sum256(encoded)
	snapshot := struct {
		jobDiagnosticInput
		Schema                string    `json:"schema"`
		JobID                 string    `json:"jobId"`
		Attempt               int       `json:"attempt"`
		Stage                 string    `json:"stage"`
		Failure               string    `json:"failure"`
		CreatedAt             time.Time `json:"createdAt"`
		InputDigest           string    `json:"inputDigest"`
		OperationDiagnosticID string    `json:"operationDiagnosticId,omitempty"`
		Missing               []string  `json:"missing,omitempty"`
	}{jobDiagnosticInput: input, Schema: "occccad.job-diagnostic.v1", JobID: job.ID, Attempt: job.AttemptCount,
		Stage: "EXECUTE", Failure: config.Redact(failure.Error()), CreatedAt: time.Now().UTC(),
		InputDigest: hex.EncodeToString(hash[:])}
	var operation interface{ DiagnosticID() string }
	if errors.As(failure, &operation) {
		snapshot.OperationDiagnosticID = operation.DiagnosticID()
	}
	data, err := json.Marshal(snapshot)
	if err == nil && len(data) > 8*1024*1024 {
		snapshot.Payload = nil
		snapshot.Missing = []string{"JOB_INPUT_OMITTED_BY_ARCHIVE_BYTE_BUDGET"}
		data, err = json.Marshal(snapshot)
	}
	if err == nil {
		var itemID string
		item, saveErr := h.diagnostics.SaveJSON(ctx, job.ID, job.ID, data)
		err, itemID = saveErr, item.ID
		if err == nil {
			return job.ID + "/" + itemID
		}
	}
	slog.WarnContext(ctx, "cannot retain job diagnostic", "job_id", job.ID, "error", err)
	return ""
}
