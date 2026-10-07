package control

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
)

func TestOffsetSignedProductHistoryThroughRouter(t *testing.T) {
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Skip("requires matching real Geometry Worker")
	}
	db := testsupport.OpenPostgres(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
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
	t.Cleanup(func() { _ = client.Close() })
	local, err := artifact.NewLocalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := artifact.NewService(db, local)
	service := workspace.NewWithArtifacts(db, client, store)
	actor := "00000000-0000-7000-8000-000000000001"
	part, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PART", Name: "Offset shared Part"})
	if err != nil {
		t.Fatal(err)
	}
	partApply := func(req workspace.CommandRequest) {
		t.Helper()
		req.ActorID = actor
		view, e := service.ApplyCommand(t.Context(), part.Document.ID, req)
		if e != nil {
			t.Fatal(req.Type, e)
		}
		part = view
	}
	for bodyIndex := 0; bodyIndex < 2; bodyIndex++ {
		if bodyIndex == 1 {
			partApply(workspace.CommandRequest{Type: "CREATE_BODY"})
		}
		body := part.Part.ActiveBodyID
		partApply(workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: body, Plane: "XY"})
		sketch := part.Part.Features[len(part.Part.Features)-1].ID
		x := float64(30 * bodyIndex)
		partApply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x, Y: 0}, Second: &workspace.SketchPoint2{X: x + 10, Y: 10}}}})
		partApply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: body, SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	}
	product, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PRODUCT", Name: "Signed Offset"})
	if err != nil {
		t.Fatal(err)
	}
	id := product.Document.ID
	seq := 0
	apply := func(request workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		seq++
		request.ActorID = actor
		if request.RequestID == "" {
			request.RequestID = fmt.Sprintf("%s-offset-%d", id, seq)
		}
		view, e := service.ApplyCommand(t.Context(), id, request)
		if e != nil {
			t.Fatalf("%s: %v", request.Type, e)
		}
		product = view
		return view
	}
	rotation := [4]float64{0, math.Sin(0.37), 0, math.Cos(0.37)}
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: "First", Translation: [3]float64{11, 4, 3}, Rotation: rotation})
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: "Second", Translation: [3]float64{18, 6, 9}, Rotation: rotation})
	a, b := product.Product.Instances[0].ID, product.Product.Instances[1].ID
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: a, Translation: [3]float64{11, 4, 3}, Rotation: rotation})
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: b, Translation: [3]float64{18, 6, 9}, Rotation: rotation})
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: a, Kind: "BODY"}})
	first, second := &workspace.AssemblyGeometryRef{InstanceID: a, Kind: "PLANE", GeometryID: "datum-xy"}, &workspace.AssemblyGeometryRef{InstanceID: b, Kind: "PLANE", GeometryID: "datum-xy"}
	check := func(view workspace.DocumentView, target float64) {
		t.Helper()
		poses := map[string]workspace.ProductInstance{}
		for _, p := range view.Product.Instances {
			poses[p.ID] = p
		}
		pa, pb := poses[a], poses[b]
		// Independent original FIRST world normal, known from the fixed rotation.
		n := [3]float64{math.Sin(0.74), 0, math.Cos(0.74)}
		got := 0.
		for i := range n {
			got += n[i] * (pa.Translation[i] - pb.Translation[i])
		}
		if math.Abs(got-target) > 1e-7 || pa.Translation != ([3]float64{11, 4, 3}) {
			t.Fatalf("signed geometry got %g want %g; poses %+v", got, target, poses)
		}
	}
	request := workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", FirstAssemblyRef: first, SecondAssemblyRef: second, Value: 3, DirectionRelation: "SAME", DistanceRelation: "SELECTED_PLANE_NORMAL_V1", QuantityKey: "Gap", RequestID: id + "-signed-preview", ActorID: actor}
	preview, err := service.PreviewCommand(t.Context(), id, request)
	if err != nil {
		t.Fatal(err)
	}
	oldRevision := product.Document.VersionID
	request.PreviewID = preview.PreviewID
	created := apply(request)
	check(created, 3)
	if created.Document.VersionID == oldRevision {
		t.Fatal("commit did not advance")
	}
	gap := created.Product.Constraints[len(created.Product.Constraints)-1].ID
	createdDefinition := created.Product.Constraints[len(created.Product.Constraints)-1]
	if createdDefinition.DefinitionVersion != 2 || createdDefinition.QuantityParameter == nil || createdDefinition.QuantityParameter.ParameterID != "offset:"+gap {
		t.Fatal("new Offset did not persist unique canonical stable quantity", createdDefinition)
	}
	literal := "-2 mm"
	apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: gap, Value: 3, QuantityExpression: &literal, DirectionRelation: "OPPOSITE", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	check(product, -2)
	edited := product
	apply(workspace.CommandRequest{Type: "UNDO"})
	check(product, 3)
	apply(workspace.CommandRequest{Type: "REDO"})
	check(product, -2)
	definition := product.Product.Constraints[len(product.Product.Constraints)-1]
	if definition.DefinitionVersion != 2 || definition.QuantityParameter == nil || definition.QuantityParameter.ParameterID != "offset:"+gap || definition.QuantityParameter.Source.Expression == nil || definition.QuantityParameter.Source.Expression.CheckedAST.Kind == "" {
		t.Fatal("Redo lost AST")
	}

	posesBeforeDisplay := append([]workspace.ProductInstance(nil), product.Product.Instances...)
	statusBeforeDisplay := definition.EvaluationStatus
	apply(workspace.CommandRequest{Type: "SET_DEFINITION_VISIBILITY", TargetKind: "ASSEMBLY_CONSTRAINT", TargetID: gap, Visible: false})
	hidden := product.Product.Constraints[len(product.Product.Constraints)-1]
	if hidden.Visible == nil || *hidden.Visible || hidden.Suppressed || hidden.EvaluationStatus != statusBeforeDisplay || !reflect.DeepEqual(posesBeforeDisplay, product.Product.Instances) {
		t.Fatal("constraint hide changed accepted solve or poses")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if p := product.Product.Constraints[len(product.Product.Constraints)-1]; p.Visible != nil && !*p.Visible {
		t.Fatal("constraint visibility Undo")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	if p := product.Product.Constraints[len(product.Product.Constraints)-1]; p.Visible == nil || *p.Visible {
		t.Fatal("constraint visibility Redo")
	}
	cold := workspace.NewWithArtifacts(db, client, artifact.NewService(db, local))
	reopened, err := cold.GetDocument(t.Context(), id, actor)
	if err != nil {
		t.Fatal(err)
	}
	check(reopened, -2)
	if p := reopened.Product.Constraints[len(reopened.Product.Constraints)-1]; p.Visible == nil || *p.Visible {
		t.Fatal("constraint visibility cold read")
	}
	if reopened.Product.Constraints[len(reopened.Product.Constraints)-1].DistanceRelation != "SELECTED_PLANE_NORMAL_V1" {
		t.Fatal("cold read changed convention")
	}
	measured := "MEASURED"
	apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: gap, Value: -2, ConstraintMode: &measured, DirectionRelation: "OPPOSITE", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	measuredPoses := append([]workspace.ProductInstance(nil), product.Product.Instances...)
	c := product.Product.Constraints[len(product.Product.Constraints)-1]
	if c.MeasuredValue == nil || math.Abs(*c.MeasuredValue+2) > 1e-7 || c.Value != -2 {
		t.Fatal("measurement lost sign/definition", c)
	}
	for _, suppressed := range []bool{true, false} {
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{gap}, Suppressed: &suppressed})
	}
	if product.Product.Constraints[len(product.Product.Constraints)-1].Mode != "MEASURED" || !reflect.DeepEqual(measuredPoses, product.Product.Instances) {
		t.Fatal("Measured lifecycle moved bodies or changed mode")
	}
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: b, Translation: product.Product.Instances[1].Translation, Rotation: [4]float64{0, 0, 0, 1}})
	c = product.Product.Constraints[len(product.Product.Constraints)-1]
	if c.MeasuredValue != nil || c.EvaluationStatus == "VERIFIED" || c.Value != -2 || product.Product.Instances[0].Translation != measuredPoses[0].Translation {
		t.Fatal("nonparallel measurement kept stale value or changed fixed pose", c)
	}
	driving := "DRIVING"
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{gap}, ConstraintMode: &driving})
	check(product, -2)
	// Invalid expression cannot commit a pose or Head.
	before := product.Document.VersionID
	bad := "2 deg"
	_, err = service.ApplyCommand(t.Context(), id, workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", ActorID: actor, TargetID: gap, Value: -2, QuantityExpression: &bad, DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	if err == nil {
		t.Fatal("dimension error committed")
	}
	current, e := service.GetDocument(t.Context(), id, actor)
	if e != nil || current.Document.VersionID != before {
		t.Fatal("invalid command changed Head", e)
	}
	var digest string
	if err = db.QueryRow(t.Context(), `SELECT digest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, product.Document.VersionID).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	var manifestRaw []byte
	if err = db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE digest=$1`, digest).Scan(&manifestRaw); err != nil {
		t.Fatal(err)
	}
	var manifest workspace.AssemblySolveManifest
	if err = json.Unmarshal(manifestRaw, &manifest); err != nil || len(manifest.Bodies) != 2 {
		t.Fatal("CAD Bodies were split into solver components", err)
	}
	replay, err := service.ReplayAssemblySolveManifest(t.Context(), id, digest, id+"-offset-replay")
	if err != nil || replay.Status != "CONVERGED" {
		t.Fatal(replay, err)
	}
	release, err := service.CreateProductRelease(t.Context(), id, workspace.CreateProductReleaseRequest{RequestID: id + "-offset-release", Name: "Frozen signed offset", ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	literal = "4 mm"
	apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: gap, Value: -2, QuantityExpression: &literal, DirectionRelation: "OPPOSITE", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	check(product, 4)
	frozen, err := service.ReplayProductRelease(t.Context(), id, release.ID, id+"-offset-release-replay")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(frozen)
	if len(raw) == 0 || edited.Product.Constraints[len(edited.Product.Constraints)-1].Value != -2 {
		t.Fatal("frozen evidence missing")
	}
	if frozen.Assembly == nil {
		t.Fatal("Release lost assembly evidence")
	}
	for _, body := range replay.Result.Bodies {
		for _, f := range frozen.Assembly.Result.Bodies {
			if body.ID == f.ID && !reflect.DeepEqual(body.Pose, f.Pose) {
				t.Fatal("Release pose changed after Head edit")
			}
		}
	}
	t.Run("stable-expression-reference-through-history", func(t *testing.T) {
		dependent := "Gap + 1 mm"
		apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", FirstAssemblyRef: first, SecondAssemblyRef: second, QuantityExpression: &dependent, QuantityKey: "Derived", DistanceRelation: "SELECTED_PLANE_NORMAL_V1", ConstraintMode: &measured})
		four := "4 mm"
		apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: gap, QuantityExpression: &four, QuantityKey: "Renamed", DirectionRelation: "OPPOSITE", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
		last := product.Product.Constraints[len(product.Product.Constraints)-1]
		if last.DefinitionVersion != 2 || last.QuantityParameter == nil || last.QuantityParameter.ParameterID != "offset:"+last.ID || last.QuantityParameter.Source.Expression == nil || last.QuantityParameter.Source.Expression.CheckedAST.Kind == "" || last.QuantityParameter.Source.Expression.CheckedAST.Left == nil || last.Value != 5 || last.MeasuredValue == nil || math.Abs(*last.MeasuredValue-4) > 1e-7 || last.QuantityParameter.Source.Expression.CheckedAST.Left.ParameterID != "offset:"+gap {
			t.Fatal("reference/measurement boundary", last)
		}
		apply(workspace.CommandRequest{Type: "UNDO"})
		undone := product.Product.Constraints[len(product.Product.Constraints)-1]
		if undone.QuantityParameter == nil || undone.QuantityParameter.Source.Expression == nil || undone.QuantityParameter.Source.Expression.CheckedAST.Kind == "" || undone.QuantityParameter.Source.Expression.CheckedAST.Left == nil || undone.QuantityParameter.Source.Expression.SourceText != "(Gap + 1 mm)" || undone.QuantityParameter.Source.Expression.CheckedAST.Left.ParameterID != "offset:"+gap {
			t.Fatal("Undo lost expression source")
		}
		apply(workspace.CommandRequest{Type: "REDO"})
		redone := product.Product.Constraints[len(product.Product.Constraints)-1]
		if redone.QuantityParameter == nil || redone.QuantityParameter.Source.Expression == nil || redone.QuantityParameter.Source.Expression.CheckedAST.Kind == "" || redone.QuantityParameter.Source.Expression.CheckedAST.Left == nil || redone.QuantityParameter.Source.Expression.SourceText != "(Renamed + 1 mm)" || redone.QuantityParameter.Source.Expression.CheckedAST.Left.ParameterID != "offset:"+gap {
			t.Fatal("Redo lost stable rename")
		}
	})
	t.Run("second-CAD-body-outward-face", func(t *testing.T) {
		body := part.Part.Bodies[1]
		aArtifact := part.Artifacts[body.GeometryKey]
		_, reader, e := store.Open(t.Context(), aArtifact.Representations["BREP"].ObjectID)
		if e != nil {
			t.Fatal(e)
		}
		brep, e := io.ReadAll(reader)
		_ = reader.Close()
		if e != nil {
			t.Fatal(e)
		}
		topology, _, e := client.GetTopology(t.Context(), aArtifact.GeometryID, brep, "FACE", 0)
		if e != nil {
			t.Fatal(e)
		}
		if len(topology.Faces) == 0 {
			t.Fatal("no exact face")
		}
		face := topology.Faces[0]
		var n, origin [3]float64
		normalFound, originFound := false, false
		for _, property := range face.Properties {
			if v := property.GetVectorValue(); v != nil {
				if property.Name == "normal" {
					n = [3]float64{v.X, v.Y, v.Z}
					normalFound = true
				}
				if property.Name == "origin" {
					origin = [3]float64{v.X, v.Y, v.Z}
					originFound = true
				}
			}
		}
		if !normalFound || !originFound {
			t.Fatal("exact plane frame missing")
		}
		selection, e := service.BindPersistentSelection(t.Context(), part.Document.ID, workspace.BindPersistentSelectionRequest{BodyID: body.ID, SourceVersionID: part.Document.VersionID, GeometryKey: body.GeometryKey, Kind: "FACE", LocalID: face.LocalId})
		if e != nil {
			t.Fatal(e)
		}
		if selection.SourceBodyID != body.ID {
			t.Fatal("wrong CAD Body")
		}
		rf := workspace.AssemblyGeometryRef{InstanceID: a, Kind: "FACE", SourceVersionID: part.Document.VersionID, PersistentSelection: &selection}
		rs := rf
		rs.InstanceID = b
		empty := ""
		apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: gap, FirstAssemblyRef: &rf, SecondAssemblyRef: &rs, Value: -3, QuantityExpression: &empty, DirectionRelation: "UNORIENTED", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
		rotate := func(q [4]float64, p [3]float64) [3]float64 {
			u := [3]float64{q[0], q[1], q[2]}
			dot := u[0]*p[0] + u[1]*p[1] + u[2]*p[2]
			cross := [3]float64{u[1]*p[2] - u[2]*p[1], u[2]*p[0] - u[0]*p[2], u[0]*p[1] - u[1]*p[0]}
			out := [3]float64{}
			for i := range out {
				out[i] = 2*dot*u[i] + (q[3]*q[3]-u[0]*u[0]-u[1]*u[1]-u[2]*u[2])*p[i] + 2*q[3]*cross[i]
			}
			return out
		}
		poses := map[string]workspace.ProductInstance{}
		for _, p := range product.Product.Instances {
			poses[p.ID] = p
		}
		na, oa, ob := rotate(poses[a].Rotation, n), rotate(poses[a].Rotation, origin), rotate(poses[b].Rotation, origin)
		got := 0.
		for i := range na {
			got += na[i] * (poses[a].Translation[i] + oa[i] - poses[b].Translation[i] - ob[i])
		}
		if math.Abs(got+3) > 1e-7 {
			t.Fatal("material normal signed geometry", got)
		}
		var c workspace.AssemblyConstraint
		for _, candidate := range product.Product.Constraints {
			if candidate.ID == gap {
				c = candidate
			}
		}
		if c.First.PersistentSelection == nil || c.First.PersistentSelection.SourceBodyID != body.ID || c.Second.InstanceID != b || c.EvaluationStatus != "VERIFIED" {
			t.Fatal("shared Part occurrence/Body support mismatch", c)
		}
	})
	t.Run("nested-occurrence-frame", func(t *testing.T) {
		sub, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PRODUCT", Name: "Offset nested"})
		if e != nil {
			t.Fatal(e)
		}
		run := func(document string, req workspace.CommandRequest) workspace.DocumentView {
			t.Helper()
			req.ActorID = actor
			v, err := service.ApplyCommand(t.Context(), document, req)
			if err != nil {
				t.Fatal(req.Type, err)
			}
			return v
		}
		sub = run(sub.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
		leaf := sub.Product.Instances[0].ID
		sub = run(sub.Document.ID, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: leaf, Translation: [3]float64{2, 3, 4}, Rotation: rotation})
		root, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PRODUCT", Name: "Offset root"})
		if e != nil {
			t.Fatal(e)
		}
		for i := 0; i < 2; i++ {
			root = run(root.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: sub.Document.ID})
		}
		x, y := root.Product.Instances[0].ID, root.Product.Instances[1].ID
		outer := [4]float64{0, 0, math.Sin(.23), math.Cos(.23)}
		root = run(root.Document.ID, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: x, Translation: [3]float64{11, 4, 3}, Rotation: outer})
		root = run(root.Document.ID, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: y, Translation: [3]float64{18, 6, 9}, Rotation: outer})
		root = run(root.Document.ID, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: x, Kind: "BODY"}})
		ref := func(instance string) *workspace.AssemblyGeometryRef {
			return &workspace.AssemblyGeometryRef{InstanceID: instance, Kind: "PLANE", GeometryID: "datum-xy", InstancePath: &workspace.InstancePath{Segments: []workspace.InstancePathSegment{{InstanceID: instance, ReferencedDocumentID: sub.Document.ID}, {InstanceID: leaf, ReferencedDocumentID: part.Document.ID}}}}
		}
		root = run(root.Document.ID, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", FirstAssemblyRef: ref(x), SecondAssemblyRef: ref(y), Value: -5, DistanceRelation: "SELECTED_PLANE_NORMAL_V1", DirectionRelation: "SAME"})
		// Independently transform the leaf origin through each FINAL outer pose;
		// normal crosses BOTH frames, not just the root component rotation.
		n := [3]float64{math.Cos(.46) * math.Sin(.74), math.Sin(.46) * math.Sin(.74), math.Cos(.74)}
		rotate := func(q [4]float64, p [3]float64) [3]float64 {
			u := [3]float64{q[0], q[1], q[2]}
			cross := [3]float64{u[1]*p[2] - u[2]*p[1], u[2]*p[0] - u[0]*p[2], u[0]*p[1] - u[1]*p[0]}
			dot, squared := u[0]*p[0]+u[1]*p[1]+u[2]*p[2], u[0]*u[0]+u[1]*u[1]+u[2]*u[2]
			var result [3]float64
			for i := range result {
				result[i] = 2*dot*u[i] + (q[3]*q[3]-squared)*p[i] + 2*q[3]*cross[i]
			}
			return result
		}
		p1 := rotate(root.Product.Instances[0].Rotation, [3]float64{2, 3, 4})
		p2 := rotate(root.Product.Instances[1].Rotation, [3]float64{2, 3, 4})
		n2 := rotate(root.Product.Instances[1].Rotation, [3]float64{math.Sin(.74), 0, math.Cos(.74)})
		got := 0.
		alignment := 0.
		for i := range n {
			got += n[i] * (root.Product.Instances[0].Translation[i] + p1[i] - root.Product.Instances[1].Translation[i] - p2[i])
			alignment += n[i] * n2[i]
		}
		if math.Abs(got+5) > 1e-7 || math.Abs(alignment-1) > 1e-8 || root.Product.Instances[0].Translation != ([3]float64{11, 4, 3}) || root.Product.Constraints[1].EvaluationStatus != "VERIFIED" {
			t.Fatal("nested signed frame", got, root.Product.Constraints)
		}
	})
}
