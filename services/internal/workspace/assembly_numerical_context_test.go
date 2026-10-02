package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/occccad/occccad/internal/modelcore"
	"testing"
	"time"
)

func TestAssemblyNumericalBudgetReservesParentAndPropagatesCancellation(t *testing.T) {
	parent, cancel := context.WithTimeout(t.Context(), time.Second)
	work, stop := assemblyNumericalContext(parent)
	defer stop()
	p, _ := parent.Deadline()
	w, _ := work.Deadline()
	if !w.Before(p) || p.Sub(w) < 200*time.Millisecond {
		t.Fatal("numerical work exhausted confirmation lifecycle")
	}
	cancel()
	if !errors.Is(work.Err(), context.Canceled) {
		t.Fatal("detached numerical request")
	}
}

func TestAssemblyCurrentDefinitionFormatAndFailureBoundary(t *testing.T) {
	for _, raw := range []string{`{"definitionVersion":1}`, `{"definitionVersion":2,"offsetParameter":{"source":"LITERAL"}}`} {
		var c AssemblyConstraint
		err := json.Unmarshal([]byte(raw), &c)
		if err == nil {
			err = validateAssemblyDefinitionFormat(ProductModel{Constraints: []AssemblyConstraint{c}})
		}
		if !errors.Is(err, ErrValidation) {
			t.Fatalf("old format accepted: %s", raw)
		}
	}
	for _, failure := range []*assemblySolveFailure{
		{status: "INVALID_MODEL", phase: assemblySolveSolving},
		{status: "NUMERICAL_FAILURE", phase: assemblySolveResolving, retryable: true},
	} {
		model := ProductModel{Constraints: []AssemblyConstraint{{EvaluationStatus: modelcore.AssemblyConstraintNotUpdated}}}
		if acceptAssemblyEvaluationFailure(&model, failure, true) == nil {
			t.Fatal("invalid preparation/protocol accepted as deferred definition")
		}
	}
}
