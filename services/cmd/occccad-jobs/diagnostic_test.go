package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/debugartifact"
	"github.com/occccad/occccad/internal/jobs"
)

func TestJobFailureRetainsAttemptAndImmutableInput(t *testing.T) {
	store, err := debugartifact.NewStore(t.TempDir(), 50, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := handler{diagnostics: store}
	source, document, revision := "source-object", "owner-document", "frozen-revision"
	job := jobs.Job{ID: "job-fixture", Type: "MOTION_RUN", AttemptCount: 2, Payload: json.RawMessage(`{"revision":"frozen","target":3}`),
		InputObjectID: &source, DocumentID: &document, VersionID: &revision, RequestedBy: "actor"}
	t.Setenv("OCCCCAD_S3_SECRET_KEY", "private-fixture-credential")
	reference := h.retainJobFailure(t.Context(), job, errors.New("worker failed private-fixture-credential"))
	parts := strings.Split(reference, "/")
	if len(parts) != 2 {
		t.Fatal("job failure has no locator")
	}
	_, before, err := store.Read(t.Context(), parts[0], parts[1], "")
	if err != nil {
		t.Fatal(err)
	}
	var captured map[string]json.RawMessage
	if err = json.Unmarshal(before, &captured); err != nil {
		t.Fatal(err)
	}
	if string(captured["attempt"]) != "2" || string(captured["payload"]) != string(job.Payload) || strings.Contains(string(before), "private-fixture-credential") {
		t.Fatal("incorrect attempt, input or credential boundary")
	}
	if string(captured["inputObjectId"]) != `"source-object"` || string(captured["versionId"]) != `"frozen-revision"` || string(captured["documentId"]) != `"owner-document"` {
		t.Fatal("job input columns outside payload were lost")
	}
	source, revision = "later-source", "later-revision"
	other := h.retainJobFailure(t.Context(), job, errors.New("later attempt"))
	otherParts := strings.Split(other, "/")
	_, otherBytes, err := store.Read(t.Context(), otherParts[0], otherParts[1], "")
	if err != nil {
		t.Fatal(err)
	}
	var changed map[string]json.RawMessage
	if err = json.Unmarshal(otherBytes, &changed); err != nil {
		t.Fatal(err)
	}
	if string(changed["inputDigest"]) == string(captured["inputDigest"]) {
		t.Fatal("job input digest ignored changed frozen source/revision references")
	}
	job.Payload = json.RawMessage(`{"revision":"later"}`)
	_, after, err := store.Read(t.Context(), parts[0], parts[1], "")
	if err != nil || string(before) != string(after) {
		t.Fatal("diagnostic followed a mutable job", err)
	}
	if h.retainJobFailure(t.Context(), job, context.Canceled) != "" {
		t.Fatal("cancellation archived as a failure")
	}
}
