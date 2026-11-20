package workspace

import (
	"context"
	"fmt"
	"maps"
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
func (s *Service) adoptNamingContinuation(ctx context.Context, checkpoint partCheckpoint, root artifactstore.Object, stored *workerv1.PartTopologyManifest, objects map[string]artifactstore.Object, displayAssociations bool) (*topologyManifest, error) {
	finish := perf.Start(ctx, "naming-continuation")
	defer finish()
	previous := checkpoint.naming
	n := len(previous.History)
	if len(previous.Bodies) != 1 || n == 0 || n >= len(stored.History) || stored.PolicyId != modelcore.TopologyNamingPolicyID || stored.PolicyDigest != modelcore.TopologyNamingPolicyDigest || stored.EvaluatorVersion != modelcore.TopologyNamingEvaluator {
		return nil, nil
	}

	nextTip, err := unpackNamingTip(stored)
	if err != nil || !topologyHistoryComplete(nextTip) {
		return nil, ctx.Err()
	}
	before := previous.GetBodies()[0]
	if before.BodyId == "" || before.BodyId != nextTip.FeatureResults[0].BodyId || before.BodyId != checkpoint.continuation.BodyId {
		return nil, nil
	}
	var seed *namingAssociationSeed
	if displayAssociations {
		seed, err = s.namingAssociationSeed(ctx, checkpoint)
		if err != nil || seed == nil {
			return nil, err
		}
	}
	prefixFeatures := make(map[string]bool, n)
	for _, stage := range previous.History {
		prefixFeatures[stage.FeatureId] = true
	}
	known := map[string]bool{}
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
		if feature.FeatureId != reference.FeatureId || feature.ResultGeometryId != reference.ResultGeometryId || block.fullRows != reference.FullLineageCount {
			return nil, fmt.Errorf("naming suffix reference mismatch")
		}
		if block.previous != predecessor.Sha256 || block.previousRows != predecessor.FullLineageCount || feature.BodyId != before.BodyId || feature.Result != nil || feature.InputFeatureId != predecessor.FeatureId || feature.TopologyHistory.InputGeometryId != predecessor.ResultGeometryId || !feature.TopologyHistoryComplete || prefixFeatures[feature.FeatureId] ||
			feature.TopologyHistory.PolicyDigest != stored.PolicyDigest || block.policyID != stored.PolicyId || block.evaluator != stored.EvaluatorVersion {
			return nil, nil
		}
		if seed != nil {
			for _, line := range feature.TopologyHistory.Lineage {
				for _, source := range line.Sources {
					k := associationRefKey(source)
					if _, present := seed.states[k]; present || known[k] {
						continue
					}
					role, id, qualified := strings.Cut(source.OutputSlot, "/")
					profile := qualified && id != "" && !prefixFeatures[source.FeatureId] && isProfileNamingRole(role)
					if role == "MEMBER" || role == "MODIFIER_GENERATED" || source.FeatureId != feature.FeatureId && !profile {
						return nil, nil
					}
				}
			}
			for _, line := range feature.TopologyHistory.Lineage {
				known[associationRefKey(line.Result)] = true
			}
		}
		prefixFeatures[feature.FeatureId] = true
		suffix = append(suffix, feature)
		suffixObjects = append(suffixObjects, object)
		predecessor = reference
	}
	dependencies, err := s.artifacts.Dependencies(ctx, checkpoint.object.ID)
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
	if seed != nil {
		nextTip.associations = deriveFeatureAssociationsFrom(nextTip, maps.Clone(seed.states), seed.features)
	}
	return nextTip, nil
}

// Immutable display contributions of an accepted tip. A suffix may update a
// private map; inherited slices are read-only in deriveFeatureAssociationsFrom.
type namingAssociationSeed struct {
	states   map[string]FeatureTopologyAssociation
	features []string
}

func (s *Service) namingAssociationSeed(ctx context.Context, checkpoint partCheckpoint) (*namingAssociationSeed, error) {
	visual := checkpoint.visual
	if visual.ObjectID == "" {
		return nil, nil
	}
	root := checkpoint.naming
	key := checkpoint.object.ID + "\x00" + checkpoint.object.SHA256 + "\x00" + visual.ObjectID + "\x00" + visual.Digest
	return s.namingSeeds.read(ctx, key, 32<<20, func() (*namingAssociationSeed, int64, error) {
		defer perf.Start(ctx, "naming-association-restore")()
		_, reader, err := s.artifacts.Open(ctx, visual.ObjectID)
		if err != nil {
			return nil, 0, ctx.Err()
		}
		associations, listBytes, err := readFrozenAssociations(reader, visual.Size, root.GeometryId, checkpoint.continuation.Naming.Sha256)
		reader.Close()
		outputs := root.Bodies[0].Tip
		if err != nil || associations == nil || len(associations.Elements) != len(outputs) {
			return nil, 0, ctx.Err()
		}
		refs, err := namingReferences(root)
		if err != nil {
			return nil, 0, nil
		}
		type locator struct {
			kind string
			id   uint64
		}
		byLocator := make(map[locator]FeatureTopologyAssociation, len(outputs))
		for _, element := range associations.Elements {
			byLocator[locator{element.Kind, element.LocalID}] = element
		}
		seed := &namingAssociationSeed{features: associations.Features, states: make(map[string]FeatureTopologyAssociation, len(outputs))}
		charge := int64(512) + listBytes
		for _, o := range outputs {
			if o.SemanticRef == 0 || int(o.SemanticRef) > len(refs) {
				return nil, 0, nil
			}
			element, ok := byLocator[locator{string(topologyType(o.TopologyType)), o.LocalId}]
			if !ok {
				return nil, 0, nil
			}
			k := associationRefKey(refs[o.SemanticRef-1])
			seed.states[k] = element
			charge += int64(320 + len(k))
		}
		return seed, charge, nil
	})
}
