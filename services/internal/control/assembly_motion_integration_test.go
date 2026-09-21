package control

import (
	"encoding/json"
	"math"
	"net"
	"os"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/debugartifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/protobuf/encoding/protojson"
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
	fixture, err := os.ReadFile("../../../tests/assembly-corpus/face4-face6.3dreplay")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := client.ReplayAssembly(t.Context(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	var replay geometry.AssemblyReplay
	if err = json.Unmarshal(replayed, &replay); err != nil {
		t.Fatal(err)
	}
	var replayResult workerv1.SolveAssemblyResponse
	if err = protojson.Unmarshal(replay.Result, &replayResult); err != nil {
		t.Fatal(err)
	}
	if replayResult.Status != "CONVERGED" || replayResult.Components[0].Preference.Status != workerv1.AssemblyPreferenceStatus_PREFERENCE_CONVERGED {
		t.Fatalf("real face replay failed: %s", replayed)
	}

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
	if result.Status != "CONVERGED" || result.SolverBuild != "assembly-m2.5-hierarchy-v6" || len(result.Components) != 1 {
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
	for _, test := range []struct {
		name, kind, firstKind, secondKind            string
		firstOrigin, firstDirection, secondDirection [3]float64
		value                                        float64
		rank                                         uint64
	}{
		{name: "parallel", kind: "PARALLEL", firstKind: "AXIS", secondKind: "AXIS", firstDirection: [3]float64{0, 0, 1}, secondDirection: [3]float64{0, 0, 1}, rank: 2},
		{name: "perpendicular", kind: "PERPENDICULAR", firstKind: "AXIS", secondKind: "PLANE", firstDirection: [3]float64{1, 0, 0}, secondDirection: [3]float64{0, 0, 1}, rank: 1},
		{name: "point-line offset", kind: "DISTANCE", firstKind: "POINT", secondKind: "AXIS", firstOrigin: [3]float64{2, 0, 0}, secondDirection: [3]float64{0, 0, 1}, value: 2, rank: 1},
		{name: "line-plane offset", kind: "DISTANCE", firstKind: "AXIS", secondKind: "PLANE", firstOrigin: [3]float64{0, 0, 2}, firstDirection: [3]float64{1, 0, 0}, secondDirection: [3]float64{0, 0, 1}, value: 2, rank: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := client.SolveAssembly(t.Context(), test.name, []geometry.AssemblyBody{{ID: "a", Pose: identity}, {ID: "b", Pose: identity}},
				[]geometry.AssemblyGeometry{{ID: "first", BodyID: "b", Kind: test.firstKind, Origin: test.firstOrigin, Direction: test.firstDirection}, {ID: "second", BodyID: "a", Kind: test.secondKind, Direction: test.secondDirection}},
				[]geometry.AssemblyConstraint{{ID: "fix", Kind: "FIX", FirstBodyID: "a", FixedPose: &identity}, {ID: "test", Kind: test.kind, FirstBodyID: "b", FirstGeometryID: "first", SecondBodyID: "a", SecondGeometryID: "second", Value: test.value}})
			if err != nil || result.Status != "CONVERGED" || len(result.Components) != 1 || result.Components[0].JacobianRank != test.rank {
				t.Fatalf("Router lost definition or rank: %+v %v", result, err)
			}
		})
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
	debugStore, err := debugartifact.NewStore(t.TempDir(), 50, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	service.SetDebugArtifactStore(debugStore)
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
	request := workspace.CommandRequest{RequestID: "replay-" + id, ActorID: "00000000-0000-7000-8000-000000000001", Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", FirstAssemblyRef: first, SecondAssemblyRef: second, Value: 2, DirectionRelation: "SAME", DistanceRelation: "UNSIGNED"}
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
	request.PreviewID = preview.PreviewID
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
	service.SetDebugArtifactStore(debugStore)
	refreshed, err = service.GetDocument(t.Context(), id, "00000000-0000-7000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	check(refreshed, 4)
	for _, key := range []string{"preview/" + request.RequestID} {
		var data []byte
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
			_, data, err = service.ReadAssemblyReplay(t.Context(), id, "latest", key)
			if err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err != nil {
			t.Fatal(err)
		}
		var file geometry.AssemblyReplay
		if err = json.Unmarshal(data, &file); err != nil || file.Schema != geometry.AssemblyReplaySchema {
			t.Fatalf("invalid archive: %s", data)
		}
		output, err := client.ReplayAssembly(t.Context(), data)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(output, &file); err != nil {
			t.Fatal(err)
		}
		var result workerv1.SolveAssemblyResponse
		if err = protojson.Unmarshal(file.Result, &result); err != nil || result.Status != "CONVERGED" {
			t.Fatalf("archived request did not replay: %s", output)
		}
	}
	if _, _, err = service.ReadAssemblyReplay(t.Context(), id, "latest", request.RequestID); err == nil {
		t.Fatal("promoted commit unexpectedly ran and archived a second solve")
	}
	failed := request
	failed.RequestID = "failed-replay-" + id
	failed.Value = 20
	if _, err = service.PreviewCommand(t.Context(), id, failed); err == nil {
		t.Fatal("conflicting preview unexpectedly succeeded")
	}
	var failedData []byte
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		_, failedData, err = service.ReadAssemblyReplay(t.Context(), id, "latest", "preview/"+failed.RequestID)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	var failedReplay geometry.AssemblyReplay
	if err = json.Unmarshal(failedData, &failedReplay); err != nil {
		t.Fatal(err)
	}
	var failedResult workerv1.SolveAssemblyResponse
	if err = protojson.Unmarshal(failedReplay.Result, &failedResult); err != nil {
		t.Fatal(err)
	}
	status := failedResult.Status
	if status == "CONVERGED" {
		t.Fatal("failed replay lost failure status")
	}
	afterFailure, err := service.GetDocument(t.Context(), id, "00000000-0000-7000-8000-000000000001")
	if err != nil || afterFailure.Document.VersionID != refreshed.Document.VersionID {
		t.Fatal("archiving failed preview changed model head")
	}

	t.Run("activation mode history and empty active set", func(t *testing.T) {
		measured := "MEASURED"
		measuredView := apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{constraintID}, ConstraintMode: &measured})
		if measuredView.Product.Constraints[1].Mode != "MEASURED" || measuredView.Product.Constraints[1].MeasuredValue == nil || math.Abs(*measuredView.Product.Constraints[1].MeasuredValue-4) > 1e-7 {
			t.Fatalf("measurement missing: %+v", measuredView.Product.Constraints)
		}
		suppressed := true
		ids := []string{measuredView.Product.Constraints[0].ID, constraintID}
		disabled := apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: ids, Suppressed: &suppressed})
		for _, c := range disabled.Product.Constraints {
			if !c.Suppressed {
				t.Fatal("batch suppression incomplete")
			}
		}
		if disabled.Product.Constraints[1].Mode != "MEASURED" {
			t.Fatal("suppression lost measured mode")
		}
		moved := apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: b, Translation: [3]float64{0, 0, 9}, Rotation: [4]float64{0, 0, 0, 1}})
		if math.Abs(moved.Product.Instances[1].Translation[2]-9) > 1e-7 {
			t.Fatal("suppressed constraint still drives placement")
		}
		check(apply(workspace.CommandRequest{Type: "UNDO"}), 4)
		restored := apply(workspace.CommandRequest{Type: "UNDO"})
		if restored.Product.Constraints[1].Suppressed || restored.Product.Constraints[1].Mode != "MEASURED" {
			t.Fatal("undo did not restore orthogonal state")
		}
		disabled = apply(workspace.CommandRequest{Type: "REDO"})
		if !disabled.Product.Constraints[1].Suppressed {
			t.Fatal("redo did not restore suppression")
		}
		plan, planErr := service.GetProductUpdatePlan(t.Context(), id)
		if planErr != nil || !plan.CanAccept || plan.HasUpdates {
			t.Fatalf("inactive constraints blocked update plan: %+v %v", plan, planErr)
		}
		var digest string
		if err := db.QueryRow(t.Context(), `SELECT digest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, disabled.Document.VersionID).Scan(&digest); err != nil {
			t.Fatal(err)
		}
		var raw []byte
		if err := db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE digest=$1`, digest).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var manifest workspace.AssemblySolveManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatal(err)
		}
		replay, err := service.ReplayAssemblySolveManifest(t.Context(), id, digest, "inactive-replay-"+id)
		if err != nil || replay.Status != "CONVERGED" {
			t.Fatalf("inactive replay failed: %+v %v", replay, err)
		}
		if len(manifest.Constraints) != 0 || len(manifest.Definitions) != 2 {
			t.Fatalf("empty active set evidence incomplete: %+v", manifest)
		}
		release, err := service.CreateProductRelease(t.Context(), id, workspace.CreateProductReleaseRequest{RequestID: "inactive-release-" + id, Name: "Inactive constraints", ActorID: "00000000-0000-7000-8000-000000000001"})
		if err != nil {
			t.Fatal(err)
		}
		for _, gate := range release.Manifest.Gates {
			if gate.Status != "PASSED" {
				t.Fatalf("inactive constraints blocked Release: %+v", gate)
			}
		}
		active := false
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: ids, Suppressed: &active})
		replayRelease, err := service.ReplayProductRelease(t.Context(), id, release.ID, "replay-release-"+id)
		if err != nil {
			t.Fatalf("frozen inactive release changed after activation: %+v %v", replayRelease, err)
		}

	})

	t.Run("relative and space fixed baselines", func(t *testing.T) {
		state, err := service.GetDocument(t.Context(), id, "00000000-0000-7000-8000-000000000001")
		if err != nil {
			t.Fatal(err)
		}
		fixID := state.Product.Constraints[0].ID
		apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: fixID, FixMode: "RELATIVE"})
		moved := apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: a, Translation: [3]float64{0, 0, 3}, Rotation: [4]float64{0, 0, 0, 1}})
		if math.Abs(moved.Product.Instances[0].Translation[2]-3) > 1e-7 || moved.Product.Constraints[0].FixedPose == nil || math.Abs(moved.Product.Constraints[0].FixedPose.Translation[2]-3) > 1e-7 {
			t.Fatal("relative fix did not accept explicit movement")
		}
		relative := apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: fixID, FixMode: "RELATIVE", FixedPose: &workspace.InstancePose{Translation: [3]float64{1, 2, 4}, Rotation: [4]float64{0, 0, 0, 1}}})
		if relative.Product.Instances[0].Translation != ([3]float64{1, 2, 4}) {
			t.Fatal("explicit relative fixed pose was discarded")
		}
		undone := apply(workspace.CommandRequest{Type: "UNDO"})
		if math.Abs(undone.Product.Instances[0].Translation[2]-3) > 1e-7 {
			t.Fatal("relative pose edit undo failed")
		}
		apply(workspace.CommandRequest{Type: "REDO"})
		fixed := apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: fixID, FixMode: "SPACE", FixedPose: &workspace.InstancePose{Translation: [3]float64{0, 0, 5}, Rotation: [4]float64{0, 0, 0, 1}}})
		if math.Abs(fixed.Product.Instances[0].Translation[2]-5) > 1e-7 {
			t.Fatal("space fix pose editing failed")
		}
		preview, err := service.PreviewCommand(t.Context(), id, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: a, Translation: [3]float64{0, 0, 8}, Rotation: [4]float64{0, 0, 0, 1}})
		if err != nil || !preview.ConstraintLimited {
			t.Fatalf("space fix unexpectedly moved: %+v %v", preview, err)
		}
	})

	t.Run("stable angle axis and canonical turn", func(t *testing.T) {
		first := &workspace.AssemblyGeometryRef{InstanceID: a, Kind: "PLANE", GeometryID: "datum-yz"}
		second := &workspace.AssemblyGeometryRef{InstanceID: b, Kind: "PLANE", GeometryID: "datum-yz"}
		axis := &workspace.AssemblyGeometryRef{InstanceID: b, Kind: "PLANE", GeometryID: "datum-xy"}
		state := apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "ANGLE", AngleRelation: "DIRECTED", FirstAssemblyRef: first, SecondAssemblyRef: second, AngleAxis: axis, Value: 2 * math.Pi})
		c := state.Product.Constraints[len(state.Product.Constraints)-1]
		if c.Value != 0 || c.AngleAxis == nil || c.AngleReferenceDirection == nil || c.EvaluationStatus != "VERIFIED" {
			t.Fatalf("angle axis/canonicalization: %+v", c)
		}
		state = apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: c.ID, AngleRelation: "DIRECTED", AngleAxis: axis, ReverseAngleAxis: new(true), Value: math.Pi / 2})
		c = state.Product.Constraints[len(state.Product.Constraints)-1]
		if c.AngleReferenceDirection == nil || c.AngleReferenceDirection[2] != -1 || c.EvaluationStatus != "VERIFIED" {
			t.Fatalf("axis reverse: %+v", c)
		}
		var manifestJSON []byte
		if err := db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, state.Document.VersionID).Scan(&manifestJSON); err != nil {
			t.Fatal(err)
		}
		var manifest workspace.AssemblySolveManifest
		if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range manifest.ResolutionEvidence {
			if e.ConstraintID == c.ID && e.Endpoint == "ANGLE_AXIS" {
				found = true
			}
		}
		if !found {
			t.Fatal("stable axis omitted from replay evidence")
		}
		apply(workspace.CommandRequest{Type: "UNDO"})
		apply(workspace.CommandRequest{Type: "REDO"})
		missing := *axis
		missing.GeometryID = "removed-axis"
		broken := apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: c.ID, AngleAxis: &missing, Value: math.Pi / 2})
		last := broken.Product.Constraints[len(broken.Product.Constraints)-1]
		if last.EvaluationStatus != "BROKEN" || broken.Product.Constraints[0].EvaluationStatus != "VERIFIED" {
			t.Fatal("missing angle axis did not isolate its broken definition")
		}
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{c.ID}, Suppressed: new(true)})
		apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: c.ID, AngleAxis: axis, Value: math.Pi / 2})
		reconnected := apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{c.ID}, Suppressed: new(false)})
		if reconnected.Product.Constraints[len(reconnected.Product.Constraints)-1].EvaluationStatus != "VERIFIED" {
			t.Fatal("axis reconnect/reactivation did not re-evaluate")
		}

	})

	t.Run("rigid capture with existing mates preview commit and history", func(t *testing.T) {
		before, err := service.GetDocument(t.Context(), id, "00000000-0000-7000-8000-000000000001")
		if err != nil {
			t.Fatal(err)
		}
		request := workspace.CommandRequest{RequestID: "rigid-" + id, ActorID: "00000000-0000-7000-8000-000000000001", Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "RIGID", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: a, Kind: "BODY"}, SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: b, Kind: "BODY"}}
		preview, err := service.PreviewCommand(t.Context(), id, request)
		if err != nil {
			t.Fatal(err)
		}
		request.PreviewID = preview.PreviewID
		after := apply(request)
		if len(after.Product.Constraints) != len(before.Product.Constraints)+1 {
			t.Fatal("rigid mate missing")
		}
		for i, instance := range after.Product.Instances {
			for axis := range instance.Translation {
				if math.Abs(instance.Translation[axis]-before.Product.Instances[i].Translation[axis]) > 1e-7 {
					t.Fatal("capture moved already feasible assembly")
				}
			}
		}
		for _, c := range after.Product.Constraints {
			if !c.Suppressed && c.EvaluationStatus != "VERIFIED" {
				t.Fatalf("rigid capture invalidated existing constraint: %+v", c)
			}
		}
		undone := apply(workspace.CommandRequest{Type: "UNDO"})
		if len(undone.Product.Constraints) != len(before.Product.Constraints) {
			t.Fatal("rigid undo failed")
		}
		redone := apply(workspace.CommandRequest{Type: "REDO"})
		if len(redone.Product.Constraints) != len(after.Product.Constraints) {
			t.Fatal("rigid redo failed")
		}
	})

	t.Run("free reflex angle composition measurement and relation history", func(t *testing.T) {
		// Undo the preceding rigid capture, retaining the original directed angle.
		state := apply(workspace.CommandRequest{Type: "UNDO"})
		c := state.Product.Constraints[len(state.Product.Constraints)-1]
		state = apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: c.ID, AngleRelation: "FREE", Value: 11 * math.Pi / 6})
		checkAngle := func(view workspace.DocumentView, relation string, value float64) {
			t.Helper()
			for _, got := range view.Product.Constraints {
				if got.ID != c.ID {
					continue
				}
				if got.AngleRelation != relation || math.Abs(got.Value-value) > 1e-9 || got.AngleAxis != nil || got.AngleReferenceDirection != nil || got.EvaluationStatus != "VERIFIED" {
					t.Fatalf("angle definition/evidence: %+v", got)
				}
				return
			}
			t.Fatal("angle missing")
		}
		checkAngle(state, "FREE", 11*math.Pi/6)
		undone := apply(workspace.CommandRequest{Type: "UNDO"})
		restored := undone.Product.Constraints[len(undone.Product.Constraints)-1]
		if restored.AngleRelation != "DIRECTED" || restored.AngleAxis == nil {
			t.Fatal("undo did not restore axis")
		}
		checkAngle(apply(workspace.CommandRequest{Type: "REDO"}), "FREE", 11*math.Pi/6)
		state = apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", DirectionRelation: "SAME",
			FirstAssemblyRef:  &workspace.AssemblyGeometryRef{InstanceID: a, Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Z"},
			SecondAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: b, Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Z"}})
		checkAngle(state, "FREE", 11*math.Pi/6)
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{c.ID}, ConstraintMode: new("MEASURED")})
		measured := apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: b, Translation: [3]float64{0, 0, 9}, Rotation: [4]float64{0, 0, math.Sin(math.Pi / 6), math.Cos(math.Pi / 6)}})
		for _, got := range measured.Product.Constraints {
			if got.ID == c.ID && (got.MeasuredValue == nil || math.Abs(*got.MeasuredValue-5*math.Pi/3) > 1e-6) {
				t.Fatalf("reflex measurement: %+v", got)
			}
		}
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{c.ID}, ConstraintMode: new("DRIVING")})
		var forwardRotation [4]float64
		for _, direction := range []string{"SAME", "OPPOSITE"} {
			want := math.Pi / 2
			if direction == "OPPOSITE" {
				want = 3 * math.Pi / 2
			}
			state = apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: c.ID, AngleRelation: "PERPENDICULAR", DirectionRelation: direction})
			checkAngle(state, "PERPENDICULAR", want)
			rotation := state.Product.Instances[1].Rotation
			if direction == "SAME" {
				forwardRotation = rotation
			} else {
				dot := 0.0
				for i := range rotation {
					dot += rotation[i] * forwardRotation[i]
				}
				if math.Abs(dot) > 1e-6 {
					t.Fatalf("90 and 270 did not select opposite rotations: %v %v", forwardRotation, rotation)
				}
				undone := apply(workspace.CommandRequest{Type: "UNDO"})
				if math.Abs(undone.Product.Instances[1].Rotation[2]-forwardRotation[2]) > 1e-6 {
					t.Fatal("perpendicular undo lost pose")
				}
				checkAngle(apply(workspace.CommandRequest{Type: "REDO"}), "PERPENDICULAR", want)
			}
		}
		state = apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: c.ID, AngleRelation: "PARALLEL", DirectionRelation: "OPPOSITE"})
		checkAngle(state, "PARALLEL", 0)
	})

}
