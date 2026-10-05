package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/occccad/occccad/internal/debugartifact"
	"github.com/occccad/occccad/internal/modelcore"
	"log/slog"
	"runtime"
	"strings"
	"time"
)

// Operation diagnostics reuse the bounded debug repository, never Revision or
// commit candidates. Capture the evaluated input, not whatever Head exists later.
type OperationDiagnostic struct {
	AssemblyReplay         json.RawMessage          `json:"assemblyReplay,omitempty"`
	AssemblyManifestDigest string                   `json:"assemblyManifestDigest,omitempty"`
	ExcludedConstraintIDs  []string                 `json:"excludedConstraintIds,omitempty"`
	Schema                 string                   `json:"schema"`
	CreatedAt              time.Time                `json:"createdAt"`
	DocumentID             string                   `json:"documentId"`
	BaseRevisionID         string                   `json:"baseRevisionId,omitempty"`
	Mode                   string                   `json:"mode"`
	Stage                  string                   `json:"stage"`
	Request                CommandRequest           `json:"request"`
	Command                *modelcore.DomainCommand `json:"command,omitempty"`
	BaseModel              json.RawMessage          `json:"baseModel,omitempty"`
	Candidate              json.RawMessage          `json:"candidateModel,omitempty"`
	Failure                string                   `json:"failure"`
	Evaluator              string                   `json:"evaluator"`
	NamingPolicy           string                   `json:"namingPolicy"`
	GoVersion              string                   `json:"goVersion"`
	Artifacts              map[string]Artifact      `json:"artifacts,omitempty"`
}
type operationDiagnosticFailure struct {
	error
	id, stage string
}

func (f *operationDiagnosticFailure) Unwrap() error        { return f.error }
func (f *operationDiagnosticFailure) DiagnosticID() string { return f.id }
func (f *operationDiagnosticFailure) Phase() string {
	var p interface{ Phase() string }
	if errors.As(f.error, &p) {
		return p.Phase()
	}
	return f.stage
}
func (service *Service) SetDiagnosticArtifactStore(store *debugartifact.Store) {
	service.diagnosticArtifacts = store
}
func (service *Service) newOperationDiagnostic(ctx context.Context, documentID string, request CommandRequest, mode string) *OperationDiagnostic {
	if service.diagnosticArtifacts == nil {
		return nil
	}
	d := &OperationDiagnostic{Schema: "occccad.operation-diagnostic.v1", DocumentID: documentID, Mode: mode, Stage: "PREPARE", Request: request, CreatedAt: time.Now().UTC(), Evaluator: evaluatorVersion, NamingPolicy: modelcore.TopologyNamingPolicyDigest, GoVersion: runtime.Version()}
	// prepareDomainMutation already reads the immutable baseline, including
	// adaptation failures. Reuse it instead of reading Head a second time.
	return d
}
func (d *OperationDiagnostic) prepared(p preparedDomainMutation) {
	if d == nil || p.headRevision == "" {
		return
	}
	d.Request.RequestID = p.requestID
	d.BaseRevisionID = p.headRevision
	d.BaseModel = p.modelJSON
	d.Command = &p.command
}
func (d *OperationDiagnostic) stage(stage string) {
	if d != nil {
		d.Stage = stage
	}
}
func (service *Service) finishOperationDiagnostic(ctx context.Context, d *OperationDiagnostic, candidate *json.RawMessage, model *PartModel, failure *error) {
	if d == nil || *failure == nil || errors.Is(*failure, context.Canceled) {
		return
	}
	var existing interface{ DiagnosticID() string }
	if errors.As(*failure, &existing) {
		return
	}
	if candidate != nil {
		d.Candidate = *candidate
	}
	if model != nil {
		d.Candidate, _ = json.Marshal(model)
	}
	d.Failure = (*failure).Error()
	archive, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	// Pair immutable baseline geometry keys with their existing BREP/Naming/GLB refs.
	var base PartModel
	if json.Unmarshal(d.BaseModel, &base) == nil {
		d.Artifacts = map[string]Artifact{}
		for _, b := range base.Bodies {
			if b.GeometryKey != "" {
				if a, e := service.loadArtifact(archive, b.GeometryKey); e == nil {
					a.Mesh = Mesh{}
					d.Artifacts[b.ID] = a
				}
			}
		}
	}
	data, err := json.Marshal(d)
	if err == nil && len(data) > 8*1024*1024 {
		err = errors.New("operation diagnostic exceeds 8 MiB")
	}
	if err == nil {
		var item debugartifact.Item
		item, err = service.diagnosticArtifacts.SaveJSON(archive, d.DocumentID, d.Request.RequestID, data)
		if err == nil {
			*failure = &operationDiagnosticFailure{error: *failure, id: d.DocumentID + "/" + item.ID, stage: d.Stage}
			return
		}
	}
	slog.WarnContext(ctx, "cannot retain operation diagnostic", "document_id", d.DocumentID, "request_id", d.Request.RequestID, "error", err)
}
func (service *Service) ReadOperationDiagnostic(ctx context.Context, documentID, id string) ([]byte, error) {
	if service.diagnosticArtifacts == nil {
		return nil, ErrNotFound
	}
	if strings.Contains(id, "/") {
		return nil, ErrNotFound
	}
	_, data, err := service.diagnosticArtifacts.Read(ctx, documentID, id, "")
	if errors.Is(err, debugartifact.ErrNotFound) {
		return nil, ErrNotFound
	}
	return data, err
}
