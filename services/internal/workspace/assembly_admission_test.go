package workspace

import (
	"errors"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

// A scalar mate per body stands in for the authoritative solve. Conflicts write
// deliberately invalid trial poses, proving admission never leaks failed results.
func scalarAssemblyEvaluator(calls *[][]string) assemblySetEvaluator {
	return func(stage string, model *ProductModel, excluded map[string]bool) error {
		values := map[string]float64{}
		var active []string
		for i := range model.Constraints {
			c := &model.Constraints[i]
			if c.Suppressed || excluded[c.ID] {
				continue
			}
			active = append(active, c.ID)
			body := c.First.InstanceID
			if previous, ok := values[body]; ok && previous != c.Value {
				model.Instances[0].Translation[0] = 999
				return &assemblySolveFailure{status: "INCONSISTENT", code: "CONFLICT", diagnostic: "incompatible offsets"}
			}
			values[body] = c.Value
		}
		*calls = append(*calls, active)
		for i := range model.Instances {
			if x, ok := values[model.Instances[i].ID]; ok {
				model.Instances[i].Translation[0] = x
			}
		}
		for i := range model.Constraints {
			c := &model.Constraints[i]
			if !c.Suppressed && !excluded[c.ID] {
				c.EvaluationStatus = modelcore.AssemblyConstraintVerified
				c.EvaluationSummary = "accepted"
			}
		}
		return nil
	}
}

func admissionModel() ProductModel {
	return ProductModel{Instances: []ProductInstance{{ID: "a", Translation: [3]float64{1, 0, 0}}, {ID: "b"}}, Constraints: []AssemblyConstraint{
		{ID: "old", First: AssemblyGeometryRef{InstanceID: "a"}, Value: 1, EvaluationStatus: modelcore.AssemblyConstraintVerified},
		{ID: "conflict", First: AssemblyGeometryRef{InstanceID: "a"}, Value: 2, EvaluationStatus: modelcore.AssemblyConstraintNotUpdated},
		{ID: "independent", First: AssemblyGeometryRef{InstanceID: "b"}, Value: 3, EvaluationStatus: modelcore.AssemblyConstraintNotUpdated},
	}}
}

func TestAssemblyAdmissionQuarantinesOnlyNewConflictAndAllowsDrag(t *testing.T) {
	model := admissionModel()
	var calls [][]string
	if err := evaluateAssemblyAdmission(&model, false, scalarAssemblyEvaluator(&calls)); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintVerified || model.Constraints[1].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated || model.Constraints[2].EvaluationStatus != modelcore.AssemblyConstraintVerified {
		t.Fatal(model.Constraints)
	}
	if model.Constraints[1].Suppressed || model.Instances[0].Translation[0] != 1 || model.Instances[1].Translation[0] != 3 {
		t.Fatal("failed candidate leaked or changed activation", model)
	}
	if !reflect.DeepEqual(calls[len(calls)-1], []string{"old", "independent"}) {
		t.Fatal("final solve retained rejected equation", calls)
	}
	calls = nil
	model.Instances[0].Translation[1] = 7
	if err := evaluateAssemblyAdmission(&model, true, scalarAssemblyEvaluator(&calls)); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], []string{"old", "independent"}) || model.Instances[0].Translation[1] != 7 {
		t.Fatal("drag retried quarantine or changed free motion", calls, model)
	}
	// Suppressing the earlier mate explicitly retries the pending definition.
	model.Constraints[0].Suppressed = true
	if err := evaluateAssemblyAdmission(&model, false, scalarAssemblyEvaluator(&calls)); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[1].EvaluationStatus != modelcore.AssemblyConstraintVerified || model.Instances[0].Translation[0] != 2 {
		t.Fatal("pending mate did not recover", model)
	}
}

func TestAssemblyAdmissionRepairsPreviouslyFailedSetInCreationOrder(t *testing.T) {
	model := admissionModel()
	model.Constraints[0].EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
	var calls [][]string
	if err := evaluateAssemblyAdmission(&model, false, scalarAssemblyEvaluator(&calls)); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintVerified || model.Constraints[1].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated {
		t.Fatal(model.Constraints)
	}
	// Upstream changes can invalidate an apparently Verified set; same policy applies.
	model.Constraints[1].EvaluationStatus = modelcore.AssemblyConstraintVerified
	if err := evaluateAssemblyAdmission(&model, false, scalarAssemblyEvaluator(&calls)); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintVerified || model.Constraints[1].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated {
		t.Fatal(model.Constraints)
	}
}

func TestAssemblyAdmissionTransportFailureIsAtomicAndDragCannotDropVerified(t *testing.T) {
	model := admissionModel()
	before, _ := cloneAssemblyProduct(model)
	unavailable := &assemblySolveFailure{status: "NUMERICAL_FAILURE", code: "UNAVAILABLE", retryable: true}
	err := evaluateAssemblyAdmission(&model, false, func(_ string, m *ProductModel, _ map[string]bool) error {
		m.Instances[0].Translation[0] = 999
		return unavailable
	})
	if !errors.Is(err, unavailable) || !reflect.DeepEqual(model, before) {
		t.Fatal("transport failure committed partial admission")
	}
	err = evaluateAssemblyAdmission(&model, true, func(_ string, m *ProductModel, _ map[string]bool) error {
		m.Constraints[0].EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
		return &assemblySolveFailure{status: "MAX_ITERATIONS"}
	})
	if err == nil || !reflect.DeepEqual(model, before) {
		t.Fatal("failed drag discarded an accepted constraint")
	}
}

func TestAssemblyAdmissionAllTrialsKeepCommandNominalPoses(t *testing.T) {
	model := admissionModel()
	var calls [][]string
	solve := scalarAssemblyEvaluator(&calls)
	if err := evaluateAssemblyAdmission(&model, false, func(stage string, m *ProductModel, excluded map[string]bool) error {
		if m.Instances[0].Translation[0] != 1 || m.Instances[1].Translation[0] != 0 {
			t.Fatal("trial redefined the command nominal", stage, m.Instances)
		}
		return solve(stage, m, excluded)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAssemblyFailureFallbackDoesNotInvalidateVerifiedConstraints(t *testing.T) {
	model := admissionModel()
	failure := &assemblySolveFailure{status: "MAX_ITERATIONS", code: "FAILED"}
	if err := acceptAssemblyEvaluationFailure(&model, failure, true); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintVerified {
		t.Fatal("blanket failure invalidated accepted constraint")
	}
	for i := range model.Constraints {
		model.Constraints[i].EvaluationStatus = modelcore.AssemblyConstraintVerified
	}
	if err := acceptAssemblyEvaluationFailure(&model, failure, true); err == nil {
		t.Fatal("unattributed failure masqueraded as successful evaluation")
	}
}

func TestAssemblyExcludedSupportsAreNotResolvedDuringDrag(t *testing.T) {
	model := admissionModel()
	model.Constraints = model.Constraints[1:2]
	model.Constraints[0].First = AssemblyGeometryRef{InstanceID: "missing", Kind: "FACE"}
	before, _ := cloneAssemblyProduct(model)
	service := &Service{}
	if err := service.resolveAssemblySupports(t.Context(), &model, map[string]bool{"conflict": true}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(model, before) {
		t.Fatal("drag reinterpreted the isolated definition", model)
	}
}
