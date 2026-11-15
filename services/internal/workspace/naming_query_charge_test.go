package workspace

import (
	"strings"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
)

func TestNamingQueryChargeAccountsForSharedReferencesAndDistinctOwners(t *testing.T) {
	ref := &workerv1.SemanticTopologyRef{FeatureId: "owner", OutputSlot: strings.Repeat("x", 1<<20)}
	feature := &workerv1.FeatureResult{TopologyHistory: &workerv1.TopologyHistory{}}
	for range 1000 {
		feature.TopologyHistory.Lineage = append(feature.TopologyHistory.Lineage, &workerv1.TopologyLineage{Result: ref, Sources: []*workerv1.SemanticTopologyRef{ref}})
	}
	shared := namingQueryBlockCharge(feature)
	if shared < len(ref.OutputSlot) || shared > 4<<20 {
		t.Fatal("shared payload was omitted or charged once per row", shared)
	}
	// Identical values can be independently owned decoded objects. The cache
	// must account for that ownership, rather than deduplicate by CAD identity.
	feature.TopologyHistory.Lineage[0].Result = &workerv1.SemanticTopologyRef{FeatureId: ref.FeatureId, OutputSlot: ref.OutputSlot}
	if distinct := namingQueryBlockCharge(feature); distinct <= shared {
		t.Fatal("distinct reference allocation was not accounted for", shared, distinct)
	}
}
