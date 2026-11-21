package api

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/exchange"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/workspace"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExchangeFormatsAndUploadLimit(t *testing.T) {
	if exchangeUploadLimit() < 1<<30 {
		t.Fatalf("exchange upload limit %d does not satisfy the 1 GiB product requirement", exchangeUploadLimit())
	}
	tests := []struct {
		input, format, extension string
		valid                    bool
	}{
		{"step", "STEP", ".step", true},
		{"STP", "STEP", ".step", true},
		{"brep", "BREP", ".brep", true},
		{"BRP", "BREP", ".brep", true},
		{"iges", "", "", false},
	}
	for _, test := range tests {
		format, extension, _, valid := exchangeFormat(test.input)
		if format != test.format || extension != test.extension || valid != test.valid {
			t.Fatalf("exchangeFormat(%q) = %q, %q, %v", test.input, format, extension, valid)
		}
	}
}

func TestExchangeUploadLimitConfiguration(t *testing.T) {
	t.Setenv("OCCCCAD_EXCHANGE_MAX_BYTES", "2147483648")
	if exchangeUploadLimit() != 2<<30 {
		t.Fatal("configured limit ignored")
	}
	for _, value := range []string{"", "-1", "+1024", "18446744073709551615", "invalid"} {
		t.Setenv("OCCCCAD_EXCHANGE_MAX_BYTES", value)
		if exchangeUploadLimit() != 16<<30 {
			t.Fatal("unsafe fallback")
		}
	}
}

func TestExchangeStreamingLimitRejectsBeforePublication(t *testing.T) {
	t.Setenv("OCCCCAD_EXCHANGE_MAX_BYTES", "1024")
	directory := t.TempDir()
	store, err := artifact.NewLocalStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{artifacts: artifact.NewService(nil, store)}
	request := httptest.NewRequest(http.MethodPost, "/api/exchange/imports?format=STEP&fileName=model.step", strings.NewReader(strings.Repeat("x", 1025)))
	request.ContentLength = -1 // chunked requests cannot bypass the limit
	writer := httptest.NewRecorder()
	server.startExchangeImport(writer, request)
	if writer.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d: %s", writer.Code, writer.Body.String())
	}
	files, _ := filepath.Glob(filepath.Join(directory, "artifacts", ".staging", "*"))
	if len(files) > 0 {
		t.Fatal("failed HTTP upload left scratch")
	}
}

func TestRecoverPartFindsItsInitialRevisionWithIndexedLookups(t *testing.T) {
	ctx := t.Context()
	db, err := database.Open(ctx, "sqlite:"+filepath.Join(t.TempDir(), "recovery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(ctx, "EXPLAIN QUERY PLAN "+importCreationRevision, "fixture/definition/one/document")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a, b, c int
		var plan string
		if err = rows.Scan(&a, &b, &c, &plan); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(plan, "SCAN ") {
			t.Fatal("unbounded recovery lookup", plan)
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	local, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := artifact.NewService(db, local)
	domain := workspace.NewWithArtifacts(db, nil, store)
	actor := access.DefaultUserID
	part, err := domain.CommitImportedEmptyPart(ctx, actor, "", "fixture/definition/one", "Fan Guide", "fixture.step")
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.Put(ctx, artifact.KindExchangeSource, "model/step", strings.NewReader("fixture"))
	if err != nil {
		t.Fatal(err)
	}
	queue := jobs.New(db)
	job, err := queue.Enqueue(ctx, jobs.EnqueueRequest{Type: "EXCHANGE_IMPORT", RequestedBy: actor, InputObjectID: source.ID, IdempotencyKey: "fixture", Payload: map[string]string{"requestId": "fixture", "fileName": "fixture.step", "format": "STEP"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = queue.Claim(ctx, "fixture", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err = queue.SucceedImport(ctx, job.ID, "fixture", part.Document.ID, &exchange.Report{Warnings: []exchange.Warning{{DefinitionID: "one", Code: "EXCHANGE_COMPONENT_DISPLAY_UNSUPPORTED", Message: "Mesh failed", DocumentID: part.Document.ID}}}); err != nil {
		t.Fatal(err)
	}
	server := &Server{jobs: queue, workspace: domain, database: db, access: access.New(db)}
	post := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/recover-part", strings.NewReader(`{"definitionId":"one"}`))
		r.SetPathValue("jobID", job.ID)
		r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: actor}))
		w := httptest.NewRecorder()
		server.recoverImportedPart(w, r)
		return w
	}
	w := post()
	var recovery jobs.Job
	if w.Code != http.StatusAccepted || json.Unmarshal(w.Body.Bytes(), &recovery) != nil || recovery.DocumentID == nil || *recovery.DocumentID != part.Document.ID || recovery.VersionID == nil || *recovery.VersionID != part.Document.VersionID {
		t.Fatalf("recovery response %d %s", w.Code, w.Body.String())
	}
	_, err = domain.ApplyCommand(ctx, part.Document.ID, workspace.CommandRequest{ActorID: actor, Type: "CREATE_PARAMETER", RequestID: "parameter", Name: "Length", Value: 2, Unit: "mm"})
	if err != nil {
		t.Fatal(err)
	}
	if w := post(); w.Code != http.StatusConflict {
		t.Fatalf("edited target accepted: %d %s", w.Code, w.Body.String())
	}
}
