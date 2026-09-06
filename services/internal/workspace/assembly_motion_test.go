package workspace

import (
	"github.com/occccad/occccad/internal/geometry"
	"testing"
)

func TestAssemblyPreferenceMustConvergeBeforeCommit(t *testing.T) {
	for _, status := range []geometry.PreferenceStatus{geometry.PreferenceNotEvaluated, geometry.PreferenceStalled, geometry.PreferenceIterationLimit, geometry.PreferenceConverged} {
		result := geometry.AssemblySolve{Status: "CONVERGED", Components: []geometry.AssemblyComponentDof{{ComponentID: "c", Solved: true, Preference: geometry.AssemblyMotionPreference{Status: status, GeometricallyFeasible: true}}}}
		err := validateAssemblyPreference(result)
		if (err == nil) != (status == geometry.PreferenceConverged) {
			t.Fatalf("unexpected commit eligibility for %v: %v", status, err)
		}
	}
}
