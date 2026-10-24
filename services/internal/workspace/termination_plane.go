package workspace

import (
	"context"
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
)

// Qualify creation picks with the actual immutable Body tip. A face's semantic
// anchor can originate much earlier and must never stand in for that stage.
func (s *Service) qualifyTerminationPlane(ctx context.Context, documentID string, f *Feature) error {
	p := f.TerminationPlane
	if p == nil {
		return nil
	}
	if p.Selection.SourceDocumentID != documentID {
		return fmt.Errorf("%w: termination plane must belong to the same Part", ErrValidation)
	}
	if p.SourceFeatureID != "" {
		return nil
	}
	key, err := s.featureStageAtVersion(ctx, documentID, p.SourceVersionID, "", p.Selection.SourceBodyID)
	if err != nil {
		return err
	}
	manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
	if err != nil {
		return err
	}
	tip := manifest.Tips[p.Selection.SourceBodyID]
	if !topologyHistoryComplete(manifest) || tip == nil {
		return fmt.Errorf("%w: termination source stage unavailable", ErrValidation)
	}
	copy := *p
	copy.SourceFeatureID = tip.FeatureId
	f.TerminationPlane = &copy
	return nil
}

// Neutral, mirror and termination planes share the same input-stage resolver.
// Only reference outcomes become row states; stage/service errors stay errors.
func (s *Service) featurePlaneInput(ctx context.Context, r FeatureInputRequest, model PartModel, index int, key, stage, bodyID, id string, pick *FeatureSelection, resolve bool) (*FeatureNeutralPick, []Artifact, error) {
	if pick == nil {
		state := FeatureInputResolution{Status: modelcore.SelectionMissing, DiagnosticCode: "FEATURE_INPUT_DATUM_MISSING", Diagnostic: "referenced datum plane is missing"}
		for _, p := range model.DatumPlanes {
			if p.ID == id {
				state = FeatureInputResolution{Status: modelcore.SelectionResolved}
				break
			}
		}
		return &FeatureNeutralPick{FeatureInputResolution: state, Kind: "PLANE"}, nil, nil
	}
	prefix := model
	prefix.Features = append([]Feature(nil), model.Features[:index]...)
	sourceBody := pick.Selection.SourceBodyID
	if sourceBody != bodyID {
		stage = ""
		for _, f := range prefix.Features {
			if f.BodyID == sourceBody && isBodyFeature(f.Type) && !f.Suppressed {
				stage = f.ID
			}
		}
	}
	state := &FeatureNeutralPick{Kind: "FACE", BodyID: sourceBody, DisplayStageFeatureID: stage}
	if stage == "" {
		state.FeatureInputResolution = FeatureInputResolution{Status: modelcore.SelectionMissing, DiagnosticCode: "FEATURE_INPUT_SOURCE_BODY_EMPTY", Diagnostic: "referenced Body has no solid before this feature"}
		return state, nil, nil
	}
	var artifacts []Artifact
	if sourceBody != bodyID {
		var err error
		key, err = s.evaluateBodyPrefix(ctx, "plane-input/"+r.VersionID+"/"+model.Features[index].ID, prefix, sourceBody)
		if err != nil {
			return nil, nil, featureInputStageError(prefix, stage, err)
		}
		a, err := s.loadArtifact(ctx, key)
		if err != nil {
			return nil, nil, err
		}
		a.BodyID = sourceBody
		a.DisplayStageFeatureID = stage
		artifacts = append(artifacts, a)
	}
	state.GeometryKey = key
	if !resolve {
		return state, artifacts, nil
	}
	resolution, err := s.resolveFeaturePick(ctx, *pick, key)
	if err != nil {
		return nil, nil, err
	}
	input, err := featureInputResolution(resolution, "PLANE")
	if err != nil {
		return nil, nil, err
	}
	state.FeatureInputResolution = input
	if input.Status == modelcore.SelectionResolved {
		state.LocalID = resolution.Candidates[0].LocalID
	}
	return state, artifacts, nil
}
