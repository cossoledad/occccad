package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"reflect"
	"testing"
	"time"
)

func conflictFixture(t *testing.T, values ...float64) AssemblySolveManifest {
	t.Helper()
	bodies := []geometry.AssemblyBody{{ID: "a", Pose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}}, {ID: "b", Pose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}}}
	geo := []geometry.AssemblyGeometry{{ID: "pa", BodyID: "a", Kind: "POINT"}, {ID: "pb", BodyID: "b", Kind: "POINT"}}
	cs := []geometry.AssemblyConstraint{}
	defs := []AssemblyConstraint{}
	for i, v := range values {
		id := string(rune('x' + i))
		cs = append(cs, geometry.AssemblyConstraint{ID: id, Kind: "DISTANCE", FirstBodyID: "a", FirstGeometryID: "pa", SecondBodyID: "b", SecondGeometryID: "pb", Value: v, DistanceRelation: "UNSIGNED"})
		second := AssemblyGeometryRef{InstanceID: "b", Kind: "POINT"}
		defs = append(defs, AssemblyConstraint{DefinitionVersion: 2, ID: id, Kind: "DISTANCE", First: AssemblyGeometryRef{InstanceID: "a", Kind: "POINT"}, Second: &second, Value: v, EvaluationStatus: modelcore.AssemblyConstraintNotUpdated})
	}
	m, err := newAssemblySolveManifest("product", "revision", "hash", bodies, geo, cs, nil, nil, nil, defs)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Controlled analytic oracle used ONLY to test search labels: the independently
// checked point-distance witness is constructed without production search.
func pointDistanceWitness(_ context.Context, m AssemblySolveManifest) (geometry.AssemblySolve, error) {
	b := append([]geometry.AssemblyBody{}, m.Bodies...)
	if len(m.Constraints) > 0 {
		b[1].Pose.Translation = [3]float64{m.Constraints[0].Value, 0, 0}
	}
	return geometry.AssemblySolve{Status: "MAX_ITERATIONS", Bodies: b}, nil
}

