package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
)

// A test-only file producer, independent of the production C++ writer. Each
// supplied transition starts as literal; patches can introduce exact carries.
func namingRootFixture(t *testing.T, v *workerv1.PartTopologyManifest, patch func(int, *workerv1.NamingHistoryBlock)) (*workerv1.PartTopologyManifest, map[string][]byte) {
	t.Helper()
	v = proto.Clone(v).(*workerv1.PartTopologyManifest)
	v.SchemaVersion = 3
	if v.PolicyId == "" {
		v.PolicyId = modelcore.TopologyNamingPolicyID
	}
	if v.EvaluatorVersion == "" {
		v.EvaluatorVersion = modelcore.TopologyNamingEvaluator
	}
	root := proto.Clone(v).(*workerv1.PartTopologyManifest)
	root.Transitions = nil
	root.History = nil
	previous := ""
	var rows uint64
	complete := true
	payloads := map[string][]byte{}
	for i, transition := range v.Transitions {
		if transition.ResultGeometryId == "" {
			transition.ResultGeometryId = v.GeometryId
		}
		values := &workerv1.PartTopologyManifest{SchemaVersion: 3, PolicyId: v.PolicyId, EvaluatorVersion: v.EvaluatorVersion, PolicyDigest: v.PolicyDigest, GeometryId: transition.ResultGeometryId, CoordinateSpace: v.CoordinateSpace, LengthUnit: v.LengthUnit, SemanticRefs: v.SemanticRefs, Evidence: v.Evidence, Transitions: []*workerv1.NamingTransition{transition}}
		if transition.Result == nil {
			values.Bodies = []*workerv1.NamingBody{{BodyId: transition.BodyId, Transitions: []uint32{1}, TipTransition: 1}}
		} else {
			values.Results = []*workerv1.NamingGeometryResult{{Reference: transition.Result, Transitions: []uint32{1}, TipTransition: 1}}
		}
		block := &workerv1.NamingHistoryBlock{SchemaVersion: 1, Values: values, PreviousSha256: previous, PreviousLineageCount: rows, FullLineageCount: uint64(len(transition.Lineage))}
		if len(transition.Lineage) > 0 {
			block.Layout = []*workerv1.NamingHistorySpan{{Count: uint64(len(transition.Lineage))}}
		}
		if patch != nil {
			patch(i, block)
		}
		data, err := proto.Marshal(block)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		digest := hex.EncodeToString(hash[:])
		payloads[digest] = data
		root.History = append(root.History, &workerv1.NamingHistoryReference{Sha256: digest, SizeBytes: uint64(len(data)), FeatureId: transition.FeatureId, ResultGeometryId: transition.ResultGeometryId, FullLineageCount: block.FullLineageCount})
		previous = digest
		rows = block.FullLineageCount
		complete = complete && transition.Complete
	}
	tip := v.Transitions[len(v.Transitions)-1].FeatureId
	for _, body := range root.Bodies {
		body.Transitions = nil
		body.TipTransition = 0
		body.TipFeatureId = tip
		body.Complete = complete
	}
	for _, result := range root.Results {
		result.Transitions = nil
		result.TipTransition = 0
		result.TipFeatureId = tip
		result.Complete = complete
	}
	return root, payloads
}
func fixtureHistoryLoader(payloads map[string][]byte) namingHistoryLoader {
	return func(ctx context.Context, ref *workerv1.NamingHistoryReference) (*decodedNamingBlock, error) {
		return decodeNamingBlock(ctx, payloads[ref.Sha256], ref, true)
	}
}

