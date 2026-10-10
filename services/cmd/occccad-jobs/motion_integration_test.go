package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/control"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/geometryrpc"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
)

type motionRemoteStore struct{ *artifact.LocalStore }

func (motionRemoteStore) Backend() string { return "TEST" }
func TestMotionStudyPersistedRouterArtifactJobLifecycle(t *testing.T) {
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Fatal("matching-source C++ Worker required")
	}
	db, e := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "motion.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(t.Context(), db); e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	pool := control.NewGeometryPool(t.Context(), control.GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, DataDirectory: root, LogDirectory: t.TempDir(), MinimumWorkers: 1, MaximumWorkers: 1, GeometryCapacity: 100})
	defer pool.Close()
	if e = pool.Start(); e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	router := grpc.NewServer(geometryrpc.ServerOptions()...)
	defer router.Stop()
	workerv1.RegisterGeometryWorkerServer(router, pool)
	go router.Serve(listener)
	local, e := artifact.NewLocalStore(root)
	if e != nil {
		t.Fatal(e)
	}
	remote, e := artifact.NewLocalStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	var store artifact.Store = motionRemoteStore{remote}
	if os.Getenv("OCCCCAD_TEST_MOTION_S3") == "1" {
		store, local, e = artifact.OpenConfigured(t.Context(), root)
		if e != nil {
			t.Fatal(e)
		}
		if store.Backend() != "S3" {
			t.Fatal("S3 verification requires configured S3 backend")
		}
	}
	client, e := geometry.Open(listener.Addr().String(), geometry.ArtifactStaging(store, local))
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	objects := artifact.NewService(db, store, local)
	service := workspace.NewWithArtifacts(db, client, objects)
	queue := jobs.New(db)
	actor := "00000000-0000-7000-8000-000000000001"
	view, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, RequestID: uuid.NewString(), Name: "fourbar-job", Type: "PRODUCT"})
	if e != nil {
		t.Fatal(e)
	}
	view, e = service.CreateFourBarDemo(t.Context(), view.Document.ID, actor, view.Document.VersionID)
	if e != nil {
		t.Fatal(e)
	}
	if len(view.Product.Kinematics.Studies) != 1 {
		t.Fatal("demo definition missing")
	}
	poses, _ := json.Marshal(view.Product.Instances)
	k := view.Product.Kinematics
	command := func(r workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		r.ActorID = actor
		r.RequestID = uuid.NewString()
		v, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	undo := command(workspace.CommandRequest{Type: "UNDO"})
	if len(undo.Product.Kinematics.Studies) != 0 {
		t.Fatal("undo study failed")
	}
	after, _ := json.Marshal(undo.Product.Instances)
	if !bytes.Equal(poses, after) {
		t.Fatal("definition undo moved assembly")
	}
	view = command(workspace.CommandRequest{Type: "REDO"})
	if len(view.Product.Kinematics.Studies) != 1 {
		t.Fatal("redo study failed")
	}
	_, e = service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{RequestID: uuid.NewString(), ActorID: actor, Type: "SAVE_KINEMATICS", VersionID: "stale", Kinematics: &k})
	if e == nil {
		t.Fatal("stale definition save admitted")
	}
	snap, e := service.FreezeMotionStudy(t.Context(), view.Document.ID, workspace.MotionRunRequest{BaseRevisionID: view.Document.VersionID, AnalysisID: k.Analyses[0].ID})
	if e != nil {
		t.Fatal(e)
	}
	if len(snap.GeometryUnits) != 4 || len(snap.Pairs) != 6 {
		t.Fatal("incomplete instance geometry", len(snap.GeometryUnits), len(snap.Pairs))
	}
	if snap.GeometryUnits[0].GeometryID != snap.GeometryUnits[2].GeometryID || snap.GeometryUnits[0].ID == snap.GeometryUnits[2].ID {
		t.Fatal("shared definition lost distinct instance identity")
	}
	enqueue := func(s workspace.MotionSnapshot) jobs.Job {
		t.Helper()
		b, _ := json.Marshal(s)
		o, e := objects.Put(t.Context(), artifact.Kind("MOTION_SNAPSHOT"), "application/json", bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		j, e := queue.Enqueue(t.Context(), jobs.EnqueueRequest{Type: "MOTION_STUDY", DocumentID: view.Document.ID, VersionID: &s.RevisionID, RequestedBy: actor, InputObjectID: o.ID, IdempotencyKey: uuid.NewString(), Payload: map[string]any{"inputDigest": s.Digest}, UserVisible: true})
		if e != nil {
			t.Fatal(e)
		}
		return j
	}
	j := enqueue(snap)
	claimed, e := queue.Claim(t.Context(), "motion-integration", time.Minute)
	if e != nil || claimed.ID != j.ID {
		t.Fatal(claimed, e)
	}
	newer := command(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &k})
	h := handler{workerID: "motion-integration", database: db, queue: queue, artifacts: objects, access: access.New(db), geometry: client, workspace: service}
	if e = h.execute(t.Context(), claimed); e != nil {
		t.Fatal(e)
	}
	completed, e := queue.Get(t.Context(), j.ID)
	if e != nil || completed.State != "SUCCEEDED" || completed.ResultObjectID == nil {
		t.Fatal(completed, e)
	}
	read := func(j jobs.Job) workspace.MotionRun {
		t.Helper()
		_, r, e := objects.Open(t.Context(), *j.ResultObjectID)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Close()
		var result workspace.MotionRun
		if e = json.NewDecoder(r).Decode(&result); e != nil {
			t.Fatal(e)
		}
		return result
	}
	result := read(completed)
	if !result.Completed || len(result.Frames) != 21 || result.Snapshot.RevisionID != snap.RevisionID {
		t.Fatalf("%s %d %+v", result.Status, len(result.Frames), result.Failure)
	}
	for _, f := range result.Frames {
		if !f.KinematicValid || f.DMU == nil || !f.DMU.Complete || len(f.DMU.Pairs) != 6 || f.DMUConclusion != "VIOLATION" {
			t.Fatal("motion/DMU state conflated", f)
		}
	}
	head, e := service.GetDocument(t.Context(), view.Document.ID)
	if e != nil || head.Document.VersionID != newer.Document.VersionID {
		t.Fatal("run wrote Head", e)
	}
	j = enqueue(snap)
	claimed, e = queue.Claim(t.Context(), h.workerID, time.Minute)
	if e != nil || claimed.ID != j.ID {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	partial, e := service.RunMotionStudy(ctx, snap, func(n int) error {
		if n == 2 {
			_, e := queue.RequestCancel(t.Context(), j.ID, actor, false)
			cancel()
			return e
		}
		return nil
	})
	if e != nil || partial.Status != "CANCELED" || len(partial.Frames) != 2 {
		t.Fatal("cancel lost frames", partial.Status, len(partial.Frames), e)
	}
	data, _ := json.Marshal(partial)
	obj, e := objects.Put(t.Context(), artifact.Kind("MOTION_RUN"), "application/json", bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	if e = queue.FinishMotion(t.Context(), j.ID, h.workerID, claimed.AttemptCount, obj.ID); e != nil {
		t.Fatal(e)
	}
	j, e = queue.Get(t.Context(), j.ID)
	if e != nil || j.State != "CANCELED" || len(read(j).Frames) != 2 {
		t.Fatal(j, e)
	}
	static, e := service.FreezeMotionStudy(t.Context(), view.Document.ID, workspace.MotionRunRequest{BaseRevisionID: newer.Document.VersionID, CurrentOnly: true, Clearance: workspace.MotionQuantity{Unit: "mm"}})
	if e != nil {
		t.Fatal(e)
	}
	r, e := service.RunMotionStudy(t.Context(), static, nil)
	if e != nil || !r.Completed || len(r.Frames) != 1 || r.Frames[0].KinematicValid || r.Frames[0].DMUConclusion != "VIOLATION" {
		t.Fatal(r, e)
	}

	// A nested Product containing a multi-Body Part remains one rigid motion unit.
	docCommand := func(id string, r workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		r.RequestID = uuid.NewString()
		r.ActorID = actor
		v, e := service.ApplyCommand(t.Context(), id, r)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	crank := view.Product.Instances[1]
	part := docCommand(crank.ReferencedDocumentID, workspace.CommandRequest{Type: "CREATE_BODY", Name: "DMU 标记 Body"})
	body := part.Part.ActiveBodyID
	part = docCommand(part.Document.ID, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY", BodyID: body})
	sketch := ""
	for _, f := range part.Part.Features {
		if f.Type == "SKETCH" && f.BodyID == body {
			sketch = f.ID
		}
	}
	points := []workspace.SketchPoint2{{X: 5, Y: 6}, {X: 7, Y: 6}, {X: 7, Y: 8}, {X: 5, Y: 8}}
	edgeIds := []string{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
	ops := []workspace.SketchOperation{}
	for i := range 4 {
		a, b := points[i], points[(i+1)%4]
		e := workspace.SketchEntity{ID: edgeIds[i], Kind: "LINE", Role: "PROFILE", Start: &a, End: &b}
		ops = append(ops, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &e})
	}
	for i := range 4 {
		c := workspace.SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: edgeIds[i], SubElement: "END"}, {Target: "ENTITY", EntityID: edgeIds[(i+1)%4], SubElement: "START"}}}
		ops = append(ops, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &c})
	}
	part = docCommand(part.Document.ID, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: ops})
	part = docCommand(part.Document.ID, workspace.CommandRequest{Type: "PAD_SKETCH", SketchID: sketch, Length: 2, BodyID: body})
	sub, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, RequestID: uuid.NewString(), Name: "rigid sub Product", Type: "PRODUCT"})
	if e != nil {
		t.Fatal(e)
	}
	sub = docCommand(sub.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	sub = docCommand(sub.Document.ID, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: sub.Product.Instances[0].ID, Translation: [3]float64{3, 4, 5}, Rotation: [4]float64{0, 0, math.Sin(math.Pi / 8), math.Cos(math.Pi / 8)}})
	view = docCommand(view.Document.ID, workspace.CommandRequest{Type: "REPLACE_INSTANCE", InstanceID: crank.ID, ReferencedDocumentID: sub.Document.ID})
	k = view.Product.Kinematics
	k.Studies[0].Frames = 3
	k.Analyses[0].IncludeSameUnit = true
	view = docCommand(view.Document.ID, workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &k})
	nested, e := service.FreezeMotionStudy(t.Context(), view.Document.ID, workspace.MotionRunRequest{BaseRevisionID: view.Document.VersionID, AnalysisID: k.Analyses[0].ID})
	if e != nil {
		t.Fatal(e)
	}
	nestedIds := map[string]bool{}
	for _, g := range nested.GeometryUnits {
		if g.MotionUnitID == crank.ID {
			if len(g.Address.InstancePath.Segments) != 2 {
				t.Fatal("lost nested occurrence path")
			}
			nestedIds[g.ID] = true
		}
	}
	if len(nestedIds) != 2 || len(nested.GeometryUnits) != 5 {
		t.Fatal("multi Body merged into one check unit", nested.GeometryUnits)
	}
	nestedRun, e := service.RunMotionStudy(t.Context(), nested, nil)
	if e != nil || !nestedRun.Completed {
		t.Fatalf("nested run %s %+v %v", nestedRun.Status, nestedRun.Failure, e)
	}
	for _, f := range nestedRun.Frames {
		found := false
		for _, p := range f.DMU.Pairs {
			if nestedIds[p.FirstID] && nestedIds[p.SecondID] {
				found = true
				if !p.Complete || p.Classification != "SEPARATED" || math.Abs(p.DistanceMM-4.5) > 1e-6 {
					t.Fatal("descendant relative pose was not frozen", p)
				}
			}
		}
		if !found {
			t.Fatal("same unit pair omitted despite explicit request")
		}
	}
	scope := []workspace.DMUAddress{}
	for _, g := range nested.GeometryUnits {
		if nestedIds[g.ID] {
			scope = append(scope, g.Address)
		}
	}
	scoped, e := service.FreezeMotionStudy(t.Context(), view.Document.ID, workspace.MotionRunRequest{BaseRevisionID: view.Document.VersionID, CurrentOnly: true, Scope: scope, IncludeSameUnit: true, Clearance: workspace.MotionQuantity{Value: 1, Unit: "mm"}})
	if e != nil || len(scoped.Pairs) != 1 {
		t.Fatal("scope", e)
	}
	rr, e := service.RunMotionStudy(t.Context(), scoped, nil)
	if e != nil || !rr.Completed || rr.Frames[0].DMUConclusion != "PASS" {
		t.Fatal("scoped static DMU", rr.Status, rr.Failure, e)
	}
	scope[0].InstancePath.Segments[1].ResolvedVersionID = "wrong-source"
	if _, e = service.FreezeMotionStudy(t.Context(), view.Document.ID, workspace.MotionRunRequest{BaseRevisionID: view.Document.VersionID, CurrentOnly: true, Scope: scope, IncludeSameUnit: true, Clearance: workspace.MotionQuantity{Unit: "mm"}}); e == nil {
		t.Fatal("stale descendant version scope accepted")
	}
	entries, e := os.ReadDir(filepath.Join(local.Root(), "exchange", "inputs"))
	if e != nil && !os.IsNotExist(e) {
		t.Fatal(e)
	}
	if len(entries) > 0 {
		t.Fatal("RPC input materialization leaked")
	}
	empty, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, RequestID: uuid.NewString(), Name: "empty geometry unit", Type: "PART"})
	if e != nil {
		t.Fatal(e)
	}
	view = docCommand(view.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: empty.Document.ID})
	if _, e = service.FreezeMotionStudy(t.Context(), view.Document.ID, workspace.MotionRunRequest{BaseRevisionID: view.Document.VersionID, CurrentOnly: true, Clearance: workspace.MotionQuantity{Unit: "mm"}}); e == nil {
		t.Fatal("all-scope DMU silently omitted a unit without exact geometry")
	}
}