func TestAssemblyConflictThreeValuedEvidence(t *testing.T) {
	t.Run("verified irreducible requires every deletion SAT", func(t *testing.T) {
		m := conflictFixture(t, 5, 8)
		before, _ := json.Marshal(m)
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision", TargetConstraintIDs: []string{"y"}}, m, pointDistanceWitness)
		if err != nil || r.Status != "UNSAT" || !r.Complete || len(r.Probes) != 3 || !r.Items[0].Irreducible || r.Items[0].Evidence != "VERIFIED_IRREDUCIBLE" {
			t.Fatal(r, err)
		}
		for _, p := range r.Probes[1:] {
			if p.Oracle != "SAT" {
				t.Fatal(p)
			}
		}
		after, _ := json.Marshal(m)
		if string(before) != string(after) {
			t.Fatal("diagnosis mutated frozen input")
		}
	})
	t.Run("redundancy is SAT not conflict", func(t *testing.T) {
		m := conflictFixture(t, 5, 5)
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision"}, m, pointDistanceWitness)
		if err != nil || r.Status != "SAT" || r.Items[0].Kind != "SATISFIED" {
			t.Fatal(r, err)
		}
	})
	t.Run("infrastructure failure not mathematical suspect", func(t *testing.T) {
		m := conflictFixture(t, 5)
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision"}, m, func(context.Context, AssemblySolveManifest) (geometry.AssemblySolve, error) {
			return geometry.AssemblySolve{}, errors.New("Worker disconnected")
		})
		if err != nil || r.Status != "UNKNOWN" || r.Items[0].Kind != "INFRASTRUCTURE_FAILURE" || r.Items[0].Oracle != "UNKNOWN" {
			t.Fatal(r, err)
		}
	})
	t.Run("Worker input rejection distinct from infrastructure and UNSAT", func(t *testing.T) {
		m := conflictFixture(t, 5)
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision"}, m, func(context.Context, AssemblySolveManifest) (geometry.AssemblySolve, error) {
			return geometry.AssemblySolve{}, status.Error(codes.InvalidArgument, "exact support changed type")
		})
		if err != nil || r.Status != "UNKNOWN" || r.Items[0].Kind != "GEOMETRY_INCOMPATIBLE" || r.Items[0].Evidence != "WORKER_MODEL_VALIDATION" {
			t.Fatal(r, err)
		}
	})
	t.Run("nonconvergence without witness is UNKNOWN", func(t *testing.T) {
		m := conflictFixture(t, 5)
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision"}, m, func(context.Context, AssemblySolveManifest) (geometry.AssemblySolve, error) {
			return geometry.AssemblySolve{Status: "CONVERGED", Bodies: m.Bodies}, nil
		})
		if err != nil || r.Status != "UNKNOWN" || r.Items[0].Evidence != "NUMERICAL_UNKNOWN" {
			t.Fatal(r, err)
		}
	})
	t.Run("valid witness SAT even preference not converged", func(t *testing.T) {
		m := conflictFixture(t, 5)
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision"}, m, pointDistanceWitness)
		if err != nil || r.Status != "SAT" || r.Probes[0].SolverStatus != "MAX_ITERATIONS" {
			t.Fatal(r, err)
		}
	})
	t.Run("probe exhaustion cannot certify irreducible", func(t *testing.T) {
		m := conflictFixture(t, 5, 8)
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision", MaxProbes: 1}, m, pointDistanceWitness)
		if err != nil || r.Status != "UNSAT" || r.Complete || r.Items[0].Irreducible || r.ProbeCount != 1 || r.BudgetReason != "PROBE_LIMIT" {
			t.Fatal(r, err)
		}
	})
	t.Run("cancel no oracle work", func(t *testing.T) {
		m := conflictFixture(t, 5)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		calls := 0
		r, err := AnalyzeFrozenAssemblyConflicts(ctx, "actor", AssemblyConflictRequest{BaseRevisionID: "revision"}, m, func(context.Context, AssemblySolveManifest) (geometry.AssemblySolve, error) {
			calls++
			return geometry.AssemblySolve{}, nil
		})
		if err != nil || r.Status != "CANCELLED" || calls != 0 || r.ProbeCount != 0 {
			t.Fatal(r, err, calls)
		}
	})
	t.Run("deadline propagated into oracle", func(t *testing.T) {
		m := conflictFixture(t, 5)
		start := time.Now()
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision", TimeBudgetMS: 2}, m, func(ctx context.Context, _ AssemblySolveManifest) (geometry.AssemblySolve, error) {
			<-ctx.Done()
			return geometry.AssemblySolve{}, ctx.Err()
		})
		if err != nil || r.Status != "BUDGET_EXHAUSTED" || time.Since(start) > time.Second {
			t.Fatal(r, err)
		}
	})
	t.Run("unresolved isolated definition retained", func(t *testing.T) {
		m := conflictFixture(t, 5)
		broken := m.Definitions[0]
		broken.ID = "broken"
		broken.EvaluationStatus = modelcore.AssemblyConstraintBroken
		broken.EvaluationSummary = "stable support deleted"
		m.Definitions = append(m.Definitions, broken)
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision", TargetConstraintIDs: []string{"broken"}}, m, pointDistanceWitness)
		if err != nil || r.Status != "UNKNOWN" || len(r.ScopeConstraintIDs) != 2 || r.Items[0].Kind != "SUPPORT_UNAVAILABLE" {
			t.Fatal(r, err)
		}
		if !reflect.DeepEqual(r.Items[0].Members[0].RepairActions, []string{"EDIT", "SUPPRESS", "RECONNECT", "MEASURE"}) {
			t.Fatal(r.Items)
		}
	})
	t.Run("suppressed measured not active conflict members", func(t *testing.T) {
		m := conflictFixture(t, 5, 8, 12)
		m.Definitions[1].Suppressed = true
		m.Definitions[2].Mode = "MEASURED"
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision", TargetConstraintIDs: []string{"x"}}, m, pointDistanceWitness)
		if err != nil || r.Status != "SAT" || !reflect.DeepEqual(r.ScopeConstraintIDs, []string{"x"}) {
			t.Fatal(r, err)
		}
	})
	t.Run("stale and mistyped target reject", func(t *testing.T) {
		m := conflictFixture(t, 5)
		for _, req := range []AssemblyConflictRequest{{BaseRevisionID: "stale"}, {BaseRevisionID: "revision", TargetConstraintIDs: []string{"typo"}}, {BaseRevisionID: "revision", MaxProbes: 129}, {BaseRevisionID: "revision", TimeBudgetMS: 5001}} {
			if _, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", req, m, pointDistanceWitness); err == nil {
				t.Fatal(req)
			}
		}
	})
}

