package workspace

import (
	"slices"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
)

func TestFeatureAssociationsTrackContributionsNotBodyReaders(t *testing.T) {
	hub, blade, split, member, blend := testRef("hub", "side"), testRef("blade", "side"), testRef("pattern", "split"), testRef("pattern", "MEMBER/2/hash"), testRef("fillet", "blend")
	transition := func(id string, lines ...*workerv1.TopologyLineage) *workerv1.FeatureResult {
		return &workerv1.FeatureResult{FeatureId: id, TopologyHistory: &workerv1.TopologyHistory{Lineage: lines}}
	}
	line := func(kind workerv1.TopologyLineageKind, result *workerv1.SemanticTopologyRef, sources ...*workerv1.SemanticTopologyRef) *workerv1.TopologyLineage {
		return &workerv1.TopologyLineage{Kind: kind, Result: result, Sources: sources}
	}
	generated, modified, splitKind := workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED, workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_MODIFIED, workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_SPLIT
	m := &topologyManifest{FeatureResults: []*workerv1.FeatureResult{transition("hub", line(generated, hub)), transition("blade", line(generated, blade)), transition("pattern", line(splitKind, split, blade), line(generated, member, blade)), transition("fillet", line(modified, split, split), line(generated, blend, member))}, Tips: map[string]*workerv1.FeatureResult{"body": {SemanticOutputs: []*workerv1.SemanticTopologyOutput{{LocalId: 1, TopologyType: 1, SemanticRef: hub}, {LocalId: 2, TopologyType: 1, SemanticRef: split}, {LocalId: 3, TopologyType: 1, SemanticRef: member}, {LocalId: 4, TopologyType: 1, SemanticRef: blend}}}}}
	index := deriveFeatureAssociations(m)
	if len(index.Elements) != 4 {
		t.Fatal(index)
	}
	trimmed := index.Elements[1]
	if !slices.Equal(trimmed.Origins, []string{"blade"}) || !slices.Equal(trimmed.Primary, []string{"blade"}) || !slices.Contains(trimmed.Modifiers, "fillet") {
		t.Fatal("trimmed face lost blade", trimmed)
	}
	copied := index.Elements[2]
	if !slices.Equal(copied.Origins, []string{"blade", "pattern"}) || len(copied.Members) != 1 || copied.Members[0].Slot != 2 {
		t.Fatal("member lost provenance", copied)
	}
	rounded := index.Elements[3]
	if !slices.Equal(rounded.Origins, []string{"fillet"}) || !slices.Equal(rounded.Primary, []string{"fillet"}) || !slices.Contains(rounded.Supports, "blade") || rounded.Members[0].Slot != 2 {
		t.Fatal("transition face confused with support", rounded)
	}
	m.FeatureResults = append(m.FeatureResults, transition("consumed", line(generated, testRef("consumed", "gone"))))
	index = deriveFeatureAssociations(m)
	if featureContributionStatuses(index)["consumed"] != "NO_CURRENT_CONTRIBUTION" {
		t.Fatal("complete history confused consumption with missing mapping")
	}
	m.Tips["body"].SemanticOutputs = append(m.Tips["body"].SemanticOutputs, &workerv1.SemanticTopologyOutput{LocalId: 5, TopologyType: 1, SemanticRef: testRef("unknown", "missing")})
	index = deriveFeatureAssociations(m)
	if !slices.Contains(index.Features, "consumed") || index.Elements[4].Status != "MAPPING_MISSING" {
		t.Fatal(index)
	}
}
func TestFeatureInputProjectionIncludesOrderedLoftSharedAndSeedReferences(t *testing.T) {
	model := PartModel{Bodies: []PartBody{{ID: "b"}}, ActiveBodyID: "b", Features: []Feature{{ID: "s1", BodyID: "b", Type: "SKETCH"}, {ID: "s2", BodyID: "b", Type: "SKETCH"}, {ID: "pad", BodyID: "b", Type: "PAD", Profile: "s1"}, {ID: "loft", BodyID: "b", Type: "LOFT", Sections: []LoftSection{{SketchID: "s2"}, {SketchID: "s1"}}}, {ID: "array", BodyID: "b", Type: "SOLID_PATTERN", Pattern: &FeaturePattern{Source: FeatureStageRef{FeatureID: "pad", BodyID: "b"}}}}}
	var body DocumentStructureNode
	for _, n := range partStructureChildren(model, "d", "doc", "rev", true) {
		if n.Kind == "BODY" {
			body = n
		}
	}
	if len(body.Children) != 4 || body.Children[0].EntityID != "s1" || body.Children[1].EntityID != "pad" || body.Children[2].EntityID != "loft" || body.Children[3].EntityID != "array" {
		t.Fatal(body.Children)
	}
	loft := body.Children[2]
	if len(loft.Children) != 2 || loft.Children[0].EntityID != "s2" || loft.Children[0].InputRole != "SECTION" || loft.Children[1].EntityID != "s1" || loft.Children[1].InputOrder != 1 || loft.Children[1].PresentationRole != "INPUT_REFERENCE" {
		t.Fatal(loft)
	}
	seed := body.Children[1]
	if !slices.Contains(seed.Capabilities, "EDIT") || slices.Contains(seed.Capabilities, "DELETE") {
		t.Fatal("referenced seed is editable but deletion is protected", seed)
	}
	if body.Children[3].Children[0].PresentationRole != "INPUT_REFERENCE" || body.Children[3].Children[0].EntityID != "pad" {
		t.Fatal("pattern reparented seed")
	}
}

