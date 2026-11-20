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

func TestNamingTipChargeAccountsForDecodedOwnership(t *testing.T) {
	root := &workerv1.PartTopologyManifest{}
	const wireBytes = 5 << 20
	sparse := namingTipCharge(root, wireBytes, 1)
	if sparse < wireBytes || sparse > 2*wireBytes {
		t.Fatal("encoded payload must be accounted for once", sparse)
	}
	for range 1000 {
		root.Evidence = append(root.Evidence, &workerv1.NamingEvidence{Adjacent: make([]uint32, 32)})
	}
	if dense := namingTipCharge(root, wireBytes, 1000); dense-sparse < 1<<20 {
		t.Fatal("same size on disk must account for more decoded objects and adjacency", sparse, dense)
	}
	ref := &workerv1.SemanticTopologyRef{FeatureId: "upstream", OutputSlot: strings.Repeat("x", 1<<12)}
	if one, two := splitParentCharge(map[int]*workerv1.SemanticTopologyRef{1: ref}), splitParentCharge(map[int]*workerv1.SemanticTopologyRef{1: ref, 2: ref}); one < len(ref.OutputSlot) || one != two {
		t.Fatal("split siblings must share the charge for their retained witness", one, two)
	}
}
