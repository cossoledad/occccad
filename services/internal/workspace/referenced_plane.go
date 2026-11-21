package workspace

import (
	"context"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

// Draft, mirror and associative extrusion share one upstream plane resolver.
func validateReferencedPlane(id string, pick *FeatureSelection, earlier map[string]Feature) error {
	if (id == "") == (pick == nil) {
		return fmt.Errorf("%w: one referenced plane required", ErrValidation)
	}
	if pick == nil {
		return nil
	}
	anchor, exists := earlier[pick.Selection.Anchor.FeatureID]
	if err := pick.Selection.Validate(); err != nil {
		return fmt.Errorf("%w: invalid plane: %v", ErrValidation, err)
	}
	if !exists || anchor.BodyID != pick.Selection.SourceBodyID || pick.SourceVersionID == "" || pick.Selection.ExpectedType != modelcore.PersistentTopologyFace || pick.Selection.CreationEvidence.GeometryType != "PLANE" {
		return fmt.Errorf("%w: plane must be an upstream planar face", ErrValidation)
	}
	if pick.SourceFeatureID != "" {
		stage, ok := earlier[pick.SourceFeatureID]
		if !ok || stage.BodyID != anchor.BodyID || !isBodyFeature(stage.Type) {
			return fmt.Errorf("%w: invalid plane source stage", ErrValidation)
		}
	}
	return nil
}

func (s *Service) resolveReferencedPlane(ctx context.Context, requestID string, model PartModel, featureID, id string, pick *FeatureSelection) ([3]float64, [3]float64, error) {
	if pick == nil {
		for _, p := range model.DatumPlanes {
			if p.ID == id {
				return p.Origin, p.Normal, nil
			}
		}
		return [3]float64{}, [3]float64{}, fmt.Errorf("%w: referenced datum plane missing", ErrValidation)
	}
	prefix := model
	found := false
	for i, f := range model.Features {
		if f.ID == featureID {
			found = true
			prefix.Features = model.Features[:i]
			break
		}
	}
	if !found {
		return [3]float64{}, [3]float64{}, fmt.Errorf("%w: plane consumer missing", ErrValidation)
	}
	key, err := s.evaluateBodyPrefix(ctx, requestID+"/plane/"+featureID, prefix, pick.Selection.SourceBodyID)
	if err != nil {
		return [3]float64{}, [3]float64{}, err
	}
	resolution, err := s.resolveFeaturePickForUse(ctx, *pick, key, analyticSupport)
	if err != nil {
		return [3]float64{}, [3]float64{}, err
	}
	origin, normal, ok := resolvedPlaneFrame(resolution, pick.Selection.CreationEvidence.Origin)
	if !ok {
		return [3]float64{}, [3]float64{}, fmt.Errorf("%w: REFERENCED_PLANE_UNRESOLVED: %s", ErrValidation, resolution.DiagnosticCode)
	}
	return origin, normal, nil
}

func (s *Service) resolvedSolidPatternDefinition(ctx context.Context, requestID string, model PartModel, feature Feature) (PatternDefinition, error) {
	p := feature.Pattern.PatternDefinition
	if p.Kind != "MIRROR" {
		resolved, err := resolvedPatternDefinition(model, p)
		if err != nil {
			return p, err
		}
		if input := feature.Pattern.AxisReference; input != nil {
			frame, err := s.solidAxisFrame(ctx, requestID, model, *input)
			if err != nil {
				return p, err
			}
			resolved.Direction = frame.direction
			if p.CenterReference == nil {
				resolved.Origin = frame.origin
			}
		}
		if input := feature.Pattern.CenterGeometry; input != nil {
			resolved.Origin, _, err = s.geometryPointPosition(ctx, requestID, model, *input)
		}
		return resolved, err
	}
	var err error
	p.Origin, p.Direction, err = s.resolveReferencedPlane(ctx, requestID, model, feature.ID, p.MirrorPlaneID, p.MirrorPlane)
	return p, err
}
