package workspace

import (
	"context"
	"math/rand"
	"reflect"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

// Arbitrary ordered event streams exercise tombstones, later reintroduction,
// generated support, same-stage rules, ambiguity and owner isolation against
// the frozen scanner, rather than assuming an unordered graph is equivalent.
func TestNamingSearchMatchesTemporalOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(6242026))
	for sample := 0; sample < 100; sample++ {
		refs := []*workerv1.SemanticTopologyRef{}
		for i := 0; i < 16; i++ {
			r := testRef(string(rune('a'+i)), "face")
			r.SourceIds = []string{"source\nvalue", string(rune('a' + i%3))}
			refs = append(refs, r)
		}
		m := &topologyManifest{GeometryID: "frozen", Tips: map[string]*workerv1.FeatureResult{}, Results: map[modelcore.GeometryResultReference]*workerv1.FeatureResult{}}
		for step := 0; step < 20; step++ {
			f := &workerv1.FeatureResult{FeatureId: "step", BodyId: "body-main", TopologyHistoryComplete: true, TopologyHistory: &workerv1.TopologyHistory{}}
			if rng.Intn(4) == 0 {
				f.BodyId = "other-body"
			}
			for range 7 {
				l := &workerv1.TopologyLineage{Result: refs[rng.Intn(len(refs))], Kind: workerv1.TopologyLineageKind(rng.Intn(6))}
				for range rng.Intn(4) {
					l.Sources = append(l.Sources, refs[rng.Intn(len(refs))])
				}
				f.TopologyHistory.Lineage = append(f.TopologyHistory.Lineage, l)
			}
			for range rng.Intn(3) {
				f.TopologyHistory.Ambiguous = append(f.TopologyHistory.Ambiguous, &workerv1.AmbiguousLineage{Sources: []*workerv1.SemanticTopologyRef{refs[rng.Intn(len(refs))]}, Candidates: []*workerv1.SemanticTopologyRef{refs[rng.Intn(len(refs))], refs[rng.Intn(len(refs))]}})
			}
			for range rng.Intn(3) {
				f.TopologyHistory.Deleted = append(f.TopologyHistory.Deleted, &workerv1.TopologyTombstone{Source: refs[rng.Intn(len(refs))]})
			}
			if step >= 18 {
				for i, ref := range refs {
					f.SemanticOutputs = append(f.SemanticOutputs, testOutput(ref, uint64(i+1), workerv1.PersistentTopologyType(1+rng.Intn(3))))
				}
				m.Tips[f.BodyId] = f
			}
			m.FeatureResults = append(m.FeatureResults, f)
		}
		var err error
		m.search, err = buildNamingSearchIndex(t.Context(), m)
		if err != nil {
			t.Fatal(err)
		}
		query := &topologyManifest{GeometryID: m.GeometryID, Tips: m.Tips, Results: m.Results}
		for _, feature := range m.FeatureResults {
			block := namingBlockForQueries(&decodedNamingBlock{view: &topologyManifest{FeatureResults: []*workerv1.FeatureResult{feature}}})
			query.FeatureResults = append(query.FeatureResults, block.view.FeatureResults[0])
		}
		query.search, err = buildNamingSearchIndex(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range append(refs, testRef("absent", "face")) {
			for _, owner := range []string{"body-main", "other-body", "missing-body"} {
				selection := testSelection()
				selection.SourceBodyID = owner
				selection.Anchor = semanticRef(ref)
				for _, recipe := range []modelcore.SelectionRecipeKind{modelcore.SelectionLineageDescendant, modelcore.SelectionDirectSemanticOutput} {
					selection.Selector.Kind = recipe
					got, want := resolveManifest(selection, "key", m), resolveManifestLinearOracle(selection, "key", m)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("sample=%d owner=%s anchor=%v recipe=%s got=%+v want=%+v", sample, owner, selection.Anchor, recipe, got, want)
					}
					if projected := resolveManifest(selection, "key", query); !reflect.DeepEqual(projected, want) {
						t.Fatalf("query projection changed ordered history: sample=%d owner=%s anchor=%v recipe=%s got=%+v want=%+v", sample, owner, selection.Anchor, recipe, projected, want)
					}
				}
			}
		}
	}
}

func TestNamingSearchSkipsUnrelatedHistoryAndIsolatesReturnedEvidence(t *testing.T) {
	m, selection := namingSearchGrowth(120)
	var err error
	m.search, err = buildNamingSearchIndex(t.Context(), m)
	if err != nil {
		t.Fatal(err)
	}
	stats := &namingQueryStats{}
	got := m.search.resolve(selection, "key", m.GeometryID, stats)
	if got.Status != modelcore.SelectionResolved || stats.Events > 40 || stats.Outputs != 1 {
		t.Fatal("query scanned unrelated history", stats, got)
	}
	output := m.search.semanticOutput(modelcore.PersistentSelection{SourceBodyID: selection.SourceBodyID, Anchor: got.Candidates[0].SemanticRef, ExpectedType: selection.ExpectedType})
	radius := 5.0
	output.Evidence.RadiusMm = &radius
	first := resolveManifest(selection, "key", m)
	*first.Candidates[0].Evidence.RadiusMM = 500
	second := resolveManifest(selection, "key", m)
	if *second.Candidates[0].Evidence.RadiusMM != 5 {
		t.Fatal("returned mutable evidence poisoned an immutable view")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = buildNamingSearchIndex(ctx, m); err != context.Canceled {
		t.Fatal("cancelled index build published a view", err)
	}
}

func TestNamingSearchKeepsGeometryResultsSeparateFromBodies(t *testing.T) {
	m, selection := namingSearchGrowth(12)
	result := modelcore.GeometryResultReference{FeatureID: "curve-owner", ResultID: "curve-result"}
	for _, feature := range m.FeatureResults {
		feature.BodyId = ""
		feature.Result = &workerv1.GeometryResultReference{FeatureId: result.FeatureID, ResultId: result.ResultID}
	}
	tip := m.Tips["body-main"]
	m.Tips = nil
	m.Results = map[modelcore.GeometryResultReference]*workerv1.FeatureResult{result: tip}
	var err error
	m.search, err = buildNamingSearchIndex(t.Context(), m)
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []*modelcore.GeometryResultReference{nil, &result, {FeatureID: result.FeatureID, ResultID: "other"}} {
		selection.SourceResult = reference
		if reference != nil {
			selection.SourceBodyID = ""
		}
		got, want := resolveManifest(selection, "key", m), resolveManifestLinearOracle(selection, "key", m)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("owner=%+v got=%+v want=%+v", reference, got, want)
		}
	}
}