func TestAssemblyConflictPhysicalAndScopeCertificates(t *testing.T) {
	t.Run("nested group provenance and deletion keep captured relationship", func(t *testing.T) {
		m := conflictFixture(t, 5)
		m.Bodies = append(m.Bodies, geometry.AssemblyBody{ID: "c", Pose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}})
		relation := func(id string, x float64) AssemblyGroupRelation {
			return AssemblyGroupRelation{InstanceID: id, RelativePose: InstancePose{Translation: [3]float64{x, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}}}
		}
		g := AssemblyConstraint{ID: "g", Name: "inner", Kind: "FIX_TOGETHER", GroupMembers: []AssemblyGroupMember{{InstanceID: "a"}, {InstanceID: "b"}}, GroupRelations: []AssemblyGroupRelation{relation("a", 0), relation("b", 5)}}
		h := AssemblyConstraint{ID: "h", Name: "outer", Kind: "FIX_TOGETHER", GroupMembers: []AssemblyGroupMember{{GroupID: "g"}, {InstanceID: "c"}}, GroupRelations: []AssemblyGroupRelation{relation("a", 0), relation("b", 5), relation("c", 7)}}
		g.GroupCaptureDigest = assemblyGroupInputDigest(m, []string{"a", "b"})
		h.GroupCaptureDigest = assemblyGroupInputDigest(m, []string{"a", "b", "c"})
		m.Definitions = append(m.Definitions, g, h)
		m.SolverProfile.MaxConflictProbes = 16
		var err error
		m.GroupStages, err = prepareAssemblyGroupStages(m, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids, bodies, err := conflictScope(m, []string{"h"})
		if err != nil || !reflect.DeepEqual(ids, []string{"g", "h", "x"}) || !reflect.DeepEqual(bodies, []string{"a", "b", "c"}) {
			t.Fatal(ids, bodies, err)
		}
		before, _ := json.Marshal(m)
		probe, err := conflictSubset(m, []string{"h"})
		if err != nil {
			t.Fatal(err)
		}
		if err = validateFrozenAssemblyGroupStages(probe); err != nil {
			t.Fatal(err)
		}
		if probe.SolverProfile.MaxConflictProbes != 0 || m.SolverProfile.MaxConflictProbes != 16 {
			t.Fatal("diagnostic probe must disable nested subset search without changing production profile")
		}
		if len(probe.GroupStages) != 1 || len(probe.GroupStages[0].Groups) != 1 || probe.GroupStages[0].Groups[0].CapturePending || !reflect.DeepEqual(probe.GroupStages[0].Groups[0].CapturedRelations, h.GroupRelations) {
			t.Fatal("deletion recaptured group", probe.GroupStages)
		}
		after, _ := json.Marshal(m)
		if string(before) != string(after) {
			t.Fatal("probe changed real group/suppression")
		}
	})
	t.Run("Parallel Perpendicular proof", func(t *testing.T) {
		m := conflictFixture(t, 0, 0)
		for i := range m.Geometry {
			m.Geometry[i].Kind = "AXIS"
			m.Geometry[i].Direction = [3]float64{0, 0, 1}
		}
		m.Constraints[0].Kind = "PARALLEL"
		m.Constraints[1].Kind = "PERPENDICULAR"
		m.Definitions[0].Kind = "PARALLEL"
		m.Definitions[1].Kind = "PERPENDICULAR"
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision"}, m, func(_ context.Context, input AssemblySolveManifest) (geometry.AssemblySolve, error) {
			b := append([]geometry.AssemblyBody{}, input.Bodies...)
			if input.Constraints[0].Kind == "PERPENDICULAR" {
				b[1].Pose.Rotation = [4]float64{0, 0.7071067811865475, 0, 0.7071067811865475}
			}
			return geometry.AssemblySolve{Status: "CONVERGED", Bodies: b}, nil
		})
		if err != nil || r.Status != "UNSAT" || !r.Items[0].Irreducible {
			t.Fatal(r, err)
		}
	})
	t.Run("all true Fix grounds are background not numerical gauge", func(t *testing.T) {
		m := conflictFixture(t, 5)
		for _, b := range m.Bodies {
			pose := b.Pose
			m.Constraints = append(m.Constraints, geometry.AssemblyConstraint{ID: "fix" + b.ID, Kind: "FIX", FirstBodyID: b.ID, FixedPose: &pose})
			m.Definitions = append(m.Definitions, AssemblyConstraint{ID: "fix" + b.ID, Kind: "FIX", First: AssemblyGeometryRef{InstanceID: b.ID, Kind: "BODY"}})
		}
		proof := conflictAnalyticProof(m)
		if proof == nil || len(proof.IDs) != 3 {
			t.Fatal(proof)
		}
	})
	t.Run("intrinsic contact mismatch reliable", func(t *testing.T) {
		m := conflictFixture(t, 0)
		m.Geometry[0].Kind, m.Geometry[1].Kind = "SPHERE", "SPHERE"
		m.Geometry[0].Radius, m.Geometry[1].Radius = 2, 3
		m.Constraints[0].Kind, m.Constraints[0].ContactKind = "CONTACT", "FACE"
		proof := conflictAnalyticProof(m)
		if proof == nil || proof.Reason != "CONTACT_FACE_RADIUS_MISMATCH" {
			t.Fatal(proof)
		}
	})
	t.Run("third axis complete connected closure independent component excluded", func(t *testing.T) {
		m := conflictFixture(t, 5)
		m.Bodies = append(m.Bodies, geometry.AssemblyBody{ID: "axis"}, geometry.AssemblyBody{ID: "other"})
		second := AssemblyGeometryRef{InstanceID: "axis"}
		m.Definitions = append(m.Definitions, AssemblyConstraint{ID: "angle", Kind: "ANGLE", First: AssemblyGeometryRef{InstanceID: "b"}, Second: &second, AngleAxis: &AssemblyGeometryRef{InstanceID: "a"}}, AssemblyConstraint{ID: "unrelated", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: "other"}})
		ids, bodies, err := conflictScope(m, []string{"angle"})
		if err != nil || !reflect.DeepEqual(ids, []string{"angle", "x"}) || !reflect.DeepEqual(bodies, []string{"a", "axis", "b"}) {
			t.Fatal(ids, bodies, err)
		}
	})
	t.Run("unknown nonlinear primitive not guessed UNSAT", func(t *testing.T) {
		m := conflictFixture(t, 0)
		m.Constraints[0].Kind = "SURFACE_INCIDENCE"
		m.Geometry[1].Kind = "CONE"
		r, err := AnalyzeFrozenAssemblyConflicts(t.Context(), "actor", AssemblyConflictRequest{BaseRevisionID: "revision"}, m, pointDistanceWitness)
		if err != nil || r.Status != "UNKNOWN" || r.Items[0].Irreducible {
			t.Fatal(r, err)
		}
	})
}
