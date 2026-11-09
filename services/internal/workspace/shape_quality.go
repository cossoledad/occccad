package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"time"
)

type ShapeQualityRequest struct {
	RevisionID  string         `json:"revisionId"`
	Input       GeometricInput `json:"input"`
	SampleCount uint32         `json:"sampleCount"`
}
type ShapeQualityReport struct {
	DocumentID string                   `json:"documentId"`
	RevisionID string                   `json:"revisionId"`
	Input      GeometricInput           `json:"input"`
	Result     geometry.CurvatureResult `json:"result"`
}

// Read only: resolve stable input against one frozen Revision. No history
// entry, sampled geometry persistence or mutable Head-derived operand.
func (s *Service) AnalyzeShapeQuality(ctx context.Context, doc string, r ShapeQualityRequest) (ShapeQualityReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	bad := func(code string) (ShapeQualityReport, error) {
		return ShapeQualityReport{}, fmt.Errorf("%w: %s", ErrValidation, code)
	}
	if r.SampleCount < 16 || r.SampleCount > 4096 {
		return bad("CURVATURE_SAMPLE_COUNT_INVALID")
	}
	var head string
	if err := s.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, doc).Scan(&head); err != nil {
		return ShapeQualityReport{}, err
	}
	if head != r.RevisionID {
		return bad("CURVATURE_STALE_REVISION")
	}
	kind, raw, err := s.measurementModel(ctx, doc, r.RevisionID)
	if err != nil {
		return ShapeQualityReport{}, err
	}
	if kind != "PART" {
		return bad("CURVATURE_PART_REQUIRED")
	}
	var part PartModel
	if err = json.Unmarshal(raw, &part); err != nil {
		return ShapeQualityReport{}, err
	}
	earlier := map[string]Feature{}
	for _, f := range part.Features {
		earlier[f.ID] = f
	}
	if r.Input.Kind != "RESULT" && r.Input.Kind != "TOPOLOGY" {
		return bad("CURVATURE_EXACT_GEOMETRY_REQUIRED")
	}
	if err = validateGeometryReference(r.Input, earlier, part); err != nil {
		return ShapeQualityReport{}, err
	}
	var key string
	element := geometry.MeasurementElement{InputID: "source", Kind: "BODY", Mode: "GEOMETRY"}
	if r.Input.Kind == "RESULT" {
		_, key, _, err = s.resultManifestForVersion(ctx, doc, r.RevisionID, &modelcore.GeometryResultReference{FeatureID: r.Input.FeatureID, ResultID: r.Input.ResultID})
	} else {
		if r.Input.Selection == nil || r.Input.Selection.Selection.SourceDocumentID != doc {
			return bad("CURVATURE_REFERENCE_INVALID")
		}
		_, key, _, err = s.selectionManifestForVersion(ctx, doc, r.RevisionID, r.Input.Selection.Selection)
		if err == nil {
			var resolution modelcore.SelectionResolution
			resolution, err = s.resolveFeaturePick(ctx, *r.Input.Selection, key)
			if err == nil {
				if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
					return bad("CURVATURE_REFERENCE_UNRESOLVED")
				}
				c := resolution.Candidates[0]
				element.Kind = string(c.Type)
				element.LocalID = c.LocalID
			}
		}
	}
	if err != nil {
		return ShapeQualityReport{}, err
	}
	if element.Kind == "VERTEX" {
		return bad("CURVATURE_CURVE_OR_SURFACE_REQUIRED")
	}
	input := geometry.AnalysisGeometry{ID: "source", Pose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}}
	if err = s.database.QueryRow(ctx, `SELECT geometry_id FROM occccad.geometry_artifacts WHERE geometry_key=$1`, key).Scan(&input.GeometryID); err != nil {
		return ShapeQualityReport{}, err
	}
	input.BRep, err = s.brepArtifactReference(ctx, key)
	if err != nil {
		return ShapeQualityReport{}, err
	}
	result, err := s.worker.AnalyzeCurvature(ctx, newID("curvature"), []geometry.AnalysisGeometry{input}, [][]geometry.MeasurementElement{{element}}, r.SampleCount)
	if err != nil {
		return ShapeQualityReport{}, err
	}
	if err = s.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, doc).Scan(&head); err != nil {
		return ShapeQualityReport{}, err
	}
	if head != r.RevisionID {
		return bad("CURVATURE_STALE_REVISION")
	}
	return ShapeQualityReport{doc, r.RevisionID, r.Input, result}, nil
}
