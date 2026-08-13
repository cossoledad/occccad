package control

import (
	"context"
	"net"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type sketchWorkerStub struct {
	workerv1.UnimplementedGeometryWorkerServer
}

func (sketchWorkerStub) SolveSketch(_ context.Context, request *workerv1.SolveSketchRequest) (*workerv1.SolveSketchResponse, error) {
	return &workerv1.SolveSketchResponse{
		Sketch:           request.GetSketch(),
		Status:           "UNDER_CONSTRAINED",
		DegreesOfFreedom: 4,
	}, nil
}

func serveGeometry(t *testing.T, service workerv1.GeometryWorkerServer) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	workerv1.RegisterGeometryWorkerServer(server, service)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	return listener.Addr().String()
}

func TestGeometryPoolRoutesSolveSketch(t *testing.T) {
	backend := serveGeometry(t, sketchWorkerStub{})
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{})
	if err := pool.SetDebugAddress(backend); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	router := serveGeometry(t, pool)
	connection, err := grpc.NewClient(router, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	response, err := workerv1.NewGeometryWorkerClient(connection).SolveSketch(t.Context(), &workerv1.SolveSketchRequest{
		RequestId: "request-1",
		Sketch:    &workerv1.SketchModel{SchemaVersion: 1},
	})
	if err != nil {
		t.Fatalf("SolveSketch was not routed: %v", err)
	}
	if response.GetStatus() != "UNDER_CONSTRAINED" || response.GetDegreesOfFreedom() != 4 {
		t.Fatalf("unexpected routed response: %#v", response)
	}
}
