package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/occccad/occccad/internal/modelcore"
)

func insertionBlocked(f Feature, reason string) error {
	return fmt.Errorf("%w: INSERT_FEATURE_BLOCKED[%s] %s: %s", ErrValidation, f.ID, f.Name, reason)
}

func insertionAnchor(m PartModel, body, id string) (int, error) {
	i := slices.IndexFunc(m.Features, func(f Feature) bool { return f.ID == id })
	if i < 0 {
		return -1, fmt.Errorf("%w: INSERT_STAGE_MISSING", ErrValidation)
	}
	f := m.Features[i]
	if body == "" || f.BodyID != body || !isBodyFeature(f.Type) || f.Suppressed {
		return -1, insertionBlocked(f, "anchor must be an active solid stage of the specified Body")
	}
	return i, nil
}

func protectFeatureRanges(before, after PartModel, blocked func(Feature, string) error) error {
	for _, f := range before.Features {
		if f.Pattern != nil {
			for _, seed := range patternSeeds(f.Pattern) {
				p := *f.Pattern
				p.Source = seed.Source
				p.SourceKind = seed.SourceKind
				p.StartFeatureID = seed.StartFeatureID
				if p.SourceKind == "FEATURE_DELTA" && !slices.Equal(rangeMembers(before, &p), rangeMembers(after, &p)) {
					return blocked(f, "FEATURE_DELTA range membership/order is protected")
				}
			}
		}
	}
	return nil
}

func insertModifierCandidate(m *PartModel, p createFeaturePayload) error {
	if p.Feature.Type != "FILLET" && p.Feature.Type != "CHAMFER" {
		return insertionBlocked(p.Feature, "only FILLET/CHAMFER can be inserted")
	}
	i, err := insertionAnchor(*m, p.Feature.BodyID, p.AfterFeatureID)
	if err != nil {
		return err
	}
	if p.InsertionVersionID == "" || p.InsertionGeometryKey == "" {
		return insertionBlocked(p.Feature, "Revision and exact stage GeometryKey are required")
	}
	for _, pick := range p.Feature.Selections {
		if pick.SourceFeatureID != p.AfterFeatureID || pick.SourceVersionID != p.InsertionVersionID || pick.Selection.SourceBodyID != p.Feature.BodyID {
			return insertionBlocked(p.Feature, "selection must be bound to the insertion stage")
		}
	}
	m.Features = slices.Insert(m.Features, i+1, p.Feature)
	for j := range m.Features {
		m.Features[j].Order = j + 1
	}
	return nil
}

func insertionPayload(c modelcore.DomainCommand) (createFeaturePayload, bool) {
	switch c.TypeURI {
	case typeCreateModifierFeature, typeCreateSolidFeature, typeCreatePad, typeCreateSketch, typeCreatePattern, typeCreateBooleanFeature, typeImportExchange:
	default:
		return createFeaturePayload{}, false
	}
	var p createFeaturePayload
	_ = json.Unmarshal(c.Payload, &p)
	return p, p.AfterFeatureID != ""
}
func requiresOrderedEvaluation(c modelcore.DomainCommand) bool {
	_, insert := insertionPayload(c)
	return c.TypeURI == typeMoveFeature || insert
}
func orderMutationBlocker(c modelcore.DomainCommand) func(Feature, string) error {
	if _, ok := insertionPayload(c); ok {
		return insertionBlocked
	}
	return moveBlocked
}

// Check the frozen pre-command stage, never a prefix containing the new feature.
// Keeping this at the service evaluation boundary also covers typed commands.
func (s *Service) validateInsertionSource(ctx context.Context, documentID, head string, c modelcore.DomainCommand, before PartModel) error {
	p, insert := insertionPayload(c)
	if !insert {
		return nil
	}
	if c.TypeURI != typeCreateModifierFeature || p.InsertionVersionID != head {
		return insertionBlocked(p.Feature, "insertion Revision or command type is invalid")
	}
	if _, err := insertionAnchor(before, p.Feature.BodyID, p.AfterFeatureID); err != nil {
		return err
	}
	key, err := s.featureStageAtVersion(ctx, documentID, head, p.AfterFeatureID, p.Feature.BodyID, true)
	if err != nil {
		return featureInputStageError(before, p.AfterFeatureID, err)
	}
	if key != p.InsertionGeometryKey {
		return insertionBlocked(p.Feature, "GeometryKey does not belong to the insertion stage")
	}
	for _, pick := range p.Feature.Selections {
		if pick.Selection.SourceDocumentID != documentID || pick.SourceVersionID != head || pick.SourceFeatureID != p.AfterFeatureID || pick.Selection.SourceBodyID != p.Feature.BodyID {
			return insertionBlocked(p.Feature, "selection must belong to the insertion Revision, Body and stage")
		}
		resolution, err := s.resolveFeaturePick(ctx, pick, key)
		if err != nil {
			return err
		}
		if resolution.Status != modelcore.SelectionResolved {
			return insertionBlocked(p.Feature, string(resolution.Status)+": "+resolution.Diagnostic)
		}
	}
	return nil
}

// Insertion is an explicit pick context, separate from display-only ResultStage.
func (s *Service) insertionInput(ctx context.Context, documentID string, request FeatureInputRequest, view DocumentView) (FeatureInput, error) {
	if request.ResultStage || request.FeatureID != "" {
		return FeatureInput{}, fmt.Errorf("%w: insertion and result/edit contexts cannot be combined", ErrValidation)
	}
	if _, err := insertionAnchor(*view.Part, request.BodyID, request.AfterFeatureID); err != nil {
		return FeatureInput{}, err
	}
	key, err := s.featureStageAtVersion(ctx, documentID, request.VersionID, request.AfterFeatureID, request.BodyID, true)
	if err != nil {
		return FeatureInput{}, featureInputStageError(*view.Part, request.AfterFeatureID, err)
	}
	a, err := s.loadArtifact(ctx, key)
	if err != nil {
		return FeatureInput{}, err
	}
	a.BodyID = request.BodyID
	a.DisplayStageFeatureID = request.AfterFeatureID
	result := FeatureInput{VersionID: request.VersionID, SourceFeatureID: request.AfterFeatureID, Artifact: a, Artifacts: []Artifact{a}, Picks: []FeatureInputPick{}}
	if request.LocalID == 0 && request.Kind == "" && request.GeometryKey == "" {
		return result, nil
	}
	if request.LocalID == 0 || request.Kind != "EDGE" || request.GeometryKey != key {
		return FeatureInput{}, fmt.Errorf("%w: INSERT_STAGE_PICK_OUTSIDE_INPUT", ErrValidation)
	}
	manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
	if err != nil {
		return FeatureInput{}, err
	}
	output, body := manifestOutput(manifest, modelcore.PersistentTopologyEdge, request.LocalID)
	if !topologyHistoryComplete(manifest) || output == nil || body != request.BodyID {
		return FeatureInput{}, fmt.Errorf("%w: insertion topology unavailable", ErrValidation)
	}
	pick := FeatureSelection{SourceVersionID: request.VersionID, SourceFeatureID: request.AfterFeatureID, Selection: modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: documentID, SourceBodyID: body, Anchor: semanticRef(output.GetSemanticRef()), ExpectedType: modelcore.PersistentTopologyEdge, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: selectionEvidence(output.GetEvidence())}}
	if err := pick.Selection.Validate(); err != nil {
		return FeatureInput{}, err
	}
	result.Selection = &pick
	return result, nil
}
