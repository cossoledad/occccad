package workspace

import (
	"testing"

	"github.com/occccad/occccad/internal/geometry"
)

func TestRedundantConstraintDoesNotOverrideDefinitionStatus(t *testing.T) {
	if got := sketchDefinitionStatus(geometry.SketchSolveRedundant, 0); got != "FULLY_CONSTRAINED" {
		t.Fatalf("redundant zero-DoF component = %q, want FULLY_CONSTRAINED", got)
	}
	if got := sketchDefinitionStatus(geometry.SketchSolveRedundant, 2); got != "UNDER_CONSTRAINED" {
		t.Fatalf("redundant two-DoF component = %q, want UNDER_CONSTRAINED", got)
	}
}
