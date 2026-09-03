package geometry

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestCppWorkerSolvesSimpleProductAssembly(t *testing.T) {
	binary := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if binary == "" {
		t.Skip("set OCCCCAD_TEST_GEOMETRY_WORKER to run the C++ assembly RPC integration")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	command := exec.Command(binary)
	command.Env = append(os.Environ(), "OCCCCAD_GEOMETRY_WORKER_LISTEN="+address)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _, _ = command.Process.Wait() })

	var client *Client
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		client, err = Open(address)
		if err == nil {
			pingContext, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			_, err = client.Ping(pingContext)
			cancel()
			if err == nil {
				break
			}
			_ = client.Close()
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("worker did not become ready: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	identity := AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}
	result, err := client.SolveAssembly(t.Context(), "product-smoke", []AssemblyBody{
		{ID: "ground", Pose: identity},
		{ID: "moving", Pose: AssemblyPose{Translation: [3]float64{25, 0, 0}, Rotation: identity.Rotation}},
	}, []AssemblyGeometry{
		{ID: "ground-origin", BodyID: "ground", Kind: "POINT"},
		{ID: "moving-origin", BodyID: "moving", Kind: "POINT"},
	}, []AssemblyConstraint{
		{ID: "fix", Kind: "FIX", FirstBodyID: "ground", FixedPose: &identity},
		{ID: "coincident", Kind: "COINCIDENT", FirstBodyID: "moving", FirstGeometryID: "moving-origin", SecondBodyID: "ground", SecondGeometryID: "ground-origin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "CONVERGED" {
		t.Fatalf("unexpected solve: %s %s", result.Status, result.Diagnostic)
	}
	if result.Classification != "SOLVED_UNDER_CONSTRAINED" {
		t.Fatalf("unexpected classification: %s", result.Classification)
	}
	if len(result.Components) != 1 || result.Components[0].RelativeDof != 3 || result.Components[0].GaugeDof != 0 {
		t.Fatalf("unexpected component DOF: %#v", result.Components)
	}
	if len(result.Components[0].TangentClusterIDs) != 1 || len(result.Components[0].NullSpaceBasis) != 3 || result.Components[0].RankThreshold <= 0 {
		t.Fatalf("numeric null space was not preserved across the Worker RPC: %#v", result.Components[0])
	}
	if len(result.EquationResiduals) == 0 {
		t.Fatal("assembly equation residual provenance was not returned")
	}
	if len(result.ConstraintRanks) == 0 || result.ConstraintRanks[len(result.ConstraintRanks)-1].DeclaredGenericRank != 3 {
		t.Fatalf("typed equation rank was not preserved across the Worker RPC: %#v", result.ConstraintRanks)
	}
	for _, body := range result.Bodies {
		if body.ID == "moving" && (body.Pose.Translation[0] > 1e-6 || body.Pose.Translation[0] < -1e-6) {
			t.Fatal(fmt.Sprintf("moving body was not solved to origin: %#v", body.Pose))
		}
	}

	intentResult, err := client.SolveAssemblyWithOptions(t.Context(), "product-intent", []AssemblyBody{
		{ID: "moving", Pose: AssemblyPose{Translation: [3]float64{5, 0, 0}, Rotation: identity.Rotation}},
		{ID: "reference", Pose: AssemblyPose{Translation: [3]float64{2, 0, 0}, Rotation: identity.Rotation}},
	}, []AssemblyGeometry{
		{ID: "moving-origin", BodyID: "moving", Kind: "POINT"},
		{ID: "reference-origin", BodyID: "reference", Kind: "POINT"},
	}, []AssemblyConstraint{
		{ID: "coincident", Kind: "COINCIDENT", FirstBodyID: "moving", FirstGeometryID: "moving-origin", SecondBodyID: "reference", SecondGeometryID: "reference-origin"},
	}, AssemblySolveOptions{Intent: &AssemblySolveIntent{
		MovingBodyIDs:    []string{"moving"},
		ReferenceBodyIDs: []string{"reference"},
		PreferencePolicy: "MOVE_FIRST_MINIMIZE_REFERENCE",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if intentResult.Status != "CONVERGED" || len(intentResult.Components) != 1 || intentResult.Components[0].GaugeDof != 6 {
		t.Fatalf("unexpected intent solve: %#v", intentResult)
	}
	for _, body := range intentResult.Bodies {
		if body.ID == "reference" && body.Pose.Translation != [3]float64{2, 0, 0} {
			t.Fatalf("reference body moved despite M1.5 gauge preference: %#v", body.Pose)
		}
		if body.ID == "moving" && (body.Pose.Translation[0] < 2-1e-6 || body.Pose.Translation[0] > 2+1e-6) {
			t.Fatalf("moving body did not reach the reference: %#v", body.Pose)
		}
	}

	classificationResult, err := client.SolveAssemblyWithOptions(t.Context(), "product-profile", []AssemblyBody{
		{ID: "ground", Pose: identity},
		{ID: "moving", Pose: AssemblyPose{Translation: [3]float64{5e-6, 0, 0}, Rotation: identity.Rotation}},
	}, []AssemblyGeometry{
		{ID: "ground-origin", BodyID: "ground", Kind: "POINT"},
		{ID: "moving-origin", BodyID: "moving", Kind: "POINT"},
	}, []AssemblyConstraint{
		{ID: "fix-ground", Kind: "FIX", FirstBodyID: "ground", FixedPose: &identity},
		{ID: "fix-moving", Kind: "FIX", FirstBodyID: "moving", FixedPose: &AssemblyPose{Translation: [3]float64{5e-6, 0, 0}, Rotation: identity.Rotation}},
		{ID: "coincident", Kind: "COINCIDENT", FirstBodyID: "moving", FirstGeometryID: "moving-origin", SecondBodyID: "ground", SecondGeometryID: "ground-origin"},
	}, AssemblySolveOptions{SolverProfile: &AssemblySolverProfile{
		SchemaVersion:                 1,
		LengthTolerance:               1e-7,
		AngleTolerance:                1e-8,
		ClassificationLengthTolerance: 1e-5,
		ClassificationAngleTolerance:  1e-8,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if classificationResult.Status != "UNSATISFIED" || classificationResult.Classification != "UNSATISFIED" {
		t.Fatalf("solver profile did not preserve unsatisfied semantics: %#v", classificationResult)
	}
	if len(classificationResult.ConflictingConstraintIDs) != 0 {
		t.Fatalf("classification tolerance incorrectly produced a proven conflict: %#v", classificationResult)
	}

	unsatisfiedResult, err := client.SolveAssemblyWithOptions(t.Context(), "product-unsatisfied", []AssemblyBody{
		{ID: "ground", Pose: identity},
		{ID: "moving", Pose: AssemblyPose{Translation: [3]float64{4, 0, 0}, Rotation: identity.Rotation}},
	}, []AssemblyGeometry{
		{ID: "ground-origin", BodyID: "ground", Kind: "POINT"},
		{ID: "moving-origin", BodyID: "moving", Kind: "POINT"},
	}, []AssemblyConstraint{
		{ID: "fix-ground", Kind: "FIX", FirstBodyID: "ground", FixedPose: &identity},
		{ID: "distance-3", Kind: "DISTANCE", Value: 3, FirstBodyID: "moving", FirstGeometryID: "moving-origin", SecondBodyID: "ground", SecondGeometryID: "ground-origin"},
		{ID: "distance-5", Kind: "DISTANCE", Value: 5, FirstBodyID: "moving", FirstGeometryID: "moving-origin", SecondBodyID: "ground", SecondGeometryID: "ground-origin"},
	}, AssemblySolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if unsatisfiedResult.Status != "UNSATISFIED" || unsatisfiedResult.Classification != "UNSATISFIED" || len(unsatisfiedResult.UnsatisfiedConstraintIDs) != 2 || len(unsatisfiedResult.ConflictingConstraintIDs) != 0 {
		t.Fatalf("unsatisfied evidence was not preserved across the Worker RPC: %#v", unsatisfiedResult)
	}
}
