package control

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/debugartifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/protobuf/encoding/protojson"
)

const p6Actor = "00000000-0000-7000-8000-000000000001"

// TestCutHolePersistentSelectionThroughRealRouter is the P6 executable
// acceptance fixture. It exercises the same Workspace, OCCT Worker, Router,
// artifact and database boundaries used by the application.
func TestCutHolePersistentSelectionThroughRealRouter(t *testing.T) {
	binary, url := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER"), os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if binary == "" || url == "" {
		t.Skip("requires disposable OCCCCAD_TEST_DATABASE_URL and OCCCCAD_TEST_GEOMETRY_WORKER")
	}
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
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port,
		DataDirectory: directory, LogDirectory: t.TempDir()})
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
	service := workspace.NewWithArtifacts(db, client, artifact.NewService(db, store))
	evidenceDirectory := strings.TrimSpace(os.Getenv("OCCCCAD_P6_EVIDENCE_DIR"))
	debugDirectory := t.TempDir()
	if evidenceDirectory != "" {
		if err = os.MkdirAll(evidenceDirectory, 0o750); err != nil {
			t.Fatal(err)
		}
		debugDirectory = filepath.Join(evidenceDirectory, "assembly-replays")
	}
	debugStore, err := debugartifact.NewStore(debugDirectory, 50, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	service.SetDebugArtifactStore(debugStore)

	part, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "P6 Cut Hole Part"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PRODUCT", Name: "P6 Naming Product"})
	if err != nil {
		t.Fatal(err)
	}
	runID := strings.ReplaceAll(product.Document.ID, "-", "")[:12]
	evidence := make([]p6ResolutionEvidence, 0, 8)
	recordEvidence := func(scenario string, view workspace.DocumentView, constraintID string) {
		t.Helper()
		for _, constraint := range view.Product.Constraints {
			if constraint.ID != constraintID {
				continue
			}
			item := p6ResolutionEvidence{Scenario: scenario, ProductRevision: view.Document.VersionID,
				ConstraintStatus: string(constraint.EvaluationStatus)}
			for index, reference := range []*workspace.AssemblyGeometryRef{&constraint.First, constraint.Second} {
				if reference == nil || reference.Resolution == nil {
					continue
				}
				item.Endpoints[index] = p6EndpointEvidence{SupportingElementStatus: string(reference.Resolution.Result.SupportingElementStatus),
					ResolutionStatus: string(reference.Resolution.Result.Status), DiagnosticCode: reference.Resolution.Result.DiagnosticCode}
			}
			evidence = append(evidence, item)
			return
		}
		t.Fatalf("constraint %s was not found while recording %s", constraintID, scenario)
	}
	sequence := 0
	apply := func(documentID string, request workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		sequence++
		if request.RequestID == "" {
			request.RequestID = fmt.Sprintf("p6-%03d", sequence)
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
		t.Helper()
		view := apply(documentID, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: plane, DatumPlaneID: datumPlaneID})
		sketchID := view.Part.Features[len(view.Part.Features)-1].ID
		view = apply(documentID, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID,
			Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &first, Second: &second}}})
		return view, sketchID
	}
	addExtrude := func(documentID, sketchID, operation string, length float64, reversed bool) workspace.DocumentView {
		t.Helper()
		return apply(documentID, workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketchID,
			Generator: "LINEAR_EXTRUDE", Operation: operation, Length: length, Reversed: reversed})
	}

	part, baseSketch := addRectangle(part.Document.ID, "XY", "", workspace.SketchPoint2{X: 0, Y: 0}, workspace.SketchPoint2{X: 20, Y: 20})
	part = addExtrude(part.Document.ID, baseSketch, "NEW_BODY", 10, false)
	baseFeatureID := part.Part.Features[len(part.Part.Features)-1].ID
	topPick := findP6FacePick(t, service, part, func(selection modelcore.PersistentSelection) bool {
		return selection.Anchor.FeatureID == baseFeatureID && strings.HasPrefix(selection.Anchor.OutputSlot, "END_CAP/")
	})
	sidePick := findP6FacePick(t, service, part, func(selection modelcore.PersistentSelection) bool {
		return selection.Anchor.FeatureID == baseFeatureID && strings.HasPrefix(selection.Anchor.OutputSlot, "SIDE_FROM_PROFILE_EDGE/") &&
			math.Abs(selection.CreationEvidence.Centroid[1]) < 1e-9
	})

	product = apply(product.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: "First"})
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: "Second"})
	firstInstance, secondInstance := product.Product.Instances[0].ID, product.Product.Instances[1].ID
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX",
		FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: firstInstance, Kind: "BODY"}})
	product = apply(product.Document.ID, p6FaceConstraintRequest(firstInstance, secondInstance, topPick, topPick))
	faceConstraintID := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	assertP6Constraint(t, product, faceConstraintID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected, "")

	// The XZ through-cut modifies the six base faces and introduces four hole
	// walls. The original top-face selection must still resolve uniquely.
	part, throughSketch := addRectangle(part.Document.ID, "XZ", "", workspace.SketchPoint2{X: 5, Y: 2}, workspace.SketchPoint2{X: 15, Y: 8})
	part = addExtrude(part.Document.ID, throughSketch, "REMOVE", 20, true)
	if got := p6TopologyCount(t, part.Artifact.Topology, "faces"); got != 10 {
		t.Fatalf("through-cut faces = %d, want six inherited plus four hole faces", got)
	}
	modifiedRequestID := runID + "-p6-update-modified"
	product = apply(product.Document.ID, workspace.CommandRequest{RequestID: "p6-update-modified", Type: "UPDATE_REFERENCES"})
	assertP6Constraint(t, product, faceConstraintID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected, "")
	recordEvidence("through-cut-modified", product, faceConstraintID)
	assertP6Replay(t, service, client, product.Document.ID, modifiedRequestID)

	part = apply(part.Document.ID, workspace.CommandRequest{Type: "UNDO"})
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	assertP6Constraint(t, product, faceConstraintID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected, "")
	recordEvidence("part-undo", product, faceConstraintID)
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "REDO"})
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	assertP6Constraint(t, product, faceConstraintID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected, "")
	recordEvidence("part-redo", product, faceConstraintID)
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "UNDO"})

	// Remove the whole upper half from an offset datum plane. The old top face
	// is truly deleted, so Product must persist Broken while the unrelated Fix
	// constraint remains Verified.
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "Cut start",
		Origin: [3]float64{0, 0, 5}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	cutPlaneID := part.DatumPlanes[len(part.DatumPlanes)-1].ID
	part, deleteSketch := addRectangle(part.Document.ID, "", cutPlaneID, workspace.SketchPoint2{X: 0, Y: 0}, workspace.SketchPoint2{X: 20, Y: 20})
	part = addExtrude(part.Document.ID, deleteSketch, "REMOVE", 5, false)
	deleteFeatureID := part.Part.Features[len(part.Part.Features)-1].ID
	product = apply(product.Document.ID, workspace.CommandRequest{RequestID: "p6-update-deleted", Type: "UPDATE_REFERENCES"})
	assertP6Constraint(t, product, faceConstraintID, modelcore.AssemblyConstraintBroken, modelcore.SupportingElementNotConnected, "MISSING")
	recordEvidence("deleted-face", product, faceConstraintID)
	if product.Product.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintVerified {
		t.Fatalf("unrelated FIX constraint = %s, want VERIFIED", product.Product.Constraints[0].EvaluationStatus)
	}

	// Reconnect both occurrences to the newly generated top face, then edit the
	// cut depth. The new selection must survive the following Part Revision.
	newTop := findP6FacePick(t, service, part, func(selection modelcore.PersistentSelection) bool {
		return selection.Anchor.FeatureID == deleteFeatureID && math.Abs(selection.CreationEvidence.Centroid[2]-5) < 1e-9
	})
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: faceConstraintID,
		FirstAssemblyRef: p6AssemblyPick(firstInstance, newTop), SecondAssemblyRef: p6AssemblyPick(secondInstance, newTop), DirectionRelation: "SAME"})
	assertP6Constraint(t, product, faceConstraintID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected, "")
	recordEvidence("reconnected", product, faceConstraintID)
	digest := findP6FeatureDigest(t, part.StructureTree, deleteFeatureID)
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: deleteFeatureID,
		ExpectedFeatureDigest: digest, Length: 6, Unit: "mm"})
	reconnectRequestID := runID + "-p6-update-after-reconnect"
	product = apply(product.Document.ID, workspace.CommandRequest{RequestID: "p6-update-after-reconnect", Type: "UPDATE_REFERENCES"})
	assertP6Constraint(t, product, faceConstraintID, modelcore.AssemblyConstraintVerified, modelcore.SupportingElementConnected, "")
	recordEvidence("reconnected-feature-edited", product, faceConstraintID)
	assertP6Replay(t, service, client, product.Document.ID, reconnectRequestID)

	// Return to the base body, reconnect the original side face and split it
	// with a side opening. Ambiguous lineage must never auto-pick a candidate.
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "UNDO"})
	part = apply(part.Document.ID, workspace.CommandRequest{Type: "UNDO"})
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	assertP6Constraint(t, product, faceConstraintID, modelcore.AssemblyConstraintBroken, modelcore.SupportingElementNotConnected, "")
	recordEvidence("reconnected-feature-removed", product, faceConstraintID)
	currentSide := findP6FacePick(t, service, part, func(selection modelcore.PersistentSelection) bool {
		return selection.Anchor.FeatureID == sidePick.Selection.Anchor.FeatureID && selection.Anchor.OutputSlot == sidePick.Selection.Anchor.OutputSlot
	})
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: faceConstraintID,
		FirstAssemblyRef: p6AssemblyPick(firstInstance, currentSide), SecondAssemblyRef: p6AssemblyPick(secondInstance, currentSide), DirectionRelation: "SAME"})
	part, notchSketch := addRectangle(part.Document.ID, "XY", "", workspace.SketchPoint2{X: 8, Y: 0}, workspace.SketchPoint2{X: 12, Y: 5})
	part = addExtrude(part.Document.ID, notchSketch, "REMOVE", 10, false)
	product = apply(product.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
	assertP6Constraint(t, product, faceConstraintID, modelcore.AssemblyConstraintBroken, modelcore.SupportingElementNotConnected, "AMBIGUOUS")
	recordEvidence("ambiguous-side-split", product, faceConstraintID)
	if evidenceDirectory != "" {
		data, marshalErr := json.MarshalIndent(struct {
			SchemaVersion string                 `json:"schemaVersion"`
			PartID        string                 `json:"partId"`
			ProductID     string                 `json:"productId"`
			Scenarios     []p6ResolutionEvidence `json:"scenarios"`
		}{SchemaVersion: "occccad.p6-resolution-evidence.v1", PartID: part.Document.ID,
			ProductID: product.Document.ID, Scenarios: evidence}, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		path := filepath.Join(evidenceDirectory, "resolution-evidence.json")
		if err = os.WriteFile(path, append(data, '\n'), 0o640); err != nil {
			t.Fatal(err)
		}
		t.Logf("P6 evidence saved to %s", evidenceDirectory)
	}
}

type p6EndpointEvidence struct {
	SupportingElementStatus string `json:"supportingElementStatus"`
	ResolutionStatus        string `json:"resolutionStatus"`
	DiagnosticCode          string `json:"diagnosticCode,omitempty"`
}

type p6ResolutionEvidence struct {
	Scenario         string                `json:"scenario"`
	ProductRevision  string                `json:"productRevision"`
	ConstraintStatus string                `json:"constraintStatus"`
	Endpoints        [2]p6EndpointEvidence `json:"endpoints"`
}

type p6FacePick struct {
	GeometryKey string
	LocalID     uint64
	Selection   modelcore.PersistentSelection
}

func findP6FacePick(t *testing.T, service *workspace.Service, view workspace.DocumentView,
	match func(modelcore.PersistentSelection) bool) p6FacePick {
	t.Helper()
	count := p6TopologyCount(t, view.Artifact.Topology, "faces")
	for localID := uint64(1); localID <= count; localID++ {
		properties, err := service.GetTopologyElementPropertiesAtVersion(t.Context(), view.Document.ID, view.Document.VersionID,
			view.Artifact.GeometryKey, "FACE", localID)
		if err == nil && properties.PersistentSelection != nil && match(*properties.PersistentSelection) {
			if properties.NamingResolution == nil || properties.NamingResolution.Status != modelcore.SelectionResolved {
				t.Fatalf("matching face %d cannot resolve in its creation revision: %#v", localID, properties.NamingResolution)
			}
			return p6FacePick{GeometryKey: view.Artifact.GeometryKey, LocalID: localID, Selection: *properties.PersistentSelection}
		}
	}
	t.Fatal("matching face was not found in the authoritative topology manifest")
	return p6FacePick{}
}

func p6TopologyCount(t *testing.T, topology map[string]any, key string) uint64 {
	t.Helper()
	switch value := topology[key].(type) {
	case uint64:
		return value
	case uint32:
		return uint64(value)
	case int:
		return uint64(value)
	case float64:
		return uint64(value)
	default:
		t.Fatalf("topology %s has unsupported value %#v", key, topology[key])
		return 0
	}
}

func p6AssemblyPick(instanceID string, pick p6FacePick) *workspace.AssemblyGeometryRef {
	return &workspace.AssemblyGeometryRef{InstanceID: instanceID, Kind: "FACE", GeometryKey: pick.GeometryKey, TopologyID: pick.LocalID}
}

func p6FaceConstraintRequest(firstInstance, secondInstance string, first, second p6FacePick) workspace.CommandRequest {
	return workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT",
		FirstAssemblyRef: p6AssemblyPick(firstInstance, first), SecondAssemblyRef: p6AssemblyPick(secondInstance, second), DirectionRelation: "SAME"}
}

