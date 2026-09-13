package workspace

import (
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

func testRef(feature, slot string) *workerv1.SemanticTopologyRef {
	return &workerv1.SemanticTopologyRef{FeatureId: feature, OutputSlot: slot}
}
func testOutput(ref *workerv1.SemanticTopologyRef, localID uint64, topologyType workerv1.PersistentTopologyType) *workerv1.SemanticTopologyOutput {
	return &workerv1.SemanticTopologyOutput{SemanticRef: ref, LocalId: localID, TopologyType: topologyType, Evidence: &workerv1.SelectionEvidence{GeometryType: "PLANE", EvidenceDigest: "evidence"}}
}
func testSelection() modelcore.PersistentSelection {
	return modelcore.PersistentSelection{SchemaVersion: 1, SourceDocumentID: "part-1", SourceBodyID: "body-main", Anchor: modelcore.SemanticTopologyRef{FeatureID: "extrude-1", OutputSlot: "END_CAP/region-1"}, ExpectedType: modelcore.PersistentTopologyFace, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}}
}

func typedTestSelection(topologyType modelcore.PersistentTopologyType, slot, geometryType string) modelcore.PersistentSelection {
	selection := testSelection()
	selection.Anchor.OutputSlot = slot
	selection.ExpectedType = topologyType
	selection.CreationEvidence.GeometryType = geometryType
	return selection
}

func TestResolveManifestPreservesExtrudeSelectionAcrossLengthEdit(t *testing.T) {
	ref := testRef("extrude-1", "END_CAP/region-1")
	manifest := &workerv1.PartTopologyManifest{FeatureResults: []*workerv1.FeatureResult{{FeatureId: "extrude-1", BodyId: "body-main", ResultGeometryId: "geometry-40", SemanticOutputs: []*workerv1.SemanticTopologyOutput{testOutput(ref, 6, workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE)}, TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Sources: []*workerv1.SemanticTopologyRef{testRef("sketch-1", "PROFILE_REGION/region-1")}, Result: ref}}}}}}
	resolution := resolveManifest(testSelection(), "geometry-key-40", manifest)
	if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 || resolution.Candidates[0].LocalID != 6 {
		t.Fatalf("resolution = %#v", resolution)
	}
}

func TestResolveManifestReportsDeletedAndSplitWithoutChoosing(t *testing.T) {
	anchor := testRef("extrude-1", "END_CAP/region-1")
	deleted := &workerv1.PartTopologyManifest{FeatureResults: []*workerv1.FeatureResult{{FeatureId: "cut-1", BodyId: "body-main", TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Sources: []*workerv1.SemanticTopologyRef{anchor}, Result: testRef("cut-1", "kept")}}, Deleted: []*workerv1.TopologyTombstone{{Source: anchor}}}}}}
	if got := resolveManifest(testSelection(), "deleted", deleted); got.Status != modelcore.SelectionMissing {
		t.Fatalf("deleted status = %s", got.Status)
	}

	left, right := testRef("cut-1", "split/1"), testRef("cut-1", "split/2")
	split := &workerv1.PartTopologyManifest{FeatureResults: []*workerv1.FeatureResult{{FeatureId: "cut-1", BodyId: "body-main", ResultGeometryId: "split-geometry", SemanticOutputs: []*workerv1.SemanticTopologyOutput{testOutput(left, 4, workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE), testOutput(right, 5, workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE)}, TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{Sources: []*workerv1.SemanticTopologyRef{anchor}, Result: left}, {Sources: []*workerv1.SemanticTopologyRef{anchor}, Result: right}}}}}}
	if got := resolveManifest(testSelection(), "split", split); got.Status != modelcore.SelectionAmbiguous || len(got.Candidates) != 2 {
		t.Fatalf("split resolution = %#v", got)
	}
}

func TestResolveManifestReportsTypeMismatchAndOutsideTip(t *testing.T) {
	anchor := testRef("extrude-1", "END_CAP/region-1")
	manifest := &workerv1.PartTopologyManifest{FeatureResults: []*workerv1.FeatureResult{{FeatureId: "extrude-1", BodyId: "body-main", SemanticOutputs: []*workerv1.SemanticTopologyOutput{testOutput(anchor, 2, workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_EDGE)}}}}
	if got := resolveManifest(testSelection(), "type", manifest); got.Status != modelcore.SelectionTypeMismatch {
		t.Fatalf("type status = %s", got.Status)
	}
	selection := testSelection()
	selection.Anchor.FeatureID = "removed-feature"
	if got := resolveManifest(selection, "outside", manifest); got.Status != modelcore.SelectionOutsideCurrentTip {
		t.Fatalf("outside status = %s", got.Status)
	}
}

