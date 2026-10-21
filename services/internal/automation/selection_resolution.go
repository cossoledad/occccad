package automation

import (
	"context"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type ResolvedSelection struct {
	SourceRevisionID        string                              `json:"sourceRevisionId"`
	TargetRevisionID        string                              `json:"targetRevisionId"`
	ManifestDigest          string                              `json:"manifestDigest"`
	PolicyDigest            string                              `json:"policyDigest"`
	OriginalSelectionDigest string                              `json:"originalSelectionDigest"`
	Status                  modelcore.SelectionResolutionStatus `json:"status"`
	EvidenceDigest          string                              `json:"evidenceDigest,omitempty"`
	TopologyID              string                              `json:"topologyId"` // Naming identity, never a mesh/OCCT locator
	Selection               workspace.FeatureSelection          `json:"selection"`
}

type formalSelectionResolver interface {
	ResolvePersistentSelection(context.Context, string, workspace.ResolvePersistentSelectionRequest) (modelcore.SelectionResolution, error)
}

func (s *Service) ResolveSelectionForSnapshot(ctx context.Context, actor, document, revision string, selection workspace.FeatureSelection) (workspace.FeatureSelection, error) {
	resolved, err := s.ResolveSelectionForSnapshotWithEvidence(ctx, actor, document, revision, selection)
	return resolved.Selection, err
}

// ResolveSelectionForSnapshotWithEvidence retains the immutable original target
// and separately proves its unique current Naming identity. Merely swapping the
// old source revision is never a valid relocation.
func (s *Service) ResolveSelectionForSnapshotWithEvidence(ctx context.Context, actor, document, revision string, selection workspace.FeatureSelection) (ResolvedSelection, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return ResolvedSelection{}, err
	}
	if revision == "" || selection.SourceVersionID == "" || selection.Selection.SourceDocumentID != document || selection.Selection.Validate() != nil {
		return ResolvedSelection{}, fault("INVALID_SELECTION", "source selection and exact target revision are required")
	}
	resolver, ok := s.workspace.(formalSelectionResolver)
	reader, hasTopology := s.workspace.(topologyReader)
	if !ok || !hasTopology {
		return ResolvedSelection{}, fault("SELECTION_RESOLVER_UNAVAILABLE", "formal selection relocation is unavailable")
	}
	snapshot, err := s.workspace.ReadModelSnapshot(ctx, document, revision)
	if err != nil {
		return ResolvedSelection{}, err
	}
	if snapshot.DocumentType != "PART" {
		return ResolvedSelection{}, fault("DOCUMENT_TYPE_UNSUPPORTED", "Part selection relocation requires its owning Part document")
	}
	data, err := reader.ReadBodyTopology(ctx, document, revision, selection.Selection.SourceBodyID, string(selection.Selection.ExpectedType))
	if err != nil {
		return ResolvedSelection{}, err
	}
	resolution, err := resolver.ResolvePersistentSelection(ctx, document, workspace.ResolvePersistentSelectionRequest{Selection: selection.Selection, SourceVersionID: selection.SourceVersionID, TargetVersionID: revision, ManifestDigest: data.ManifestDigest, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
	if err != nil {
		return ResolvedSelection{}, err
	}
	result := ResolvedSelection{SourceRevisionID: selection.SourceVersionID, TargetRevisionID: revision, ManifestDigest: data.ManifestDigest, PolicyDigest: modelcore.TopologyNamingPolicyDigest, OriginalSelectionDigest: modelcore.ValueDigest(marshal(selection)), Status: resolution.Status, EvidenceDigest: resolution.EvidenceDigest}
	if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
		code := resolution.DiagnosticCode
		if code == "" {
			code = "SELECTION_" + string(resolution.Status)
		}
		return result, &Fault{Code: code, Message: "frozen selection does not resolve uniquely in the requested snapshot: " + string(resolution.Status)}
	}
	candidate := resolution.Candidates[0]
	if candidate.Type != selection.Selection.ExpectedType || candidate.GeometryKey != data.GeometryKey {
		return result, fault("SELECTION_TYPE_MISMATCH", "resolver candidate differs from the requested topology snapshot")
	}
	current := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: document, SourceBodyID: selection.Selection.SourceBodyID, Anchor: candidate.SemanticRef, ExpectedType: candidate.Type, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: candidate.Evidence}
	if err := current.Validate(); err != nil {
		return result, fault("SELECTION_CONTRACT_MISMATCH", err.Error())
	}
	id := modelcore.ValueDigest(marshal(current))
	for _, item := range data.Items {
		if item.ID == id {
			result.TopologyID = id
			result.Selection = item.Selection
			return result, nil
		}
	}
	return result, fault("SELECTION_NOT_FOUND", "unique resolver candidate has no formal current Naming identity")
}
