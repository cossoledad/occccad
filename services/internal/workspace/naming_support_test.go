package workspace

import (
	"math"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestNamingCommonAnalyticCarrier(t *testing.T) {
	type evidence = modelcore.TopologySelectionEvidence
	direction := [3]float64{0, 0, 1}
	radius := 4.0
	side := int32(-1)
	line := evidence{GeometryType: "LINE", Origin: [3]float64{2, 3, 5}, Direction: direction}
	plane := evidence{GeometryType: "PLANE", Origin: line.Origin, Direction: direction}
	cylinder := evidence{GeometryType: "CYLINDER", Origin: line.Origin, Direction: direction, RadiusMM: &radius, MaterialSide: &side}
	for _, test := range []struct {
		name   string
		first  evidence
		second [3]float64
		want   bool
	}{
		{"collinear", line, [3]float64{2, 3, -9}, true}, {"parallel-offset", line, [3]float64{2, 4, 5}, false},
		{"coplanar", plane, [3]float64{-7, 12, 5}, true}, {"distinct-planes", plane, [3]float64{2, 3, 5.1}, false},
		{"same-cylinder", cylinder, [3]float64{2, 3, -2}, true}, {"offset-axes", cylinder, [3]float64{3, 3, 5}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := modelcore.ResolvedTopologyElement{Evidence: test.first}
			b := a
			b.Evidence.Origin = test.second
			got := commonAnalyticSupport([]modelcore.ResolvedTopologyElement{a, b})
			if (got != nil) != test.want {
				t.Fatal(got)
			}
			if !test.want {
				return
			}
			b.Evidence.Direction = [3]float64{0, 0, -1}
			if commonAnalyticSupport([]modelcore.ResolvedTopologyElement{a, b}) != nil {
				t.Fatal("opposed orientation")
			}
			b = a
			b.Evidence.Origin[0] = math.NaN()
			if commonAnalyticSupport([]modelcore.ResolvedTopologyElement{a, b}) != nil {
				t.Fatal("invalid origin")
			}
			if test.first.GeometryType == "CYLINDER" {
				b = a
				r := radius + .01
				b.Evidence.RadiusMM = &r
				if commonAnalyticSupport([]modelcore.ResolvedTopologyElement{a, b}) != nil {
					t.Fatal("different radii")
				}
				b = a
				opposite := -side
				b.Evidence.MaterialSide = &opposite
				if commonAnalyticSupport([]modelcore.ResolvedTopologyElement{a, b}) != nil {
					t.Fatal("opposite material side")
				}
			}
		})
	}
}

func TestFiniteSplitFollowsBoundaryAcrossReorderedSiblings(t *testing.T) {
	parent := testRef("base", "edge")
	left, right := testRef("cut", "SPLIT_FROM/base/edge/1"), testRef("cut", "SPLIT_FROM/base/edge/2")
	a, b := testRef("base", "start"), testRef("base", "end")
	manifest := func(first, second *workerv1.SemanticTopologyRef) *topologyManifest {
		f := &workerv1.FeatureResult{FeatureId: "cut", BodyId: "body-main", ResultGeometryId: "same-brep", TopologyHistoryComplete: true, TopologyHistory: &workerv1.TopologyHistory{}}
		for i, ref := range []*workerv1.SemanticTopologyRef{left, right} {
			out := testOutput(ref, uint64(i+1), workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_EDGE)
			out.Evidence.GeometryType = "LINE"
			out.Evidence.Adjacent = []*workerv1.SemanticTopologyRef{[]*workerv1.SemanticTopologyRef{first, second}[i]}
			f.SemanticOutputs = append(f.SemanticOutputs, out)
			f.TopologyHistory.Lineage = append(f.TopologyHistory.Lineage, &workerv1.TopologyLineage{Sources: []*workerv1.SemanticTopologyRef{parent}, Result: ref, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_SPLIT})
		}
		return testBodyTips(&topologyManifest{FeatureResults: []*workerv1.FeatureResult{f}})
	}
	src := manifest(a, b)
	selection := typedTestSelection(modelcore.PersistentTopologyEdge, left.OutputSlot, "LINE")
	selection.Anchor = semanticRef(left)
	got := resolveBetweenNamingViews(selection, src, manifest(b, a), "key", finiteElement)
	if got.Status != modelcore.SelectionResolved || len(got.Candidates) != 1 || got.Candidates[0].LocalID != 2 {
		t.Fatal("branch jumped across the cut", got)
	}
	// The selected ordinal disappears, while the same cut still has a
	// certified branch with the original unique left boundary.
	fewer := manifest(b, a)
	changed := testRef("cut", "SPLIT_FROM/base/edge/3")
	fewer.FeatureResults[0].TopologyHistory.Lineage[0].Result = changed
	fewer.FeatureResults[0].SemanticOutputs[0].SemanticRef = changed
	got = resolveBetweenNamingViews(selection, src, fewer, "key", finiteElement)
	if got.Status != modelcore.SelectionResolved || got.Candidates[0].LocalID != 2 {
		t.Fatal("an ordinal was treated as persistent identity", got)
	}
	got = resolveBetweenNamingViews(selection, src, manifest(a, a), "key", finiteElement)
	if got.Status != modelcore.SelectionAmbiguous {
		t.Fatal("both branches share the same witness", got)
	}
	family := selection
	family.Anchor = semanticRef(parent)
	set := resolveBetweenNamingViews(family, src, manifest(b, a), "key", descendantSet)
	if set.Status != modelcore.SelectionResolved || len(set.Candidates) != 2 || set.Support != nil {
		t.Fatal("explicit set lost descendants", set)
	}
}
