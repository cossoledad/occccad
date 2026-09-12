package geometry

import (
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
)

func TestProfilePadProtoCarriesStableNamingIdentityAndPolicy(t *testing.T) {
	t.Parallel()
	pads := profilePadsProto([]ProfilePad{{
		FeatureID: "extrude-2", BodyID: "body-main", InputFeatureID: "extrude-1",
		ProfileFeatureID: "sketch-2", Regions: []ProfileRegion{{ID: "region-2"}},
	}})
	if len(pads) != 1 || pads[0].GetFeatureId() != "extrude-2" || pads[0].GetBodyId() != "body-main" ||
		pads[0].GetInputFeatureId() != "extrude-1" || pads[0].GetProfileFeatureId() != "sketch-2" {
		t.Fatalf("stable feature identity was lost at the Proto boundary: %#v", pads)
	}
	policy := topologyNamingPolicyProto()
	if policy.GetSchemaVersion() != modelcore.TopologyNamingSchemaVersion ||
		policy.GetPolicyId() != modelcore.TopologyNamingPolicyID ||
		policy.GetEvaluatorVersion() != modelcore.TopologyNamingEvaluator ||
		policy.GetLinearToleranceMeters() <= 0 || policy.GetAngularToleranceRadians() <= 0 {
		t.Fatalf("topology naming policy is incomplete: %#v", policy)
	}
}

func TestPersistentSelectionAndTopologyHistoryProtoRoundTrip(t *testing.T) {
	t.Parallel()
	measure := 0.0025
	selection := &workerv1.PersistentSelection{
		SchemaVersion:    modelcore.TopologyNamingSchemaVersion,
		SourceDocumentId: "part-1",
		SourceBodyId:     "body-main",
		Anchor: &workerv1.SemanticTopologyRef{
			FeatureId: "pad-1", OutputSlot: "generated-face", SourceIds: []string{"sketch-edge-2"},
		},
		ExpectedType: workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE,
		Selector: &workerv1.SelectionRecipe{
			Kind: workerv1.SelectionRecipeKind_SELECTION_RECIPE_LINEAGE_DESCENDANT,
		},
		CreationEvidence: &workerv1.SelectionEvidence{
			GeometryType: "PLANE", MeasureSi: &measure, MeasureDimension: "AREA",
		},
	}
	history := &workerv1.TopologyHistory{
		SchemaVersion: 1, FeatureId: "cut-1", InputGeometryId: "before", ResultGeometryId: "after",
		Lineage: []*workerv1.TopologyLineage{{
			Sources: []*workerv1.SemanticTopologyRef{selection.Anchor},
			Result:  &workerv1.SemanticTopologyRef{FeatureId: "cut-1", OutputSlot: "modified-face"},
			Kind:    workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_SPLIT,
		}},
		Ambiguous: []*workerv1.AmbiguousLineage{{
			Sources: []*workerv1.SemanticTopologyRef{selection.Anchor},
			Candidates: []*workerv1.SemanticTopologyRef{
				{FeatureId: "cut-1", OutputSlot: "modified-face", SourceIds: []string{"candidate-a"}},
				{FeatureId: "cut-1", OutputSlot: "modified-face", SourceIds: []string{"candidate-b"}},
			},
			DiagnosticCode: "TOPOLOGY_SPLIT_AMBIGUOUS",
		}},
		PolicyDigest: "policy-digest", EvidenceDigest: "evidence-digest",
	}

	for name, message := range map[string]proto.Message{"selection": selection, "history": history} {
		encoded, err := proto.Marshal(message)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		clone := message.ProtoReflect().Type().New().Interface()
		if err := proto.Unmarshal(encoded, clone); err != nil {
			t.Fatalf("unmarshal %s: %v", name, err)
		}
		if !proto.Equal(message, clone) {
			t.Fatalf("%s contract changed during protobuf round trip", name)
		}
	}
}
