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
	regions, err := preparedResult(ctx, &service.prepared, "profile:"+feature.ID, input, func() ([]geometry.ProfileRegion, error) {
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
		if feature.Sketch == nil {
			continue
		}
		analysis := output[feature.ID]
		// Connectivity diagnostics are model facts. Curved region validation is
		// performed on the same accepted curves and same kernel as the solid gate.
		if analysis.Status == "EMPTY" || analysis.Status == "OPEN" {
			continue
		}
		hasTopologyIssue := false
		for _, issue := range analysis.Issues {
			if issue.Code != "INVALID_REGION" {
				hasTopologyIssue = true
			}
		}
		if hasTopologyIssue {
			continue
		}
		regions, err := service.buildExactProfileRegions(ctx, feature, "profile-analysis/"+feature.ID)
		analysis.GeometryVerified = service.worker != nil
		analysis.RegionCount = 0
		analysis.LoopCount = 0
		analysis.RegionIDs = nil
		analysis.Regions = nil
		if err != nil {
			analysis.Status = "INVALID"
			analysis.Issues = []SketchProfileIssue{{Code: "INVALID_REGION", Message: err.Error(), EntityIDs: []string{}}}
		} else {
			analysis.Status = "CLOSED"
			analysis.Issues = []SketchProfileIssue{}
			analysis.Regions = regions
			analysis.RegionCount = len(regions)
			for _, r := range regions {
				analysis.RegionIDs = append(analysis.RegionIDs, r.ID)
				analysis.LoopCount += 1 + len(r.Holes)
			}
		}
		output[feature.ID] = analysis
	}
	return output
}
