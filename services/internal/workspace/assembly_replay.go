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
			service.debugArtifactQueuedBytes.Add(-int64(len(value.data)))
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
		if len(data) > assemblyDiagnosticBudget {
			slog.WarnContext(ctx, "drop assembly replay: byte budget exceeded", "document_id", documentID)
			return
		}
		size := int64(len(data))
		for {
			old := service.debugArtifactQueuedBytes.Load()
			if old+size > 32<<20 {
				slog.WarnContext(ctx, "drop assembly replay: queue byte budget exceeded", "document_id", documentID)
				return
			}
			if service.debugArtifactQueuedBytes.CompareAndSwap(old, old+size) {
				break
			}
		}
		// Owned serialized bytes are immutable; archiving does not duplicate them.
		select {
		case service.debugArtifactWrites <- debugArtifactWrite{documentID: documentID, requestID: requestID, data: data}:
		default:
			service.debugArtifactQueuedBytes.Add(-size)
			slog.WarnContext(ctx, "drop assembly replay: queue frame budget exceeded", "document_id", documentID, "request_id", requestID)
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
type AssemblyDiagnosticExportOptions struct{ DetailedNaming bool }

func (service *Service) ExportAssemblyDiagnostic(ctx context.Context, documentID string) ([]byte, error) {
	return service.ExportAssemblyDiagnosticWithOptions(ctx, documentID, AssemblyDiagnosticExportOptions{})
}
func (service *Service) ExportAssemblyDiagnosticWithOptions(ctx context.Context, documentID string, options AssemblyDiagnosticExportOptions) ([]byte, error) {
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
	capture := &assemblyDiagnosticCapture{invalid: map[string]bool{}}
	ctx = context.WithValue(ctx, assemblyDiagnosticCaptureKey{}, capture)
	frozen, err := service.FreezeAssemblyInput(ctx, documentID, revision, all)
	if err != nil {
		capture.record("", "", "INPUT_COMPILATION", err)
		if frozen.SchemaVersion == 0 {
			frozen = AssemblySolveManifest{RootProductDocumentID: documentID, RootProductRevisionID: revision, SolverProfile: defaultAssemblySolverProfile(), SolverBuildPolicy: assemblySolverBuildPolicy}
			for _, instance := range product.Instances {
				frozen.Bodies = append(frozen.Bodies, geometry.AssemblyBody{ID: instance.ID, Pose: geometry.AssemblyPose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)}})
			}
		}
	}
	frozen.ModelHash = canonicalModelHash(raw)
	// Definitions are saved states, never compilation's projected state.
	frozen.Definitions = product.Constraints
	snapshot, err := buildAssemblyDiagnosticSnapshot(frozen, product.Constraints, capture.failures)
	if err != nil {
		return nil, err
	}
	snapshot.Documents, err = service.assemblyDiagnosticDocuments(ctx, product)
	if err != nil {
		return nil, err
	}
	if options.DetailedNaming {
		if err := snapshot.appendNamingEvidence(frozen.ResolutionEvidence); err != nil {
			return nil, err
		}
	}
	service.appendDiagnosticAttempts(ctx, &snapshot, product.Constraints)
	return encodeAssemblyDiagnostic(snapshot)
}

func encodeAssemblyDiagnostic(snapshot AssemblyDiagnosticSnapshot) ([]byte, error) {
	encode := func() ([]byte, error) {
		snapshot.Digest = snapshot.contentDigest()
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
		return json.Marshal(geometry.AssemblyReplay{Schema: geometry.AssemblyReplaySchema, Units: "mm,rad; body-local geometry; quaternion xyzw", Snapshot: raw, Outcome: "NOT_ATTEMPTED"})
	}
	data, err := encode()
	if err != nil {
		return nil, err
	}
	if len(data) > snapshot.ByteBudget {
		// Preserve the core numerical tables/definitions; budgeted historical input loss is explicit.
		for i := range snapshot.Attempts {
			snapshot.Attempts[i].Sequence = nil
			snapshot.Attempts[i].Missing = append(snapshot.Attempts[i].Missing, "EXPORT_BYTE_BUDGET_EXCEEDED")
		}
		snapshot.Missing = append(snapshot.Missing, "HISTORICAL_INPUTS_OMITTED_BY_BYTE_BUDGET")
		snapshot.Documents = nil
		snapshot.NamingEvidence = nil
		snapshot.NamingLinks = nil
		snapshot.Missing = append(snapshot.Missing, "OPTIONAL_METADATA_OMITTED_BY_BYTE_BUDGET")
		// Remove historical-only table objects as well as stage references.
		expanded, expandErr := snapshot.ExpandFrame(snapshot.Current)
		if expandErr != nil {
			return nil, expandErr
		}
		snapshot.Geometry = nil
		snapshot.Primitives = nil
		var packErr error
		snapshot.Current, packErr = packDiagnosticFrame(expanded, newDiagnosticTable(&snapshot.Geometry), newDiagnosticTable(&snapshot.Primitives))
		if packErr != nil {
			return nil, packErr
		}
		data, err = encode()
	}
	if err == nil && len(data) > snapshot.ByteBudget {
		return nil, fmt.Errorf("assembly diagnostic core requires %d bytes; configured budget %d", len(data), snapshot.ByteBudget)
	}
	return data, err
}

func (service *Service) assemblyDiagnosticDocuments(ctx context.Context, product ProductModel) ([]AssemblyDiagnosticDocument, error) {
	sources := []AssemblyDiagnosticDocument{}
	seen := map[string]bool{}
	queue := append([]ProductInstance(nil), product.Instances...)
	for len(queue) > 0 {
		instance := queue[0]
		queue = queue[1:]
		key := instance.ReferencedDocumentID + "/" + instance.ReferencedVersionID
		if seen[key] {
			continue
		}
		seen[key] = true
		if len(seen) > maxManifestBodies {
			return nil, fmt.Errorf("diagnostic document limit exceeded")
		}
		source := AssemblyDiagnosticDocument{DocumentID: instance.ReferencedDocumentID, RevisionID: instance.ReferencedVersionID}
		var raw []byte
		err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id WHERE v.document_id=$1 AND v.id=$2`, source.DocumentID, source.RevisionID).Scan(&source.Type, &raw)
		if err != nil {
			source.Unavailable = "SOURCE_REVISION_UNAVAILABLE"
		} else if source.Type == "PRODUCT" {
			var nested ProductModel
			if err := json.Unmarshal(raw, &nested); err != nil {
				source.Unavailable = "SOURCE_MODEL_UNREADABLE"
			} else {
				queue = append(queue, nested.Instances...)
			}
		} else {
			// No Part history/BREP bytes: exact mathematical descriptors already exist in the geometry table.
			var part struct {
				Bodies []struct {
					ID string `json:"id"`
				} `json:"bodies"`
			}
			if err := json.Unmarshal(raw, &part); err != nil {
				source.Unavailable = "SOURCE_MODEL_UNREADABLE"
			} else {
				for _, b := range part.Bodies {
					source.BodyIDs = append(source.BodyIDs, b.ID)
				}
			}
		}
		sources = append(sources, source)
	}
	return sources, nil
}
