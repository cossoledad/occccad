package workspace

import (
	"context"
	"errors"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
)

// Checkpoints are published alongside the accepting Revision. The durable
// index stores one association per exact stage input instead of copying an
// index of all earlier stages into every Revision.
func acceptedPartCheckpoints(model PartModel, current modelcore.EvaluationManifest) []modelcore.PartCheckpoint {
	tips := map[string]string{}
	for _, feature := range model.Features {
		if isBodyFeature(feature.Type) && !feature.Suppressed {
			tips[feature.BodyID] = feature.ID
		}
	}
	var out []modelcore.PartCheckpoint
	for _, body := range model.Bodies {
		id := tips[body.ID]
		node := current.NodeResults[modelcore.DependencyKey("feature:"+id)]
		if id != "" && body.GeometryKey != "" && node.InputDigest != "" && node.OutputDigest != "" {
			out = append(out, modelcore.PartCheckpoint{BodyID: body.ID, FeatureID: id, InputDigest: node.InputDigest, GeometryKey: body.GeometryKey})
		}
	}
	return out
}

func queuePartCheckpoints(batch *database.Batch, manifest modelcore.EvaluationManifest) {
	for _, stage := range manifest.PartCheckpoints {
		batch.Queue(`INSERT INTO occccad.part_stage_checkpoints(body_id,feature_id,input_digest,geometry_key) VALUES($1,$2,$3,$4) ON CONFLICT(body_id,feature_id,input_digest) DO NOTHING`, stage.BodyID, stage.FeatureID, stage.InputDigest, stage.GeometryKey)
	}
}

type partCheckpoint struct {
	visual       Representation
	object       artifactstore.Object
	key          string
	position     int
	continuation *workerv1.PartContinuation
	naming       *workerv1.PartTopologyManifest
}
type namingContinuationKey struct{}
type skipPartCheckpointKey struct{}

func (s *Service) partCheckpoint(ctx context.Context, model PartModel) (partCheckpoint, error) {
	plan := executionPlan(ctx)
	if plan == nil || s.artifacts == nil || geometry.PartRuntime(ctx).ForceCold || ctx.Value(skipPartCheckpointKey{}) != nil {
		return partCheckpoint{}, nil
	}
	var ordered []string
	var positions []int
	lastPattern, imported := -1, 0
	for i, f := range model.Features {
		if f.Suppressed || f.BodyID != model.ActiveBodyID || !isBodyFeature(f.Type) {
			continue
		}
		if f.Type == "IMPORT_BODY" {
			imported++
		}
		if f.Type == "SOLID_PATTERN" {
			lastPattern = len(ordered)
		}
		ordered = append(ordered, f.ID)
		positions = append(positions, i)
	}
	// Pattern in the suffix requires its original native construction context.
	// The candidate's unchanged predecessor is looked up by its accepted DAG input.
	var checkpoint partCheckpoint
	for i := len(ordered) - 2; i >= max(imported, lastPattern); i-- {
		id := ordered[i]
		node := modelcore.DependencyKey("feature:" + id)
		input := plan.priorResults[node]
		if plan.dirty[node] || input.Status != "SUCCEEDED" || input.InputDigest == "" {
			continue
		}
		key := plan.acceptedStages[id]
		if key == "" {
			err := s.database.QueryRow(ctx, `SELECT geometry_key FROM occccad.part_stage_checkpoints WHERE body_id=$1 AND feature_id=$2 AND input_digest=$3`, model.ActiveBodyID, id, input.InputDigest).Scan(&key)
			if errors.Is(err, database.ErrNoRows) {
				continue
			}
			if err != nil {
				return partCheckpoint{}, err
			}
		}
		checkpoint = partCheckpoint{key: key, position: positions[i], continuation: &workerv1.PartContinuation{FeatureId: id, BodyId: model.ActiveBodyID, StageCount: uint32(i + 1 - imported)}}
		break
	}
	if checkpoint.continuation == nil {
		return partCheckpoint{}, nil
	}
	a, err := s.loadArtifact(ctx, checkpoint.key)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return partCheckpoint{}, nil
		}
		return partCheckpoint{}, err
	}
	if a.ValidationPolicy != geometryValidationPolicy || !a.Naming.CanBind || a.EvaluatorVersion != evaluatorVersion {
		return partCheckpoint{}, nil
	}
	object, err := s.representationObject(ctx, checkpoint.key, "NAMING")
	if err != nil {
		return partCheckpoint{}, ctx.Err()
	}
	raw, err := s.readRepresentationObject(ctx, object, "NAMING")
	if err != nil {
		return partCheckpoint{}, ctx.Err()
	}
	root := &workerv1.PartTopologyManifest{}
	if err = proto.Unmarshal(raw, root); err != nil {
		return partCheckpoint{}, nil
	}
	n := int(checkpoint.continuation.StageCount)
	if len(root.History) != n && len(root.History) != n+1 || len(root.History) >= len(ordered) || len(root.TipLineageLocators) == 0 || root.ReferenceDictionary == nil ||
		len(root.Bodies) != 1 || root.Bodies[0].BodyId != model.ActiveBodyID || root.Bodies[0].TipFeatureId != checkpoint.continuation.FeatureId || !root.Bodies[0].Complete || root.GeometryId != a.GeometryID || root.BrepSha256 != a.Representations["BREP"].Digest {
		return partCheckpoint{}, nil
	}
	for i, stage := range root.History {
		if stage.FeatureId != ordered[i] {
			return partCheckpoint{}, nil
		}
	}
	checkpoint.continuation.ValidationPolicy = a.ValidationPolicy
	checkpoint.continuation.Naming = &workerv1.ArtifactReference{Backend: object.Backend, ObjectKey: object.Key, Sha256: object.SHA256, SizeBytes: uint64(object.Size), ContentType: object.ContentType}
	checkpoint.visual = a.Representations["VISUAL"]
	checkpoint.object = object
	checkpoint.naming = root
	return checkpoint, nil
}
