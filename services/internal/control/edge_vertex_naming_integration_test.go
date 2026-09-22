package control

import (
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/debugartifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

// TestEdgeVertexPersistentSelectionThroughRealRouter is the P7 executable
// acceptance fixture. It uses the application Router and exact OCCT topology
// lookup for Vertex-Vertex, Vertex-Plane, Edge-Edge and Edge-Plane constraints.
func TestEdgeVertexPersistentSelectionThroughRealRouter(t *testing.T) {
	binary, databaseURL := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER"), os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if binary == "" || databaseURL == "" {
		t.Skip("requires disposable OCCCCAD_TEST_DATABASE_URL and OCCCCAD_TEST_GEOMETRY_WORKER")
	}
	db, err := database.Open(t.Context(), databaseURL)
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
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1",
		FirstWorkerPort: port, DataDirectory: directory, LogDirectory: t.TempDir()})
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
	artifactService := artifact.NewService(db, store)
	service := workspace.NewWithArtifacts(db, client, artifactService)
	debugStore, err := debugartifact.NewStore(t.TempDir(), 50, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	service.SetDebugArtifactStore(debugStore)

	part, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "P7 Edge Vertex Part"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PRODUCT", Name: "P7 Edge Vertex Product"})
	if err != nil {
		t.Fatal(err)
	}
	runID := strings.ReplaceAll(product.Document.ID, "-", "")[:12]
	isolationProduct, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PRODUCT", Name: "P7 Pre-solver Failure Product"})
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	apply := func(documentID string, request workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		sequence++
		if request.RequestID == "" {
			request.RequestID = fmt.Sprintf("p7-%03d", sequence)
		}
		request.RequestID = runID + "-" + request.RequestID
		request.ActorID = p6Actor
		view, applyErr := service.ApplyCommand(t.Context(), documentID, request)
		if applyErr != nil {
			t.Fatalf("%s on %s: %v", request.Type, documentID, applyErr)
		}
		return view
	}
	addRectangle := func(documentID, plane, datumPlaneID string, first, second workspace.SketchPoint2) (workspace.DocumentView, string) {
		view := apply(documentID, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: plane, DatumPlaneID: datumPlaneID})
		sketchID := view.Part.Features[len(view.Part.Features)-1].ID
		view = apply(documentID, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID,
			Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &first, Second: &second}}})
		return view, sketchID
	}
	addExtrude := func(documentID, sketchID, operation string, length float64, reversed bool) workspace.DocumentView {
		return apply(documentID, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketchID,
			Generator: "LINEAR_EXTRUDE", Operation: operation, Length: length, Reversed: reversed})
	}

	part, baseSketch := addRectangle(part.Document.ID, "XY", "", workspace.SketchPoint2{X: 0, Y: 0}, workspace.SketchPoint2{X: 20, Y: 20})
	part = addExtrude(part.Document.ID, baseSketch, "NEW_BODY", 10, false)
	baseFeatureID := part.Part.Features[len(part.Part.Features)-1].ID
	// Exercise face -> sketch -> reversed pocket through the real Router on
	// both caps and all four side faces, then restore the base for each face.
	for _, expected := range [][3]float64{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}} {
		pick := findP7TopologyPick(t, service, part, "FACE", func(properties workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
			n, ok := properties.Properties["normal"].([3]float64)
			return ok && n == expected
		})
		part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", GeometryKey: pick.GeometryKey, TopologyID: pick.LocalID, VersionID: pick.SourceVersionID})
		feature := part.Part.Features[len(part.Part.Features)-1]
		support := feature.Sketch.Support
		if support.Normal != expected {
			t.Fatalf("face sketch normal = %v, want %v", support.Normal, expected)
		}
		u, n := support.XDirection, support.Normal
		v := [3]float64{n[1]*u[2] - n[2]*u[1], n[2]*u[0] - n[0]*u[2], n[0]*u[1] - n[1]*u[0]}
		var x, y float64
		for i := 0; i < 3; i++ {
			delta := pick.Selection.CreationEvidence.Centroid[i] - support.Origin[i]
			x += delta * u[i]
			y += delta * v[i]
		}
		first, second := workspace.SketchPoint2{X: x - 1, Y: y - 1}, workspace.SketchPoint2{X: x + 1, Y: y + 1}
		part = apply(part.Document.ID, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: feature.ID, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &first, Second: &second}}})
		part = addExtrude(part.Document.ID, feature.ID, "REMOVE", 1, true)
		if math.Abs(part.Artifact.Volume-3996) > 1e-5 {
			t.Fatalf("pocket on %v volume = %g", expected, part.Artifact.Volume)
		}
		for undo := 0; undo < 3; undo++ {
			part = apply(part.Document.ID, workspace.CommandRequest{Type: "UNDO"})
		}
	}
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Original top",
		Origin: [3]float64{0, 0, 10}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	originalTopPlaneID := part.DatumPlanes[len(part.DatumPlanes)-1].ID
	topEdge := findP7TopologyPick(t, service, part, "EDGE", func(properties workspace.TopologyElementProperties, selection modelcore.PersistentSelection) bool {
		return selection.Anchor.FeatureID == baseFeatureID &&
			strings.HasPrefix(selection.Anchor.OutputSlot, "END_BOUNDARY_FROM_PROFILE_EDGE/") &&
			properties.GeometryType == "LINE" && math.Abs(selection.CreationEvidence.Centroid[2]-10) < 1e-9
	})
	topVertex := findP7TopologyPick(t, service, part, "VERTEX", func(_ workspace.TopologyElementProperties, selection modelcore.PersistentSelection) bool {
		return selection.Anchor.FeatureID == baseFeatureID &&
			strings.HasPrefix(selection.Anchor.OutputSlot, "END_VERTEX_FROM_PROFILE_ENDPOINTS/") &&
			math.Abs(selection.CreationEvidence.Centroid[2]-10) < 1e-9
	})
	if topEdge.Selection.CreationEvidence.ParameterStart == nil || topEdge.Selection.CreationEvidence.ParameterEnd == nil ||
		topEdge.Selection.CreationEvidence.EndpointRole != "END_CAP_BOUNDARY" {
		t.Fatalf("edge evidence lacks parameter interval or endpoint role: %#v", topEdge.Selection.CreationEvidence)
	}
	if topVertex.Selection.CreationEvidence.EndpointRole != "END_CAP_VERTEX" {
		t.Fatalf("vertex endpoint role = %q", topVertex.Selection.CreationEvidence.EndpointRole)
	}

	type pair struct{ first, second string }
	insertPair := func(targetID, name string) (workspace.DocumentView, pair) {
		view := apply(targetID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: name + " A"})
		first := view.Product.Instances[len(view.Product.Instances)-1].ID
		view = apply(targetID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: name + " B"})
		second := view.Product.Instances[len(view.Product.Instances)-1].ID
		return view, pair{first: first, second: second}
	}
	pairs := make([]pair, 4)
	for index, name := range []string{"Vertex Vertex", "Vertex Plane", "Edge Edge", "Edge Plane"} {
		product, pairs[index] = insertPair(product.Document.ID, name)
		product = apply(product.Document.ID, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX",
			FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: pairs[index].first, Kind: "BODY"}})
	}
	requests := []workspace.CommandRequest{
		p7Coincident(p7AssemblyPick(pairs[0].first, topVertex), p7AssemblyPick(pairs[0].second, topVertex)),
		p7Coincident(p7AssemblyPick(pairs[1].first, topVertex), &workspace.AssemblyGeometryRef{InstanceID: pairs[1].second, Kind: "PLANE", GeometryID: originalTopPlaneID}),
		p7Coincident(p7AssemblyPick(pairs[2].first, topEdge), p7AssemblyPick(pairs[2].second, topEdge)),
		p7Coincident(p7AssemblyPick(pairs[3].first, topEdge), &workspace.AssemblyGeometryRef{InstanceID: pairs[3].second, Kind: "PLANE", GeometryID: originalTopPlaneID}),
	}
	constraintIDs := make([]string, 0, len(requests))
	for _, request := range requests {
		product = apply(product.Document.ID, request)
		constraintID := product.Product.Constraints[len(product.Product.Constraints)-1].ID
		constraintIDs = append(constraintIDs, constraintID)
		assertP7Constraint(t, product, constraintID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected)
	}

	// A through opening changes the body history without touching the selected
	// top boundary supports. Every point/axis descriptor must re-resolve and be
	// solved by the formal Router path.
	part, throughSketch := addRectangle(part.Document.ID, "XZ", "", workspace.SketchPoint2{X: 5, Y: 2}, workspace.SketchPoint2{X: 15, Y: 8})
	part = addExtrude(part.Document.ID, throughSketch, "REMOVE", 20, true)
	product = apply(product.Document.ID, workspace.CommandRequest{RequestID: "p7-update-through-cut", Type: "UPDATE_REFERENCES"})
	for _, constraintID := range constraintIDs {
		assertP7Constraint(t, product, constraintID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected)
	}
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "UNDO"})

	// A product containing only one topology constraint proves that a failed
	// support resolution stops before SolveAssembly and produces no 3dreplay.
	currentTopVertex := findP7TopologyPick(t, service, part, "VERTEX", func(_ workspace.TopologyElementProperties, selection modelcore.PersistentSelection) bool {
		return selection.Anchor.FeatureID == baseFeatureID && selection.Anchor.OutputSlot == topVertex.Selection.Anchor.OutputSlot
	})
	isolationProduct, isolatedPair := insertPair(isolationProduct.Document.ID, "Isolated Vertex")
	isolationProduct = apply(isolationProduct.Document.ID,
		p7Coincident(p7AssemblyPick(isolatedPair.first, currentTopVertex), p7AssemblyPick(isolatedPair.second, currentTopVertex)))
	isolatedConstraintID := isolationProduct.Product.Constraints[len(isolationProduct.Product.Constraints)-1].ID

	part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Cut start",
		Origin: [3]float64{0, 0, 5}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	cutPlaneID := part.DatumPlanes[len(part.DatumPlanes)-1].ID
	part, deleteSketch := addRectangle(part.Document.ID, "", cutPlaneID, workspace.SketchPoint2{X: 0, Y: 0}, workspace.SketchPoint2{X: 20, Y: 20})
	part = addExtrude(part.Document.ID, deleteSketch, "REMOVE", 5, false)
	deleteFeatureID := part.Part.Features[len(part.Part.Features)-1].ID
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	for _, constraintID := range constraintIDs {
		assertP7Constraint(t, product, constraintID, modelcore.AssemblyConstraintBroken, modelcore.SupportingElementNotConnected)
	}
	for _, constraint := range product.Product.Constraints {
		if constraint.Kind == "FIX" && constraint.EvaluationStatus != modelcore.AssemblyConstraintVerified {
			t.Fatalf("unrelated FIX constraint = %s, want VERIFIED", constraint.EvaluationStatus)
		}
	}
	isolationRequestID := runID + "-p7-resolution-failed-before-solve"
	isolationProduct = apply(isolationProduct.Document.ID, workspace.CommandRequest{RequestID: "p7-resolution-failed-before-solve", Type: "UPDATE_REFERENCES"})
	assertP7Constraint(t, isolationProduct, isolatedConstraintID, modelcore.AssemblyConstraintBroken, modelcore.SupportingElementNotConnected)
	if _, _, replayErr := service.ReadAssemblyReplay(t.Context(), isolationProduct.Document.ID, "latest", isolationRequestID); !errors.Is(replayErr, workspace.ErrNotFound) {
		t.Fatalf("resolution failure produced a solver replay: %v", replayErr)
	}

	part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Replacement top",
		Origin: [3]float64{0, 0, 5}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	replacementTopPlaneID := part.DatumPlanes[len(part.DatumPlanes)-1].ID
	// The automatic FOLLOW_HEAD path includes a plan digest. Existing Broken
	// constraints must neither reject the next Head nor schedule endless updates.
	plan, err := service.GetProductUpdatePlan(t.Context(), product.Document.ID)
	if err != nil || !plan.CanAccept || !plan.HasUpdates {
		t.Fatalf("Broken constraints blocked new Head: %+v %v", plan, err)
	}
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES", UpdatePlanDigest: plan.Digest})
	for _, instance := range product.Product.Instances {
		if instance.ReferencedDocumentID == part.Document.ID && instance.ResolvedVersionID != part.Document.VersionID {
			t.Fatalf("instance did not accept source Head: %+v", instance)
		}
	}
	for _, constraintID := range constraintIDs {
		assertP7Constraint(t, product, constraintID, modelcore.AssemblyConstraintBroken, modelcore.SupportingElementNotConnected)
	}
	plan, err = service.GetProductUpdatePlan(t.Context(), product.Document.ID)
	if err != nil || !plan.CanAccept || plan.HasUpdates {
		t.Fatalf("Broken constraints schedule repeated updates: %+v %v", plan, err)
	}
	newTopEdge := findP7TopologyPick(t, service, part, "EDGE", func(properties workspace.TopologyElementProperties, selection modelcore.PersistentSelection) bool {
		return selection.Anchor.FeatureID == deleteFeatureID && properties.GeometryType == "LINE" &&
			math.Abs(selection.CreationEvidence.Centroid[2]-5) < 1e-9
	})
	newTopVertex := findP7TopologyPick(t, service, part, "VERTEX", func(_ workspace.TopologyElementProperties, selection modelcore.PersistentSelection) bool {
		return selection.Anchor.FeatureID == deleteFeatureID && math.Abs(selection.CreationEvidence.Centroid[2]-5) < 1e-9
	})
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	reconnect := []workspace.CommandRequest{
		{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: constraintIDs[0], FirstAssemblyRef: p7AssemblyPick(pairs[0].first, newTopVertex), SecondAssemblyRef: p7AssemblyPick(pairs[0].second, newTopVertex)},
		{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: constraintIDs[1], FirstAssemblyRef: p7AssemblyPick(pairs[1].first, newTopVertex), SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: pairs[1].second, Kind: "PLANE", GeometryID: replacementTopPlaneID}},
		{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: constraintIDs[2], FirstAssemblyRef: p7AssemblyPick(pairs[2].first, newTopEdge), SecondAssemblyRef: p7AssemblyPick(pairs[2].second, newTopEdge)},
		{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: constraintIDs[3], FirstAssemblyRef: p7AssemblyPick(pairs[3].first, newTopEdge), SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: pairs[3].second, Kind: "PLANE", GeometryID: replacementTopPlaneID}},
	}
	for _, request := range reconnect {
		product = apply(product.Document.ID, request)
		assertP7Constraint(t, product, request.TargetID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected)
	}
	digest := findP6FeatureDigest(t, part.StructureTree, deleteFeatureID)
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: deleteFeatureID,
		ExpectedFeatureDigest: digest, Length: 6, Unit: "mm"})
	product = apply(product.Document.ID, workspace.CommandRequest{RequestID: "p7-update-after-reconnect", Type: "UPDATE_REFERENCES"})
	for _, constraintID := range constraintIDs {
		assertP7Constraint(t, product, constraintID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected)
	}

	// Constructing a fresh Service clears the process-local resolver cache. The
	// original Edge and Vertex selections must still resolve from immutable
	// manifests/artifacts alone.
	coldService := workspace.NewWithArtifacts(db, client, artifactService)
	for _, pick := range []p7TopologyPick{newTopEdge, newTopVertex} {
		resolution, resolveErr := coldService.ResolvePersistentSelection(t.Context(), part.Document.ID,
			workspace.ResolvePersistentSelectionRequest{Selection: pick.Selection,
				SourceVersionID: pick.SourceVersionID, TargetVersionID: part.Document.VersionID,
				PolicyDigest: modelcore.TopologyNamingPolicyDigest})
		if resolveErr != nil || resolution.Status != modelcore.SelectionResolved {
			t.Fatalf("cold resolver %s = %#v, %v", pick.Kind, resolution, resolveErr)
		}
	}
}