func assertP6Constraint(t *testing.T, view workspace.DocumentView, constraintID string,
	wantConstraint modelcore.AssemblyConstraintEvaluationStatus, wantSupport modelcore.SupportingElementStatus, diagnosticFragment string) {
	t.Helper()
	for _, constraint := range view.Product.Constraints {
		if constraint.ID != constraintID {
			continue
		}
		if constraint.EvaluationStatus != wantConstraint {
			t.Fatalf("constraint status = %s (%s), want %s", constraint.EvaluationStatus, constraint.EvaluationSummary, wantConstraint)
		}
		for index, reference := range []*workspace.AssemblyGeometryRef{&constraint.First, constraint.Second} {
			if reference == nil || reference.PersistentSelection == nil || reference.GeometryKey != "" || reference.TopologyID != 0 {
				t.Fatalf("endpoint %d did not persist only PersistentSelection: %#v", index, reference)
			}
			if reference.Resolution == nil || reference.Resolution.Result.SupportingElementStatus != wantSupport {
				t.Fatalf("endpoint %d resolution = %#v, want %s", index, reference.Resolution, wantSupport)
			}
			if diagnosticFragment != "" && !strings.Contains(reference.Resolution.Result.DiagnosticCode, diagnosticFragment) {
				t.Fatalf("endpoint %d diagnostic = %q, want fragment %q", index, reference.Resolution.Result.DiagnosticCode, diagnosticFragment)
			}
		}
		return
	}
	t.Fatalf("constraint %s was not found", constraintID)
}

