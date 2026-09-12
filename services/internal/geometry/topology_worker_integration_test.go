package geometry

import (
	"context"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func openCppWorkerForNamingContract(t *testing.T) *Client {
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
		client, err = Open(address)
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
	manifest := response.GetEvaluationManifest()
	if manifest.GetTopologyPolicyId() != topologyNamingPolicyProto().GetPolicyId() ||
		manifest.GetTopologyEvaluatorVersion() != topologyNamingPolicyProto().GetEvaluatorVersion() ||
		len(manifest.GetFeatures()) != 1 || manifest.GetFeatures()[0].GetFeatureId() != "pad-1" {
		t.Fatalf("Worker dropped the naming contract: %#v", manifest)
	}

	_, err = client.worker.EvaluatePart(t.Context(), &workerv1.EvaluatePartRequest{
		RequestId: "naming-invalid", GeometryKey: "naming-invalid-key", ProfilePads: profilePadsProto([]ProfilePad{pad}),
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("missing naming policy must be rejected, got %v", err)
	}
}
