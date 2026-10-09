package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/occccad/occccad/internal/debugartifact"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/visual"
	"github.com/occccad/occccad/internal/workspace"
)

func TestFeatureAssociationFanLifecycleThroughRouter(t *testing.T) {
	service, artifacts, db, client := featureAssociationTestService(t)
	verifyFeatureAssociationFanLifecycle(t, service, artifacts, db, client)
}

func TestPostgresFeatureAssociationFanLifecycleThroughRouter(t *testing.T) {
	service, artifacts, db, client := postgresGeometryTestService(t)
	verifyFeatureAssociationFanLifecycle(t, service, artifacts, db, client)
}

func verifyFeatureAssociationFanLifecycle(t *testing.T, service *workspace.Service, artifacts *artifact.Service, db database.DB, client *geometry.Client) {
	t.Helper()
	diagnostics, err := debugartifact.NewStore(t.TempDir(), 50, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	service.SetDiagnosticArtifactStore(diagnostics)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Fan associations"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-fan-%d", view.Document.ID, seq)
		updated, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
		if e != nil {
			t.Fatalf("%s: %v", r.Type, e)
		}
		view = updated
		for _, f := range view.Part.Features {
			if f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
				t.Fatalf("%s: %s", f.Type, f.Diagnostic)
			}
		}
	}
	circle := func(x, y, r float64, operation string) workspace.Feature {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		s := view.Part.Features[len(view.Part.Features)-1]
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: s.ID, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: x, Y: y}, Radius: r}}}})
		apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: s.ID, Generator: "LINEAR_EXTRUDE", Operation: operation, Length: 10})
		return view.Part.Features[len(view.Part.Features)-1]
	}
	hub := circle(0, 0, 10, "ADD")
	eccentric := circle(3, 2, 1, "REMOVE")
	baseVolume := activeBodyArtifact(t, view).Volume
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := view.Part.Features[len(view.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 8, Y: -2}, Second: &workspace.SketchPoint2{X: 20, Y: 2}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch.ID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	blade := view.Part.Features[len(view.Part.Features)-1]
	bladeVolume := activeBodyArtifact(t, view).Volume - baseVolume
	apply(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SOLID_PATTERN", BodyID: hub.BodyID, Operation: "ADD", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "CIRCULAR", Distribution: "FULL_CIRCLE", Count: 4, Origin: [3]float64{}, Direction: [3]float64{0, 0, 1}}, Source: workspace.FeatureStageRef{BodyID: hub.BodyID, FeatureID: blade.ID}, SourceKind: "GENERATOR_TOOL", ResultMode: "COMBINE"}}})
	pattern := view.Part.Features[len(view.Part.Features)-1]
	if math.Abs(activeBodyArtifact(t, view).Volume-(baseVolume+4*bladeVolume)) > 1e-5 {
		t.Fatal("pattern copied hub/eccentric feature instead of blade tool")
	}
	loadVisual := func(a workspace.Artifact) (visual.Mesh, workspace.VisualizationManifest) {
		t.Helper()
		_, r, e := artifacts.Open(t.Context(), a.Representations["VISUAL"].ObjectID)
		if e != nil {
			t.Fatal(e)
		}
		data, e := io.ReadAll(r)
		r.Close()
		if e != nil {
			t.Fatal(e)
		}
		mesh, raw, e := visual.Decode(data)
		if e != nil {
			t.Fatal(e)
		}
		var v workspace.VisualizationManifest
		if e = json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		if v.FeatureAssociations == nil {
			t.Fatal("GLB lost derived associations")
		}
		return mesh, v
	}
	mesh, before := loadVisual(activeBodyArtifact(t, view))
	copiedBladeFace := false
	for _, e := range before.FeatureAssociations.Elements {
		if e.Kind == "FACE" && slices.Contains(e.Origins, blade.ID) {
			for _, m := range e.Members {
				if m.PatternID == pattern.ID && m.Slot == 1 {
					copiedBladeFace = true
				}
			}
		}
	}
	if !copiedBladeFace {
		t.Fatal("tree blade contribution omitted replicated member faces")
	}
	// Seed selection follows material continuation, never replica generation.
	var seedFace, seedEdge uint64
	for _, f := range mesh.FaceIDs {
		props, e := service.GetTopologyElementPropertiesAtVersion(t.Context(), view.Document.ID, view.Document.VersionID, activeBodyArtifact(t, view).GeometryKey, "FACE", uint64(f))
		if e != nil {
			t.Fatal(e)
		}
		if props.PersistentSelection != nil && props.PersistentSelection.Anchor.FeatureID == blade.ID && props.GeometryType == "PLANE" {
			seedFace = uint64(f)
			break
		}
	}
	for _, edge := range mesh.Edges {
		props, e := service.GetTopologyElementPropertiesAtVersion(t.Context(), view.Document.ID, view.Document.VersionID, activeBodyArtifact(t, view).GeometryKey, "EDGE", edge.LocalID)
		if e != nil {
			t.Fatal(e)
		}
		if props.PersistentSelection != nil && props.PersistentSelection.Anchor.FeatureID == blade.ID && props.GeometryType == "LINE" && math.Abs(props.PersistentSelection.CreationEvidence.Direction[2]) < 1e-6 {
			seedEdge = edge.LocalID
			break
		}
	}
	if seedFace == 0 || seedEdge == 0 {
		t.Fatal("seed face/edge absent")
	}
	preview, e := service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: "seed-face-preview", Type: "CREATE_SKETCH", TargetKind: "FACE", TopologyID: seedFace, GeometryKey: activeBodyArtifact(t, view).GeometryKey, VersionID: view.Document.VersionID})
	if e != nil || preview.PreviewID == "" {
		t.Fatal("seed face support after pattern", e)
	}
	service.DiscardPreview(view.Document.ID, p6Actor, preview.PreviewID)
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	referenceSketch := view.Part.Features[len(view.Part.Features)-1]
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: referenceSketch.ID, Operations: []workspace.SketchOperation{{Type: "ADD_EXTERNAL_GEOMETRY", ExternalID: "seed-edge-reference", TopologyKind: "EDGE", TopologyID: seedEdge, GeometryKey: activeBodyArtifact(t, view).GeometryKey, SourceVersionID: view.Document.VersionID}}})
	referenceSketch = view.Part.Features[len(view.Part.Features)-1]
	if len(referenceSketch.Sketch.ExternalGeometry) != 1 || referenceSketch.Sketch.ExternalGeometry[0].Status != "CONNECTED" {
		t.Fatal("seed projection after pattern", referenceSketch.Sketch.ExternalGeometry)
	}
	var edgeID uint64
	for _, e := range mesh.Edges {
		if len(e.Points) < 2 {
			continue
		}
		a, b := e.Points[0], e.Points[len(e.Points)-1]
		if math.Abs(a[0]-2) < 1e-4 && math.Abs(a[1]-math.Sqrt(96)) < 1e-4 && math.Abs(b[0]-a[0]) < 1e-4 && math.Abs(b[1]-a[1]) < 1e-4 && math.Abs(b[2]-a[2]) > 9 {
			edgeID = e.LocalID
			break
		}
	}
	if edgeID == 0 {
		t.Fatal("member 1 leaf-root edge absent")
	}
	foundMember := false
	for _, e := range before.FeatureAssociations.Elements {
		if e.Kind == "EDGE" && e.LocalID == edgeID {
			for _, member := range e.Members {
				if member.PatternID == pattern.ID && member.Slot == 1 {
					foundMember = true
				}
			}
		}
	}
	if !foundMember {
		t.Fatal("root edge lost actual pattern member context")
	}
	p, err := service.GetTopologyElementPropertiesAtVersion(t.Context(), view.Document.ID, view.Document.VersionID, activeBodyArtifact(t, view).GeometryKey, "EDGE", edgeID)
	if err != nil || p.PersistentSelection == nil {
		t.Fatal("member edge bind", err)
	}

	beforeFailure := view.Document.VersionID
	_, e = service.PreviewCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: "failed-real-fillet", Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: hub.BodyID, Length: 1e9, Selections: []workspace.FeatureSelection{{Selection: *p.PersistentSelection, SourceVersionID: view.Document.VersionID, SourceFeatureID: pattern.ID}}}})
	var diagnostic interface{ DiagnosticID() string }
	if e == nil || !errors.As(e, &diagnostic) {
		t.Fatal("failed real Worker preview lost diagnostic", e)
	}
	identity := strings.Split(diagnostic.DiagnosticID(), "/")
	evidence, e := service.ReadOperationDiagnostic(t.Context(), identity[0], identity[1])
	if e != nil {
		t.Fatal(e)
	}
	var failure workspace.OperationDiagnostic
	if e = json.Unmarshal(evidence, &failure); e != nil {
		t.Fatal(e)
	}
	var failedModel workspace.PartModel
	if e = json.Unmarshal(failure.Candidate, &failedModel); e != nil {
		t.Fatal(e)
	}
	if failure.BaseRevisionID != beforeFailure || failure.Stage != "GEOMETRY" || len(failure.Artifacts) == 0 || failedModel.Features[len(failedModel.Features)-1].Length != 1e9 {
		t.Fatal("failure not captured at evaluated candidate", failure.Stage)
	}
	unchanged, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || unchanged.Document.VersionID != beforeFailure {
		t.Fatal("failed preview advanced Head", e)
	}
	apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: hub.BodyID, Length: 0.5, Selections: []workspace.FeatureSelection{{Selection: *p.PersistentSelection, SourceVersionID: view.Document.VersionID, SourceFeatureID: pattern.ID}}}})
	fillet := view.Part.Features[len(view.Part.Features)-1]
	verify := func() {
		t.Helper()
		_, v := loadVisual(activeBodyArtifact(t, view))
		bladeFaces, filletFaces, hubFaces := 0, 0, 0
		for _, e := range v.FeatureAssociations.Elements {
			if e.Kind != "FACE" {
				continue
			}
			if slices.Contains(e.Origins, blade.ID) {
				bladeFaces++
			}
			if slices.Contains(e.Origins, hub.ID) {
				hubFaces++
			}
			if slices.Contains(e.Origins, fillet.ID) {
				filletFaces++
				if len(e.Members) != 1 || e.Members[0].Slot != 1 {
					t.Fatal("fillet face jumped to seed", e)
				}
				if !slices.Contains(e.Primary, fillet.ID) || slices.Contains(e.Origins, blade.ID) {
					t.Fatal("fillet face inherited support as generator", e)
				}
			}
		}
		if bladeFaces == 0 || filletFaces == 0 || hubFaces == 0 {
			t.Fatalf("face contributions blade=%d fillet=%d hub=%d", bladeFaces, filletFaces, hubFaces)
		}
	}
	verify()
	head := view.Document.VersionID
	firstInput, err := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: head, FeatureID: hub.ID})
	if err != nil || firstInput.SourceFeatureID != "" || firstInput.Artifact.Volume != 0 || firstInput.Artifact.DisplayStageFeatureID != "" {
		t.Fatal("first generator edit retained downstream Body", err)
	}
	input, err := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: head, FeatureID: fillet.ID})
	if err != nil || input.SourceFeatureID != pattern.ID || len(input.Picks) != 1 {
		t.Fatal("fillet edit stage", err, input)
	}
	history, err := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: head, FeatureID: blade.ID, ResultStage: true})
	if err != nil || history.SourceFeatureID != blade.ID {
		t.Fatal("historical blade stage", err)
	}
	if math.Abs(history.Artifact.Volume-(baseVolume+bladeVolume)) > 1e-5 {
		t.Fatal("historical result used downstream Body")
	}
	reread, err := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if err != nil || reread.Document.VersionID != head {
		t.Fatal("stage read changed Head")
	}
	edit := func(f workspace.Feature) {
		t.Helper()
		var digest string
		var visit func(workspace.DocumentStructureNode)
		visit = func(n workspace.DocumentStructureNode) {
			if n.EntityID == f.ID && n.PresentationRole != "INPUT_REFERENCE" {
				digest = n.DefinitionDigest
			}
			for _, c := range n.Children {
				visit(c)
			}
		}
		visit(*view.StructureTree)
		apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, ExpectedFeatureDigest: digest, Feature: &f})
	}
	blade.Length = 12
	edit(blade)
	verify()
	pattern.Pattern.Count = 5
	edit(pattern)
	verify()
	apply(workspace.CommandRequest{Type: "UNDO"})
	verify()
	apply(workspace.CommandRequest{Type: "REDO"})
	verify()
	// New control-plane consumer proves the index survives cold document reads.
	cold := workspace.NewWithArtifacts(db, client, artifacts)
	view, err = cold.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if err != nil {
		t.Fatal(err)
	}
	verify()
	// Display metadata replacement must keep the immutable topology association.
	apply(workspace.CommandRequest{Type: "RENAME_FEATURE", TargetID: eccentric.ID, Name: "Eccentric hole"})
	verify()
}

