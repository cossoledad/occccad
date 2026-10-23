package workspace

import (
	"context"
	"fmt"
	"slices"

	"github.com/occccad/occccad/internal/geometry"
)

func validateProfileRegionSelection(feature Feature) error {
	selection := feature.ProfileSelection
	if selection == nil {
		return nil
	}
	if !isSolidGenerator(feature.Type) {
		return fmt.Errorf("%w: profile region selection requires extrude or revolve", ErrValidation)
	}
	switch selection.Mode {
	case "WHOLE_SKETCH":
		if len(selection.RegionIDs) != 0 {
			return fmt.Errorf("%w: whole sketch cannot also specify regions", ErrValidation)
		}
	case "REGIONS":
		if len(selection.RegionIDs) == 0 || len(selection.RegionIDs) > 256 {
			return fmt.Errorf("%w: explicit profile requires 1..256 regions", ErrValidation)
		}
		seen := map[string]bool{}
		for _, id := range selection.RegionIDs {
			if id == "" || seen[id] {
				return fmt.Errorf("%w: invalid or duplicate profile region", ErrValidation)
			}
			seen[id] = true
		}
	default:
		return fmt.Errorf("%w: unknown profile selection mode", ErrValidation)
	}
	return nil
}

// Region identity is the exact Builder's stable outer loop identity. No ordinal,
// point, sampled polygon or geometric similarity participates in resolution.
func selectFeatureProfileRegions(feature Feature, regions []geometry.ProfileRegion) ([]geometry.ProfileRegion, error) {
	if err := validateProfileRegionSelection(feature); err != nil {
		return nil, err
	}
	if feature.ProfileSelection == nil || feature.ProfileSelection.Mode == "WHOLE_SKETCH" {
		return regions, nil
	}
	ids := slices.Clone(feature.ProfileSelection.RegionIDs)
	slices.Sort(ids)
	selected := make([]geometry.ProfileRegion, 0, len(ids))
	for _, id := range ids {
		found := false
		for _, region := range regions {
			if region.ID == id {
				if found {
					return nil, fmt.Errorf("%w: PROFILE_REGION_AMBIGUOUS: %s", ErrValidation, id)
				}
				selected = append(selected, region)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: PROFILE_REGION_MISSING: %s; explicitly reselect the profile region", ErrValidation, id)
		}
	}
	return selected, nil
}

func (s *Service) featureProfileRegions(ctx context.Context, model PartModel, feature Feature, requestID string) (Feature, []geometry.ProfileRegion, error) {
	earlier := map[string]Feature{}
	for _, f := range model.Features {
		if f.ID == feature.ID {
			break
		}
		earlier[f.ID] = f
	}
	sketch, err := resolvePatternSketch(model, earlier, feature.Profile, feature.ProfileMemberSlot)
	if err != nil {
		return Feature{}, nil, err
	}
	regions, err := s.buildExactProfileRegions(ctx, sketch, requestID)
	if err != nil {
		return sketch, nil, err
	}
	selected, err := selectFeatureProfileRegions(feature, regions)
	return sketch, selected, err
}
