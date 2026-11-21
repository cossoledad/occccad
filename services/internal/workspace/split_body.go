package workspace

import (
	"context"
	"fmt"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func splitResultID(parentID string) string     { return commandEntityID("split-result", parentID) }
func splitResultBodyID(parentID string) string { return "body-" + splitResultID(parentID) }

// The sibling is the first immutable stage of its own Body. Both stages read
// the same pre-split stock and the single plane definition on the owner.
func appendSplitResult(model *PartModel, parent Feature) ([]modelcore.ModelChange, error) {
	child := Feature{ID: splitResultID(parent.ID), Type: "SPLIT_RESULT", Name: "分割结果", SplitParentID: parent.ID, Suppressed: parent.Suppressed, Order: 1}
	if bodyIndex(*model, parent.SplitBodyID) >= 0 {
		return nil, fmt.Errorf("%w: duplicate split Body", ErrValidation)
	}
	for _, f := range model.Features {
		if f.ID == child.ID {
			return nil, fmt.Errorf("%w: duplicate split result", ErrValidation)
		}
		child.Order = max(child.Order, f.Order+1)
	}
	created, body := appendOwnedBody(model, child.ID)
	child.BodyID = body.ID
	model.Features = append(model.Features, child)
	stage, err := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: child.ID, SlotID: "entity"}, nil, featureHistoryDefinition(child))
	return []modelcore.ModelChange{created, stage}, err
}

func (s *Service) splitBodyInput(ctx context.Context, reqID string, model PartModel, feature Feature) (geometry.ProfilePad, error) {
	spec := geometry.ProfilePad{FeatureID: feature.ID, BodyID: feature.BodyID, Generator: feature.Type}
	parent := feature
	if feature.Type == "SPLIT_RESULT" {
		for _, f := range model.Features {
			if f.ID == feature.SplitParentID {
				parent = f
				break
			}
		}
		if parent.Type != "SPLIT_BODY" || parent.SplitBodyID != feature.BodyID || parent.Suppressed {
			return spec, fmt.Errorf("%w: SPLIT_PARENT_INVALID", ErrValidation)
		}
		var stock string
		for _, f := range model.Features {
			if f.ID == parent.ID {
				break
			}
			if f.BodyID == parent.BodyID && isBodyFeature(f.Type) && !f.Suppressed {
				stock = f.ID
			}
		}
		if stock == "" {
			return spec, fmt.Errorf("%w: SPLIT_REQUIRES_SINGLE_SOLID", ErrValidation)
		}
		tools, err := s.booleanToolInputs(ctx, reqID, model, Feature{Tools: []FeatureStageRef{{BodyID: parent.BodyID, FeatureID: stock}}})
		if err != nil {
			return spec, err
		}
		spec.Tools = tools
	}
	origin, normal, err := s.resolveReferencedPlane(ctx, reqID, model, parent.ID, parent.NeutralPlaneID, parent.NeutralPlane)
	spec.NeutralOrigin, spec.NeutralNormal = origin, normal
	spec.Reversed = (parent.SplitKeepSide == "NEGATIVE") != (feature.Type == "SPLIT_RESULT")
	return spec, err
}
