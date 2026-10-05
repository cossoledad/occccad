package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

// FeatureInput is a read-only reconstruction of the stage before an edited
// feature. It never moves Head or writes a compensable model change.
type FeatureInputRequest struct {
	ResultStage bool   `json:"resultStage,omitempty"`
	BodyID      string `json:"bodyId,omitempty"`
	VersionID   string `json:"versionId"`
	FeatureID   string `json:"featureId"`
	GeometryKey string `json:"geometryKey,omitempty"`
	Kind        string `json:"kind,omitempty"`
	LocalID     uint64 `json:"localId,omitempty"`
}
type FeatureInputPick struct {
	Index   int    `json:"index"`
	Kind    string `json:"kind"`
	LocalID uint64 `json:"localId"`
}
type FeatureNeutralPick struct {
	DisplayStageFeatureID string `json:"displayStageFeatureId,omitempty"`
	Kind                  string `json:"kind"`
	LocalID               uint64 `json:"localId"`
	BodyID                string `json:"bodyId"`
	GeometryKey           string `json:"geometryKey"`
}
type FeatureInput struct {
	NeutralPick     *FeatureNeutralPick `json:"neutralPick,omitempty"`
	VersionID       string              `json:"versionId"`
	SourceFeatureID string              `json:"sourceFeatureId"`
	Artifact        Artifact            `json:"artifact"`
	Artifacts       []Artifact          `json:"artifacts,omitempty"`
	Picks           []FeatureInputPick  `json:"picks"`
	Selection       *FeatureSelection   `json:"selection,omitempty"`
}

