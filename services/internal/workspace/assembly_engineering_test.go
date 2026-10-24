package workspace

import (
	"github.com/occccad/occccad/internal/geometry"
	"testing"
)

func TestEngineeringEvidenceRequiresAcceptedLinearizationPose(t *testing.T) {
	pose := geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}
	makeResult := func() geometry.AssemblySolve {
		return geometry.AssemblySolve{SolverBuild: assemblySolverBuildPolicy, Status: "CONVERGED", Bodies: []geometry.AssemblyBody{{ID: "a", Pose: pose}}, Components: []geometry.AssemblyComponentDof{{BodyIDs: []string{"a"}, Solved: true, GaugeDof: 6, Freedoms: []geometry.AssemblyBodyFreedom{{BodyID: "a", RelativeToBodyID: "a", LinearizationPose: pose}}}}}
	}
	for _, tc := range []struct {
		name      string
		mutate    func(*geometry.AssemblySolve)
		available bool
	}{
		{"same", func(*geometry.AssemblySolve) {}, true},
		{"missing build", func(r *geometry.AssemblySolve) { r.SolverBuild = "" }, false},
		{"moved", func(r *geometry.AssemblySolve) { r.Bodies[0].Pose.Translation[0] = 1 }, false},
		{"different linearization", func(r *geometry.AssemblySolve) { r.Components[0].Freedoms[0].LinearizationPose.Translation[1] = 1 }, false},
		{"missing freedom", func(r *geometry.AssemblySolve) { r.Components[0].Freedoms = nil }, false},
		{"unsolved", func(r *geometry.AssemblySolve) { r.Components[0].Solved = false }, false},
		{"missing body", func(r *geometry.AssemblySolve) { r.Bodies = nil }, false},
		{"failed", func(r *geometry.AssemblySolve) { r.Status = "MAX_ITERATIONS" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := makeResult()
			tc.mutate(&r)
			var e AssemblyEngineeringEvidence
			engineeringResult(&e, []geometry.AssemblyBody{{ID: "a", Pose: pose}}, r, defaultAssemblySolverProfile())
			if e.Available != tc.available {
				t.Fatal(e)
			}
			if !tc.available {
				for _, c := range e.Components {
					if len(c.Freedoms) > 0 {
						t.Fatal("unknown component retained directions", c)
					}
				}
			}
		})
	}
}
