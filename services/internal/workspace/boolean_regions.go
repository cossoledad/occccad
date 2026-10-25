package workspace

import (
	"context"
	"fmt"

	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/modelcore"
)

// Analysis reconstructs only legal exact inputs, independently of the Boolean
// result or its saved region references. No candidate is a bindable Body stage.
type BooleanRegionsRequest struct {
	VersionID string          `json:"versionId"`
	FeatureID string          `json:"featureId,omitempty"`
	BodyID    string          `json:"bodyId"`
	Tool      FeatureStageRef `json:"tool"`
	Operation string          `json:"operation"`
}
type BooleanRegionCell struct {
	Reference  modelcore.BooleanRegionRef `json:"reference"`
	InTarget   bool                       `json:"inTarget"`
	InTool     bool                       `json:"inTool"`
	Eligible   bool                       `json:"eligible"`
	Operations []string                   `json:"operations"`
	Artifact   Artifact                   `json:"artifact"`
}
type BooleanRegionsAnalysis struct {
	VersionID string              `json:"versionId"`
	Regions   []BooleanRegionCell `json:"regions"`
	Inputs    []Artifact          `json:"inputs"`
}

func (s *Service) AnalyzeBooleanRegions(ctx context.Context, documentID string, request BooleanRegionsRequest) (BooleanRegionsAnalysis, error) {
	result := BooleanRegionsAnalysis{VersionID: request.VersionID, Regions: []BooleanRegionCell{}}
	view, err := s.GetDocument(ctx, documentID)
	if err != nil {
		return result, err
	}
	if view.Part == nil || view.Document.VersionID != request.VersionID {
		return result, fmt.Errorf("%w: Boolean analysis revision changed", ErrValidation)
	}
	model := *view.Part
	index := len(model.Features)
	if request.FeatureID != "" {
		index = -1
		for i, f := range model.Features {
			if f.ID == request.FeatureID && f.Type == "BOOLEAN" {
				index = i
				break
			}
		}
		if index < 0 {
			return result, fmt.Errorf("%w: Boolean feature missing", ErrValidation)
		}
	}
	earlier := map[string]Feature{}
	stage := ""
	for _, f := range model.Features[:index] {
		earlier[f.ID] = f
		if f.BodyID == request.BodyID && isBodyFeature(f.Type) && !f.Suppressed {
			stage = f.ID
		}
	}
	f := Feature{Type: "BOOLEAN", BodyID: request.BodyID, Operation: request.Operation, Tools: []FeatureStageRef{request.Tool}}
	if err := validateSolidStage(f, earlier); err != nil {
		return result, err
	}
	if stage == "" {
		return result, fmt.Errorf("%w: Boolean target input unavailable", ErrValidation)
	}
	refs := []FeatureStageRef{{BodyID: request.BodyID, FeatureID: stage}, request.Tool}
	// booleanToolInputs validates the paired immutable BREP/Naming snapshots.
	inputs, err := s.booleanToolInputs(ctx, "boolean-analysis/"+request.VersionID, model, Feature{Tools: refs})
	if err != nil {
		return result, err
	}
	for _, ref := range refs {
		key, e := s.featureStageAtVersion(ctx, documentID, request.VersionID, ref.FeatureID, ref.BodyID, true)
		if e != nil {
			return result, e
		}
		a, e := s.loadArtifact(ctx, key)
		if e != nil {
			return result, e
		}
		a.BodyID = ref.BodyID
		a.DisplayStageFeatureID = ref.FeatureID
		result.Inputs = append(result.Inputs, a)
	}
	attempt := newID("boolean-analysis")
	response, err := s.worker.AnalyzeBooleanRegions(ctx, attempt, artifactstore.StagingKey(attempt, "cells"), inputs[0], inputs[1])
	if err != nil {
		return result, err
	}
	for _, cell := range response.Regions {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		e := cell.Geometry
		key := "boolean-cell:" + e.GeometryId
		// Analysis geometry deliberately has no bindable naming manifest. The final
		// assembly goes through ordinary Feature evaluation and full naming gates.
		if _, err := s.loadArtifact(ctx, key); err != nil {
			if err := s.storeEvaluation(ctx, key, e, VisualizationManifest{}); err != nil {
				return result, err
			}
		}
		a, err := s.loadArtifact(ctx, key)
		if err != nil {
			return result, err
		}
		a.RepresentationKind = "TRANSIENT_PREVIEW"
		operations := []string{"ADD"}
		if cell.InTarget && !cell.InTool {
			operations = append(operations, "REMOVE")
		}
		if cell.InTarget && cell.InTool {
			operations = append(operations, "INTERSECT")
		}
		eligible := request.Operation == "ADD" && (cell.InTarget || cell.InTool) || request.Operation == "REMOVE" && cell.InTarget && !cell.InTool || request.Operation == "INTERSECT" && cell.InTarget && cell.InTool
		result.Regions = append(result.Regions, BooleanRegionCell{Reference: modelcore.BooleanRegionRef{BoundaryUnique: cell.Reference.BoundaryUnique, Boundary: cell.Reference.Boundary, InputDigest: cell.Reference.InputDigest, CellDigest: cell.Reference.CellDigest}, InTarget: cell.InTarget, InTool: cell.InTool, Eligible: eligible, Operations: operations, Artifact: a})
	}
	return result, nil
}
