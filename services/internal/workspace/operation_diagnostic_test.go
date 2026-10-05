package workspace

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/debugartifact"
)

func TestOperationDiagnosticFailureSnapshotAndColdRead(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "diagnostics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	local, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := NewWithArtifacts(db, nil, artifact.NewService(db, local))
	root := t.TempDir()
	store, err := debugartifact.NewStore(root, 50, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	s.SetDiagnosticArtifactStore(store)
	view, err := s.CreateDocument(t.Context(), CreateDocumentRequest{ActorID: access.DefaultUserID, Type: "PART", Name: "Diagnostic baseline"})
	if err != nil {
		t.Fatal(err)
	}
	// Adaptation can fail before a candidate exists; retain the request and
	// the exact prepared baseline without creating a Revision.
	view, err = s.ApplyCommand(t.Context(), view.Document.ID, CommandRequest{ActorID: access.DefaultUserID, RequestID: "create-length", Type: "CREATE_PARAMETER", Name: "length", Value: 20, Unit: "mm"})
	if err != nil {
		t.Fatal(err)
	}
	initial := view.Document.VersionID
	request := CommandRequest{ActorID: access.DefaultUserID, RequestID: "failed-expression", Type: "EDIT_PARAMETER", ParameterID: view.Part.Parameters[0].ParameterID, Expression: "missing_alias + 5", Name: "length"}
	_, err = s.PreviewCommand(t.Context(), view.Document.ID, request)
	if err == nil {
		t.Fatal("invalid expression accepted")
	}
	var diagnostic interface{ DiagnosticID() string }
	if !errors.As(err, &diagnostic) {
		t.Fatal("missing diagnostic", err)
	}
	reference := strings.Split(diagnostic.DiagnosticID(), "/")
	data, err := s.ReadOperationDiagnostic(t.Context(), reference[0], reference[1])
	if err != nil {
		t.Fatal(err)
	}
	var record OperationDiagnostic
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.BaseRevisionID != initial || len(record.BaseModel) == 0 || record.Request.RequestID != request.RequestID || record.Failure == "" {
		t.Fatalf("incomplete failure snapshot: %+v", record)
	}
	head, err := s.GetDocument(t.Context(), view.Document.ID, access.DefaultUserID)
	if err != nil || head.Document.VersionID != initial {
		t.Fatal("preview advanced Head", err)
	}
	_, err = s.ApplyCommand(t.Context(), view.Document.ID, CommandRequest{ActorID: access.DefaultUserID, RequestID: "advance-head", Type: "CREATE_PARAMETER", Name: "later", Value: 3, Unit: "mm"})
	if err != nil {
		t.Fatal(err)
	}
	cold := NewWithArtifacts(db, nil, artifact.NewService(db, local))
	reopened, e := debugartifact.NewStore(root, 50, 7*24*time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	cold.SetDiagnosticArtifactStore(reopened)
	again, e := cold.ReadOperationDiagnostic(t.Context(), reference[0], reference[1])
	if e != nil || string(again) != string(data) {
		t.Fatal("diagnostic substituted current Head", e)
	}
	request.RequestID = "failed-apply"
	_, err = s.ApplyCommand(t.Context(), view.Document.ID, request)
	if err == nil || !errors.As(err, &diagnostic) {
		t.Fatal("application failure missing diagnostic", err)
	}
}

func TestAssemblySolveFailureCapturesImmutableProductContext(t *testing.T) {
	store, e := debugartifact.NewStore(t.TempDir(), 50, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	service := &Service{}
	service.SetDiagnosticArtifactStore(store)
	model := ProductModel{Instances: []ProductInstance{{ID: "occurrence", Translation: [3]float64{1, 2, 3}}}, Constraints: []AssemblyConstraint{{ID: "pending", Kind: "CONCENTRIC"}}}
	err := service.solveAssemblySet(t.Context(), "product", "frozen-revision", "constraint-preview", "", nil, &model, "", map[string]bool{"other": true}, true)
	var diagnostic interface{ DiagnosticID() string }
	if !errors.As(err, &diagnostic) {
		t.Fatal("assembly error bypassed operation diagnostics", err)
	}
	ids := strings.Split(diagnostic.DiagnosticID(), "/")
	data, e := service.ReadOperationDiagnostic(t.Context(), ids[0], ids[1])
	if e != nil {
		t.Fatal(e)
	}
	var record OperationDiagnostic
	if e = json.Unmarshal(data, &record); e != nil {
		t.Fatal(e)
	}
	if record.BaseRevisionID != "frozen-revision" || record.Mode != "ASSEMBLY" || record.Stage != "RESOLVING_GEOMETRY" || len(record.ExcludedConstraintIDs) != 1 || len(record.BaseModel) == 0 || len(record.Candidate) == 0 {
		t.Fatal("assembly context missing", record)
	}
}