func TestFeatureAssociationsMergeRetainsCandidatesAndDeletion(t *testing.T) {
	a, b, merged := testRef("a", "side"), testRef("b", "side"), testRef("merge", "face")
	generated := workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED
	m := &topologyManifest{FeatureResults: []*workerv1.FeatureResult{
		{FeatureId: "a", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Kind: generated, Result: a}}}},
		{FeatureId: "b", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Kind: generated, Result: b}}}},
		{FeatureId: "merge", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_MERGED, Result: merged, Sources: []*workerv1.SemanticTopologyRef{a, b}}}}},
	}, Tips: map[string]*workerv1.FeatureResult{"body": {SemanticOutputs: []*workerv1.SemanticTopologyOutput{{SemanticRef: merged, LocalId: 1, TopologyType: 1}}}}}
	index := deriveFeatureAssociations(m)
	if !slices.Equal(index.Elements[0].Origins, []string{"a", "b"}) || !slices.Equal(index.Elements[0].Primary, []string{"a", "b"}) {
		t.Fatal("merge invented unique owner", index)
	}
	m.FeatureResults = append(m.FeatureResults, &workerv1.FeatureResult{FeatureId: "cut", TopologyHistory: &workerv1.TopologyHistory{Deleted: []*workerv1.TopologyTombstone{{Source: merged, Reason: "OCCT_IS_DELETED_OR_OUTSIDE_RESULT"}}}})
	m.Tips["body"].SemanticOutputs = nil
	index = deriveFeatureAssociations(m)
	if len(index.Elements) != 0 || featureContributionStatuses(index)["a"] != "NO_CURRENT_CONTRIBUTION" {
		t.Fatal("deleted result resurrected", index)
	}
}

func TestFeatureAssociationsProfileLineageBelongsToGenerator(t *testing.T) {
	generated := workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED
	merged := workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_MERGED
	split := workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_SPLIT
	first, second := testRef("section-a", "PROFILE_EDGE/a"), testRef("section-b", "PROFILE_EDGE/b")
	base := testRef("base", "PROFILE_EDGE/actually-named-solid")
	for _, tc := range []struct {
		name              string
		kind              workerv1.TopologyLineageKind
		sources           []*workerv1.SemanticTopologyRef
		primary, supports []string
	}{
		{"merged sections", merged, []*workerv1.SemanticTopologyRef{first, second}, []string{"loft"}, []string{"section-a", "section-b"}},
		{"split section", split, []*workerv1.SemanticTopologyRef{first}, []string{"loft"}, []string{"section-a"}},
		{"solid and generated section", merged, []*workerv1.SemanticTopologyRef{base, second}, []string{"base", "loft"}, []string{"section-b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := testRef("loft", "MERGED_FROM/semantic-source-digest")
			m := &topologyManifest{FeatureResults: []*workerv1.FeatureResult{
				{FeatureId: "base", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Kind: generated, Result: base}}}},
				{FeatureId: "loft", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Kind: tc.kind, Result: result, Sources: tc.sources}}}},
			}, Tips: map[string]*workerv1.FeatureResult{"body": {SemanticOutputs: []*workerv1.SemanticTopologyOutput{{SemanticRef: result, LocalId: 3, TopologyType: 1}}}}}
			check := func() {
				t.Helper()
				a := deriveFeatureAssociations(m).Elements[0]
				if !slices.Equal(a.Primary, tc.primary) || !slices.Equal(a.Origins, tc.primary) || !slices.Equal(a.Supports, tc.supports) || a.Status != "MAPPED" {
					t.Fatal("profile supports confused with solid ownership", a)
				}
			}
			check()
			// A later local modification must retain that ownership, not restore the sections.
			m.FeatureResults = append(m.FeatureResults, &workerv1.FeatureResult{FeatureId: "fillet", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_MODIFIED, Result: result, Sources: []*workerv1.SemanticTopologyRef{result}}}}})
			check()
		})
	}
}