type p7TopologyPick struct {
	Kind            string
	GeometryKey     string
	LocalID         uint64
	SourceVersionID string
	Selection       modelcore.PersistentSelection
}

func findP7TopologyPick(t *testing.T, service *workspace.Service, view workspace.DocumentView, kind string,
	match func(workspace.TopologyElementProperties, modelcore.PersistentSelection) bool) p7TopologyPick {
	t.Helper()
	countKey := map[string]string{"FACE": "faces", "EDGE": "edges", "VERTEX": "vertices"}[kind]
	count := p6TopologyCount(t, view.Artifact.Topology, countKey)
	for localID := uint64(1); localID <= count; localID++ {
		properties, err := service.GetTopologyElementPropertiesAtVersion(t.Context(), view.Document.ID,
			view.Document.VersionID, view.Artifact.GeometryKey, kind, localID)
		if err == nil && properties.PersistentSelection != nil && match(properties, *properties.PersistentSelection) {
			return p7TopologyPick{Kind: kind, GeometryKey: view.Artifact.GeometryKey, LocalID: localID,
				SourceVersionID: view.Document.VersionID, Selection: *properties.PersistentSelection}
		}
	}
	t.Fatalf("matching %s was not found", kind)
	return p7TopologyPick{}
}

func p7AssemblyPick(instanceID string, pick p7TopologyPick) *workspace.AssemblyGeometryRef {
	return &workspace.AssemblyGeometryRef{InstanceID: instanceID, Kind: pick.Kind,
		GeometryKey: pick.GeometryKey, TopologyID: pick.LocalID}
}

