package control

import (
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestSolidBooleanLifecycleThroughRouter(t *testing.T) {
	binary, url := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER"), os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if binary == "" || url == "" {
		t.Skip("requires geometry worker and test database")
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
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Solid Boolean lifecycle"})
	if err != nil {
		t.Fatal(err)
	}
	doc := view.Document.ID
	sequence := 0
	apply := func(request workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		sequence++
		request.ActorID = p6Actor
		request.RequestID = fmt.Sprintf("%s-solid-%d", doc, sequence)
		if request.Type == "CREATE_MODIFY_FEATURE" || (request.Type == "CREATE_SOLID_FEATURE" && request.Feature != nil && request.Feature.Type == "LOFT") {
			head := view.Document.VersionID
			candidate, e := service.PreviewCommand(t.Context(), doc, request)
			if e != nil || candidate.Artifact == nil || candidate.Artifact.Volume <= 0 {
				t.Fatalf("%s preview: %v %+v", request.Type, e, candidate.Artifact)
			}
			unchanged, e := service.GetDocument(t.Context(), doc, p6Actor)
			if e != nil || unchanged.Document.VersionID != head {
				t.Fatal("feature preview changed Head")
			}
		}
		v, e := service.ApplyCommand(t.Context(), doc, request)
		if e != nil {
			t.Fatalf("%s: %v", request.Type, e)
		}
		return v
	}
	rectangle := func(x, y, w, h, length float64) workspace.Feature {
		t.Helper()
		view = apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sketch := view.Part.Features[len(view.Part.Features)-1].ID
		view = apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: y}, Second: &workspace.SketchPoint2{X: x + w, Y: y + h}}}})
		view = apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: length})
		return view.Part.Features[len(view.Part.Features)-1]
	}
	base := rectangle(0, 0, 20, 20, 10)
	tool := rectangle(5, 5, 5, 5, 20)
	before := view.Document.VersionID
	input := workspace.CommandRequest{Type: "CREATE_BOOLEAN_FEATURE", BodyID: base.BodyID, Operation: "REMOVE", Tools: []workspace.FeatureStageRef{{BodyID: tool.BodyID, FeatureID: tool.ID}}, RequestID: doc + "-preview", ActorID: p6Actor}
	preview, err := service.PreviewCommand(t.Context(), doc, input)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Artifact == nil || math.Abs(preview.Artifact.Volume-3750) > 1e-6 {
		t.Fatalf("incorrect exact preview: %+v", preview.Artifact)
	}
	reopened, err := service.GetDocument(t.Context(), doc, p6Actor)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Document.VersionID != before {
		t.Fatal("preview mutated head")
	}
	view = apply(input)
	boolean := view.Part.Features[len(view.Part.Features)-1]
	check := func(v workspace.DocumentView, volume float64, consumed bool) {
		t.Helper()
		for _, b := range v.Part.Bodies {
			if b.ID == base.BodyID {
				if a := v.Artifacts[b.GeometryKey]; math.Abs(a.Volume-volume) > 1e-6 {
					t.Fatalf("volume %v want %v", a.Volume, volume)
				}
			}
			if b.ID == tool.BodyID && (b.Consumed != consumed || !b.Visible) {
				t.Fatal("incorrect tool output/visibility")
			}
		}
	}
	check(view, 3750, true)
	view = apply(workspace.CommandRequest{Type: "UNDO"})
	check(view, 4000, false)
	view = apply(workspace.CommandRequest{Type: "REDO"})
	check(view, 3750, true)
	var digest string
	var find func([]workspace.DocumentStructureNode)
	find = func(nodes []workspace.DocumentStructureNode) {
		for _, n := range nodes {
			if n.EntityID == boolean.ID {
				digest = n.DefinitionDigest
			}
			find(n.Children)
		}
	}
	find([]workspace.DocumentStructureNode{*view.StructureTree})
	boolean.KeepTools = true
	view = apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: boolean.ID, ExpectedFeatureDigest: digest, Feature: &boolean})
	check(view, 3750, false)
	// Change both target and explicit tool stage; Body identities remain stable.
	digest = ""
	find([]workspace.DocumentStructureNode{*view.StructureTree})
	swapped := boolean
	swapped.BodyID = tool.BodyID
	swapped.Tools = []workspace.FeatureStageRef{{BodyID: base.BodyID, FeatureID: base.ID}}
	swapped.Operation = "INTERSECT"
	view = apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: boolean.ID, ExpectedFeatureDigest: digest, Feature: &swapped})
	for _, body := range view.Part.Bodies {
		if body.ID == tool.BodyID && math.Abs(view.Artifacts[body.GeometryKey].Volume-250) > 1e-6 {
			t.Fatal("editing Boolean target/tool failed")
		}
	}
	view = apply(workspace.CommandRequest{Type: "UNDO"})
	check(view, 3750, false)
	service = workspace.NewWithArtifacts(db, client, artifacts)
	reopened, err = service.GetDocument(t.Context(), doc, p6Actor)
	if err != nil {
		t.Fatal(err)
	}
	check(reopened, 3750, false)
	view = apply(workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "FEATURE", TargetID: boolean.ID})
	check(view, 4000, false)
	view = apply(workspace.CommandRequest{Type: "UNDO"})
	check(view, 3750, false)
	// First acceptance chain: cross-Body cut -> persistent edge chamfer -> upstream edit.
	view = apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: base.BodyID})
	pick := findP7TopologyPick(t, service, view, "EDGE", func(_ workspace.TopologyElementProperties, selection modelcore.PersistentSelection) bool {
		return selection.Anchor.FeatureID == base.ID && strings.HasPrefix(selection.Anchor.OutputSlot, "VERTICAL_FROM_PROFILE_ENDPOINTS/")
	})
	modifier := workspace.Feature{Type: "CHAMFER", BodyID: base.BodyID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: pick.Selection, SourceVersionID: pick.SourceVersionID}}}
	view = apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &modifier})
	chamfer := view.Part.Features[len(view.Part.Features)-1]
	pick = findP7TopologyPick(t, service, view, "EDGE", func(properties workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
		return properties.GeometryType == "LINE"
	})
	view = apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: base.BodyID, Length: 0.5, Selections: []workspace.FeatureSelection{{Selection: pick.Selection, SourceVersionID: pick.SourceVersionID}}}})
	// Editing restores the exact upstream edge instead of picking the final
	// fillet result. A replacement pick carries its immutable source stage.
	editedFillet := view.Part.Features[len(view.Part.Features)-1]
	editingInput, inputErr := service.GetFeatureInput(t.Context(), doc, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: editedFillet.ID})
	if inputErr != nil {
		t.Fatal(inputErr)
	}
	if len(editingInput.Picks) != 1 || editingInput.SourceFeatureID != chamfer.ID {
		t.Fatal("editing did not restore exact upstream selection")
	}
	rebound, inputErr := service.GetFeatureInput(t.Context(), doc, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: editedFillet.ID, GeometryKey: editingInput.Artifact.GeometryKey, Kind: editingInput.Picks[0].Kind, LocalID: editingInput.Picks[0].LocalID})
	if inputErr != nil || rebound.Selection == nil {
		t.Fatalf("input rebind: %v", inputErr)
	}
	if _, inputErr = service.GetFeatureInput(t.Context(), doc, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: editedFillet.ID, GeometryKey: "wrong-body", Kind: "EDGE", LocalID: editingInput.Picks[0].LocalID}); inputErr == nil {
		t.Fatal("accepted foreign stage geometry")
	}
	unchanged, inputErr := service.GetDocument(t.Context(), doc)
	if inputErr != nil || unchanged.Document.VersionID != view.Document.VersionID {
		t.Fatal("input query changed Head")
	}
	editedFillet.Selections = []workspace.FeatureSelection{*rebound.Selection}
	var inputDigest string
	var findInputDigest func([]workspace.DocumentStructureNode)
	findInputDigest = func(nodes []workspace.DocumentStructureNode) {
		for _, node := range nodes {
			if node.EntityID == editedFillet.ID {
				inputDigest = node.DefinitionDigest
			}
			findInputDigest(node.Children)
		}
	}
	findInputDigest([]workspace.DocumentStructureNode{*view.StructureTree})
	view = apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: editedFillet.ID, ExpectedFeatureDigest: inputDigest, Feature: &editedFillet})
	if math.Abs(activeBodyArtifact(t, view).Volume-activeBodyArtifact(t, unchanged).Volume) > 1e-7 {
		t.Fatal("stage selection edit changed geometry")
	}

	downstream := view.Part.Features[len(view.Part.Features)-1].ID
	expectedVolume := activeBodyArtifact(t, view).Volume
	if !(expectedVolume < 3750 && expectedVolume > 3700) {
		t.Fatalf("bad chamfer volume %g", expectedVolume)
	}
	view = apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + base.ID + ":length", Value: 12, Unit: "mm"})
	if activeBodyArtifact(t, view).Volume <= expectedVolume {
		t.Fatal("upstream edit did not recompute chamfer")
	}
	// Excessive radius/distance fails without pretending the previous result is current.
	digest = ""
	var findFeature func([]workspace.DocumentStructureNode, string)
	findFeature = func(nodes []workspace.DocumentStructureNode, id string) {
		for _, n := range nodes {
			if n.EntityID == id {
				digest = n.DefinitionDigest
			}
			findFeature(n.Children, id)
		}
	}
	findFeature([]workspace.DocumentStructureNode{*view.StructureTree}, chamfer.ID)
	chamfer.Length = 1000
	view = apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: chamfer.ID, ExpectedFeatureDigest: digest, Feature: &chamfer})
	var revisionState string
	if e := db.QueryRow(t.Context(), `SELECT state FROM occccad.document_versions WHERE id=$1`, view.Document.VersionID).Scan(&revisionState); e != nil {
		t.Fatal(e)
	}
	for _, f := range view.Part.Features {
		if f.ID == downstream && f.EvaluationStatus != "BLOCKED" {
			t.Fatal("failed descendant is not blocked")
		}
	}
	if revisionState != "FAILED" {
		t.Fatalf("failed feature accepted as %s", revisionState)
	}
	for _, b := range view.Part.Bodies {
		if b.ID == base.BodyID && b.GeometryKey != "" {
			t.Fatal("failed Body retained stale geometry")
		}
	}
	if _, e := service.ExchangeExportGraph(t.Context(), doc, ""); e == nil {
		t.Fatal("failed Part exported stale result")
	}
	view = apply(workspace.CommandRequest{Type: "RENAME_FEATURE", TargetID: chamfer.ID, Name: "Failed chamfer"})
	if e := db.QueryRow(t.Context(), `SELECT state FROM occccad.document_versions WHERE id=$1`, view.Document.VersionID).Scan(&revisionState); e != nil || revisionState != "FAILED" {
		t.Fatal("metadata edit cleared failed Revision state")
	}
	view = apply(workspace.CommandRequest{Type: "UNDO"}) // Undo metadata while geometry is still failed.
	view = apply(workspace.CommandRequest{Type: "UNDO"}) // Undo the invalid dimension.
	view = apply(workspace.CommandRequest{Type: "REDO"}) // Failed definitions are still replayable history.
	if e := db.QueryRow(t.Context(), `SELECT state FROM occccad.document_versions WHERE id=$1`, view.Document.VersionID).Scan(&revisionState); e != nil || revisionState != "FAILED" {
		t.Fatal("redo lost failed definition")
	}
	view = apply(workspace.CommandRequest{Type: "UNDO"})
	if activeBodyArtifact(t, view).Volume <= expectedVolume {
		t.Fatal("undo did not recover evaluated output")
	}
	edit := func(feature workspace.Feature) workspace.DocumentView {
		t.Helper()
		digest = ""
		findFeature([]workspace.DocumentStructureNode{*view.StructureTree}, feature.ID)
		return apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: feature.ID, ExpectedFeatureDigest: digest, Feature: &feature})
	}
	assertReady := func(v workspace.DocumentView) {
		t.Helper()
		for _, f := range v.Part.Features {
			if f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
				t.Fatalf("unexpected %s %s: %s", f.ID, f.EvaluationStatus, f.Diagnostic)
			}
		}
	}
	// Second acceptance chain: draft all walls, shell, then cut a through opening.
	secondBase := rectangle(40, 0, 20, 20, 10)
	view = apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: secondBase.BodyID})
	var sides []workspace.FeatureSelection
	artifactValue := activeBodyArtifact(t, view)
	for id := uint64(1); id <= p6TopologyCount(t, artifactValue.Topology, "faces"); id++ {
		properties, e := service.GetTopologyElementPropertiesAtVersion(t.Context(), doc, view.Document.VersionID, artifactValue.GeometryKey, "FACE", id)
		if e != nil {
			t.Fatal(e)
		}
		if properties.PersistentSelection != nil && strings.HasPrefix(properties.PersistentSelection.Anchor.OutputSlot, "SIDE_FROM_PROFILE_EDGE/") {
			sides = append(sides, workspace.FeatureSelection{Selection: *properties.PersistentSelection, SourceVersionID: view.Document.VersionID})
		}
	}
	view = apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "DRAFT", BodyID: secondBase.BodyID, Angle: 5, NeutralPlaneID: "datum-xy", Selections: sides}})
	top := findP6FacePick(t, service, view, func(selection modelcore.PersistentSelection) bool {
		return strings.HasPrefix(selection.Anchor.OutputSlot, "END_CAP/")
	})
	view = apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "SHELL", BodyID: secondBase.BodyID, Length: 1, Selections: []workspace.FeatureSelection{{Selection: top.Selection, SourceVersionID: view.Document.VersionID}}}})
	shellFeature := view.Part.Features[len(view.Part.Features)-1]
	shellVolume := activeBodyArtifact(t, view).Volume
	opening := rectangle(49, -1, 2, 4, 20)
	view = apply(workspace.CommandRequest{Type: "CREATE_BOOLEAN_FEATURE", BodyID: secondBase.BodyID, Operation: "REMOVE", Tools: []workspace.FeatureStageRef{{BodyID: opening.BodyID, FeatureID: opening.ID}}})
	view = apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: secondBase.BodyID})
	if activeBodyArtifact(t, view).Volume >= shellVolume {
		t.Fatal("shell opening did not remove material")
	}
	secondBefore := activeBodyArtifact(t, view).Volume
	view = apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + secondBase.ID + ":length", Value: 12, Unit: "mm"})
	assertReady(view)
	if math.Abs(activeBodyArtifact(t, view).Volume-secondBefore) < 1 {
		t.Fatal("draft/shell chain did not recompute")
	}
	brokenShell := shellFeature
	brokenShell.Length = 1000
	view = edit(brokenShell)
	if view.Part.Features[len(view.Part.Features)-1].EvaluationStatus != "BLOCKED" {
		t.Fatalf("shell failure did not block cut: %s", view.Part.Features[len(view.Part.Features)-1].EvaluationStatus)
	}
	view = edit(shellFeature)
	assertReady(view)
	// Third acceptance chain: three offset datum sections -> loft -> Boolean -> fillet.
	var sections []workspace.LoftSection
	for i := 0; i < 3; i++ {
		view = apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: fmt.Sprintf("Section %d", i), Origin: [3]float64{0, 0, float64(i * 10)}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
		plane := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
		view = apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: plane})
		sketch := view.Part.Features[len(view.Part.Features)-1].ID
		view = apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 80, Y: 0}, Second: &workspace.SketchPoint2{X: 100, Y: 20}}}})
		length := 20.0
		sectionSketch := view.Part.Features[len(view.Part.Features)-1]
		view = apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: fmt.Sprintf("section-width-%d", i), Kind: "LENGTH", Unit: "mm", Value: &length, References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: sectionSketch.Sketch.Entities[0].ID, SubElement: "WHOLE"}}}}}})
		sections = append(sections, workspace.LoftSection{SketchID: sketch})
	}
	view = apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "LOFT", Operation: "NEW_BODY", Sections: sections}})
	loft := view.Part.Features[len(view.Part.Features)-1]
	hole := rectangle(85, 5, 5, 5, 25)
	view = apply(workspace.CommandRequest{Type: "CREATE_BOOLEAN_FEATURE", BodyID: loft.BodyID, Operation: "REMOVE", Tools: []workspace.FeatureStageRef{{BodyID: hole.BodyID, FeatureID: hole.ID}}})
	view = apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: loft.BodyID})
	pick = findP7TopologyPick(t, service, view, "EDGE", func(properties workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
		return properties.GeometryType == "LINE"
	})
	view = apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: loft.BodyID, Length: 0.5, Selections: []workspace.FeatureSelection{{Selection: pick.Selection, SourceVersionID: pick.SourceVersionID}}}})
	if activeBodyArtifact(t, view).Volume >= 7500 {
		t.Fatal("loft Boolean/fillet chain volume")
	}
	last := view.Part.Features[len(view.Part.Features)-1]
	finalVolume := activeBodyArtifact(t, view).Volume
	last.Suppressed = true
	view = edit(last)
	assertReady(view)
	if math.Abs(activeBodyArtifact(t, view).Volume-7500) > 1e-5 {
		t.Fatalf("suppression did not restore Boolean input: %.12g", activeBodyArtifact(t, view).Volume)
	}
	if view.Part.Features[len(view.Part.Features)-1].EvaluationStatus != "SUPPRESSED" {
		t.Fatal("suppression was confused with failure")
	}
	view = apply(workspace.CommandRequest{Type: "UNDO"})
	assertReady(view)
	if math.Abs(activeBodyArtifact(t, view).Volume-finalVolume) > 1e-6 {
		t.Fatal("modifier undo failed")
	}
	view = apply(workspace.CommandRequest{Type: "REDO"})
	assertReady(view)
	last.Suppressed = false
	view = edit(last)
	assertReady(view)
	view = apply(workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "FEATURE", TargetID: last.ID})
	if math.Abs(activeBodyArtifact(t, view).Volume-7500) > 1e-5 {
		t.Fatal("modifier delete failed")
	}
	view = apply(workspace.CommandRequest{Type: "UNDO"})
	assertReady(view)
	loft.Ruled = true
	view = edit(loft)
	assertReady(view)
	if math.Abs(activeBodyArtifact(t, view).Volume-finalVolume) > 1e-4 {
		t.Fatal("loft edit changed straight prism volume")
	}
	view = apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: "parameter:" + sections[1].SketchID + ":constraint:section-width-1:value", Value: 21, Unit: "mm"})
	assertReady(view)
	if activeBodyArtifact(t, view).Volume <= finalVolume {
		t.Fatal("upstream section parameter did not recompute loft/Boolean/fillet")
	}
	view = apply(workspace.CommandRequest{Type: "UNDO"})
	assertReady(view)
	if math.Abs(activeBodyArtifact(t, view).Volume-finalVolume) > 1e-4 {
		t.Fatal("section history did not recover original loft")
	}
	last.Length = 1000
	view = edit(last)
	if view.Part.Features[len(view.Part.Features)-1].EvaluationStatus != "FAILED" {
		t.Fatal("bad loft-chain fillet not failed")
	}
	view = apply(workspace.CommandRequest{Type: "UNDO"})
	assertReady(view)
	graph, e := service.ExchangeExportGraph(t.Context(), doc, "", view.Document.VersionID)
	if e != nil {
		t.Fatal(e)
	}
	expectedBodies, expectedSolids := 0, 0
	for _, b := range view.Part.Bodies {
		if b.GeometryKey != "" && !b.Consumed {
			expectedBodies++
			if view.Artifacts[b.GeometryKey].Volume > 0 {
				expectedSolids++
			}
		}
	}
	exportedBodies := 0
	for _, d := range graph.Definitions {
		exportedBodies += len(d.BodyBreps)
	}
	if exportedBodies != expectedSolids {
		t.Fatalf("export includes consumed body: %d want %d", exportedBodies, expectedSolids)
	}
	exported, e := client.ExportExchange(t.Context(), doc+"-export", "STEP", artifact.StagingKey(doc, "solid-chains.step"), graph)
	if e != nil || exported.Size == 0 {
		t.Fatalf("STEP chain export: %v", e)
	}
	product, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PRODUCT", Name: "Solid Feature output acceptance"})
	if e != nil {
		t.Fatal(e)
	}
	product, e = service.ApplyCommand(t.Context(), product.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: doc + "-insert", Type: "INSERT_INSTANCE", ReferencedDocumentID: doc})
	if e != nil {
		t.Fatal(e)
	}
	if len(product.ResolvedInstances) != expectedBodies {
		t.Fatalf("Product output count %d want %d", len(product.ResolvedInstances), expectedBodies)
	}
	for _, instance := range product.ResolvedInstances {
		for _, body := range view.Part.Bodies {
			if instance.BodyID == body.ID && body.Consumed {
				t.Fatal("Product exposes consumed Body")
			}
		}
	}
	boolean.KeepTools = false
	view = edit(boolean)
	assertReady(view)
	plan, e := service.GetProductUpdatePlan(t.Context(), product.Document.ID)
	if e != nil || !plan.CanAccept || !plan.HasUpdates {
		t.Fatalf("Product update plan: %+v %v", plan, e)
	}
	product, e = service.ApplyCommand(t.Context(), product.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: doc + "-update-output", Type: "UPDATE_REFERENCES", UpdatePlanDigest: plan.Digest})
	if e != nil {
		t.Fatal(e)
	}
	if len(product.ResolvedInstances) != expectedBodies-1 {
		t.Fatal("Product did not accept changed tool output participation")
	}
	for _, instance := range product.ResolvedInstances {
		if instance.BodyID == tool.BodyID {
			t.Fatal("Product retained consumed tool")
		}
	}
	pool.Close()
	coldPool := NewGeometryPool(t.Context(), GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: directory, LogDirectory: t.TempDir()})
	t.Cleanup(coldPool.Close)
	if err = coldPool.Start(); err != nil {
		t.Fatal(err)
	}
	coldClient, err := geometry.Open(serveGeometry(t, coldPool))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coldClient.Close() })
	coldService := workspace.NewWithArtifacts(db, coldClient, artifacts)
	reopened, err = coldService.GetDocument(t.Context(), doc, p6Actor)
	if err != nil {
		t.Fatal(err)
	}
	if activeBodyArtifact(t, reopened).GeometryID != activeBodyArtifact(t, view).GeometryID {
		t.Fatal("saved chain changed after reopen")
	}
	coldArtifact := activeBodyArtifact(t, reopened)
	properties, e := coldService.GetTopologyElementPropertiesAtVersion(t.Context(), doc, reopened.Document.VersionID, coldArtifact.GeometryKey, "FACE", 1)
	if e != nil || properties.PersistentSelection == nil {
		t.Fatalf("cold BREP/Naming restore: %v", e)
	}

}
