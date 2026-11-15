//go:build integration

package geometry

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func openCppWorkerForNamingContract(t *testing.T, options ...grpc.DialOption) *Client {
	t.Helper()
	client, closeWorker, err := OpenDevelopmentWorker(t.Context(), options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeWorker)
	return client
}

// Inspect the actual Worker files. A root's empty inline transition array must
// not let history-completeness assertions silently become vacuous.
func readWorkerNamingArtifact(t *testing.T, directory string, response *workerv1.EvaluatePartResponse) (*workerv1.PartTopologyManifest, []*workerv1.NamingHistoryBlock) {
	t.Helper()
	read := func(ref *workerv1.ArtifactReference) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(directory, ref.GetObjectKey()))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if uint64(len(data)) != ref.GetSizeBytes() || hex.EncodeToString(sum[:]) != ref.GetSha256() {
			t.Fatal("Worker artifact integrity mismatch")
		}
		return data
	}
	root := &workerv1.PartTopologyManifest{}
	if err := proto.Unmarshal(read(response.GetEvaluationManifest().GetTopologyManifestArtifact()), root); err != nil {
		t.Fatal(err)
	}
	if root.SchemaVersion != 3 || len(root.Transitions) != 0 || len(root.History) == 0 {
		t.Fatal("invalid Naming root")
	}
	objects := map[string]*workerv1.ArtifactReference{}
	for _, ref := range response.GetEvaluationManifest().GetNamingHistoryArtifacts() {
		objects[ref.Sha256] = ref
	}
	var blocks []*workerv1.NamingHistoryBlock
	previous := ""
	var rows uint64
	for _, ref := range root.History {
		object := objects[ref.Sha256]
		if object == nil || object.SizeBytes != ref.SizeBytes {
			t.Fatal("missing Naming history output")
		}
		block := &workerv1.NamingHistoryBlock{}
		if err := proto.Unmarshal(read(object), block); err != nil {
			t.Fatal(err)
		}
		if block.SchemaVersion != 1 || block.PreviousSha256 != previous || block.PreviousLineageCount != rows || block.FullLineageCount != ref.FullLineageCount || len(block.GetValues().GetTransitions()) != 1 {
			t.Fatal("invalid Naming history chain")
		}
		stage := block.Values.Transitions[0]
		if stage.FeatureId != ref.FeatureId || stage.ResultGeometryId != ref.ResultGeometryId || !stage.Complete || block.Values.PolicyDigest != root.PolicyDigest {
			t.Fatal("invalid Naming history stage")
		}
		previous, rows = ref.Sha256, ref.FullLineageCount
		blocks = append(blocks, block)
	}
	return root, blocks
}

func TestCppWorkerAcceptsPartNamingContract(t *testing.T) {
	artifactRoot := os.Getenv("OCCCCAD_DATA_DIR")
	client := openCppWorkerForNamingContract(t)
	pad := ProfilePad{
		FeatureID: "pad-1", BodyID: "body-main", ProfileFeatureID: "sketch-1",
		Length: 20, Plane: "XY", BodyOperation: "ADD", Generator: "LINEAR_EXTRUDE",
		Regions: []ProfileRegion{{
			ID: "region-1",
			Outer: ProfileLoop{ID: "outer", Curves: []ProfileCurve{
				{EntityID: "edge-1", Kind: "LINE", Start: [2]float64{0, 0}, End: [2]float64{10, 0}},
				{EntityID: "edge-2", Kind: "LINE", Start: [2]float64{10, 0}, End: [2]float64{10, 10}},
				{EntityID: "edge-3", Kind: "LINE", Start: [2]float64{10, 10}, End: [2]float64{0, 10}},
				{EntityID: "edge-4", Kind: "LINE", Start: [2]float64{0, 10}, End: [2]float64{0, 0}},
			}},
		}},
	}

	response, err := client.EvaluateProfilePart(t.Context(), "naming-valid", "naming-valid-key", []ProfilePad{pad}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.GetOcctVersion() != "7.9.1" {
		t.Fatalf("topology-history conformance requires OCCT 7.9.1, got %q", response.GetOcctVersion())
	}
	manifest := response.GetEvaluationManifest()
	if manifest.GetTopologyPolicyId() != topologyNamingPolicyProto().GetPolicyId() ||
		manifest.GetTopologyEvaluatorVersion() != topologyNamingPolicyProto().GetEvaluatorVersion() ||
		manifest.GetTopologyPolicyDigest() != topologyNamingPolicyProto().GetPolicyDigest() ||
		len(manifest.GetFeatures()) != 1 || manifest.GetFeatures()[0].GetFeatureId() != "pad-1" {
		t.Fatalf("Worker dropped the naming contract: %#v", manifest)
	}

	external, err := client.EvaluateProfilePartFromArtifact(t.Context(), "naming-artifact", "naming-artifact-key",
		[]ProfilePad{pad}, ArtifactReference{}, "parts/naming.brep", "parts/naming.glb")
	if err != nil {
		t.Fatal(err)
	}
	if external.GetPreviewMesh() != nil || external.GetRepresentationKind() != "PERSISTENT" || proto.Size(external) > 16384 {
		t.Fatal("persistent RPC leaked large data")
	}
	reference := external.GetEvaluationManifest().GetTopologyManifestArtifact()
	if reference.GetObjectKey() == "" || reference.GetSha256() == "" || reference.GetContentType() != "application/vnd.occccad.topology-manifest.v3+protobuf" {
		t.Fatalf("topology artifact reference = %#v", reference)
	}
	persisted, blocks := readWorkerNamingArtifact(t, artifactRoot, external)
	if len(blocks) != 1 || len(persisted.GetBodies()[0].GetTip()) != 26 || len(blocks[0].Values.Transitions[0].Lineage) != 26 {
		t.Fatal("Worker omitted Linear Extrude topology history")
	}
	for _, output := range persisted.GetBodies()[0].GetTip() {
		if output.GetTopologyType() == workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_UNSPECIFIED ||
			output.GetLocalId() == 0 || persisted.Evidence[output.GetEvidence()-1].GetEvidenceDigest() == "" || len(persisted.Evidence[output.GetEvidence()-1].GetAdjacent()) == 0 {
			t.Fatalf("invalid semantic output evidence: %#v", output)
		}
	}

	if blocks[0].Values.Transitions[0].GetEvidenceDigest() == "" {
		t.Fatalf("serialized topology manifest lost feature history: %#v", &persisted)
	}
	if persisted.GetPolicyDigest() != modelcore.TopologyNamingPolicyDigest {
		t.Fatalf("serialized topology manifest lost policy digest: %#v", &persisted)
	}

	_, err = client.worker.EvaluatePart(t.Context(), &workerv1.EvaluatePartRequest{
		RequestId: "naming-invalid", GeometryKey: "naming-invalid-key", ProfilePads: profilePadsProto([]ProfilePad{pad}),
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing naming policy must be rejected, got %v", err)
	}
	invalidPolicy := topologyNamingPolicyProto()
	invalidPolicy.PolicyDigest = "sha256:wrong-policy"
	_, err = client.worker.EvaluatePart(t.Context(), &workerv1.EvaluatePartRequest{
		RequestId: "naming-invalid-digest", GeometryKey: "naming-invalid-digest-key",
		ProfilePads: profilePadsProto([]ProfilePad{pad}), TopologyPolicy: invalidPolicy,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("mismatched naming policy digest must be rejected, got %v", err)
	}
}