func p7Coincident(first, second *workspace.AssemblyGeometryRef) workspace.CommandRequest {
	return workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT",
		FirstAssemblyRef: first, SecondAssemblyRef: second, DirectionRelation: "UNORIENTED"}
}

func assertP7Constraint(t *testing.T, view workspace.DocumentView, constraintID string,
	wantStatus modelcore.AssemblyConstraintEvaluationStatus, wantSupport modelcore.SupportingElementStatus) {
	t.Helper()
	for _, constraint := range view.Product.Constraints {
		if constraint.ID != constraintID {
			continue
		}
		if constraint.EvaluationStatus != wantStatus {
			t.Fatalf("constraint %s = %s (%s), want %s", constraintID,
				constraint.EvaluationStatus, constraint.EvaluationSummary, wantStatus)
		}
		for index, reference := range []*workspace.AssemblyGeometryRef{&constraint.First, constraint.Second} {
			if reference == nil || (reference.Kind != "EDGE" && reference.Kind != "VERTEX") {
				continue
			}
			if reference.PersistentSelection == nil || reference.GeometryKey != "" || reference.TopologyID != 0 {
				t.Fatalf("topology endpoint %d persisted revision-local evidence: %#v", index, reference)
			}
			if reference.Resolution == nil || reference.Resolution.Result.SupportingElementStatus != wantSupport {
				t.Fatalf("topology endpoint %d resolution = %#v, want %s", index, reference.Resolution, wantSupport)
			}
		}
		return
	}
	t.Fatalf("constraint %s was not found", constraintID)
}
