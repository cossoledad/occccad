package control

import (
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"net"
	"os"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
)

// Exercises the actual Router -> C++ Worker path, including every M2.5 field.
func TestAssemblyMotionThroughRealRouter(t *testing.T) {
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Skip("OCCCCAD_TEST_GEOMETRY_WORKER is not set")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	pool := NewGeometryPool(t.Context(), GeometryPoolConfig{WorkerBinary: binary, WorkerHost: "127.0.0.1", FirstWorkerPort: port, LogDirectory: t.TempDir(), DataDirectory: t.TempDir()})
	t.Cleanup(pool.Close)
	if err = pool.Start(); err != nil {
		t.Fatal(err)
	}
	client, err := geometry.Open(serveGeometry(t, pool))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	identity := geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}
	bodies := []geometry.AssemblyBody{{ID: "a", Pose: identity}, {ID: "b", Pose: geometry.AssemblyPose{Translation: [3]float64{3, 0, 0}, Rotation: identity.Rotation}}}
	descriptors := []geometry.AssemblyGeometry{{ID: "p", BodyID: "a", Kind: "POINT"}, {ID: "p", BodyID: "b", Kind: "POINT"}}
	constraints := []geometry.AssemblyConstraint{{ID: "fixed", Kind: "FIX", FirstBodyID: "a", FixedPose: &identity}, {ID: "join", Kind: "COINCIDENT", FirstBodyID: "a", FirstGeometryID: "p", SecondBodyID: "b", SecondGeometryID: "p"}}
	budget := uint64(100)
	options := geometry.AssemblySolveOptions{Intent: &geometry.AssemblySolveIntent{MovingBodyIDs: []string{"a"}, ReferenceBodyIDs: []string{"b"}, PreferencePolicy: "MOVE_FIRST_MINIMIZE_REFERENCE"}, SolverProfile: &geometry.AssemblySolverProfile{SchemaVersion: 2, MotionLengthScale: 2, MotionAngleScale: 3, MaxPreferenceIterations: &budget, VerifyAnalyticJacobians: true}}
	result, err := client.SolveAssemblyWithOptions(t.Context(), "m25-router", bodies, descriptors, constraints, options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "CONVERGED" || result.SolverBuild != "assembly-m2.5-hierarchy-v1" || len(result.Components) != 1 {
		t.Fatalf("invalid result: %+v", result)
	}
	p := result.Components[0].Preference
	if p.Status != geometry.PreferenceConverged || !p.GeometricallyFeasible || p.LengthScale != 2 || p.AngleScale != 3 || len(p.Bodies) != 2 || p.ReferenceObjective < 2.249999 || p.ReferenceObjective > 2.250001 {
		t.Fatalf("preference lost or incorrect: %+v", p)
	}
	f := result.Components[0].Freedoms
	if len(f) != 2 || f[1].Kind != geometry.FreedomSpherical || len(f[1].AllowedBasis) != 3 || len(f[1].BlockedBasis) != 3 || len(f[1].Rotations) != 3 {
		t.Fatalf("freedoms lost: %+v", f)
	}
	budget = 0
	result, err = client.SolveAssemblyWithOptions(t.Context(), "m25-budget", bodies, descriptors, constraints, options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "CONVERGED" || result.Components[0].Preference.Status != geometry.PreferenceIterationLimit {
		t.Fatalf("preference budget conflated with geometry: %+v", result)
	}
	options.SolverProfile.SchemaVersion = 1
	if _, err = client.SolveAssemblyWithOptions(t.Context(), "m25-old-profile", bodies, descriptors, constraints, options); err == nil {
		t.Fatal("old development profile unexpectedly accepted")
	}
	options.SolverProfile.SchemaVersion = 2
	budget = 100
	// The nominal pose must survive transport separately from a feasible seed.
	bodies = bodies[1:]
	bodies[0].InitialGuess = &identity
	options.Intent = nil
	result, err = client.SolveAssemblyWithOptions(t.Context(), "m25-warm", bodies, nil, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Bodies[0].Pose.Translation[0] < 2.999999 || result.Components[0].Preference.TotalObjective > 1e-12 {
		t.Fatalf("nominal pose overwritten by seed: %+v", result)
	}
}

func TestAssemblyProductMotionHistoryThroughRouter(t *testing.T) {
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
	service := workspace.NewWithArtifacts(db, client, artifact.NewService(db, store))
	part, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: "00000000-0000-7000-8000-000000000001", Type: "PART", Name: "M25 Part"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: "00000000-0000-7000-8000-000000000001", Type: "PRODUCT", Name: "M25 Product"})
	if err != nil {
		t.Fatal(err)
	}
	id := product.Document.ID
	apply := func(request workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		request.ActorID = "00000000-0000-7000-8000-000000000001"
		view, applyErr := service.ApplyCommand(t.Context(), id, request)
		if applyErr != nil {
			t.Fatalf("%s: %v", request.Type, applyErr)
		}
		return view
	}
	product = apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: "First", Translation: [3]float64{0, 0, 0}})
	product = apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: "Second", Translation: [3]float64{0, 0, 8}})
	a, b := product.Product.Instances[0].ID, product.Product.Instances[1].ID
	product = apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: a, Kind: "BODY"}})
	first := &workspace.AssemblyGeometryRef{InstanceID: a, Kind: "PLANE", GeometryID: "datum-xy"}
	second := &workspace.AssemblyGeometryRef{InstanceID: b, Kind: "PLANE", GeometryID: "datum-xy"}
	request := workspace.CommandRequest{ActorID: "00000000-0000-7000-8000-000000000001", Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", FirstAssemblyRef: first, SecondAssemblyRef: second, Value: 2, DirectionRelation: "SAME", DistanceRelation: "UNSIGNED"}
	preview, err := service.PreviewCommand(t.Context(), id, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.AssemblyComponents) != 1 || preview.AssemblyComponents[0].Preference.Status != geometry.PreferenceConverged {
		t.Fatalf("preview evidence missing: %+v", preview)
	}
	refreshed, err := service.GetDocument(t.Context(), id, "00000000-0000-7000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Document.VersionID != product.Document.VersionID {
		t.Fatal("preview advanced head")
	}
	created := apply(request)
	check := func(view workspace.DocumentView, want float64) {
		t.Helper()
		if math.Abs(view.Product.Instances[0].Translation[2]) > 1e-7 || math.Abs(view.Product.Instances[1].Translation[2]-want) > 1e-6 {
			t.Fatalf("fixed first/reference movement: %+v", view.Product.Instances)
		}
	}
	check(created, 2)
	for _, p := range preview.InstancePoses {
		for _, instance := range created.Product.Instances {
			if p.InstanceID == instance.ID && math.Abs(p.Translation[2]-instance.Translation[2]) > 1e-7 {
				t.Fatal("preview/commit diverged")
			}
		}
	}
	constraintID := created.Product.Constraints[len(created.Product.Constraints)-1].ID
	edited := apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: constraintID, Value: 4, DirectionRelation: "SAME", DistanceRelation: "UNSIGNED"})
	check(edited, 4)
	refreshed, err = service.GetDocument(t.Context(), id, "00000000-0000-7000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	check(refreshed, 4)
	check(apply(workspace.CommandRequest{Type: "UNDO"}), 2)
	undone := apply(workspace.CommandRequest{Type: "UNDO"})
	check(undone, 8)
	if !undone.Document.CanRedo {
		t.Fatal("redo capability missing")
	}
	check(apply(workspace.CommandRequest{Type: "REDO"}), 2)
	redone := apply(workspace.CommandRequest{Type: "REDO"})
	check(redone, 4)
	if redone.Document.CanRedo {
		t.Fatal("redo capability not exhausted")
	}
	// Dependency snapshots and pose compensation must also survive a fresh service instance.
	service = workspace.NewWithArtifacts(db, client, artifact.NewService(db, store))
	refreshed, err = service.GetDocument(t.Context(), id, "00000000-0000-7000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	check(refreshed, 4)
}
