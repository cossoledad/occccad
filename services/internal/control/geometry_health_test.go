package control

import (
	"context"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type probeClient struct {
	workerv1.GeometryWorkerClient
	err error
}

func (client *probeClient) Ping(context.Context, *workerv1.PingRequest, ...grpc.CallOption) (*workerv1.PingResponse, error) {
	return &workerv1.PingResponse{ResidentGeometryCount: 7}, client.err
}

func TestGeometryPoolBalancesBusyWorkersWithLargeCacheCapacity(t *testing.T) {
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{GeometryCapacity: 100, MaximumWorkers: 2})
	first := &workerInstance{id: "first", inFlight: 3, known: map[string]bool{}}
	second := &workerInstance{id: "second", known: map[string]bool{}}
	pool.workers = []*workerInstance{first, second}
	_, selected, err := pool.selectClient("new")
	if err != nil || selected != second {
		t.Fatalf("did not use idle worker: %v %v", selected, err)
	}
	_, selected, err = pool.selectClient("other")
	if err != nil || selected != second {
		t.Fatalf("did not use least busy worker at limit: %v %v", selected, err)
	}
	pool.affinity["owned"] = first
	_, selected, err = pool.selectClient("owned")
	if err != nil || selected != first {
		t.Fatal("lost immutable geometry affinity")
	}
}

func TestGeometryPoolTransientProbeFailureDoesNotEvictWorker(t *testing.T) {
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{GeometryCapacity: 100, MaximumWorkers: 1})
	client := &probeClient{err: status.Error(codes.DeadlineExceeded, "busy host")}
	worker := &workerInstance{id: "worker", client: client, known: map[string]bool{}}
	pool.workers = []*workerInstance{worker}
	for count := 1; count <= 2; count++ {
		worker.inFlight++
		pool.release(worker, "body", true)
		if len(pool.workers) != 1 || worker.failedProbes != count {
			t.Fatal("transient probe evicted worker")
		}
	}
	client.err = nil
	worker.inFlight++
	pool.release(worker, "body", true)
	if worker.failedProbes != 0 || worker.resident != 7 {
		t.Fatal("successful probe did not reset failure state")
	}
	client.err = status.Error(codes.DeadlineExceeded, "busy host again")
	worker.inFlight++
	pool.release(worker, "body", true)
	if worker.failedProbes != 1 {
		t.Fatal("failures must be consecutive")
	}
}

func TestGeometryPoolLateCompletionCannotResurrectEvictedOwner(t *testing.T) {
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{})
	evicted := &workerInstance{id: "evicted", inFlight: 1, known: map[string]bool{}}
	pool.release(evicted, "late-key", true)
	pool.remember(evicted, "late-id")
	if len(pool.affinity) != 0 || len(evicted.known) != 0 {
		t.Fatal("late completion resurrected evicted owner")
	}
}

func TestGeometryPoolRepeatedProbeFailuresEvictOnlyFailedWorker(t *testing.T) {
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{MinimumWorkers: 1, MaximumWorkers: 2})
	connection, err := grpc.NewClient("127.0.0.1:1", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	worker := &workerInstance{id: "failed", client: &probeClient{err: status.Error(codes.Unavailable, "unreachable")}, connection: connection, known: map[string]bool{}}
	healthy := &workerInstance{id: "healthy", known: map[string]bool{}}
	pool.workers = []*workerInstance{worker, healthy}
	pool.affinity["body"] = worker
	for count := 0; count < 3; count++ {
		worker.inFlight++
		pool.release(worker, "", false)
	}
	if len(pool.workers) != 1 || pool.workers[0] != healthy || pool.affinity["body"] != nil {
		t.Fatal("persistent failure did not evict failed owner independently")
	}
}