func findP6FeatureDigest(t *testing.T, node *workspace.DocumentStructureNode, featureID string) string {
	t.Helper()
	if node == nil {
		t.Fatal("document has no structure tree")
	}
	if node.EntityID == featureID && node.DefinitionDigest != "" {
		return node.DefinitionDigest
	}
	for index := range node.Children {
		if digest := findP6FeatureDigestOptional(&node.Children[index], featureID); digest != "" {
			return digest
		}
	}
	t.Fatalf("definition digest for feature %s was not found", featureID)
	return ""
}

func findP6FeatureDigestOptional(node *workspace.DocumentStructureNode, featureID string) string {
	if node.EntityID == featureID {
		return node.DefinitionDigest
	}
	for index := range node.Children {
		if digest := findP6FeatureDigestOptional(&node.Children[index], featureID); digest != "" {
			return digest
		}
	}
	return ""
}

func assertP6Replay(t *testing.T, service *workspace.Service, client *geometry.Client, documentID, requestID string) {
	t.Helper()
	var data []byte
	var err error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		_, data, err = service.ReadAssemblyReplay(t.Context(), documentID, "latest", requestID)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("read replay %s: %v", requestID, err)
	}
	replayed, err := client.ReplayAssembly(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	var file geometry.AssemblyReplay
	if err = json.Unmarshal(replayed, &file); err != nil {
		t.Fatal(err)
	}
	var result workerv1.SolveAssemblyResponse
	if err = protojson.Unmarshal(file.Result, &result); err != nil || result.Status != "CONVERGED" {
		t.Fatalf("replay %s did not converge: %s", requestID, replayed)
	}
}
