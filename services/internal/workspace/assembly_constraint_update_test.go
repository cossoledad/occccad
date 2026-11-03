package workspace

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestRefreshAssemblyConstraintsRepairsOnlySelectedLinksWithoutSolving(t *testing.T) {
	s := &Service{} // No Worker or DB: body-level supports need neither.
	model := ProductModel{Instances: []ProductInstance{{ID: "a", Translation: [3]float64{12, 0, 0}}, {ID: "b"}}, Constraints: []AssemblyConstraint{
		{ID: "recovered", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: "a", Kind: "BODY"}, EvaluationStatus: modelcore.AssemblyConstraintBroken},
		{ID: "still-missing", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: "missing", Kind: "BODY"}, EvaluationStatus: modelcore.AssemblyConstraintBroken},
		{ID: "unselected", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: "b", Kind: "BODY"}, EvaluationStatus: modelcore.AssemblyConstraintBroken},
		{ID: "inactive", Kind: "FIX", Suppressed: true, First: AssemblyGeometryRef{InstanceID: "b", Kind: "BODY"}, EvaluationStatus: modelcore.AssemblyConstraintBroken},
	}}
	before, _ := cloneAssemblyProduct(model)
	payload := json.RawMessage(`{"constraintIds":["recovered","still-missing","inactive"]}`)
	raw, _ := json.Marshal(model)
	_, changes, err := applyAssemblyConstraintUpdate(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.evaluateAssemblyConstraintUpdate(t.Context(), "product", "rev", "refresh", modelcore.DomainCommand{TypeURI: typeRefreshAssemblyConstraints, Payload: payload}, &model); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated || model.Constraints[1].EvaluationStatus != modelcore.AssemblyConstraintBroken {
		t.Fatal(model.Constraints)
	}
	if !model.Constraints[3].Suppressed || model.Constraints[3].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated {
		t.Fatal("refresh must repair an inactive link without activating its constraint", model.Constraints[3])
	}
	if !reflect.DeepEqual(before.Instances, model.Instances) || !reflect.DeepEqual(before.Constraints[2], model.Constraints[2]) {
		t.Fatal("refresh moved an instance or touched an unselected constraint")
	}
	values, err := modelValues("PRODUCT", raw, changes)
	if err != nil {
		t.Fatal(err)
	}
	next, _ := json.Marshal(model)
	restored, err := applyModelValues("PRODUCT", next, values)
	if err != nil {
		t.Fatal(err)
	}
	var undone ProductModel
	_ = json.Unmarshal(restored, &undone)
	if !reflect.DeepEqual(before, undone) {
		t.Fatal("refresh compensation lost evidence", undone)
	}
}

func TestAssemblyConstraintUpdateRejectsAmbiguousOrMissingTargets(t *testing.T) {
	for _, payload := range []string{`{}`, `{"constraintIds":[""]}`, `{"constraintIds":["a","a"]}`, `{"constraintIds":["missing"]}`} {
		_, _, err := applyAssemblyConstraintUpdate(json.RawMessage(`{"constraints":[{"id":"a"}]}`), json.RawMessage(payload))
		if err == nil {
			t.Fatal("invalid update target accepted", payload)
		}
	}
}

func TestRefreshAssemblyConstraintsKeepsNumericalDiagnostics(t *testing.T) {
	for _, status := range []modelcore.AssemblyConstraintEvaluationStatus{modelcore.AssemblyConstraintNotUpdated, modelcore.AssemblyConstraintImpossible} {
		model := ProductModel{Instances: []ProductInstance{{ID: "a"}}, Constraints: []AssemblyConstraint{{ID: "c", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: "a", Kind: "BODY"}, EvaluationStatus: status, EvaluationSummary: "existing numeric diagnosis", EvaluationFailure: &AssemblyEvaluationFailure{Code: "CONFLICT"}}}}
		if err := (&Service{}).evaluateAssemblyConstraintUpdate(t.Context(), "p", "r", "refresh", modelcore.DomainCommand{TypeURI: typeRefreshAssemblyConstraints, Payload: json.RawMessage(`{"constraintIds":["c"]}`)}, &model); err != nil {
			t.Fatal(err)
		}
		c := model.Constraints[0]
		if c.EvaluationStatus != status || c.EvaluationSummary != "existing numeric diagnosis" || c.EvaluationFailure == nil {
			t.Fatal("link refresh erased numerical evidence", c)
		}
	}
}
