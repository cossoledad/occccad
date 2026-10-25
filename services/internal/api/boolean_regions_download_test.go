package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/control"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
)

type countingRegionStore struct {
	artifact.Store
	puts atomic.Int32
}

func (s *countingRegionStore) Put(ctx context.Context, kind artifact.Kind, contentType string, r io.Reader) (artifact.StoredObject, error) {
	s.puts.Add(1)
	return s.Store.Put(ctx, kind, contentType, r)
}

func TestBooleanRegionDownloadsUseFrozenManifestThroughRouter(t *testing.T) {
	binary := os.Getenv("OCCCCAD_GEOMETRY_WORKER_BIN")
	if binary == "" {
		t.Skip("requires Geometry Worker")
	}
	db := testsupport.OpenDevelopmentDatabase(t)
	env := testsupport.Environment(t)
	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := portListener.Addr().(*net.TCPAddr).Port
	portListener.Close()
	pool := control.NewGeometryPool(t.Context(), control.GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: env.DataDirectory, LogDirectory: env.LogDirectory})
	t.Cleanup(pool.Close)
	if err = pool.Start(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var analyses atomic.Int32
	rpc := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if info.FullMethod == workerv1.GeometryWorker_AnalyzeBooleanRegions_FullMethodName {
			analyses.Add(1)
		}
		return next(ctx, req)
	}))
	workerv1.RegisterGeometryWorkerServer(rpc, pool)
	go rpc.Serve(listener)
	t.Cleanup(rpc.Stop)
	store, staging, err := artifact.OpenConfigured(t.Context(), env.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	counted := &countingRegionStore{Store: store}
	objects := artifact.NewService(db, counted, staging)
	client, err := geometry.Open(listener.Addr().String(), geometry.ArtifactStaging(store, staging))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	domain := workspace.NewWithArtifacts(db, client, objects)
	actor := "00000000-0000-7000-8000-000000000001"
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PART", Name: "Frozen Boolean downloads"})
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		sequence++
		r.ActorID = actor
		r.RequestID = fmt.Sprintf("%s-download-%d", view.Document.ID, sequence)
		view, err = domain.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(err)
		}
	}
	rectangle := func(x, w float64) workspace.Feature {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sketch := view.Part.Features[len(view.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: -5}, Second: &workspace.SketchPoint2{X: x + w, Y: 5}}}})
		apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: 10})
		return view.Part.Features[len(view.Part.Features)-1]
	}
	target, tool := rectangle(0, 30), rectangle(10, 10)
	scope := workspace.BooleanRegionsRequest{ActorID: actor, VersionID: view.Document.VersionID, BodyID: target.BodyID, Tool: workspace.FeatureStageRef{BodyID: tool.BodyID, FeatureID: tool.ID}, Operation: "REMOVE"}
	result, err := domain.AnalyzeBooleanRegions(t.Context(), view.Document.ID, scope)
	if err != nil {
		t.Fatal(err)
	}
	if analyses.Load() != 1 || result.AnalysisID == "" || result.ManifestDigest == "" {
		t.Fatal("analysis was not frozen", analyses.Load())
	}
	puts := counted.puts.Load()
	server := &Server{database: db, workspace: domain, access: access.New(db), artifacts: objects}
	query := url.Values{"versionId": {scope.VersionID}, "bodyId": {scope.BodyID}, "booleanToolBodyId": {tool.BodyID}, "booleanToolFeatureId": {tool.ID}, "booleanOperation": {scope.Operation}, "booleanAnalysisId": {result.AnalysisID}, "booleanManifestDigest": {result.ManifestDigest}}
	get := func(user, object string, q url.Values) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/representation?"+q.Encode(), nil)
		request.SetPathValue("documentID", view.Document.ID)
		request.SetPathValue("objectID", object)
		request = request.WithContext(access.WithPrincipal(request.Context(), access.User{ID: user}))
		response := httptest.NewRecorder()
		server.downloadRepresentation(response, request)
		return response
	}
	var id string
	downloads := 0
	for _, cell := range result.Regions {
		for _, rep := range cell.Artifact.Representations {
			id = rep.ObjectID
			for repeat := 0; repeat < 2; repeat++ {
				response := get(actor, id, query)
				if response.Code != 200 || response.Body.Len() != int(rep.Size) {
					t.Fatalf("download %d: %s", response.Code, response.Body.String())
				}
				downloads++
			}
		}
	}
	if downloads < 6 || analyses.Load() != 1 || counted.puts.Load() != puts {
		t.Fatal("downloads recomputed or republished", downloads, analyses.Load(), counted.puts.Load(), puts)
	}
	for _, field := range []string{"versionId", "featureId", "bodyId", "booleanToolBodyId", "booleanToolFeatureId", "booleanOperation", "booleanAnalysisId", "booleanManifestDigest"} {
		bad := url.Values{}
		for key, values := range query {
			bad[key] = append([]string(nil), values...)
		}
		bad.Set(field, "00000000-0000-7000-8000-000000000099")
		if response := get(actor, id, bad); response.Code != 404 {
			t.Fatal("scope accepted", field, response.Code)
		}
	}
	if response := get("00000000-0000-7000-8000-000000000099", id, query); response.Code != 403 {
		t.Fatal("permission", response.Code)
	}
	unrelated, e := objects.Put(t.Context(), artifact.KindGLB, "model/gltf-binary", nilReader{})
	if e != nil {
		t.Fatal(e)
	}
	if get(actor, unrelated.ID, query).Code != 404 {
		t.Fatal("unlisted object accepted")
	}
	if domain.PreviewVisualAllowed(view.Document.ID, actor, result.AnalysisID, id) {
		t.Fatal("analysis bypassed scoped authorization through preview")
	}
	apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	if get(actor, id, query).Code != 404 {
		t.Fatal("stale Head accepted")
	}
	if analyses.Load() != 1 {
		t.Fatal("authorization triggered analysis")
	}
	t.Logf("%d downloads: AnalyzeBooleanRegions=1, artifact publications unchanged (%d)", downloads, puts)
}

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) { return 0, io.EOF }
