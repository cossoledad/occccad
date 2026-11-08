package workspace

import (
	"context"
	"fmt"
	"math"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func associatedExtrudeExtent(extent string) bool {
	return extent == "TO_PLANE" || extent == "TO_NEXT" || extent == "TO_SURFACE"
}
func linearExtrudeFeature(f Feature) bool { return f.Type == "PAD" || f.Type == "LINEAR_EXTRUDE" }

type extrudeParameter struct {
	slot, label string
	value       float64
}

func extrudeParameters(f Feature) []extrudeParameter {
	if !linearExtrudeFeature(f) || f.Extent == "THROUGH_ALL" {
		return nil
	}
	out := []extrudeParameter{{"startOffset", "Start offset", f.StartOffset}}
	if associatedExtrudeExtent(f.Extent) {
		out = append(out, extrudeParameter{"endOffset", "End offset", f.EndOffset})
	}
	return out
}
func validateExtrudeTermination(f Feature, earlier map[string]Feature) error {
	if math.IsNaN(f.StartOffset) || math.IsInf(f.StartOffset, 0) || math.IsNaN(f.EndOffset) || math.IsInf(f.EndOffset, 0) {
		return fmt.Errorf("%w: EXTRUDE_OFFSET_INVALID", ErrValidation)
	}
	if (f.StartOffset != 0 || f.EndOffset != 0) && (!linearExtrudeFeature(f) || f.Extent == "THROUGH_ALL") {
		return fmt.Errorf("%w: EXTRUDE_OFFSET_UNSUPPORTED", ErrValidation)
	}
	if f.EndOffset != 0 && !associatedExtrudeExtent(f.Extent) {
		return fmt.Errorf("%w: END_OFFSET_REQUIRES_ASSOCIATED_TERMINATION", ErrValidation)
	}
	if f.Extent == "TO_SURFACE" {
		p := f.TerminationSurface
		if p == nil {
			return fmt.Errorf("%w: TO_SURFACE_TARGET_REQUIRED", ErrValidation)
		}
		if err := p.Selection.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrValidation, err)
		}
		anchor, ok := earlier[p.Selection.Anchor.FeatureID]
		stage, stageOK := earlier[p.SourceFeatureID]
		if !ok || !stageOK || !isBodyFeature(stage.Type) || anchor.BodyID != p.Selection.SourceBodyID || stage.BodyID != anchor.BodyID || p.SourceVersionID == "" || p.Selection.ExpectedType != modelcore.PersistentTopologyFace {
			return fmt.Errorf("%w: TO_SURFACE_REQUIRES_UPSTREAM_FACE", ErrValidation)
		}
	} else if f.TerminationSurface != nil {
		return fmt.Errorf("%w: TERMINATION_SURFACE_REQUIRES_TO_SURFACE", ErrValidation)
	}
	if f.Extent == "TO_NEXT" {
		if f.Operation == "NEW_BODY" {
			return fmt.Errorf("%w: TO_NEXT_REQUIRES_TARGET_BODY", ErrValidation)
		}
		found := false
		for _, prior := range earlier {
			if prior.BodyID == f.BodyID && isBodyFeature(prior.Type) && !prior.Suppressed {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: TO_NEXT_REQUIRES_TARGET_BODY", ErrValidation)
		}
	}
	return nil
}

// Resolve against the consumer's current upstream prefix, retaining the original
// immutable selection witness. Transport the exact BREP/Naming, never sampled
// evidence or a mesh approximation of the selected surface.
func (s *Service) resolveTerminationSurface(ctx context.Context, requestID string, model PartModel, feature Feature) (*modelcore.SemanticTopologyRef, []geometry.BodyToolInput, error) {
	p := feature.TerminationSurface
	if p == nil {
		return nil, nil, fmt.Errorf("%w: TO_SURFACE_TARGET_REQUIRED", ErrValidation)
	}
	prefix := model
	found := false
	for i, f := range model.Features {
		if f.ID == feature.ID {
			prefix.Features = model.Features[:i]
			found = true
			break
		}
	}
	if !found {
		return nil, nil, fmt.Errorf("%w: termination consumer missing", ErrValidation)
	}
	key, err := s.evaluateBodyPrefix(ctx, requestID+"/surface/"+feature.ID, prefix, p.Selection.SourceBodyID)
	if err != nil {
		return nil, nil, err
	}
	resolution, err := s.resolveFeaturePick(ctx, *p, key)
	if err != nil {
		return nil, nil, err
	}
	if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
		return nil, nil, fmt.Errorf("%w: TERMINATION_SURFACE_UNRESOLVED: %s", ErrValidation, resolution.DiagnosticCode)
	}
	manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	if !topologyHistoryComplete(manifest) || manifest.Tips[p.Selection.SourceBodyID] == nil {
		return nil, nil, fmt.Errorf("%w: termination surface history incomplete", ErrValidation)
	}
	brep, err := s.brepArtifactReference(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	naming, err := s.representationObject(ctx, key, "NAMING")
	if err != nil {
		return nil, nil, err
	}
	ref := resolution.Candidates[0].SemanticRef
	return &ref, []geometry.BodyToolInput{{BodyID: p.Selection.SourceBodyID, FeatureID: manifest.Tips[p.Selection.SourceBodyID].FeatureId, BRep: brep, Naming: geometry.ArtifactReference{Backend: naming.Backend, ObjectKey: naming.Key, SHA256: naming.SHA256, Size: naming.Size, ContentType: naming.ContentType}}}, nil
}
