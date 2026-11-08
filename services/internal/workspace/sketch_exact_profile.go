package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/occccad/occccad/internal/geometry"
)

func (service *Service) buildExactProfileRegions(ctx context.Context, feature Feature, requestID string) ([]geometry.ProfileRegion, error) {
	loops, err := buildProfileLoops(feature, true)
	if err != nil {
		return nil, err
	}
	return service.classifyExactProfileLoops(ctx, feature.ID, loops, requestID)
}

func (service *Service) buildAvailableProfileRegions(ctx context.Context, feature Feature, requestID string) ([]geometry.ProfileRegion, error) {
	loops, err := buildAvailableProfileLoops(feature, true)
	if err != nil {
		return nil, err
	}
	return service.classifyExactProfileLoops(ctx, feature.ID, loops, requestID)
}

func (service *Service) classifyExactProfileLoops(ctx context.Context, featureID string, loops []profileLoop, requestID string) ([]geometry.ProfileRegion, error) {
	if len(loops) == 0 {
		return []geometry.ProfileRegion{}, nil
	}
	input := []geometry.ProfileLoop{}
	for _, loop := range loops {
		for _, curve := range loop.value.Curves {
			if curve.Kind == "SPLINE" && len(curve.Poles) == 0 {
				return nil, fmt.Errorf("%w: spline has no accepted exact canonical curve", ErrValidation)
			}
		}
		input = append(input, loop.value)
	}
	if service.worker == nil {
		return nil, fmt.Errorf("%w: exact profile worker unavailable", ErrValidation)
	}
	regions, err := preparedResult(ctx, &service.prepared, "profile:"+featureID, input, func() ([]geometry.ProfileRegion, error) {
		return service.worker.ClassifySketchProfile(ctx, requestID, input)
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	for i := range regions {
		digest := sha256.Sum256([]byte(regions[i].Outer.ID))
		regions[i].ID = "profile-region:" + hex.EncodeToString(digest[:12])
		sort.Slice(regions[i].Holes, func(a, b int) bool { return regions[i].Holes[a].ID < regions[i].Holes[b].ID })
	}
	sort.Slice(regions, func(a, b int) bool { return regions[a].ID < regions[b].ID })
	return regions, nil
}

func (service *Service) projectExactSketchAnalyses(ctx context.Context, model PartModel) map[string]SketchProfileAnalysis {
	output := projectSketchAnalyses(model)
	for _, feature := range model.Features {
		if feature.Sketch == nil || feature.Suppressed || feature.Sketch.Support.Status == "FAILED_SUPPORT" || feature.Sketch.Support.Status == "BLOCKED" {
			continue
		}
		analysis := output[feature.ID]
		// Connectivity diagnostics are model facts. Curved region validation is
		// performed on the same accepted curves and same kernel as the solid gate.
		if analysis.Status == "EMPTY" {
			continue
		}
		issues := []SketchProfileIssue{}
		for _, issue := range analysis.Issues {
			if issue.Code != "INVALID_REGION" {
				issues = append(issues, issue)
			}
		}
		regions, err := service.buildAvailableProfileRegions(ctx, feature, "profile-analysis/"+feature.ID)
		analysis.GeometryVerified = service.worker != nil && len(regions) > 0
		analysis.RegionCount = 0
		analysis.LoopCount = 0
		analysis.RegionViews = nil
		analysis.RegionIDs = nil
		analysis.Regions = nil
		if err != nil {
			analysis.Status = "INVALID"
			analysis.Issues = append(issues, SketchProfileIssue{Code: "INVALID_REGION", Message: err.Error(), EntityIDs: []string{}})
		} else {
			if len(issues) == 0 {
				analysis.Status = "CLOSED"
			}
			analysis.Issues = issues
			analysis.Regions = regions
			analysis.RegionCount = len(regions)
			for _, r := range regions {
				display := SketchProfileRegionView{ID: r.ID, Outer: sampleProfileLoop(r.Outer)}
				for _, hole := range r.Holes {
					display.Holes = append(display.Holes, sampleProfileLoop(hole))
				}
				analysis.RegionViews = append(analysis.RegionViews, display)
				analysis.RegionIDs = append(analysis.RegionIDs, r.ID)
				analysis.LoopCount += 1 + len(r.Holes)
			}
		}
		output[feature.ID] = analysis
	}
	return output
}
