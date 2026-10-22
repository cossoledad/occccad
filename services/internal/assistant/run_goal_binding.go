package assistant

import (
	"context"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

func (s *Service) goalReceiptCandidate(ctx context.Context, r Run, req GoalRequirement, part workspace.PartModel, feature workspace.Feature, revision string, stageVersion uint64) bool {
	if req.Coverage == "UNRESOLVED" || req.IntroducedGoalVersion > stageVersion {
		return false
	}
	if bound := r.Targets[goalTargetKey(req.ID)]; bound != "" && bound != feature.ID {
		return false
	}
	if !automation.PartGoalTarget(part, feature, req.Kind) {
		return false
	}
	check := automation.PartCheck{ID: req.ID, Kind: req.Kind, FeatureID: feature.ID, BodyID: feature.BodyID, Width: req.Width, Height: req.Height, Length: req.Length, Radius: req.Radius, RadiusPolicy: req.RadiusPolicy, Plane: req.Plane, Unit: "mm"}
	if req.Kind == "RECTANGLE_DIMENSIONS" {
		check.SketchID = feature.ID
		check.FeatureID = ""
	}
	if req.Kind == "CENTERED_THROUGH_HOLE" || req.Kind == "EXTRUSION_LENGTH" {
		for _, base := range r.Requirements {
			if base.Kind == "RECTANGLE_DIMENSIONS" && base.BodyID == req.BodyID {
				if req.Kind == "EXTRUSION_LENGTH" {
					check.SketchID = r.Targets[goalTargetKey(base.ID)]
				} else {
					check.ReferenceSketchID = r.Targets[goalTargetKey(base.ID)]
				}
				break
			}
		}
	}
	if req.Kind == "SELECTED_EDGES_FILLET" {
		applied, e := s.selectionSetForFeature(ctx, r, feature)
		if e != nil {
			return false
		}
		token, count, e := s.requiredSelectedEdges(ctx, r, req, applied)
		if e != nil {
			return false
		}
		check.SelectionSet, check.ExpectedCount = token, count
	}
	if req.Kind == "HOLE_RIM_FILLET" {
		check.ReferenceFeatureID = r.Targets[goalTargetKey(req.ReferenceRequirementID)]
	}
	// Candidate classification cannot let an old/global/local fillet lend its
	// receipt to a different target. Use the existing acceptance implementation.
	verified, e := s.automation.VerifyPart(ctx, r.ActorID, r.DocumentID, automation.PartVerificationRequest{RevisionID: revision, GoalVersion: r.GoalVersion, Checks: []automation.PartCheck{check}})
	return e == nil && verified.Passed
}
