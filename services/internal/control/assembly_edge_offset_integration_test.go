package control

import (
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"testing"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

// Real independent Part B-Reps, persistent EDGE selections, Router, Worker and
// transaction history: not a Datum approximation of the reported edge failure.
func TestOffsetParallelPartEdgesThroughRouter(t *testing.T) {
	binary, url := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER"), os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if binary == "" || url == "" {
		t.Skip("requires disposable TEST_DATABASE_URL and matching real Worker")
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
	parts := make([]workspace.DocumentView, 2)
	for i := range parts {
		part, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PART", Name: fmt.Sprintf("Independent edge Part %d", i)})
		if e != nil {
			t.Fatal(e)
		}
		apply := func(req workspace.CommandRequest) {
			t.Helper()
			req.ActorID = actor
			part, e = service.ApplyCommand(t.Context(), part.Document.ID, req)
			if e != nil {
				t.Fatal(req.Type, e)
			}
		}
		body := part.Part.ActiveBodyID
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: body, Plane: "XY"})
		sketch := part.Part.Features[len(part.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -60, Y: -60}, Second: &workspace.SketchPoint2{X: 50, Y: 60}}}})
		apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", BodyID: body, SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 40})
		parts[i] = part
	}
	product, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PRODUCT", Name: "Parallel Part edge Offset"})
	if err != nil {
		t.Fatal(err)
	}
	id, seq := product.Document.ID, 0
	apply := func(req workspace.CommandRequest) {
		t.Helper()
		seq++
		req.ActorID = actor
		req.RequestID = fmt.Sprintf("%s-edge-%d", id, seq)
		product, err = service.ApplyCommand(t.Context(), id, req)
		if err != nil {
			t.Fatal(req.Type, err)
		}
	}
	half := math.Sqrt(.5)
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: parts[0].Document.ID, Name: "Reference"})
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: parts[1].Document.ID, Name: "Moving"})
	a, b := product.Product.Instances[0].ID, product.Product.Instances[1].ID
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: a, Translation: [3]float64{0, 0, 10}, Rotation: [4]float64{0, -half, 0, half}})
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: b, Rotation: [4]float64{0, 0, 0, 1}})
	refs := make([]workspace.AssemblyGeometryRef, 2)
	var origins, directions [2][3]float64
	for i, part := range parts {
		body := part.Part.Bodies[0]
		art := part.Artifacts[body.GeometryKey]
		_, reader, e := store.Open(t.Context(), art.Representations["BREP"].ObjectID)
		if e != nil {
			t.Fatal(e)
		}
		brep, e := io.ReadAll(reader)
		_ = reader.Close()
		if e != nil {
			t.Fatal(e)
		}
		topology, _, e := client.GetTopology(t.Context(), art.GeometryID, brep, "EDGE", 0)
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, edge := range topology.Edges {
			selection, e := service.BindPersistentSelection(t.Context(), part.Document.ID, workspace.BindPersistentSelectionRequest{BodyID: body.ID, SourceVersionID: part.Document.VersionID, GeometryKey: body.GeometryKey, Kind: "EDGE", LocalID: edge.LocalId})
			if e != nil {
				t.Fatal(e)
			}
			o, d := selection.CreationEvidence.Origin, selection.CreationEvidence.Direction
			x := 50.
			if i == 1 {
				x = -60
			}
			if math.Abs(o[0]-x) > 1e-7 || math.Abs(o[2]-40) > 1e-7 || math.Abs(math.Abs(d[1])-1) > 1e-7 {
				continue
			}
			instance := a
			if i == 1 {
				instance = b
			}
			refs[i] = workspace.AssemblyGeometryRef{InstanceID: instance, Kind: "EDGE", SourceVersionID: part.Document.VersionID, PersistentSelection: &selection}
			origins[i], directions[i], found = o, d, true
			break
		}
		if !found {
			t.Fatal("exact off-origin parallel edge not found", i)
		}
	}
	rotate := func(q [4]float64, p [3]float64) [3]float64 {
		u := [3]float64{q[0], q[1], q[2]}
		dot := u[0]*p[0] + u[1]*p[1] + u[2]*p[2]
		c := [3]float64{u[1]*p[2] - u[2]*p[1], u[2]*p[0] - u[0]*p[2], u[0]*p[1] - u[1]*p[0]}
		var out [3]float64
		for j := range out {
			out[j] = 2*dot*u[j] + (q[3]*q[3]-u[0]*u[0]-u[1]*u[1]-u[2]*u[2])*p[j] + 2*q[3]*c[j]
		}
		return out
	}
	initial := product
	check := func(view workspace.DocumentView, target float64) {
		t.Helper()
		var p [2]workspace.ProductInstance
		for _, instance := range view.Product.Instances {
			if instance.ID == a {
				p[0] = instance
			}
			if instance.ID == b {
				p[1] = instance
			}
		}
		oa, ob, da, db := rotate(p[0].Rotation, origins[0]), rotate(p[1].Rotation, origins[1]), rotate(p[0].Rotation, directions[0]), rotate(p[1].Rotation, directions[1])
		var delta [3]float64
		for j := range delta {
			delta[j] = p[1].Translation[j] + ob[j] - p[0].Translation[j] - oa[j]
		}
		cross := [3]float64{da[1]*db[2] - da[2]*db[1], da[2]*db[0] - da[0]*db[2], da[0]*db[1] - da[1]*db[0]}
		crossNorm := math.Sqrt(cross[0]*cross[0] + cross[1]*cross[1] + cross[2]*cross[2])
		projection := delta[0]*da[0] + delta[1]*da[1] + delta[2]*da[2]
		actual := math.Sqrt(math.Max(0, delta[0]*delta[0]+delta[1]*delta[1]+delta[2]*delta[2]-projection*projection))
		if crossNorm > 1e-12 {
			actual = math.Abs(delta[0]*cross[0]+delta[1]*cross[1]+delta[2]*cross[2]) / crossNorm
		}
		if math.Abs(actual-target) > 1e-7 {
			t.Fatalf("independent edge distance %g want %g", actual, target)
		}
		if p[0].Translation != initial.Product.Instances[0].Translation || p[0].Rotation != initial.Product.Instances[0].Rotation {
			t.Fatal("reference preference moved the second selection")
		}
		c := view.Product.Constraints[0]
		if c.EvaluationStatus != "VERIFIED" || c.First.InstanceID != b || c.Second.InstanceID != a || c.First.PersistentSelection.SourceDocumentID == c.Second.PersistentSelection.SourceDocumentID {
			t.Fatalf("wrong evaluation/support identity: %+v", c)
		}
	}
	request := workspace.CommandRequest{ActorID: actor, Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", FirstAssemblyRef: &refs[1], SecondAssemblyRef: &refs[0], Value: 30, DistanceRelation: "UNSIGNED"}
	preview, err := service.PreviewCommand(t.Context(), id, request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ConstraintEvaluation == nil || preview.ConstraintEvaluation.Status != "VERIFIED" || preview.PreviewID == "" {
		t.Fatalf("preview: %+v", preview)
	}
	request.PreviewID = preview.PreviewID
	apply(request)
	check(product, 30)
	if product.Product.Instances[1].Translation == initial.Product.Instances[1].Translation && product.Product.Instances[1].Rotation == initial.Product.Instances[1].Rotation {
		t.Fatal("unsatisfied edges did not move")
	}
	constraint := product.Product.Constraints[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: constraint, Value: 35, DistanceRelation: "UNSIGNED"})
	check(product, 35)
	apply(workspace.CommandRequest{Type: "UNDO"})
	check(product, 30)
	apply(workspace.CommandRequest{Type: "REDO"})
	check(product, 35)
	cold := workspace.NewWithArtifacts(db, client, artifact.NewService(db, local))
	reopened, err := cold.GetDocument(t.Context(), id, actor)
	if err != nil {
		t.Fatal(err)
	}
	check(reopened, 35)
}
