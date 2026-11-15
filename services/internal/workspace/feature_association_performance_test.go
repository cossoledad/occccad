package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
)

func associationHistoryWorkload(steps int) *topologyManifest {
	m := &topologyManifest{Tips: map[string]*workerv1.FeatureResult{}}
	var previous []*workerv1.SemanticTopologyRef
	for step := 0; step <= steps; step++ {
		id := fmt.Sprintf("cut-%04d", step)
		f := &workerv1.FeatureResult{FeatureId: id, TopologyHistory: &workerv1.TopologyHistory{}}
		var current []*workerv1.SemanticTopologyRef
		for element := 0; element < 26; element++ {
			r := testRef(id, fmt.Sprintf("face-edge-vertex/%d", element))
			l := &workerv1.TopologyLineage{Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED, Result: r}
			if step > 0 {
				l.Sources = []*workerv1.SemanticTopologyRef{previous[element]}
				if element < 8 {
					l.Kind = workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_MODIFIED
				} else {
					r = previous[element]
					l.Result, l.Kind = r, workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_UNCHANGED
				}
			}
			current = append(current, r)
			f.TopologyHistory.Lineage = append(f.TopologyHistory.Lineage, l)
		}
		m.FeatureResults = append(m.FeatureResults, f)
		previous = current
	}
	tip := &workerv1.FeatureResult{}
	for element, r := range previous {
		tip.SemanticOutputs = append(tip.SemanticOutputs, &workerv1.SemanticTopologyOutput{SemanticRef: r, LocalId: uint64(element + 1), TopologyType: workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE})
	}
	m.Tips["body"] = tip
	return m
}

func BenchmarkFeatureAssociationHistory(b *testing.B) {
	for _, steps := range []int{100, 300, 1000} {
		b.Run(fmt.Sprint(steps), func(b *testing.B) {
			m := associationHistoryWorkload(steps)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if index := deriveFeatureAssociations(m); len(index.Elements) != 26 || len(index.Features) != steps+1 {
					b.Fatal("lost association history")
				}
			}
		})
	}
}

func TestFeatureAssociationTransitionsReadPreviousStage(t *testing.T) {
	a, b := testRef("a", "face"), testRef("b", "face")
	m := &topologyManifest{FeatureResults: []*workerv1.FeatureResult{
		{FeatureId: "a", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Result: a, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED}}}},
		{FeatureId: "b", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Result: b, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED}}}},
		{FeatureId: "swap", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{
			{Result: a, Sources: []*workerv1.SemanticTopologyRef{b}, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_MODIFIED},
			{Result: b, Sources: []*workerv1.SemanticTopologyRef{a}, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_MODIFIED},
		}}},
	}, Tips: map[string]*workerv1.FeatureResult{"body": {SemanticOutputs: []*workerv1.SemanticTopologyOutput{{SemanticRef: a, LocalId: 1, TopologyType: 1}, {SemanticRef: b, LocalId: 2, TopologyType: 1}}}}}
	for range 2 {
		index := deriveFeatureAssociations(m)
		if !slices.Equal(index.Elements[0].Primary, []string{"b"}) || !slices.Equal(index.Elements[1].Primary, []string{"a"}) {
			t.Fatal("same-stage write changed another lineage's input", index)
		}
		slices.Reverse(m.FeatureResults[2].TopologyHistory.Lineage)
	}
}

func TestFeatureAssociationHistoryRetainsFrozenProjection(t *testing.T) {
	m := associationHistoryWorkload(100)
	seed := m.FeatureResults[0].TopologyHistory.Lineage[0].Result
	member := testRef("array", "MEMBER/2/"+associationSeedDigest(seed))
	alias := testRef("blend", "MODIFIER_GENERATED/"+associationSeedDigest(member)[7:]+"/side")
	result := testRef("blend", "face")
	m.FeatureResults = append(m.FeatureResults,
		&workerv1.FeatureResult{FeatureId: "array", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Result: member, Sources: []*workerv1.SemanticTopologyRef{seed}, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED}}}},
		&workerv1.FeatureResult{FeatureId: "blend", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Result: result, Sources: []*workerv1.SemanticTopologyRef{alias}, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED}}}})
	m.Tips["body"].SemanticOutputs = append(m.Tips["body"].SemanticOutputs, &workerv1.SemanticTopologyOutput{SemanticRef: result, LocalId: 27, TopologyType: 1})
	raw, err := json.Marshal(deriveFeatureAssociations(m))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	// Frozen from the original full-state-copy implementation. It includes
	// self refs, a growing modifier chain, member aliases and blend supports.
	if got := hex.EncodeToString(sum[:]); got != "2ac17f954d60eae22bb78723b073fe1f61d7feadbc4f4ba77a60d2cd9cd8f486" {
		t.Fatal("display contribution projection changed", got)
	}
}
