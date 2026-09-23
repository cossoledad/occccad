package workspace

import (
	"encoding/json"
	"errors"
	"github.com/occccad/occccad/internal/modelcore"
	"testing"
)

func TestAssemblyHistoryPreservesFailedDefinitionAcrossUndoRedo(t *testing.T) {
	base := ProductModel{Instances: []ProductInstance{{ID: "a", Name: "A.1", Rotation: [4]float64{0, 0, 0, 1}}, {ID: "b", Name: "B.1", Rotation: [4]float64{0, 0, 0, 1}}}}
	c := AssemblyConstraint{ID: "mate", Kind: "CONCENTRIC", First: AssemblyGeometryRef{InstanceID: "a", Kind: "AXIS", GeometryID: "axis"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "AXIS", GeometryID: "axis"}, EvaluationStatus: modelcore.AssemblyConstraintNotUpdated, EvaluationSummary: "preference did not converge"}
	raw, _ := json.Marshal(base)
	payload, _ := json.Marshal(addAssemblyConstraintPayload{Constraint: c})
	failed, add, err := applyAddAssemblyConstraint(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	var refreshed ProductModel
	_ = json.Unmarshal(failed, &refreshed)
	refreshed.Constraints[0].EvaluationStatus = modelcore.AssemblyConstraintVerified
	refreshed.Constraints[0].EvaluationSummary = "resolved"
	refreshed.Instances[0].Translation = [3]float64{2, 3, 4}
	refreshed.Instances[1].Translation = [3]float64{-2, -3, -4}
	good, _ := json.Marshal(refreshed)
	var failedModel ProductModel
	_ = json.Unmarshal(failed, &failedModel)
	refresh := appendAssemblyEvaluationChanges(modelcore.ChangeSet{}, failedModel, refreshed)
	refresh, err = reconcilePersistedChanges("PRODUCT", failed, good, refresh)
	if err != nil {
		t.Fatal(err)
	}
	compensate := func(current json.RawMessage, set modelcore.ChangeSet, undo bool) json.RawMessage {
		t.Helper()
		values, err := modelValues("PRODUCT", current, set)
		if err != nil {
			t.Fatal(err)
		}
		var desired map[modelcore.PropertyAddress]json.RawMessage
		if undo {
			desired, err = set.Compensate(values)
		} else {
			desired, err = set.Reapply(values)
		}
		if err != nil {
			t.Fatal(err)
		}
		next, err := applyModelValues("PRODUCT", current, desired)
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	for i := 0; i < 3; i++ {
		undone := compensate(good, refresh, true)
		var restored ProductModel
		_ = json.Unmarshal(undone, &restored)
		if assemblyHistoryNeedsVerification(restored) {
			t.Fatal("undo must not recompute a previously failed outcome")
		}
		if modelcore.ValueDigest(undone) != modelcore.ValueDigest(failed) {
			t.Fatalf("undo changed historical outcome\n%s\n%s", undone, failed)
		}
		empty := compensate(undone, add, true)
		recreated := compensate(empty, add, false)
		good = compensate(recreated, refresh, false)
	}
	// A previous evaluator may have solved the REVERT result again. Only that
	// immutable outcome is an allowed precondition; unrelated later edits conflict.
	guarded, err := historyChangeSetAgainstOutcome("PRODUCT", refresh, good, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = compensate(good, guarded, false)
	var edited ProductModel
	_ = json.Unmarshal(good, &edited)
	edited.Constraints[0].Value = 17
	editedJSON, _ := json.Marshal(edited)
	values, _ := modelValues("PRODUCT", editedJSON, guarded)
	if _, err = guarded.Reapply(values); !errors.Is(err, modelcore.ErrChangeConflict) {
		t.Fatalf("later edit must remain protected: %v", err)
	}
}

func TestAssemblyHistoryVerificationCannotMoveRestoredInstances(t *testing.T) {
	original := ProductModel{Instances: []ProductInstance{{ID: "a", Rotation: [4]float64{0, 0, 0, 1}}}, Constraints: []AssemblyConstraint{{ID: "c", EvaluationStatus: modelcore.AssemblyConstraintVerified}}}
	if !assemblyHistoryNeedsVerification(original) {
		t.Fatal("verified snapshots need evidence")
	}
	raw, _ := json.Marshal(original)
	var evaluated ProductModel
	_ = json.Unmarshal(raw, &evaluated)
	evaluated.Instances[0].Rotation = [4]float64{0, 0, 0, -1}
	if err := validateAssemblyHistoryPoses(original, evaluated); err != nil {
		t.Fatal(err)
	}
	evaluated.Instances[0].Translation[0] = 1
	if err := validateAssemblyHistoryPoses(original, evaluated); err == nil {
		t.Fatal("history re-evaluation silently moved an instance")
	}
}
