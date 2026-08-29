package geometry

import "testing"

func TestSketchSolveStatusNormalizesWorkerVocabulary(t *testing.T) {
	tests := map[string]SketchSolveStatus{
		"SOLVED":               SketchSolveFullyConstrained,
		"FULLY_CONSTRAINED":    SketchSolveFullyConstrained,
		"UNDER_CONSTRAINED":    SketchSolveUnderConstrained,
		"CONFLICTING":          SketchSolveConflicting,
		"REDUNDANT":            SketchSolveRedundant,
		"INVALID_MODEL":        SketchSolveInvalid,
		"unknown-backend-code": SketchSolveFailed,
	}
	for input, want := range tests {
		if got := sketchSolveStatus(input); got != want {
			t.Errorf("sketchSolveStatus(%q) = %q, want %q", input, got, want)
		}
	}
}
