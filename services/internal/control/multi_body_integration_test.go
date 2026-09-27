package control

import (
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
	"net"
	"os"
	"testing"
)

func TestMultiBodyIndependentGeometryAndHistory(t *testing.T) {
	binary, url := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER"), os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if binary == "" || url == "" {
		t.Skip("requires isolated DB and real worker")
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
	listener.Close()
	dir := t.TempDir()
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: dir, LogDirectory: t.TempDir()})
	t.Cleanup(pool.Close)
	if err = pool.Start(); err != nil {
		t.Fatal(err)
	}
	client, err := geometry.Open(serveGeometry(t, pool))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	local, err := artifact.NewLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := artifact.NewService(db, local)
	service := workspace.NewWithArtifacts(db, client, store)
	part, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Multi Body"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(req workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		seq++
		req.ActorID = p6Actor
		req.RequestID = fmt.Sprintf("%s-%d", part.Document.ID, seq)
		v, e := service.ApplyCommand(t.Context(), part.Document.ID, req)
		if e != nil {
			t.Fatalf("%s: %v", req.Type, e)
		}
		part = v
		return v
	}
	rectangle := func(body string, x, y, w, h float64) string {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: body, Plane: "XY"})
		id := part.Part.Features[len(part.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: y}, Second: &workspace.SketchPoint2{X: x + w, Y: y + h}}}})
		return id
	}
	extrude := func(body, sketch, op string, length float64) {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: body, SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: op, Length: length})
	}
	first := part.Part.ActiveBodyID
	extrude(first, rectangle(first, 0, 0, 10, 10), "ADD", 10)
	apply(workspace.CommandRequest{Type: "CREATE_BODY"})
	second := part.Part.ActiveBodyID
	extrude(second, rectangle(second, 30, 0, 10, 10), "ADD", 10)
	if len(part.Part.Bodies) != 2 || part.Artifact != nil {
		t.Fatal("Part must expose bodies, not one artifact")
	}
	baseline := part
	secondKey := part.Part.Bodies[1].GeometryKey
	for _, b := range part.Part.Bodies {
		a := part.Artifacts[b.GeometryKey]
		if len(a.Representations) != 3 || a.BodyID != b.ID || !a.Naming.CanBind {
			t.Fatalf("incomplete body: %+v %+v", b, a)
		}
		for _, r := range a.Representations {
			_, reader, e := store.Open(t.Context(), r.ObjectID)
			if e != nil {
				t.Fatal(e)
			}
			reader.Close()
		}
	}
	selections := map[string]modelcore.PersistentSelection{}
	for _, b := range part.Part.Bodies {
		sel, e := service.BindPersistentSelection(t.Context(), part.Document.ID, workspace.BindPersistentSelectionRequest{BodyID: b.ID, SourceVersionID: part.Document.VersionID, GeometryKey: b.GeometryKey, Kind: "FACE", LocalID: 1})
		if e != nil || sel.SourceBodyID != b.ID {
			t.Fatalf("bind %s: %v %+v", b.ID, e, sel)
		}
		selections[b.ID] = sel
	}
	if _, e := service.BindPersistentSelection(t.Context(), part.Document.ID, workspace.BindPersistentSelectionRequest{BodyID: first, SourceVersionID: part.Document.VersionID, GeometryKey: secondKey, Kind: "FACE", LocalID: 1}); e == nil {
		t.Fatal("cross Body locator accepted")
	}
	extrude(first, rectangle(first, 2, 2, 2, 2), "REMOVE", 10)
	if part.Part.Bodies[1].GeometryKey != secondKey || part.Artifacts[secondKey].Representations["NAMING"] != baseline.Artifacts[secondKey].Representations["NAMING"] {
		t.Fatal("unmodified Body regenerated")
	}
	resolution, e := service.ResolvePersistentSelection(t.Context(), part.Document.ID, workspace.ResolvePersistentSelectionRequest{Selection: selections[second], SourceVersionID: baseline.Document.VersionID, TargetVersionID: part.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
	if e != nil || resolution.Status != modelcore.SelectionResolved {
		t.Fatalf("unchanged body resolve: %+v %v", resolution, e)
	}

	// ADD and preview promotion retain the other Body's frozen representation.
	sketch := rectangle(first, 8, 2, 4, 4)
	seq++
	request := workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-%d", part.Document.ID, seq), Type: "CREATE_SOLID_FEATURE", BodyID: first, SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10}
	preview, e := service.PreviewCommand(t.Context(), part.Document.ID, request)
	if e != nil || preview.Artifact == nil || preview.Artifact.BodyID != first {
		t.Fatalf("Body preview: %+v %v", preview, e)
	}
	request.PreviewID = preview.PreviewID
	part, e = service.ApplyCommand(t.Context(), part.Document.ID, request)
	if e != nil {
		t.Fatal(e)
	}
	if part.Part.Bodies[1].GeometryKey != secondKey {
		t.Fatal("preview commit changed sibling Body")
	}
	beforeDelete := part
	apply(workspace.CommandRequest{Type: "DELETE_BODY", BodyID: first})
	if len(part.Part.Bodies) != 1 {
		t.Fatal("delete")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if len(part.Part.Bodies) != 2 || part.Part.Bodies[0].GeometryKey != beforeDelete.Part.Bodies[0].GeometryKey {
		t.Fatal("undo did not restore Body geometry")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if len(part.Part.Bodies) != 1 {
		t.Fatal("redo")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	apply(workspace.CommandRequest{Type: "SET_BODY_VISIBILITY", BodyID: second, Visible: false})
	if part.Part.Bodies[1].Visible {
		t.Fatal("hide")
	}
	apply(workspace.CommandRequest{Type: "RENAME_BODY", BodyID: second, Name: "Second"})
	apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: first})
	product, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PRODUCT", Name: "Multi Body Product"})
	if e != nil {
		t.Fatal(e)
	}
	product, e = service.ApplyCommand(t.Context(), product.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: product.Document.ID + "-insert", Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	if e != nil {
		t.Fatal(e)
	}
	if len(product.ResolvedInstances) != 2 || len(product.Artifacts) != 2 {
		t.Fatalf("occurrence bodies: %+v", product.ResolvedInstances)
	}
	raw, _ := json.Marshal(part)
	var view map[string]json.RawMessage
	json.Unmarshal(raw, &view)
	if _, ok := view["artifact"]; ok {
		t.Fatal("single Part artifact remains")
	}
	graph, e := service.ExchangeExportGraph(t.Context(), part.Document.ID, "")
	if e != nil {
		t.Fatal(e)
	}
	if len(graph.Definitions) != 1 || len(graph.Definitions[0].BodyBreps) != 2 {
		t.Fatal("export split business Part definition")
	}
	exported, e := client.ExportExchange(t.Context(), part.Document.ID+"-export", "STEP", artifact.StagingKey(part.Document.ID, "multi.step"), graph)
	if e != nil {
		t.Fatal(e)
	}
	inspected, e := client.InspectExchange(t.Context(), part.Document.ID+"-inspect", "STEP", exported)
	if e != nil {
		t.Fatal(e)
	}
	if len(inspected.Definitions) != 1 || inspected.Definitions[0].Kind != "PART" {
		t.Fatal("multi-body export changed Part business identity")
	}
	// NEW_BODY is structural and Undo/Redo must cover its Body and feature together.
	thirdSketch := rectangle(first, 50, 0, 2, 2)
	extrude(first, thirdSketch, "NEW_BODY", 2)
	if len(part.Part.Bodies) != 3 || part.Part.ActiveBodyID == first {
		t.Fatal("NEW_BODY did not create a Body")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if len(part.Part.Bodies) != 2 {
		t.Fatal("NEW_BODY undo")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if len(part.Part.Bodies) != 3 {
		t.Fatal("NEW_BODY redo")
	}

	// Deleting the generator removes its Body, retaining the independent profile.
	explicitFeature := part.Part.Features[len(part.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "FEATURE", TargetID: explicitFeature})
	if len(part.Part.Bodies) != 2 {
		t.Fatal("orphaned explicit generator Body")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if len(part.Part.Bodies) != 3 {
		t.Fatal("generator deletion undo lost Body")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if len(part.Part.Bodies) != 2 {
		t.Fatal("generator deletion redo retained Body")
	}

	// An ordinary ADD, not NEW_BODY, must route a disconnected exact solid.
	isolatedSketch := rectangle(first, 80, 0, 4, 4)
	firstKey := part.Part.Bodies[0].GeometryKey
	seq++
	autoRequest := workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-%d", part.Document.ID, seq), Type: "CREATE_SOLID_FEATURE", BodyID: first, SketchID: isolatedSketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 5}
	autoPreview, e := service.PreviewCommand(t.Context(), part.Document.ID, autoRequest)
	if e != nil || autoPreview.Artifact == nil || autoPreview.Artifact.BodyID == first {
		t.Fatalf("disconnected preview: %+v %v", autoPreview, e)
	}
	autoRequest.PreviewID = autoPreview.PreviewID
	part, e = service.ApplyCommand(t.Context(), part.Document.ID, autoRequest)
	if e != nil {
		t.Fatal(e)
	}
	autoBody := part.Part.Bodies[len(part.Part.Bodies)-1]
	autoFeature := part.Part.Features[len(part.Part.Features)-1]
	if len(part.Part.Bodies) != 3 || autoBody.ID != autoPreview.Artifact.BodyID || autoFeature.BodyID != autoBody.ID || autoBody.CreatedByFeatureID != autoFeature.ID || part.Part.Bodies[0].GeometryKey != firstKey {
		t.Fatal("automatic Body identity or sibling geometry changed")
	}
	if len(part.Artifacts[autoBody.GeometryKey].Representations) != 3 {
		t.Fatal("auto Body lacks authoritative artifacts")
	}
	autoSelection, e := service.BindPersistentSelection(t.Context(), part.Document.ID, workspace.BindPersistentSelectionRequest{BodyID: autoBody.ID, SourceVersionID: part.Document.VersionID, GeometryKey: autoBody.GeometryKey, Kind: "FACE", LocalID: 1})
	if e != nil || autoSelection.SourceBodyID != autoBody.ID {
		t.Fatalf("auto Body naming: %+v %v", autoSelection, e)
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if len(part.Part.Bodies) != 2 {
		t.Fatal("auto create undo")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if len(part.Part.Bodies) != 3 || part.Part.Bodies[2].GeometryKey != autoBody.GeometryKey {
		t.Fatal("auto create redo")
	}
	apply(workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "FEATURE", TargetID: autoFeature.ID})
	if len(part.Part.Bodies) != 2 || part.Part.Bodies[0].GeometryKey != firstKey {
		t.Fatal("auto generator delete")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if len(part.Part.Bodies) != 3 || part.Part.Bodies[2].GeometryKey != autoBody.GeometryKey {
		t.Fatal("auto delete undo")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if len(part.Part.Bodies) != 2 {
		t.Fatal("auto delete redo")
	}
	// The non-preview path uses the same classification and keeps REMOVE failures.
	extrude(first, isolatedSketch, "ADD", 5)
	if len(part.Part.Bodies) != 3 {
		t.Fatal("direct ADD did not create Body")
	}
	seq++
	_, e = service.ApplyCommand(t.Context(), part.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-%d", part.Document.ID, seq), Type: "CREATE_SOLID_FEATURE", BodyID: first, SketchID: isolatedSketch, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: 5})
	if e == nil {
		t.Fatal("non-intersecting REMOVE must fail instead of creating Body")
	}

}