func (s *Service) featureStageAtVersion(ctx context.Context, documentID, versionID, stageID, bodyID string) (string, error) {
	var raw []byte
	if err := s.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE document_id=$1 AND id=$2`, documentID, versionID).Scan(&raw); err != nil {
		return "", err
	}
	var model PartModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return "", err
	}
	normalizePartModel(&model)
	index := -1
	for i, f := range model.Features {
		if (stageID == "" || f.ID == stageID) && f.BodyID == bodyID && isBodyFeature(f.Type) && !f.Suppressed {
			index = i
			if stageID != "" {
				break
			}
		}
	}
	if index < 0 {
		return "", fmt.Errorf("%w: source feature stage missing", ErrValidation)
	}
	model.Features = append([]Feature(nil), model.Features[:index+1]...)
	return s.evaluateBodyPrefix(ctx, "selection-source/"+versionID+"/"+stageID, model, bodyID)
}

func (s *Service) resolveFeaturePick(ctx context.Context, pick FeatureSelection, targetKey string) (modelcore.SelectionResolution, error) {
	// Rebuild either the explicit stage or the historical revision's final
	// Body stage under the current evaluator. Derived artifacts from an older
	// evaluator are not the business truth and cannot prevent an exact rebuild.
	key, err := s.featureStageAtVersion(ctx, pick.Selection.SourceDocumentID, pick.SourceVersionID, pick.SourceFeatureID, pick.Selection.SourceBodyID)
	if err != nil {
		return modelcore.SelectionResolution{}, err
	}
	source, _, err := s.topologyManifestForGeometryKey(ctx, key)
	if err != nil {
		return modelcore.SelectionResolution{}, err
	}
	output := manifestSemanticOutput(source, pick.Selection)
	if !topologyHistoryComplete(source) || output == nil || pick.Selection.CreationEvidence.EvidenceDigest != output.GetEvidence().GetEvidenceDigest() {
		return unavailableSelectionResolution("CREATION_EVIDENCE_MISMATCH", "source stage does not contain the selected topology"), nil
	}
	target, _, err := s.topologyManifestForGeometryKey(ctx, targetKey)
	if err != nil {
		return modelcore.SelectionResolution{}, err
	}
	if !topologyHistoryComplete(target) {
		return unavailableSelectionResolution("TOPOLOGY_HISTORY_INCOMPLETE", "feature input history is incomplete"), nil
	}
	return resolveManifest(pick.Selection, targetKey, target), nil
}

func (s *Service) GetFeatureInput(ctx context.Context, documentID string, request FeatureInputRequest) (FeatureInput, error) {
	view, err := s.GetDocument(ctx, documentID)
	if err != nil {
		return FeatureInput{}, err
	}
	if view.Part == nil || request.VersionID != view.Document.VersionID {
		return FeatureInput{}, fmt.Errorf("%w: feature editing revision changed", ErrValidation)
	}
	index := -1
	for i, f := range view.Part.Features {
		if f.ID == request.FeatureID {
			index = i
			break
		}
	}
	if index < 0 {
		return FeatureInput{}, fmt.Errorf("%w: feature missing", ErrValidation)
	}
	feature := view.Part.Features[index]
	if request.ResultStage {
		if request.GeometryKey != "" || request.Kind != "" || request.LocalID != 0 || request.BodyID != "" {
			return FeatureInput{}, fmt.Errorf("%w: historical result is display only", ErrValidation)
		}
		key, err := s.featureStageAtVersion(ctx, documentID, request.VersionID, feature.ID, feature.BodyID)
		if err != nil {
			return FeatureInput{}, err
		}
		a, err := s.loadArtifact(ctx, key)
		if err != nil {
			return FeatureInput{}, err
		}
		a.BodyID = feature.BodyID
		a.DisplayStageFeatureID = feature.ID
		return FeatureInput{VersionID: request.VersionID, SourceFeatureID: feature.ID, Artifact: a, Picks: []FeatureInputPick{}}, nil
	}
	bodyID := feature.BodyID
	if request.BodyID != "" {
		if feature.Type != "DRAFT" && !(feature.Type == "SOLID_PATTERN" && feature.Pattern != nil && feature.Pattern.Kind == "MIRROR") {
			return FeatureInput{}, fmt.Errorf("%w: alternate input Body requires a referenced plane", ErrValidation)
		}
		bodyID = request.BodyID
	}
	stage := ""
	for _, f := range view.Part.Features[:index] {
		if f.BodyID == bodyID && isBodyFeature(f.Type) && !f.Suppressed {
			stage = f.ID
		}
	}
	if stage == "" && !isSolidGenerator(feature.Type) && feature.Type != "LOFT" {
		return FeatureInput{}, fmt.Errorf("%w: feature has no solid input", ErrValidation)
	}
	var key string
	if stage == "" {
		prefix := *view.Part
		prefix.Features = append([]Feature(nil), prefix.Features[:index]...)
		key, err = s.evaluateBodyPrefix(ctx, "empty-feature-input/"+request.VersionID+"/"+feature.ID, prefix, bodyID)
	} else {
		key, err = s.featureStageAtVersion(ctx, documentID, request.VersionID, stage, bodyID)
	}
	if err != nil {
		return FeatureInput{}, err
	}
	a, err := s.loadArtifact(ctx, key)
	if err != nil {
		return FeatureInput{}, err
	}
	a.BodyID = bodyID
	a.DisplayStageFeatureID = stage
	result := FeatureInput{VersionID: request.VersionID, SourceFeatureID: stage, Artifact: a, Picks: []FeatureInputPick{}}
	if request.LocalID != 0 {
		if request.GeometryKey != key {
			return FeatureInput{}, fmt.Errorf("%w: pick does not belong to feature input", ErrValidation)
		}
		manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
		if err != nil {
			return FeatureInput{}, err
		}
		kind := modelcore.PersistentTopologyType(request.Kind)
		output, pickedBodyID := manifestOutput(manifest, kind, request.LocalID)
		if !topologyHistoryComplete(manifest) || output == nil || pickedBodyID != bodyID {
			return FeatureInput{}, fmt.Errorf("%w: input topology unavailable", ErrValidation)
		}
		pick := FeatureSelection{SourceVersionID: request.VersionID, SourceFeatureID: stage, Selection: modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: documentID, SourceBodyID: bodyID, Anchor: semanticRef(output.GetSemanticRef()), ExpectedType: kind, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: selectionEvidence(output.GetEvidence())}}
		if err := pick.Selection.Validate(); err != nil {
			return FeatureInput{}, fmt.Errorf("%w: %v", ErrValidation, err)
		}
		result.Selection = &pick
		return result, nil
	}
	result.Artifacts = []Artifact{a}
	for _, tool := range feature.Tools {
		toolKey, err := s.featureStageAtVersion(ctx, documentID, request.VersionID, tool.FeatureID, tool.BodyID)
		if err != nil {
			return FeatureInput{}, err
		}
		toolArtifact, err := s.loadArtifact(ctx, toolKey)
		if err != nil {
			return FeatureInput{}, err
		}
		toolArtifact.BodyID = tool.BodyID
		toolArtifact.DisplayStageFeatureID = tool.FeatureID
		result.Artifacts = append(result.Artifacts, toolArtifact)
	}
	planePick := feature.NeutralPlane
	if feature.Pattern != nil && feature.Pattern.Kind == "MIRROR" {
		planePick = feature.Pattern.MirrorPlane
	}
	if planePick != nil {
		pick := *planePick
		neutralKey := key
		neutralStage := stage
		if pick.Selection.SourceBodyID != feature.BodyID {
			prefix := *view.Part
			prefix.Features = append([]Feature(nil), prefix.Features[:index]...)
			for _, f := range prefix.Features {
				if f.BodyID == pick.Selection.SourceBodyID && isBodyFeature(f.Type) && !f.Suppressed {
					neutralStage = f.ID
				}
			}
			neutralKey, err = s.evaluateBodyPrefix(ctx, "neutral-input/"+request.VersionID+"/"+feature.ID, prefix, pick.Selection.SourceBodyID)
			if err != nil {
				return FeatureInput{}, err
			}
			neutralArtifact, loadErr := s.loadArtifact(ctx, neutralKey)
			if loadErr != nil {
				return FeatureInput{}, loadErr
			}
			neutralArtifact.BodyID = pick.Selection.SourceBodyID
			neutralArtifact.DisplayStageFeatureID = neutralStage
			result.Artifacts = append(result.Artifacts, neutralArtifact)
		}
		resolution, resolveErr := s.resolveFeaturePick(ctx, pick, neutralKey)
		if resolveErr != nil {
			return FeatureInput{}, resolveErr
		}
		if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
			return FeatureInput{}, fmt.Errorf("%w: neutral plane unresolved", ErrValidation)
		}
		result.NeutralPick = &FeatureNeutralPick{DisplayStageFeatureID: neutralStage, Kind: "FACE", LocalID: resolution.Candidates[0].LocalID, BodyID: pick.Selection.SourceBodyID, GeometryKey: neutralKey}
	}
	for i, pick := range feature.Selections {
		resolution, err := s.resolveFeaturePick(ctx, pick, key)
		if err != nil {
			return FeatureInput{}, err
		}
		if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
			return FeatureInput{}, fmt.Errorf("%w: %s: %s", ErrValidation, resolution.Status, resolution.DiagnosticCode)
		}
		result.Picks = append(result.Picks, FeatureInputPick{Index: i, Kind: string(pick.Selection.ExpectedType), LocalID: resolution.Candidates[0].LocalID})
	}
	return result, nil
}
