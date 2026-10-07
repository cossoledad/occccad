package workspace

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/valuecopy"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestAssemblyOfflineReplayProductionSolver(t *testing.T) {
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Skip("requires current production Geometry Worker binary")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	client, close, err := geometry.OpenReplayWorker(ctx, binary)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	identity := geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}
	bodies := []geometry.AssemblyBody{{ID: "a", Pose: identity}, {ID: "b", Pose: geometry.AssemblyPose{Translation: [3]float64{4, 0, 0}, Rotation: identity.Rotation}}}
	values := []geometry.AssemblyGeometry{{ID: "point", BodyID: "a", Kind: "POINT", LengthUnit: "mm"}, {ID: "point", BodyID: "b", Kind: "POINT", LengthUnit: "mm"}}
	// The compiler distinguishes the same local ID at two different occurrences.
	compiled, cs, _, err := geometry.CompileAssemblyInput(values, []geometry.AssemblyConstraint{{ID: "fix", Kind: "FIX", FirstBodyID: "a"}, {ID: "coincidence", Kind: "COINCIDENT", FirstBodyID: "b", FirstGeometryID: "point", SecondBodyID: "a", SecondGeometryID: "point"}, {ID: "pending", Kind: "DISTANCE", FirstBodyID: "b", FirstGeometryID: "point", SecondBodyID: "a", SecondGeometryID: "point", Value: 5}, {ID: "suppressed", Kind: "DISTANCE", FirstBodyID: "b", FirstGeometryID: "point", SecondBodyID: "a", SecondGeometryID: "point", Value: 9}, {ID: "measurement", Kind: "DISTANCE", Mode: "MEASURED", FirstBodyID: "b", FirstGeometryID: "point", SecondBodyID: "a", SecondGeometryID: "point"}})
	if err != nil {
		t.Fatal(err)
	}
	defs := []AssemblyConstraint{}
	for _, c := range cs {
		state := AssemblyConstraint{DefinitionVersion: 2, ID: c.ID, Kind: c.Kind, Mode: c.Mode, Value: c.Value, EvaluationStatus: "VERIFIED", First: AssemblyGeometryRef{InstanceID: c.FirstBodyID, Kind: "BODY"}}
		if c.SecondBodyID != "" {
			state.Second = &AssemblyGeometryRef{InstanceID: c.SecondBodyID, Kind: "BODY"}
		}
		if c.ID == "pending" {
			state.EvaluationStatus = "NOT_UPDATED"
		}
		if c.ID == "suppressed" {
			state.Suppressed = true
		}
		defs = append(defs, state)
	}
	manifest, err := newAssemblySolveManifest("offline-product", "offline-revision", "offline-model", bodies, compiled, cs, nil, nil, nil, defs)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := buildAssemblyDiagnosticSnapshot(manifest, defs, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(manifest)
	run := func(t *testing.T, options AssemblyReplayOptions) (geometry.AssemblyReplay, workerv1.SolveAssemblyResponse) {
		t.Helper()
		data, err := encodeAssemblyDiagnostic(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		output, err := ReplayAssemblyDiagnosticWithOptions(ctx, client, data, options)
		if err != nil {
			t.Fatal(err)
		}
		var file geometry.AssemblyReplay
		if err = json.Unmarshal(output, &file); err != nil {
			t.Fatal(err)
		}
		var result workerv1.SolveAssemblyResponse
		if len(file.Result) > 0 {
			if err = protojson.Unmarshal(file.Result, &result); err != nil {
				t.Fatal(err)
			}
		}
		return file, result
	}
	t.Run("accepted subset and production parity", func(t *testing.T) {
		file, restored := run(t, AssemblyReplayOptions{Mode: "accepted"})
		if file.Outcome != "FEASIBLE" || restored.Status != "CONVERGED" || restored.Metrics.GetHotStringLookups() != 0 {
			t.Fatal("offline solver failed", file.Outcome, restored.Status, restored.Diagnostic)
		}
		copy, cloneErr := valuecopy.Clone(manifest)
		if cloneErr != nil {
			t.Fatal(cloneErr)
		}
		if _, err := selectDiagnosticConstraints(&copy, AssemblyReplayOptions{Mode: "accepted"}); err != nil {
			t.Fatal(err)
		}
		live, err := client.SolveAssemblyWithOptions(ctx, "production-parity", copy.Bodies, copy.Geometry, copy.Constraints, geometry.AssemblySolveOptions{SolverProfile: &copy.SolverProfile, DisableConflictProbes: true})
		if err != nil {
			t.Fatal(err)
		}
		if live.Status != restored.Status || live.NormalizedResidual != restored.NormalizedResidual || len(live.Bodies) != len(restored.Bodies) {
			t.Fatal("offline and production numerical paths differ")
		}
		for i, b := range live.Bodies {
			p := restored.Bodies[i].Pose
			if b.ID != restored.Bodies[i].Id || b.Pose.Translation != [3]float64{p.Translation.GetX(), p.Translation.GetY(), p.Translation.GetZ()} {
				t.Fatal("offline adopted different body pose")
			}
		}
	})
	t.Run("NotUpdated is explicitly selected without becoming verified", func(t *testing.T) {
		file, result := run(t, AssemblyReplayOptions{Mode: "accepted-pending", ConstraintIDs: []string{"pending"}})
		if result.Status == "CONVERGED" || file.Outcome == "PROVEN_CONTRADICTION" || file.Outcome == "FEASIBLE" {
			t.Fatal("numerical failure incorrectly classified", file.Outcome, result.Status)
		}
	})
	t.Run("trace matrices and budget truncation", func(t *testing.T) {
		file, result := run(t, AssemblyReplayOptions{Mode: "accepted", Trace: true, FullMatrices: true, TraceByteBudget: 1 << 20})
		if result.Status != "CONVERGED" || len(result.EvaluationTrace) == 0 || len(file.Derivation) == 0 {
			t.Fatal("missing optional trace")
		}
		found := false
		for _, event := range result.EvaluationTrace {
			if len(event.BodyPoses) != len(bodies) {
				t.Fatal("trace body ordering lost")
			}
			found = found || len(event.Jacobian) > 0
		}
		if !found {
			t.Fatal("matrix capture was ignored")
		}
		_, limited := run(t, AssemblyReplayOptions{Mode: "accepted", Trace: true, TraceByteBudget: 1})
		if !limited.TraceTruncated || len(limited.EvaluationTrace) != 0 || limited.Status != "CONVERGED" {
			t.Fatal("diagnostic byte budget changed solve")
		}
	})
	t.Run("explicit offline budget can exceed the normal RPC cap", func(t *testing.T) {
		budget := 20000.0
		file, result := run(t, AssemblyReplayOptions{Mode: "accepted", WallClockMS: &budget})
		if result.Status != "CONVERGED" || len(file.Stages) != 1 {
			t.Fatal("budget experiment failed")
		}
		var stage geometry.AssemblyReplay
		if err := json.Unmarshal(file.Stages[0], &stage); err != nil {
			t.Fatal(err)
		}
		if stage.Budget == nil || stage.Budget.WallClockMS < 19000 || stage.Budget.WallClockMS > budget {
			t.Fatal("effective budget did not record the requested override", stage.Budget)
		}
	})
	t.Run("deadline is execution failure", func(t *testing.T) {
		budget := 0.000001
		file, _ := run(t, AssemblyReplayOptions{Mode: "accepted", WallClockMS: &budget})
		if file.Outcome != "EXECUTION_FAILURE" || file.TransportError == "" {
			t.Fatal("deadline declared global contradiction", file.Outcome)
		}
	})
	t.Run("fully grounded contradiction has explicit evidence", func(t *testing.T) {
		fixed := append([]geometry.AssemblyConstraint(nil), cs[:3]...)
		fixed[1] = geometry.AssemblyConstraint{ID: "fix-b", Kind: "FIX", FirstBodyID: "b"}
		fixed[2].Value = 8
		input, err := geometry.MakeAssemblyReplay("known-infeasible", bodies, compiled, fixed, geometry.AssemblySolveOptions{SolverProfile: &manifest.SolverProfile, DisableConflictProbes: true})
		if err != nil {
			t.Fatal(err)
		}
		out, err := client.ReplayAssembly(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		var file geometry.AssemblyReplay
		_ = json.Unmarshal(out, &file)
		if file.Outcome != "PROVEN_CONTRADICTION" {
			t.Fatal("missing explicit grounded contradiction certificate", string(out))
		}
	})
	t.Run("directed angle retains independently owned third axis", func(t *testing.T) {
		values := []geometry.AssemblyGeometry{{ID: "a-plane", BodyID: "a", Kind: "PLANE", Direction: [3]float64{1, 0, 0}, LengthUnit: "mm"}, {ID: "b-plane", BodyID: "b", Kind: "PLANE", Direction: [3]float64{1, 0, 0}, LengthUnit: "mm"}, {ID: "third-axis", BodyID: "c", Kind: "AXIS", Direction: [3]float64{0, 0, 1}, LengthUnit: "mm"}}
		third := append(append([]geometry.AssemblyBody(nil), bodies...), geometry.AssemblyBody{ID: "c", Pose: identity})
		angle := []geometry.AssemblyConstraint{{ID: "fix-a", Kind: "FIX", FirstBodyID: "a"}, {ID: "fix-c", Kind: "FIX", FirstBodyID: "c"}, {ID: "angle", Kind: "ANGLE", FirstBodyID: "b", FirstGeometryID: "b-plane", SecondBodyID: "a", SecondGeometryID: "a-plane", AngleReferenceBodyID: "c", AngleReferenceGeometryID: "third-axis", Value: math.Pi / 2, DirectionRelation: "SAME"}}
		input, err := geometry.MakeAssemblyReplay("directed-third-axis", third, values, angle, geometry.AssemblySolveOptions{SolverProfile: &manifest.SolverProfile, DisableConflictProbes: true})
		if err != nil {
			t.Fatal(err)
		}
		out, err := client.ReplayAssembly(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		var file geometry.AssemblyReplay
		_ = json.Unmarshal(out, &file)
		if file.Outcome != "FEASIBLE" {
			t.Fatal("directed angle replay failed", string(out))
		}
		// Exercise the actual selector and production staged adapter as well.
		third = append(third, geometry.AssemblyBody{ID: "unrelated", Pose: identity})
		angle = append(angle, geometry.AssemblyConstraint{ID: "fix-unrelated", Kind: "FIX", FirstBodyID: "unrelated"})
		definitions := []AssemblyConstraint{}
		for _, c := range angle {
			d := AssemblyConstraint{DefinitionVersion: 2, ID: c.ID, Kind: c.Kind, EvaluationStatus: "VERIFIED", First: AssemblyGeometryRef{InstanceID: c.FirstBodyID, Kind: "BODY"}}
			if c.SecondBodyID != "" {
				d.Second = &AssemblyGeometryRef{InstanceID: c.SecondBodyID, Kind: "BODY"}
			}
			if c.AngleReferenceBodyID != "" {
				d.AngleAxis = &AssemblyGeometryRef{InstanceID: c.AngleReferenceBodyID, Kind: "AXIS"}
			}
			definitions = append(definitions, d)
		}
		m, err := newAssemblySolveManifest("third-axis", "revision", "hash", third, values, angle, nil, nil, nil, definitions)
		if err != nil {
			t.Fatal(err)
		}
		s, err := buildAssemblyDiagnosticSnapshot(m, definitions, nil)
		if err != nil {
			t.Fatal(err)
		}
		data, err := encodeAssemblyDiagnostic(s)
		if err != nil {
			t.Fatal(err)
		}
		out, err = ReplayAssemblyDiagnosticWithOptions(ctx, client, data, AssemblyReplayOptions{Mode: "subsystem", TargetConstraintID: "angle"})
		if err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal(out, &file)
		var subResult workerv1.SolveAssemblyResponse
		if err = protojson.Unmarshal(file.Result, &subResult); err != nil {
			t.Fatal(err)
		}
		if file.Outcome != "FEASIBLE" || len(subResult.Bodies) != 3 {
			t.Fatal("offline subsystem lost third axis or retained unrelated body", string(out))
		}
	})
	after, _ := json.Marshal(manifest)
	if string(before) != string(after) {
		t.Fatal("offline replay modified the source model/input")
	}
}

func TestAssemblyOfflineReplayProductFixture(t *testing.T) {
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Skip("requires production Geometry Worker")
	}
	data, err := os.ReadFile("../../../tests/test.data/assembly-notupdated-product.3dreplay")
	if err != nil {
		t.Fatal(err)
	}
	var file geometry.AssemblyReplay
	var snapshot AssemblyDiagnosticSnapshot
	if err = json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(file.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	definitions, err := snapshot.RestoreDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	pending := ""
	for _, d := range definitions {
		if d.EvaluationStatus == "NOT_UPDATED" {
			pending = d.ID
		}
	}
	if pending == "" || len(definitions) != 3 || len(snapshot.Attempts) != 1 || len(snapshot.Attempts[0].Sequence) == 0 {
		t.Fatal("fixture is not a real preserved NotUpdated failure")
	}
	client, close, err := geometry.OpenReplayWorker(t.Context(), binary)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	for _, options := range []AssemblyReplayOptions{{Mode: "accepted"}, {Mode: "original", TargetConstraintID: pending}, {Mode: "accepted-pending", ConstraintIDs: []string{pending}}} {
		t.Run(options.Mode, func(t *testing.T) {
			out, err := ReplayAssemblyDiagnosticWithOptions(t.Context(), client, data, options)
			if err != nil {
				t.Fatal(err)
			}
			var replay geometry.AssemblyReplay
			if err = json.Unmarshal(out, &replay); err != nil {
				t.Fatal(err)
			}
			// This is an observed failure, not a global infeasibility expectation.
			if replay.Outcome == "INPUT_ERROR" || len(replay.Result) == 0 || len(replay.Stages) == 0 || string(replay.Snapshot) != string(file.Snapshot) {
				t.Fatal("fixture cannot run or saved states changed", string(out))
			}
			var result workerv1.SolveAssemblyResponse
			if err = protojson.Unmarshal(replay.Result, &result); err != nil {
				t.Fatal(err)
			}
			if result.ImplementationId == "" || result.Metrics == nil || result.Metrics.HotStringLookups != 0 {
				t.Fatal("missing production implementation or numerical metrics")
			}
		})
	}
}
