package control

import (
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

func TestLoftCorrespondenceAndPointLifecycleThroughRouter(t *testing.T) {
	t.Run("explicit-point", func(t *testing.T) { testLoftPointLifecycle(t, false) })
	t.Run("whole-point-sketch", func(t *testing.T) { testLoftPointLifecycle(t, true) })
}

func testLoftPointLifecycle(t *testing.T, wholeSketch bool) {
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Skip("requires built Geometry Worker")
	}
	url := "sqlite:" + filepath.Join(t.TempDir(), "loft.db")
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

	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Loft lifecycle"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(request workspace.CommandRequest) {
		t.Helper()
		seq++
		request.ActorID = p6Actor
		request.RequestID = fmt.Sprintf("%s-loft-%d", view.Document.ID, seq)
		if request.Feature != nil && request.Feature.Type == "LOFT" {
			before := view.Document.VersionID
			preview, e := service.PreviewCommand(t.Context(), view.Document.ID, request)
			if e != nil {
				t.Fatal("preview", e)
			}
			if preview.Artifact == nil || len(preview.LoftConnections) != 4 || len(preview.LoftSections) != len(request.Feature.Sections) {
				t.Fatalf("missing loft preview: %+v", preview)
			}
			reread, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || reread.Document.VersionID != before {
				t.Fatal("preview mutated Head", e)
			}
			request.PreviewID = preview.PreviewID
		}
		v, e := service.ApplyCommand(t.Context(), view.Document.ID, request)
		if e != nil {
			t.Fatalf("%s: %v", request.Type, e)
		}
		view = v
	}
	sketch := func(z float64, point bool) string {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: fmt.Sprintf("z%g", z), Origin: [3]float64{0, 0, z}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
		plane := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: plane})
		id := view.Part.Features[len(view.Part.Features)-1].ID
		op := workspace.SketchOperation{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -5, Y: -3}, Second: &workspace.SketchPoint2{X: 5, Y: 3}}
		if point {
			op = workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "tip", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}}
		}
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{op}})
		return id
	}
	base := sketch(0, false)
	// A dimensional parameter drives source geometry and therefore the accepted loft.
	edge := view.Part.Features[len(view.Part.Features)-1].Sketch.Entities[0].ID
	width := 10.0
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: base, Operations: []workspace.SketchOperation{{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "width", Kind: "LENGTH", Unit: "mm", Value: &width, References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: edge, SubElement: "WHOLE"}}}}}})
	tip := sketch(20, true)
	point := &workspace.PatternPointReference{SketchID: tip, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "tip", SubElement: "POINT"}}
	section := workspace.LoftSection{SketchID: tip, Point: point}
	if wholeSketch {
		section.Point = nil
	}
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "LOFT", Operation: "NEW_BODY", Sections: []workspace.LoftSection{{SketchID: base}, section}}})
	loft := view.Part.Features[len(view.Part.Features)-1]
	if !reflect.DeepEqual(loft.Sections[1].Point, point) {
		t.Fatal("point sketch did not persist its stable entity reference")
	}
	if !loft.Sections[0].CorrespondenceResolved || loft.Sections[0].SeamEntityID == "" {
		t.Fatal("accepted correspondence not persisted")
	}
	if v := activeBodyArtifact(t, view).Volume; math.Abs(v-400) > 1e-5 {
		t.Fatal("point loft volume", v)
	}
	accepted := append([]workspace.LoftSection(nil), loft.Sections...)
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + base + ":constraint:width:value", Value: 12, Unit: "mm"})
	if v := activeBodyArtifact(t, view).Volume; math.Abs(v-480) > 1e-4 {
		t.Fatal("parameter loft volume", v)
	}
	if !reflect.DeepEqual(view.Part.Features[len(view.Part.Features)-1].Sections, accepted) {
		t.Fatal("parameter changed correspondence")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if math.Abs(activeBodyArtifact(t, view).Volume-400) > 1e-4 {
		t.Fatal("undo")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if math.Abs(activeBodyArtifact(t, view).Volume-480) > 1e-4 {
		t.Fatal("redo")
	}
	cold := workspace.NewWithArtifacts(db, client, artifact.NewService(db, store))
	reopened, e := cold.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || !reflect.DeepEqual(reopened.Part.Features[len(reopened.Part.Features)-1].Sections, accepted) {
		t.Fatal("cold correspondence", e)
	}
	// Saved seam loss must fail preview, with Head and accepted geometry retained.
	invalid := loft
	invalid.Sections = append([]workspace.LoftSection(nil), loft.Sections...)
	invalid.Sections[0].SeamEntityID = "removed"
	before := view.Document.VersionID
	_, e = service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-bad", TargetID: loft.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, loft.ID), Feature: &invalid})
	if e == nil {
		t.Fatal("lost seam accepted")
	}
	reread, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || reread.Document.VersionID != before {
		t.Fatal("failed preview changed Head", e)
	}
	// Reverse the section order: point at start, same material and edit/preview contract.
	edited := view.Part.Features[len(view.Part.Features)-1]
	edited.Sections = append([]workspace.LoftSection(nil), edited.Sections...)
	edited.Sections[0], edited.Sections[1] = edited.Sections[1], edited.Sections[0]
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: loft.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, loft.ID), Feature: &edited})
	if math.Abs(activeBodyArtifact(t, view).Volume-480) > 1e-4 {
		t.Fatal("reversed point endpoint")
	}
}