func featureAssociationTestService(t *testing.T) (*workspace.Service, *artifact.Service, database.DB, *geometry.Client) {
	s, a, d, c, _ := featureAssociationRuntimeService(t)
	return s, a, d, c
}
func featureAssociationRuntimeService(t *testing.T) (*workspace.Service, *artifact.Service, database.DB, *geometry.Client, *GeometryPool) {
	t.Helper()
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Skip("requires built Geometry Worker")
	}
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "fan.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return geometryRuntimeTestService(t, db)
}

func postgresGeometryTestService(t *testing.T) (*workspace.Service, *artifact.Service, database.DB, *geometry.Client) {
	t.Helper()
	if os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER") == "" {
		t.Skip("requires built Geometry Worker")
	}
	service, artifacts, db, client, _ := geometryRuntimeTestService(t, testsupport.OpenTestDatabase(t))
	return service, artifacts, db, client
}

func geometryRuntimeTestService(t *testing.T, db *database.Pool) (*workspace.Service, *artifact.Service, database.DB, *geometry.Client, *GeometryPool) {
	t.Helper()
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
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
	t.Cleanup(func() { client.Close() })
	local, err := artifact.NewLocalStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := artifact.NewService(db, local)
	service := workspace.NewWithArtifacts(db, client, artifacts)
	return service, artifacts, db, client, pool

}
