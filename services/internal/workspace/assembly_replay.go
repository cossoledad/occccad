package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/debugartifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

type debugArtifactWrite struct {
	documentID, requestID string
	data                  []byte
}

func (service *Service) SetDebugArtifactStore(store *debugartifact.Store) {
	service.debugArtifacts = store
	service.debugArtifactWrites = make(chan debugArtifactWrite, 64)
	go func() {
		for value := range service.debugArtifactWrites {
			archiveCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, err := store.Save(archiveCtx, value.documentID, value.requestID, value.data)
			cancel()
			if err != nil {
				slog.Error("archive assembly replay", "document_id", value.documentID, "request_id", value.requestID, "error", err)
			}
		}
	}()
}

func (service *Service) captureAssemblyReplay(ctx context.Context, documentID, requestID string) func([]byte, error) {
	return func(data []byte, err error) {
		if err != nil {
			slog.ErrorContext(ctx, "archive assembly replay", "document_id", documentID, "request_id", requestID, "error", err)
			return
		}
		if service.debugArtifacts == nil {
			return
		}
		// A bounded single-writer queue keeps diagnostics outside latency and load-bearing state.
		select {
		case service.debugArtifactWrites <- debugArtifactWrite{documentID: documentID, requestID: requestID, data: append([]byte(nil), data...)}:
		default:
			slog.WarnContext(ctx, "drop assembly replay because debug queue is full", "document_id", documentID, "request_id", requestID)
		}
	}
}

func (service *Service) ListAssemblyReplays(ctx context.Context, documentID string) ([]debugartifact.Item, error) {
	if service.debugArtifacts == nil {
		return []debugartifact.Item{}, nil
	}
	return service.debugArtifacts.List(ctx, documentID, 50)
}

func (service *Service) ReadAssemblyReplay(ctx context.Context, documentID, id, requestID string) (debugartifact.Item, []byte, error) {
	if service.debugArtifacts == nil {
		return debugartifact.Item{}, nil, ErrNotFound
	}
	item, data, err := service.debugArtifacts.Read(ctx, documentID, id, requestID)
	if errors.Is(err, debugartifact.ErrNotFound) {
		return item, data, ErrNotFound
	}
	return item, data, err
}