func TestNamingHistoryCarriesRemainLosslessAndQueriesUseOnlyEvents(t *testing.T) {
	a, b := testRef("base", "first"), testRef("base", "second")
	ev := &workerv1.NamingEvidence{EvidenceDigest: "evidence", Geometry: &workerv1.NamingEvidence_Plane{Plane: &workerv1.NamingFrame{Direction: &workerv1.Vec3{Z: 1}}}}
	values := &workerv1.PartTopologyManifest{SchemaVersion: 3, PolicyId: modelcore.TopologyNamingPolicyID, EvaluatorVersion: modelcore.TopologyNamingEvaluator, PolicyDigest: modelcore.TopologyNamingPolicyDigest, GeometryId: "second", CoordinateSpace: "PART_LOCAL", LengthUnit: "mm", SemanticRefs: []*workerv1.SemanticTopologyRef{a, b}, Evidence: []*workerv1.NamingEvidence{ev}, Transitions: []*workerv1.NamingTransition{
		{FeatureId: "base", BodyId: "body-main", ResultGeometryId: "base", Complete: true, Lineage: []*workerv1.NamingLineage{{Result: 1, Evidence: 1, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED}, {Result: 2, Evidence: 1, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED}}},
		{FeatureId: "second", BodyId: "body-main", InputFeatureId: "base", ResultGeometryId: "second", Complete: true, Lineage: []*workerv1.NamingLineage{{Result: 1, Sources: []uint32{1}, Evidence: 1, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_UNCHANGED}, {Result: 2, Sources: []uint32{2}, Evidence: 1, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_UNCHANGED}}},
	}, Bodies: []*workerv1.NamingBody{{BodyId: "body-main", Transitions: []uint32{1, 2}, TipTransition: 2, Tip: []*workerv1.NamingTopologyLocator{{TopologyType: 1, LocalId: 1, SemanticRef: 1, Evidence: 1}, {TopologyType: 1, LocalId: 2, SemanticRef: 2, Evidence: 1}}}}}
	root, payloads := namingRootFixture(t, values, func(i int, b *workerv1.NamingHistoryBlock) {
		if i == 1 {
			b.Values.Transitions[0].Lineage = nil
			b.Layout = []*workerv1.NamingHistorySpan{{Carry: true, Count: 2}}
		}
	})
	sparse, err := unpackNamingRoot(t.Context(), root, fixtureHistoryLoader(payloads))
	if err != nil {
		t.Fatal(err)
	}
	full, err := unpackNamingValues(values)
	if err != nil {
		t.Fatal(err)
	}
	if len(sparse.FeatureResults[1].TopologyHistory.Lineage) != 0 {
		t.Fatal("query path expanded identity rows")
	}
	selection := testSelection()
	selection.Anchor = semanticRef(a)
	if got, want := resolveManifest(selection, "key", sparse), resolveManifestLinearOracle(selection, "key", full); !reflect.DeepEqual(got, want) {
		t.Fatal(got, want)
	}
	if !reflect.DeepEqual(deriveFeatureAssociations(sparse), deriveFeatureAssociations(full)) {
		t.Fatal("carried ownership changed")
	}
	query, err := unpackNamingRoot(t.Context(), root, func(ctx context.Context, ref *workerv1.NamingHistoryReference) (*decodedNamingBlock, error) {
		block, err := fixtureHistoryLoader(payloads)(ctx, ref)
		if err != nil {
			return nil, err
		}
		projected := namingBlockForQueries(block)
		if len(block.layout) == 0 || len(block.view.FeatureResults[0].TopologyHistory.Lineage) > 0 && block.view.FeatureResults[0].TopologyHistory.Lineage[0].Evidence == nil {
			t.Fatal("query projection mutated authoritative decoded values")
		}
		for _, row := range projected.view.FeatureResults[0].TopologyHistory.Lineage {
			if row.Evidence != nil {
				t.Fatal("query cache retained historical evidence")
			}
		}
		return projected, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resolveManifest(selection, "key", query), resolveManifestLinearOracle(selection, "key", full); !reflect.DeepEqual(got, want) || !reflect.DeepEqual(deriveFeatureAssociations(query), deriveFeatureAssociations(full)) {
		t.Fatal("query projection changed semantic resolution or display ownership", got, want)
	}
	if _, err := expandNamingHistory(t.Context(), query, 4); err == nil {
		t.Fatal("query projection pretended to be a full history")
	}
	expanded, err := expandNamingHistory(t.Context(), sparse, 4)
	if err != nil {
		t.Fatal(err)
	}
	for i := range expanded {
		if !proto.Equal(expanded[i], full.FeatureResults[i]) {
			t.Fatalf("full transition %d changed", i)
		}
	}
	if _, err = expandNamingHistory(t.Context(), sparse, 3); err == nil {
		t.Fatal("expansion ignored explicit budget")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = unpackNamingRoot(ctx, root, fixtureHistoryLoader(payloads)); err != context.Canceled {
		t.Fatal(err)
	}
	for _, mutate := range []func(*workerv1.NamingHistoryBlock){
		func(b *workerv1.NamingHistoryBlock) { b.Layout[0].Offset = ^uint64(0) },
		func(b *workerv1.NamingHistoryBlock) { b.Layout = append(b.Layout, b.Layout[0]); b.FullLineageCount = 4 },
		func(b *workerv1.NamingHistoryBlock) { b.PreviousLineageCount = 1 },
	} {
		badRoot, badFiles := namingRootFixture(t, values, func(i int, b *workerv1.NamingHistoryBlock) {
			if i == 1 {
				b.Values.Transitions[0].Lineage = nil
				b.Layout = []*workerv1.NamingHistorySpan{{Carry: true, Count: 2}}
				mutate(b)
			}
		})
		if _, err = unpackNamingRoot(t.Context(), badRoot, fixtureHistoryLoader(badFiles)); err == nil {
			t.Fatal("invalid carry accepted")
		}
	}
}
