package workspace

import (
	"context"
	"fmt"
	"strings"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
)

// Carry the accepted display association state through ordinary Naming
// events. A reference to a retired state or an alias that needs prior events
// uses the full ordered reader. The resolver still independently reads the
// reachable history when a caller requests lineage or an audit.
func (s *Service) adoptNamingContinuation(ctx context.Context, checkpoint partCheckpoint, root artifactstore.Object, stored *workerv1.PartTopologyManifest, objects map[string]artifactstore.Object) (*topologyManifest, error) {
	finish := perf.Start(ctx, "naming-continuation")
	defer finish()
	previous := checkpoint.naming
	n := len(previous.History)
	if n == 0 || n >= len(stored.History) || stored.PolicyId != modelcore.TopologyNamingPolicyID || stored.PolicyDigest != modelcore.TopologyNamingPolicyDigest || stored.EvaluatorVersion != modelcore.TopologyNamingEvaluator {
		return nil, nil
	}
	source, err := s.loadArtifact(ctx, checkpoint.key)
	if err != nil {
		return nil, err
	}
	oldTip, err := unpackNamingTip(previous)
	if err != nil || !topologyHistoryComplete(oldTip) {
		return nil, ctx.Err()
	}
	nextTip, err := unpackNamingTip(stored)
	if err != nil || !topologyHistoryComplete(nextTip) {
		return nil, ctx.Err()
	}
	before, after := oldTip.FeatureResults[0], nextTip.FeatureResults[0]
	if before.BodyId == "" || before.BodyId != after.BodyId || before.BodyId != checkpoint.continuation.BodyId {
		return nil, nil
	}
	visual := source.Representations["VISUAL"]
	if visual.ObjectID == "" {
		return nil, nil
	}
	_, reader, err := s.artifacts.Open(ctx, visual.ObjectID)
	if err != nil {
		return nil, ctx.Err()
	}
	v, err := frozenVisualization(reader, visual.Size, previous.GeometryId, checkpoint.continuation.Naming.Sha256)
	reader.Close()
	if err != nil || v.FeatureAssociations == nil || len(v.FeatureAssociations.Elements) != len(before.SemanticOutputs) {
		return nil, ctx.Err()
	}
	states := make(map[string]FeatureTopologyAssociation, len(before.SemanticOutputs))
	byLocator := make(map[string]FeatureTopologyAssociation, len(v.FeatureAssociations.Elements))
	for _, element := range v.FeatureAssociations.Elements {
		byLocator[fmt.Sprintf("%s:%d", element.Kind, element.LocalID)] = element
	}
	for _, output := range before.SemanticOutputs {
		kind := strings.TrimPrefix(output.TopologyType.String(), "PERSISTENT_TOPOLOGY_TYPE_")
		element, ok := byLocator[fmt.Sprintf("%s:%d", kind, output.LocalId)]
		if !ok {
			return nil, nil
		}
		states[associationRefKey(output.SemanticRef)] = element
	}
	prefixFeatures := make(map[string]bool, n)
	for _, stage := range previous.History {
		prefixFeatures[stage.FeatureId] = true
	}
	known := make(map[string]bool, len(states))
	for key := range states {
		known[key] = true
	}
	predecessor := previous.History[n-1]
	var suffix []*workerv1.FeatureResult
	var suffixObjects []artifactstore.Object
	for _, reference := range stored.History[n:] {
		object, ok := objects[reference.Sha256]
		if !ok {
			return nil, nil
		}
		block, err := s.readNamingBlock(ctx, object, reference)
		if err != nil {
			return nil, err
		}
		feature := block.view.FeatureResults[0]
		if block.previous != predecessor.Sha256 || block.previousRows != predecessor.FullLineageCount || feature.BodyId != before.BodyId || feature.Result != nil || feature.InputFeatureId != predecessor.FeatureId || feature.TopologyHistory.InputGeometryId != predecessor.ResultGeometryId || !feature.TopologyHistoryComplete || prefixFeatures[feature.FeatureId] ||
			feature.TopologyHistory.PolicyDigest != stored.PolicyDigest || block.policyID != stored.PolicyId || block.evaluator != stored.EvaluatorVersion {
			return nil, nil
		}
		for _, line := range feature.TopologyHistory.Lineage {
			for _, source := range line.Sources {
				if known[associationRefKey(source)] {
					continue
				}
				role, _, _ := strings.Cut(source.OutputSlot, "/")
				profile := role == "PROFILE_EDGE" || role == "PROFILE_REGION" || role == "PROFILE_POINT"
				if role == "MEMBER" || role == "MODIFIER_GENERATED" || source.FeatureId != feature.FeatureId && !profile {
					return nil, nil
				}
			}
		}
		for _, line := range feature.TopologyHistory.Lineage {
			known[associationRefKey(line.Result)] = true
		}
		prefixFeatures[feature.FeatureId] = true
		suffix = append(suffix, feature)
		suffixObjects = append(suffixObjects, object)
		predecessor = reference
	}
	parent, err := s.representationObject(ctx, checkpoint.key, "NAMING")
	if err != nil {
		return nil, err
	}
	dependencies, err := s.artifacts.Dependencies(ctx, parent.ID)
	if err != nil {
		return nil, err
	}
	last := previous.History[n-1]
	if len(dependencies) != 1 || dependencies[0].SHA256 != last.Sha256 || uint64(dependencies[0].Size) != last.SizeBytes {
		return nil, nil
	}
	if err := s.artifacts.RecordNamingContinuation(ctx, root, dependencies[0], suffixObjects); err != nil {
		return nil, err
	}
	nextTip.FeatureResults = suffix
	nextTip.associations = deriveFeatureAssociationsFrom(nextTip, states, v.FeatureAssociations.Features)
	return nextTip, nil
}
