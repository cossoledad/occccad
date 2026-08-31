package geometry

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestCppWorkerSolvesSimpleProductAssembly(t *testing.T) {
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Skip("set OCCCCAD_TEST_GEOMETRY_WORKER to run the C++ assembly RPC integration")
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
	identity := AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}
	result, err := client.SolveAssembly(t.Context(), "product-smoke", []AssemblyBody{
		{ID: "ground", Pose: identity},
		{ID: "moving", Pose: AssemblyPose{Translation: [3]float64{25, 0, 0}, Rotation: identity.Rotation}},
	}, []AssemblyGeometry{
		{ID: "ground-origin", BodyID: "ground", Kind: "POINT"},
		{ID: "moving-origin", BodyID: "moving", Kind: "POINT"},
	}, []AssemblyConstraint{
		{ID: "fix", Kind: "FIX", FirstBodyID: "ground", FixedPose: &identity},
		{ID: "coincident", Kind: "COINCIDENT", FirstBodyID: "moving", FirstGeometryID: "moving-origin", SecondBodyID: "ground", SecondGeometryID: "ground-origin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "CONVERGED" {
		t.Fatalf("unexpected solve: %s %s", result.Status, result.Diagnostic)
	}
	for _, body := range result.Bodies {
		if body.ID == "moving" && (body.Pose.Translation[0] > 1e-6 || body.Pose.Translation[0] < -1e-6) {
			t.Fatal(fmt.Sprintf("moving body was not solved to origin: %#v", body.Pose))
		}
	}
}
