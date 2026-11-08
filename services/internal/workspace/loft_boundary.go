package workspace

import (
	"context"
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

type LoftBoundaryDefinition struct {
	Condition         string             `json:"condition"` // NORMAL | DIRECTION | G1 | G2; nil is free
	DirectionEntityID string             `json:"directionEntityId,omitempty"`
	Reversed          bool               `json:"reversed,omitempty"`
	Magnitude         float64            `json:"magnitude"` // dimensionless tangent influence
	Supports          []FeatureSelection `json:"supports,omitempty"`
}

func loftBoundaryPicks(f Feature) []FeatureSelection {
	var out []FeatureSelection
	for _, b := range []*LoftBoundaryDefinition{f.LoftStart, f.LoftEnd} {
		if b != nil {
			out = append(out, b.Supports...)
		}
	}
	return out
}
func validateLoftBoundaries(f Feature, earlier map[string]Feature) error {
	if f.Type != "LOFT" {
		if f.LoftStart != nil || f.LoftEnd != nil {
			return fmt.Errorf("%w: LOFT_BOUNDARY_REQUIRES_LOFT", ErrValidation)
		}
		return nil
	}
	for _, b := range []*LoftBoundaryDefinition{f.LoftStart, f.LoftEnd} {
		if b == nil {
			continue
		}
		if f.Ruled || (b.Condition != "NORMAL" && b.Condition != "DIRECTION" && b.Condition != "G1" && b.Condition != "G2") || !positiveFinite(b.Magnitude) || b.Magnitude > 10 || b.Magnitude < .01 {
			return fmt.Errorf("%w: LOFT_BOUNDARY_DEFINITION_INVALID", ErrValidation)
		}
		if (b.Condition == "G1" || b.Condition == "G2") && (len(b.Supports) == 0 || len(b.Supports) > 128) {
			return fmt.Errorf("%w: LOFT_CONTINUITY_SUPPORT_REQUIRED", ErrValidation)
		}
		if (b.Condition == "NORMAL" || b.Condition == "DIRECTION") && len(b.Supports) > 0 {
			return fmt.Errorf("%w: LOFT_FREE_DIRECTION_HAS_NO_SUPPORT", ErrValidation)
		}
		if b.Condition == "DIRECTION" && b.DirectionEntityID == "" {
			return fmt.Errorf("%w: LOFT_DIRECTION_REQUIRED", ErrValidation)
		}
		for _, p := range b.Supports {
			if err := p.Selection.Validate(); err != nil {
				return fmt.Errorf("%w: %v", ErrValidation, err)
			}
			a, ok := earlier[p.Selection.Anchor.FeatureID]
			s, stage := earlier[p.SourceFeatureID]
			if !ok || !stage || a.BodyID != p.Selection.SourceBodyID || s.BodyID != a.BodyID || p.Selection.ExpectedType != modelcore.PersistentTopologyFace || p.SourceVersionID == "" {
				return fmt.Errorf("%w: LOFT_SUPPORT_REQUIRES_EARLIER_FACE", ErrValidation)
			}
		}
	}
	return nil
}
func (s *Service) qualifyLoftBoundaries(ctx context.Context, documentID string, f *Feature) error {
	for _, b := range []*LoftBoundaryDefinition{f.LoftStart, f.LoftEnd} {
		if b == nil {
			continue
		}
		for i, p := range b.Supports {
			copy := *f
			copy.TerminationSurface = &p
			copy.TerminationPlane = nil
			if err := s.qualifyTerminationPlane(ctx, documentID, &copy); err != nil {
				return err
			}
			b.Supports[i] = *copy.TerminationSurface
		}
	}
	return nil
}
func (s *Service) loftBoundaryInput(ctx context.Context, requestID string, model PartModel, f Feature, b *LoftBoundaryDefinition) (*geometry.LoftBoundary, []geometry.BodyToolInput, error) {
	if b == nil {
		return nil, nil, nil
	}
	out := &geometry.LoftBoundary{Condition: b.Condition, Magnitude: b.Magnitude, Reversed: b.Reversed}
	if b.DirectionEntityID != "" {
		p, err := resolvePatternAxisDefinition(model, PatternDefinition{AxisEntityID: b.DirectionEntityID})
		if err != nil {
			return nil, nil, fmt.Errorf("%w: LOFT_DIRECTION_UNAVAILABLE: %v", ErrValidation, err)
		}
		out.Direction = p.Direction
	}
	var tools []geometry.BodyToolInput
	for _, p := range b.Supports {
		copy := f
		copy.TerminationSurface = &p
		ref, source, err := s.resolveTerminationSurface(ctx, requestID, model, copy)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: LOFT_SUPPORT_UNRESOLVED: %v", ErrValidation, err)
		}
		out.Supports = append(out.Supports, *ref)
		tools = append(tools, source...)
	}
	return out, tools, nil
}
func loftBoundaryParameters(f Feature) []holeParameter {
	var out []holeParameter
	for i, b := range []*LoftBoundaryDefinition{f.LoftStart, f.LoftEnd} {
		if b != nil {
			slot := "loft:startMagnitude"
			if i == 1 {
				slot = "loft:endMagnitude"
			}
			out = append(out, holeParameter{slot, "Loft tangent magnitude", "1", b.Magnitude, modelcore.Dimensionless})
		}
	}
	return out
}
