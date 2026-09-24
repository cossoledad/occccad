package geometry

import (
	"bytes"
	"context"
	"errors"
	"github.com/occccad/occccad/internal/config"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/artifact"
	"google.golang.org/grpc"
)

type remoteTestStore struct{ *artifact.LocalStore }

func (remoteTestStore) Backend() string { return "TEST" }
func TestArtifactStagingNestedReferencesAndCleanup(t *testing.T) {
	source, _ := artifact.NewLocalStore(t.TempDir())
	remote := remoteTestStore{source}
	local, _ := artifact.NewLocalStore(t.TempDir())
	ctx := context.Background()
	o, err := source.Put(ctx, artifact.KindBREP, "brep", strings.NewReader("a valid transport payload"))
	if err != nil {
		t.Fatal(err)
	}
	original := &workerv1.ArtifactReference{Backend: "TEST", ObjectKey: o.Key, Sha256: o.SHA256, SizeBytes: uint64(o.Size)}
	request := &workerv1.ExportExchangeRequest{Components: []*workerv1.ExchangeComponent{{Brep: original}, {Brep: original}}}
	failure := errors.New("worker failed")
	interceptor := artifactStagingInterceptor(remote, local)
	err = interceptor(ctx, "ExportExchange", request, nil, nil, func(ctx context.Context, _ string, req, reply any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		value := req.(*workerv1.ExportExchangeRequest)
		for _, c := range value.Components {
			if c.Brep.Backend != "LOCAL" {
				t.Fatal("not localized")
			}
			reader, err := local.Open(ctx, c.Brep.ObjectKey)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(reader)
			reader.Close()
			if string(data) != "a valid transport payload" {
				t.Fatal("incorrect payload")
			}
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if original.Backend != "TEST" || original.ObjectKey != o.Key {
		t.Fatal("mutated immutable caller reference")
	}
	entries, _ := os.ReadDir(filepath.Join(local.Root(), "exchange", "inputs"))
	if len(entries) != 0 {
		t.Fatal("RPC scratch leaked")
	}
	original.Sha256 = "bad"
	err = interceptor(ctx, "ExportExchange", request, nil, nil, func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		t.Fatal("corrupted input reached worker")
		return nil
	})
	if err == nil {
		t.Fatal("missing integrity validation")
	}
	entries, _ = os.ReadDir(filepath.Join(local.Root(), "exchange", "inputs"))
	if len(entries) != 0 {
		t.Fatal("failed validation scratch leaked")
	}
}

func TestS3WorkerExchangeRoundTrip(t *testing.T) {
	if os.Getenv("OCCCCAD_TEST_S3") != "1" {
		t.Skip("requires configured MinIO and C++ Worker")
	}
	if _, err := config.LoadProjectEnv(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("OCCCCAD_DATA_DIR", root)
	store, local, err := artifact.OpenConfigured(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	client := openCppWorkerForNamingContract(t, ArtifactStaging(store, local))
	evaluation, err := client.EvaluateRectangularPad(t.Context(), "s3-test", "s3-test-geometry", 0, 0, 13.7319+float64(time.Now().UnixNano()%1000000)/1000000, 22.1377, 19.1927, "XY")
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.Put(t.Context(), artifact.KindBREP, "application/vnd.opencascade.brep", bytes.NewReader(evaluation.GetBrepData()))
	if err != nil {
		t.Fatal(err)
	}
	// Generated model is reserved to this test; remove only its own object.
	defer store.Delete(context.Background(), source.Key)
	reference := ArtifactReference{Backend: store.Backend(), ObjectKey: source.Key, SHA256: source.SHA256, Size: source.Size, ContentType: source.ContentType}
	inspection, err := client.InspectExchange(t.Context(), "s3-inspect", "BREP", reference)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Components) != 1 {
		t.Fatal("unexpected component count")
	}
	imported, err := client.ImportExchange(t.Context(), "s3-import", "s3-import-geometry", "BREP", reference, 0, "exchange/test/import.brep", "exchange/test/import.glb")
	if err != nil {
		t.Fatal(err)
	}
	if imported.GetVolume() <= 0 {
		t.Fatal("missing solid")
	}
	exported, err := client.ExportExchange(t.Context(), "s3-export", "STEP", "exchange/test/export.step", []ExchangeComponent{{Name: "test", BRep: reference}})
	if err != nil {
		t.Fatal(err)
	}
	reader, err := local.Open(t.Context(), exported.ObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Put(t.Context(), artifact.KindExchangeExport, "application/step", reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Delete(context.Background(), result.Key)
	stepRef := ArtifactReference{Backend: store.Backend(), ObjectKey: result.Key, SHA256: result.SHA256, Size: result.Size, ContentType: result.ContentType}
	if _, err := client.InspectExchange(t.Context(), "s3-inspect-export", "STEP", stepRef); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "exchange", "inputs"))
	if len(entries) != 0 {
		t.Fatal("worker input scratch leaked")
	}
}
