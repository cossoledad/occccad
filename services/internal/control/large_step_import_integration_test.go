package control

import (
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/geometryrpc"
	"google.golang.org/protobuf/proto"
)

func TestLargeSTEPImportThroughManagedRouter(t *testing.T) {
	binary, sourcePath := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER"), os.Getenv("OCCCCAD_TEST_EXCHANGE_STEP")
	if binary == "" || sourcePath == "" {
		t.Skip("requires Worker binary and STEP corpus path")
	}
	if _, err := config.LoadProjectEnv(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	root := t.TempDir()
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: root, LogDirectory: t.TempDir(), MinimumWorkers: 1, MaximumWorkers: 1})
	t.Cleanup(pool.Close)
	if err := pool.Start(); err != nil {
		t.Fatal(err)
	}
	store, local, err := artifact.OpenConfigured(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	client, err := geometry.Open(serveGeometry(t, pool), geometry.ArtifactStaging(store, local))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	file, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.Put(t.Context(), artifact.KindExchangeSource, "application/step", file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Content-addressed source may already belong to a user's import: never delete.
	ref := geometry.ArtifactReference{Backend: store.Backend(), ObjectKey: source.Key, SHA256: source.SHA256, Size: source.Size, ContentType: source.ContentType}
	inspection, err := client.InspectExchange(t.Context(), "large-step-inspect", "STEP", ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Components) == 0 {
		t.Fatal("no components")
	}
	largest := 0
	for _, component := range inspection.Components {
		prefix := fmt.Sprintf("large-step-%d", component.SourceIndex)
		response, err := client.ImportExchange(t.Context(), prefix, prefix, "STEP", ref, component.SourceIndex, artifact.StagingKey(prefix, "shape.brep"), artifact.StagingKey(prefix, "mesh.glb"))
		if err != nil {
			t.Fatalf("component %d: %v", component.SourceIndex, err)
		}
		size := proto.Size(response)
		largest = max(largest, size)
		if size > geometryrpc.MaxMessageBytes || response.GetVolume() <= 0 || len(response.GetMesh().GetTriangles()) == 0 {
			t.Fatalf("invalid component %d result", component.SourceIndex)
		}
		if len(response.GetBrepData()) != 0 || len(response.GetGlbData()) != 0 {
			t.Fatal("files leaked into gRPC response")
		}
		for _, key := range []string{response.GetBrepArtifact().GetObjectKey(), response.GetGlbArtifact().GetObjectKey()} {
			r, err := local.Open(t.Context(), key)
			if err != nil {
				t.Fatal(err)
			}
			r.Close()
		}
		t.Logf("component=%d response_bytes=%d vertices=%d triangles=%d solids=%d", component.SourceIndex, size, len(response.GetMesh().GetVertices()), len(response.GetMesh().GetTriangles()), response.GetTopology().GetSolidCount())
	}
	if largest <= 4<<20 {
		t.Fatalf("corpus did not exercise old receive limit: largest=%d", largest)
	}
	t.Logf("imported %d components through %s and managed Router; largest response=%d bytes", len(inspection.Components), store.Backend(), largest)
}
