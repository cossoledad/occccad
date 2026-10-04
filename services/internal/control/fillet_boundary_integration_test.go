package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundaryFilletLifecycleThroughRouter(t *testing.T) {
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Skip("requires built Geometry Worker")
	}
	url := "sqlite:" + filepath.Join(t.TempDir(), "fillet.db")
	db, err := database.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	directory := t.TempDir()
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: directory, LogDirectory: t.TempDir()})
	t.Cleanup(pool.Close)
	if err = pool.Start(); err != nil {
		t.Fatal(err)
	}
	client, err := geometry.Open(serveGeometry(t, pool))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store, err := artifact.NewLocalStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := artifact.NewService(db, store)
	service := workspace.NewWithArtifacts(db, client, artifacts)

	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Boundary fillet lifecycle"})
	if err != nil {
		t.Fatal(err)
	}

	seq := 0
	apply := func(request workspace.CommandRequest) {
		t.Helper()
		seq++
		request.ActorID = p6Actor
		request.RequestID = fmt.Sprintf("%s-fillet-%d", view.Document.ID, seq)
		if request.Type == "CREATE_MODIFY_FEATURE" {
			head := view.Document.VersionID
			preview, e := service.PreviewCommand(t.Context(), view.Document.ID, request)
			if e != nil || preview.Artifact == nil {
				t.Fatalf("preview: %v", e)
			}
			current, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || current.Document.VersionID != head {
				t.Fatal("preview changed head", e)
			}
			request.PreviewID = preview.PreviewID
		}
		next, e := service.ApplyCommand(t.Context(), view.Document.ID, request)
		if e != nil {
			t.Fatalf("%s: %v", request.Type, e)
		}
		view = next
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: 60, Y: 30}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: 2.2})
	base := view.Part.Features[len(view.Part.Features)-1]
	var selected []workspace.FeatureSelection
	a := activeBodyArtifact(t, view)
	for id := uint64(1); id <= p6TopologyCount(t, a.Topology, "edges"); id++ {
		p, e := service.GetTopologyElementPropertiesAtVersion(t.Context(), view.Document.ID, view.Document.VersionID, a.GeometryKey, "EDGE", id)
		if e != nil {
			t.Fatal(e)
		}
		if p.PersistentSelection != nil && strings.HasPrefix(p.PersistentSelection.Anchor.OutputSlot, "START_BOUNDARY_FROM_PROFILE_EDGE/") {
			selected = append(selected, workspace.FeatureSelection{Selection: *p.PersistentSelection, SourceVersionID: view.Document.VersionID})
		}
	}
	if len(selected) != 4 {
		t.Fatalf("bottom edges=%d", len(selected))
	}
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: base.BodyID, Length: 3, Selections: selected}})
	fillet := view.Part.Features[len(view.Part.Features)-1]
	initial := activeBodyArtifact(t, view).Volume
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + fillet.ID + ":length", Value: 4, Unit: "mm"})
	changed := activeBodyArtifact(t, view).Volume
	if changed >= initial {
		t.Fatal("radius did not remove material")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if math.Abs(activeBodyArtifact(t, view).Volume-initial) > 1e-7 {
		t.Fatal("undo")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if math.Abs(activeBodyArtifact(t, view).Volume-changed) > 1e-7 {
		t.Fatal("redo")
	}
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + base.ID + ":length", Value: 2.5, Unit: "mm"})
	if activeBodyArtifact(t, view).Volume <= changed {
		t.Fatal("upstream dimension did not propagate")
	}
	readyVolume := activeBodyArtifact(t, view).Volume
	readyKey := activeBodyArtifact(t, view).GeometryKey
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + fillet.ID + ":length", Value: 1000, Unit: "mm"})
	for _, f := range view.Part.Features {
		if f.ID == fillet.ID && f.EvaluationStatus != "FAILED" {
			t.Fatal("invalid radius accepted", f.EvaluationStatus)
		}
	}
	for _, b := range view.Part.Bodies {
		if b.ID == base.BodyID && (b.GeometryKey != "" || b.DisplayFallback == nil || b.DisplayFallback.GeometryKey != readyKey) {
			t.Fatal("failed candidate replaced ready geometry")
		}
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if math.Abs(activeBodyArtifact(t, view).Volume-readyVolume) > 1e-7 {
		t.Fatal("failure recovery changed geometry")
	}
	cold := workspace.NewWithArtifacts(db, client, artifacts)
	reopened, e := cold.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || math.Abs(activeBodyArtifact(t, reopened).Volume-activeBodyArtifact(t, view).Volume) > 1e-7 {
		t.Fatal("cold read", e)
	}
}
