package workspace

import (
	"fmt"
	"reflect"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

// Like Naming v2, refs are shared immutable values. Each step adds new topology
// while carrying earlier outputs. This measures naming, not OCCT construction.
func namingSearchGrowth(steps int) (*topologyManifest, modelcore.PersistentSelection) {
	anchor := testRef("base", "cap")
	current := []*workerv1.SemanticTopologyRef{anchor}
	m := &topologyManifest{GeometryID: "growth", Tips: map[string]*workerv1.FeatureResult{}}
	for step := 0; step <= steps; step++ {
		f := &workerv1.FeatureResult{FeatureId: fmt.Sprintf("feature-%04d", step), BodyId: "body-main", TopologyHistoryComplete: true, TopologyHistory: &workerv1.TopologyHistory{}}
		if step > 0 {
			for _, r := range current {
				f.TopologyHistory.Lineage = append(f.TopologyHistory.Lineage, &workerv1.TopologyLineage{Result: r, Sources: []*workerv1.SemanticTopologyRef{r}, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_UNCHANGED})
			}
			for element := 0; element < 2; element++ {
				r := testRef(f.FeatureId, fmt.Sprintf("new-face/%d", element))
				f.TopologyHistory.Lineage = append(f.TopologyHistory.Lineage, &workerv1.TopologyLineage{Result: r, Sources: []*workerv1.SemanticTopologyRef{current[0]}, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED})
				current = append(current, r)
			}
			if step%9 == 0 {
				r := testRef(f.FeatureId, "modified-cap")
				f.TopologyHistory.Lineage = append(f.TopologyHistory.Lineage, &workerv1.TopologyLineage{Result: r, Sources: []*workerv1.SemanticTopologyRef{current[0]}, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_MODIFIED})
				f.TopologyHistory.Deleted = []*workerv1.TopologyTombstone{{Source: current[0]}}
				current[0] = r
			}
		} else {
			f.TopologyHistory.Lineage = []*workerv1.TopologyLineage{{Result: anchor, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED}}
		}
		m.FeatureResults = append(m.FeatureResults, f)
	}
	tip := m.FeatureResults[len(m.FeatureResults)-1]
	for i, r := range current {
		tip.SemanticOutputs = append(tip.SemanticOutputs, testOutput(r, uint64(i+1), workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE))
	}
	m.Tips["body-main"] = tip
	selection := testSelection()
	selection.Anchor = semanticRef(anchor)
	return m, selection
}

func TestNamingGrowingHistoryMatchesTemporalOracle(t *testing.T) {
	m, selection := namingSearchGrowth(120)
	want := resolveManifestLinearOracle(selection, "key", m)
	got := resolveManifest(selection, "key", m)
	if !reflect.DeepEqual(got, want) || got.Status != modelcore.SelectionResolved || len(got.Candidates) != 1 {
		t.Fatalf("growing history changed resolution: got=%+v want=%+v", got, want)
	}
}

func BenchmarkNamingResolveGrowingHistory(b *testing.B) {
	for _, steps := range []int{100, 300, 1000} {
		b.Run(fmt.Sprint(steps), func(b *testing.B) {
			m, selection := namingSearchGrowth(steps)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if r := resolveManifest(selection, "key", m); r.Status != modelcore.SelectionResolved || len(r.Candidates) != 1 {
					b.Fatal(r)
				}
			}
		})
	}
}

// Artifact decoding builds the immutable index once. Keep that first-use cost
// in BenchmarkNamingResolveGrowingHistory and measure subsequent queries here.
func BenchmarkNamingIndexedQueryGrowingHistory(b *testing.B) {
	for _, steps := range []int{100, 300, 1000} {
		b.Run(fmt.Sprint(steps), func(b *testing.B) {
			m, selection := namingSearchGrowth(steps)
			var err error
			m.search, err = buildNamingSearchIndex(b.Context(), m)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if r := resolveManifest(selection, "key", m); r.Status != modelcore.SelectionResolved || len(r.Candidates) != 1 {
					b.Fatal(r)
				}
			}
		})
	}
}
