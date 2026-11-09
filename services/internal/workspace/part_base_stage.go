package workspace

import (
	"context"
	"fmt"
	"math"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
)

// Restore the explicitly owned import transition from the accepted Body's
// immutable Naming artifact. Later Body transitions do not alter that frozen
// root. The input mass comes from the exact import artifact, not the Body tip.
func (s *Service) importBaseStage(ctx context.Context, baseKey string, base geometry.ArtifactReference, seed *workerv1.ImportTopologySeed, model PartModel) (*workerv1.PartBaseStage, error) {
	if seed == nil || geometry.PartRuntime(ctx).ForceCold || s.artifacts == nil {
		return nil, nil
	}
	input, err := s.loadArtifact(ctx, baseKey)
	if err != nil {
		return nil, err
	}
	if input.GeometryID != "sha256:"+base.SHA256 || math.IsNaN(input.Volume) || math.IsInf(input.Volume, 0) || input.Volume <= 1e-9 {
		return nil, fmt.Errorf("%w: IMPORT_BASE_STAGE_GEOMETRY_MISMATCH", ErrValidation)
	}
	for _, body := range model.Bodies {
		if body.ID != seed.BodyId {
			continue
		}
		key := body.GeometryKey
		if key == "" && body.DisplayFallback != nil {
			key = body.DisplayFallback.GeometryKey
		}
		if key == "" {
			return nil, nil
		}
		accepted, err := s.loadArtifact(ctx, key)
		if err != nil {
			return nil, err
		}
		if !accepted.Naming.CanBind {
			return nil, nil
		}
		manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
		if err != nil {
			return nil, err
		}
		for _, root := range manifest.FeatureResults {
			if root.FeatureId != seed.FeatureId || root.BodyId != seed.BodyId || root.ResultGeometryId != input.GeometryID || !root.TopologyHistoryComplete {
				continue
			}
			object, err := s.representationObject(ctx, key, "NAMING")
			if err != nil {
				return nil, err
			}
			return &workerv1.PartBaseStage{GeometryId: input.GeometryID, Volume: input.Volume, Naming: &workerv1.ArtifactReference{
				Backend: object.Backend, ObjectKey: object.Key, Sha256: object.SHA256, SizeBytes: uint64(object.Size), ContentType: object.ContentType,
			}}, nil
		}
	}
	return nil, nil // A newly imported root has not been evaluated yet.
}
