package control

import (
	"bytes"
	"context"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/geometryrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type largeMeshWorker struct {
	workerv1.UnimplementedGeometryWorkerServer
}

func (largeMeshWorker) Ping(context.Context, *workerv1.PingRequest) (*workerv1.PingResponse, error) {
	return &workerv1.PingResponse{WorkerId: "large-mesh"}, nil
}
func (largeMeshWorker) EvaluatePart(_ context.Context, r *workerv1.EvaluatePartRequest) (*workerv1.EvaluatePartResponse, error) {
	if len(r.BaseBrepData) != 5<<20 || r.BaseBrepData[0] != 7 {
		return nil, status.Error(codes.InvalidArgument, "large request truncated")
	}
	mesh := &workerv1.Mesh{}
	for i := 0; i < 200000; i++ {
		mesh.Vertices = append(mesh.Vertices, &workerv1.Vec3{X: 1, Y: 2, Z: 3})
	}
	return &workerv1.EvaluatePartResponse{GeometryKey: r.GeometryKey, Mesh: mesh}, nil
}
func TestGeometryRouterLargeRequestAndMeshResponse(t *testing.T) {
	backend := serveGeometry(t, largeMeshWorker{})
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{})
	t.Cleanup(pool.Close)
	if err := pool.SetDebugAddress(backend); err != nil {
		t.Fatal(err)
	}
	router := serveGeometry(t, pool)
	client, err := geometry.Open(router)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	payload := bytes.Repeat([]byte{7}, 5<<20)
	response, err := client.EvaluatePart(t.Context(), "large-request", "large-result", nil, payload)
	if err != nil {
		t.Fatal(err)
	}
	size := proto.Size(response)
	if size <= 4<<20 || size >= geometryrpc.MaxMessageBytes || len(response.GetMesh().GetVertices()) != 200000 {
		t.Fatalf("unexpected response size or mesh: %d", size)
	}
	// Demonstrate this payload still fails for an unconfigured 4 MiB receiver.
	connection, err := grpc.NewClient(router, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	_, err = workerv1.NewGeometryWorkerClient(connection).EvaluatePart(t.Context(), &workerv1.EvaluatePartRequest{BaseBrepData: payload})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected old default to reject response, got %v", err)
	}
}
