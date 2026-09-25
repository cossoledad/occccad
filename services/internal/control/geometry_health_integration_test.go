package control

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/geometryrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Force concurrent RPCs onto one real process: a short RPC completing during
// STEP transfer must not cause its release probe to kill the busy Worker.
func TestGeometryWorkerRemainsHealthyDuringSTEPTransfer(t *testing.T) {
	binary, sourcePath := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER"), os.Getenv("OCCCCAD_TEST_EXCHANGE_STEP")
	if binary == "" || sourcePath == "" {
		t.Skip("requires real Worker and large STEP fixture")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	store, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.Put(ctx, artifact.KindExchangeSource, "application/step", file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	pool := NewGeometryPool(ctx, GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: store.Root(), LogDirectory: t.TempDir(), MinimumWorkers: 1, MaximumWorkers: 1, GeometryCapacity: 100})
	defer pool.Close()
	if err := pool.Start(); err != nil {
		t.Fatal(err)
	}
	worker := pool.workers[0]
	connection, err := grpc.NewClient(serveGeometry(t, pool), grpc.WithTransportCredentials(insecure.NewCredentials()), geometryrpc.ClientOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	client := workerv1.NewGeometryWorkerClient(connection)
	type result struct {
		response *workerv1.InspectExchangeResponse
		err      error
	}
	done := make(chan result, 1)
	go func() {
		response, err := client.InspectExchange(ctx, &workerv1.InspectExchangeRequest{RequestId: "busy-health", Format: "STEP", Source: &workerv1.ArtifactReference{Backend: "LOCAL", ObjectKey: source.Key, Sha256: source.SHA256, SizeBytes: uint64(source.Size), ContentType: source.ContentType}, ComponentOutputPrefix: "exchange/health/components"})
		done <- result{response, err}
	}()
	probes := 0
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case result := <-done:
			if result.err != nil {
				t.Fatal(result.err)
			}
			if len(result.response.GetGraph().GetDefinitions()) < 2 || probes < 5 {
				t.Fatalf("large fixture required: components=%d probes=%d", len(result.response.GetGraph().GetDefinitions()), probes)
			}
			t.Logf("STEP transfer completed with %d components and %d successful concurrent health probes", len(result.response.GetGraph().GetDefinitions()), probes)
			goto transferred
		case <-ticker.C:
			probeCtx, stop := context.WithTimeout(ctx, time.Second)
			_, err := worker.client.Ping(probeCtx, &workerv1.PingRequest{})
			stop()
			if err != nil {
				t.Fatalf("busy Worker Ping: %v", err)
			}
			if probes == 5 {
				shortCtx, stop := context.WithTimeout(ctx, 2*time.Second)
				_, err := client.SolveSketch(shortCtx, &workerv1.SolveSketchRequest{})
				stop()
				if status.Code(err) != codes.InvalidArgument {
					t.Fatalf("short concurrent RPC: %v", err)
				}
			}
			probes++
		}
	}
transferred:
	pool.mu.Lock()
	retained := pool.registeredLocked(worker)
	pool.mu.Unlock()
	if !retained {
		t.Fatal("busy Worker was replaced")
	}
	// Actual process death must still cause immediate replacement, not wait for
	// three requests, and the former owner must not survive in routing state.
	_, reserved, err := pool.selectClient("crash-owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.process.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-worker.process.done
	pool.release(reserved, "crash-owner", false)
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if len(pool.workers) != 1 || pool.workers[0] == worker || !pool.workers[0].process.Running() || pool.affinity["crash-owner"] != nil {
		t.Fatal("dead Worker recovery did not replace process and clear owner")
	}
}
