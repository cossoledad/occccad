package geometry

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func openCppWorkerForNamingContract(t *testing.T, options ...grpc.DialOption) *Client {
	t.Helper()
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Skip("set OCCCCAD_TEST_GEOMETRY_WORKER to run the C++ naming contract integration")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	command := exec.Command(binary)
	command.Env = append(os.Environ(), "OCCCCAD_GEOMETRY_WORKER_LISTEN="+address)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _, _ = command.Process.Wait() })

	var client *Client
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		client, err = Open(address, options...)
		if err == nil {
			pingContext, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			_, err = client.Ping(pingContext)
			cancel()
			if err == nil {
				break
			}
			_ = client.Close()
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("worker did not become ready: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestCppWorkerAcceptsPartNamingContract(t *testing.T) {
	artifactRoot := t.TempDir()
	t.Setenv("OCCCCAD_DATA_DIR", artifactRoot)
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
	if reference.GetObjectKey() == "" || reference.GetSha256() == "" || reference.GetContentType() != "application/vnd.occccad.topology-manifest.v1+protobuf" {
		t.Fatalf("topology artifact reference = %#v", reference)
	}
	data, err := os.ReadFile(filepath.Join(artifactRoot, reference.GetObjectKey()))
	if err != nil {
		t.Fatal(err)
	}
	var persisted workerv1.PartTopologyManifest
	if err := proto.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.GetFeatureResults()) != 1 || len(persisted.GetFeatureResults()[0].GetSemanticOutputs()) != 26 ||
		len(persisted.GetFeatureResults()[0].GetTopologyHistory().GetLineage()) != 26 ||
		!persisted.GetFeatureResults()[0].GetTopologyHistoryComplete() {
		t.Fatalf("Worker omitted Linear Extrude topology history: %#v", persisted.GetFeatureResults())
	}
	for _, output := range persisted.GetFeatureResults()[0].GetSemanticOutputs() {
		if output.GetTopologyType() == workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_UNSPECIFIED ||
			output.GetLocalId() == 0 || output.GetEvidence().GetEvidenceDigest() == "" || len(output.GetEvidence().GetAdjacent()) == 0 {
			t.Fatalf("invalid semantic output evidence: %#v", output)
		}
	}

	if len(persisted.GetFeatureResults()) != 1 || persisted.GetFeatureResults()[0].GetTopologyHistory().GetEvidenceDigest() == "" {
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