func TestResolveManifestPreservesEdgeAndVertexAcrossFeatureEdit(t *testing.T) {
	tests := []struct {
		name         string
		topologyType modelcore.PersistentTopologyType
		protoType    workerv1.PersistentTopologyType
		slot         string
		geometryType string
		localID      uint64
	}{
		{name: "linear edge", topologyType: modelcore.PersistentTopologyEdge,
			protoType: workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_EDGE,
			slot:      "END_BOUNDARY_FROM_PROFILE_EDGE/line-1", geometryType: "LINE", localID: 9},
		{name: "vertex", topologyType: modelcore.PersistentTopologyVertex,
			protoType: workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_VERTEX,
			slot:      "END_VERTEX_FROM_PROFILE_ENDPOINTS/point-1", geometryType: "POINT", localID: 5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection := typedTestSelection(test.topologyType, test.slot, test.geometryType)
			ref := testRef(selection.Anchor.FeatureID, selection.Anchor.OutputSlot)
			output := testOutput(ref, test.localID, test.protoType)
			output.Evidence.GeometryType = test.geometryType
			manifest := &workerv1.PartTopologyManifest{FeatureResults: []*workerv1.FeatureResult{{
				FeatureId: "extrude-1", BodyId: "body-main", ResultGeometryId: "edited-geometry",
				SemanticOutputs: []*workerv1.SemanticTopologyOutput{output},
				TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{{
					Sources: []*workerv1.SemanticTopologyRef{testRef("sketch-1", "PROFILE_REGION/region-1")}, Result: ref,
				}}},
			}}}
			got := resolveManifest(selection, "edited-key", manifest)
			if got.Status != modelcore.SelectionResolved || len(got.Candidates) != 1 ||
				got.Candidates[0].Type != test.topologyType || got.Candidates[0].LocalID != test.localID {
				t.Fatalf("resolution = %#v", got)
			}
		})
	}
}

func TestResolveManifestReportsEdgeSplitAndVertexDeletion(t *testing.T) {
	edge := typedTestSelection(modelcore.PersistentTopologyEdge,
		"END_BOUNDARY_FROM_PROFILE_EDGE/line-1", "LINE")
	left, right := testRef("cut-1", "BOUNDARY_SPLIT/left"), testRef("cut-1", "BOUNDARY_SPLIT/right")
	leftOutput := testOutput(left, 4, workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_EDGE)
	rightOutput := testOutput(right, 5, workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_EDGE)
	leftOutput.Evidence.GeometryType, rightOutput.Evidence.GeometryType = "LINE", "LINE"
	split := &workerv1.PartTopologyManifest{FeatureResults: []*workerv1.FeatureResult{{
		FeatureId: "cut-1", BodyId: "body-main", ResultGeometryId: "split-geometry",
		SemanticOutputs: []*workerv1.SemanticTopologyOutput{leftOutput, rightOutput},
		TopologyHistory: &workerv1.TopologyHistory{Lineage: []*workerv1.TopologyLineage{
			{Sources: []*workerv1.SemanticTopologyRef{testRef(edge.Anchor.FeatureID, edge.Anchor.OutputSlot)}, Result: left},
			{Sources: []*workerv1.SemanticTopologyRef{testRef(edge.Anchor.FeatureID, edge.Anchor.OutputSlot)}, Result: right},
		}},
	}}}
	if got := resolveManifest(edge, "split", split); got.Status != modelcore.SelectionAmbiguous || len(got.Candidates) != 2 {
		t.Fatalf("edge split resolution = %#v", got)
	}

	vertex := typedTestSelection(modelcore.PersistentTopologyVertex,
		"END_VERTEX_FROM_PROFILE_ENDPOINTS/point-1", "POINT")
	deleted := &workerv1.PartTopologyManifest{FeatureResults: []*workerv1.FeatureResult{{
		FeatureId: "cut-1", BodyId: "body-main", TopologyHistory: &workerv1.TopologyHistory{
			Lineage: []*workerv1.TopologyLineage{{Sources: []*workerv1.SemanticTopologyRef{
				testRef(vertex.Anchor.FeatureID, vertex.Anchor.OutputSlot)}, Result: testRef("cut-1", "unrelated")}},
			Deleted: []*workerv1.TopologyTombstone{{Source: testRef(vertex.Anchor.FeatureID, vertex.Anchor.OutputSlot)}},
		},
	}}}
	if got := resolveManifest(vertex, "deleted", deleted); got.Status != modelcore.SelectionMissing {
		t.Fatalf("vertex deletion status = %s", got.Status)
	}
}

func TestTopologyHistoryCompleteRequiresEveryFeature(t *testing.T) {
	if topologyHistoryComplete(&workerv1.PartTopologyManifest{}) {
		t.Fatal("empty manifest was accepted as complete")
	}
	manifest := &workerv1.PartTopologyManifest{FeatureResults: []*workerv1.FeatureResult{
		{FeatureId: "pad", TopologyHistoryComplete: true},
		{FeatureId: "cut", TopologyHistoryComplete: false},
	}}
	if topologyHistoryComplete(manifest) {
		t.Fatal("incomplete downstream history was accepted")
	}
	manifest.FeatureResults[1].TopologyHistoryComplete = true
	if !topologyHistoryComplete(manifest) {
		t.Fatal("complete feature chain was rejected")
	}
}