// ExportAssemblyDiagnostic freezes the complete current revision, including
// disabled/unaccepted definitions. Compilation uses copies; no solve, history,
// admission, warm start or manifest persistence runs during this read.
func (service *Service) ExportAssemblyDiagnostic(ctx context.Context, documentID string) ([]byte, error) {
	var kind, revision string
	var raw []byte
	if err := service.database.QueryRow(ctx, `SELECT d.document_type,d.head_version_id::text,v.model_json FROM occccad.documents d JOIN occccad.document_versions v ON v.id=d.head_version_id WHERE d.id=$1 AND d.deleted_at IS NULL`, documentID).Scan(&kind, &revision, &raw); err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if kind != "PRODUCT" {
		return nil, fmt.Errorf("%w: assembly diagnostics require a Product", ErrValidation)
	}
	var product ProductModel
	if err := json.Unmarshal(raw, &product); err != nil {
		return nil, err
	}
	all, err := cloneAssemblyProduct(product)
	if err != nil {
		return nil, err
	}
	for i := range all.Constraints {
		all.Constraints[i].Suppressed = false
	}
	frozen, err := service.FreezeAssemblyInput(ctx, documentID, revision, all)
	if err != nil {
		return nil, err
	}
	// Preserve the actual definition states, not compilation's projected states.
	frozen.Definitions = product.Constraints
	frozen.ModelHash = canonicalModelHash(raw)
	if err := prepareDiagnosticManifest(&frozen, false); err != nil {
		return nil, err
	}
	input, err := geometry.MakeAssemblyReplay("diagnostic/"+documentID+"/"+revision, frozen.Bodies, frozen.Geometry, frozen.Constraints, geometry.AssemblySolveOptions{SolverProfile: &frozen.SolverProfile, DisableConflictProbes: true})
	if err != nil {
		return nil, err
	}
	var file map[string]json.RawMessage
	if err = json.Unmarshal(input, &file); err != nil {
		return nil, err
	}
	snapshot := struct {
		Schema     string                `json:"schema"`
		DocumentID string                `json:"documentId"`
		RevisionID string                `json:"revisionId"`
		Product    ProductModel          `json:"product"`
		Manifest   AssemblySolveManifest `json:"manifest"`
		Sources    []json.RawMessage     `json:"sources"`
	}{Schema: "occccad.assembly-diagnostic.v1", DocumentID: documentID, RevisionID: revision, Product: product, Manifest: frozen}
	seen := map[string]bool{}
	queue := append([]ProductInstance(nil), product.Instances...)
	for len(queue) > 0 {
		instance := queue[0]
		queue = queue[1:]
		key := instance.ReferencedDocumentID + "/" + instance.ReferencedVersionID
		if seen[key] {
			continue
		}
		if len(seen) >= maxManifestBodies {
			return nil, fmt.Errorf("%w: diagnostic source limit exceeded", ErrValidation)
		}
		seen[key] = true
		var model []byte
		var kind string
		err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id WHERE v.document_id=$1 AND v.id=$2`, instance.ReferencedDocumentID, instance.ReferencedVersionID).Scan(&kind, &model)
		source := struct {
			DocumentID  string          `json:"documentId"`
			RevisionID  string          `json:"revisionId"`
			Model       json.RawMessage `json:"model,omitempty"`
			Unavailable string          `json:"unavailable,omitempty"`
		}{DocumentID: instance.ReferencedDocumentID, RevisionID: instance.ReferencedVersionID, Model: model}
		if errors.Is(err, database.ErrNoRows) {
			source.Unavailable = "SOURCE_REVISION_UNAVAILABLE"
		} else if err != nil {
			return nil, err
		}
		if kind == "PRODUCT" && len(model) > 0 {
			var nested ProductModel
			if err := json.Unmarshal(model, &nested); err != nil {
				return nil, err
			}
			queue = append(queue, nested.Instances...)
		}
		encoded, err := json.Marshal(source)
		if err != nil {
			return nil, err
		}
		snapshot.Sources = append(snapshot.Sources, encoded)
	}
	file["snapshot"], err = json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(file, "", "  ")
}

// prepareDiagnosticManifest changes only a replay copy. NotUpdated definitions
// remain outside the accepted solve unless explicitly requested by a debugger.
func prepareDiagnosticManifest(manifest *AssemblySolveManifest, includeUnverified bool) error {
	states := map[string]AssemblyConstraint{}
	excluded := map[string]bool{}
	for _, definition := range manifest.Definitions {
		states[definition.ID] = definition
		excluded[definition.ID] = definition.Suppressed || (definition.EvaluationStatus != modelcore.AssemblyConstraintVerified && !(includeUnverified && definition.EvaluationStatus == modelcore.AssemblyConstraintNotUpdated))
	}
	for i := range manifest.Constraints {
		c := &manifest.Constraints[i]
		definition, ok := states[c.ID]
		if !ok {
			return fmt.Errorf("%w: diagnostic primitive has no definition", ErrValidation)
		}
		c.Mode = definition.Mode
		if c.Mode == "" {
			c.Mode = "DRIVING"
		}
		if excluded[c.ID] {
			c.Mode = "SUPPRESSED"
		}
	}
	var err error
	manifest.GroupStages, err = prepareAssemblyGroupStages(*manifest, excluded)
	if err != nil {
		return err
	}
	manifest.Digest = assemblyManifestDigest(*manifest)
	return validateAssemblySolveManifest(*manifest)
}

// ReplayAssemblyDiagnostic uses the production frozen-manifest/group solve with
// no database, source models, admission, history or artifact writes.
func ReplayAssemblyDiagnostic(ctx context.Context, client *geometry.Client, data []byte, includeUnverified bool) ([]byte, error) {
	var file geometry.AssemblyReplay
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if len(file.Snapshot) == 0 {
		if includeUnverified {
			return nil, fmt.Errorf("include-unverified requires a complete assembly diagnostic")
		}
		return client.ReplayAssembly(ctx, data)
	}
	var snapshot struct {
		Schema   string                `json:"schema"`
		Manifest AssemblySolveManifest `json:"manifest"`
	}
	if err := json.Unmarshal(file.Snapshot, &snapshot); err != nil {
		return nil, err
	}
	if file.Schema != geometry.AssemblyReplaySchema || snapshot.Schema != "occccad.assembly-diagnostic.v1" || snapshot.Manifest.Digest != assemblyManifestDigest(snapshot.Manifest) {
		return nil, fmt.Errorf("%w: invalid assembly diagnostic schema or manifest digest", ErrValidation)
	}
	if err := prepareDiagnosticManifest(&snapshot.Manifest, includeUnverified); err != nil {
		return nil, err
	}
	file.Result, file.TransportError, file.AssemblyResult = nil, "", nil
	var captured []byte
	result, solveErr := New(nil, client).solveFrozenManifest(ctx, "diagnostic/replay", snapshot.Manifest, func(data []byte, err error) {
		if err == nil {
			captured = data
		}
	})
	if len(captured) > 0 {
		var numeric geometry.AssemblyReplay
		if err := json.Unmarshal(captured, &numeric); err != nil {
			return nil, err
		}
		file.Result, file.TransportError = numeric.Result, numeric.TransportError
	}
	if solveErr != nil {
		file.TransportError = solveErr.Error()
	}
	var err error
	file.AssemblyResult, err = json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return json.Marshal(file)
}
