package automation

import (
	"fmt"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

type structuredFailure struct{}

func (structuredFailure) Error() string        { return "solver iteration budget exhausted" }
func (structuredFailure) Code() string         { return "NUMERICAL_NONCONVERGENCE" }
func (structuredFailure) Retryable() bool      { return false }
func (structuredFailure) Phase() string        { return "SOLVING" }
func (structuredFailure) DiagnosticID() string { return "diagnostic-example" }

func TestAutomationFailurePreservesDomainDiagnosis(t *testing.T) {
	failure := classify(fmt.Errorf("operation failed: %w", structuredFailure{}))
	if failure.Code != "NUMERICAL_NONCONVERGENCE" || failure.Phase != "SOLVING" || failure.DiagnosticID != "diagnostic-example" || failure.Retryable {
		t.Fatalf("diagnostic fields were lost: %+v", failure)
	}
	if failure := classify(fmt.Errorf("%w: WORKSPACE_HEAD_CONFLICT", workspace.ErrValidation)); failure.Code != "WORKSPACE_HEAD_CONFLICT" {
		t.Fatalf("caller baseline conflict misclassified: %+v", failure)
	}
}
