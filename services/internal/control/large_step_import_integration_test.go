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
	inspection, err := client.InspectExchange(t.Context(), "large-step-inspect", "STEP", ref, artifact.StagingKey("large-step-inspect", "definitions"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Definitions) == 0 {
		t.Fatal("no components")
	}
	largest := 0
	for index, component := range inspection.Definitions {
		if component.Kind != "PART" {
			continue
		}
		prefix := fmt.Sprintf("large-step-%d", index+1)
		response, err := client.ImportExchange(t.Context(), prefix, prefix, "BREP", geometry.ArtifactReference{Backend: component.Brep.Backend, ObjectKey: component.Brep.ObjectKey, SHA256: component.Brep.Sha256, Size: int64(component.Brep.SizeBytes), ContentType: component.Brep.ContentType}, "", artifact.StagingKey(prefix, "shape.brep"), artifact.StagingKey(prefix, "mesh.glb"))
		if err != nil {
			t.Fatalf("component %d: %v", index+1, err)
		}
		size := proto.Size(response)
		largest = max(largest, size)
		if size > geometryrpc.MaxMessageBytes || response.GetVolume() <= 0 || response.GetTriangleCount() == 0 {
			t.Fatalf("invalid component %d result", index+1)
		}
		if response.GetPreviewMesh() != nil || response.GetRepresentationKind() != "PERSISTENT" {
			t.Fatal("files leaked into gRPC response")
		}
		for _, key := range []string{response.GetBrepArtifact().GetObjectKey(), response.GetGlbArtifact().GetObjectKey()} {
			r, err := local.Open(t.Context(), key)
			if err != nil {
				t.Fatal(err)
			}
			r.Close()
		}
		t.Logf("component=%d response_bytes=%d vertices=%d triangles=%d solids=%d", index+1, size, response.GetDisplayVertexCount(), response.GetTriangleCount(), response.GetTopology().GetSolidCount())
	}
	if largest > 64<<10 {
		t.Fatalf("persistent response is not bounded: largest=%d", largest)
	}
	t.Logf("imported %d components through %s and managed Router; largest response=%d bytes", len(inspection.Definitions), store.Backend(), largest)
}
