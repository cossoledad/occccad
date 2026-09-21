package workspace

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestAssemblyActivationPreservesModeAndCompensation(t *testing.T) {
	original := ProductModel{Constraints: []AssemblyConstraint{{ID: "angle", Kind: "ANGLE", Mode: "MEASURED", EvaluationStatus: modelcore.AssemblyConstraintBroken, EvaluationSummary: "missing support"}}}
	raw, _ := json.Marshal(original)
	next, changes, err := applyAssemblyConstraintState(raw, json.RawMessage(`{"constraintIds":["angle"],"suppressed":true}`))
	if err != nil {
		t.Fatal(err)
	}
	var disabled ProductModel
	_ = json.Unmarshal(next, &disabled)
	if !disabled.Constraints[0].Suppressed || disabled.Constraints[0].Mode != "MEASURED" || disabled.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintBroken {
		t.Fatalf("lost mode or diagnostic: %+v", disabled)
	}
	beforeValues, err := modelValues("PRODUCT", raw, changes)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PRODUCT", next, beforeValues)
	if err != nil {
		t.Fatal(err)
	}
	var undone ProductModel
	_ = json.Unmarshal(restored, &undone)
	if !reflect.DeepEqual(original, undone) {
		t.Fatalf("compensation lost state: %+v", undone)
	}
	enabled, _, err := applyAssemblyConstraintState(next, json.RawMessage(`{"constraintIds":["angle"],"suppressed":false}`))
	if err != nil {
		t.Fatal(err)
	}
	var active ProductModel
	_ = json.Unmarshal(enabled, &active)
	if active.Constraints[0].Suppressed || active.Constraints[0].Mode != "MEASURED" || active.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated {
		t.Fatalf("activation reused old evidence: %+v", active)
	}
}

func TestAssemblyStateBatchValidationIsAtomic(t *testing.T) {
	raw := json.RawMessage(`{"instances":[],"constraints":[{"id":"a","kind":"ANGLE","mode":"DRIVING"},{"id":"f","kind":"FIX","mode":"DRIVING"}]}`)
	for _, payload := range []string{
		`{"constraintIds":["a","missing"],"suppressed":true}`,
		`{"constraintIds":["a","a"],"suppressed":true}`,
		`{"constraintIds":["a","f"],"mode":"MEASURED"}`,
		`{"constraintIds":["a"],"mode":"SUPPRESSED"}`,
		`{"constraintIds":[],"suppressed":true}`,
		`{"constraintIds":["a"]}`,
	} {
		next, changes, err := applyAssemblyConstraintState(raw, json.RawMessage(payload))
		if err == nil || next != nil || len(changes.Changes) != 0 {
			t.Fatalf("non-atomic invalid request %s: %s %+v %v", payload, next, changes, err)
		}
	}
	next, changes, err := applyAssemblyConstraintState(raw, json.RawMessage(`{"constraintIds":["a","f"],"suppressed":true}`))
	if err != nil || len(changes.Changes) != 2 {
		t.Fatalf("batch: %s %+v %v", next, changes, err)
	}
	_, repeated, err := applyAssemblyConstraintState(next, json.RawMessage(`{"constraintIds":["a","f"],"suppressed":true}`))
	if err != nil || len(repeated.Changes) != 0 {
		t.Fatalf("repeat must be no-op: %+v %v", repeated, err)
	}
}

func TestAssemblyManifestEmptyActiveSetFreezesInactiveDefinitions(t *testing.T) {
	pose := InstancePose{Rotation: [4]float64{0, 0, 0, 1}}
	definitions := []AssemblyConstraint{{ID: "fix", Kind: "FIX", Mode: "DRIVING", Suppressed: true, FixedPose: &pose, EvaluationStatus: modelcore.AssemblyConstraintBroken}}
	manifest, err := newAssemblySolveManifest("product", "revision", "hash", []geometry.AssemblyBody{{ID: "body", Pose: geometry.AssemblyPose{Rotation: pose.Rotation}}}, nil, nil, nil, nil, nil, definitions)
	if err != nil {
		t.Fatal(err)
	}
	pose.Translation[0] = 42
	definitions[0].Suppressed = false
	if len(manifest.Constraints) != 0 || !manifest.Definitions[0].Suppressed || manifest.Definitions[0].FixedPose.Translation[0] != 0 {
		t.Fatal("inactive definitions are not frozen independently of active equations")
	}
	digest := manifest.Digest
	manifest.Digest = ""
	if digest != resolvedDigest(manifest) {
		t.Fatal("definition change escaped manifest digest")
	}
}

func TestAssemblyMeasuredOffsetRejectsNonParallelSupports(t *testing.T) {
	stale := 99.0
	model := ProductModel{Constraints: []AssemblyConstraint{{ID: "offset", Kind: "DISTANCE", Mode: "MEASURED", Value: 10, MeasuredValue: &stale, EvaluationStatus: modelcore.AssemblyConstraintVerified}}}
	result := geometry.AssemblySolve{EquationResiduals: []geometry.AssemblyEquationResidual{
		{ConstraintID: "offset", EquationID: "offset/equation/NORMAL_X", NormalizedValue: 0.1},
		{ConstraintID: "offset", EquationID: "offset/equation/PLANE_DISTANCE", NormalizedValue: 2},
	}}
	applyAssemblyMeasurements(&model, result, defaultAssemblySolverProfile())
	if model.Constraints[0].MeasuredValue != nil {
		t.Fatal("nonparallel plane measurement retained stale value")
	}
	result.EquationResiduals[0].NormalizedValue = 0
	applyAssemblyMeasurements(&model, result, defaultAssemblySolverProfile())
	if model.Constraints[0].MeasuredValue == nil || *model.Constraints[0].MeasuredValue != 12 || model.Constraints[0].Value != 10 {
		t.Fatal("measurement must not rewrite driving definition")
	}
	model.Constraints[0].Suppressed = true
	applyAssemblyMeasurements(&model, result, defaultAssemblySolverProfile())
	if model.Constraints[0].MeasuredValue != nil {
		t.Fatal("inactive measurement remained current")
	}
}

func TestRelativeFixPoseEditAndCompensation(t *testing.T) {
	original := ProductModel{Instances: []ProductInstance{{ID: "body", Translation: [3]float64{0, 0, 3}, Rotation: [4]float64{0, 0, 0, 1}}}, Constraints: []AssemblyConstraint{{ID: "fix", Kind: "FIX", FixMode: "RELATIVE", First: AssemblyGeometryRef{Kind: "BODY", InstanceID: "body"}, FixedPose: &InstancePose{Translation: [3]float64{0, 0, 3}, Rotation: [4]float64{0, 0, 0, 1}}}}}
	raw, _ := json.Marshal(original)
	next, changes, err := applyEditAssemblyConstraint(raw, json.RawMessage(`{"constraintId":"fix","fixMode":"RELATIVE","fixedPose":{"translation":[1,2,4],"rotation":[0,0,1,0]}}`))
	if err != nil {
		t.Fatal(err)
	}
	var updated ProductModel
	_ = json.Unmarshal(next, &updated)
	if updated.Instances[0].Translation != ([3]float64{1, 2, 4}) || updated.Instances[0].Rotation != ([4]float64{0, 0, 1, 0}) {
		t.Fatal("explicit pose ignored")
	}
	values, err := modelValues("PRODUCT", raw, changes)
	if err != nil {
		t.Fatal(err)
	}
	undone, err := applyModelValues("PRODUCT", next, values)
	if err != nil {
		t.Fatal(err)
	}
	var restored ProductModel
	_ = json.Unmarshal(undone, &restored)
	if !reflect.DeepEqual(original, restored) {
		t.Fatalf("pose and definition compensation lost: %+v", restored)
	}
}

func TestStableAngleAxisValidation(t *testing.T) {
	c := AssemblyConstraint{Kind: "ANGLE", AngleRelation: "DIRECTED", First: AssemblyGeometryRef{InstanceID: "a", Kind: "PLANE"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "PLANE"}}
	if validateInstanceConstraintReferences(c) == nil {
		t.Fatal("missing reference axis accepted")
	}
	c.AngleAxis = &AssemblyGeometryRef{InstanceID: "a", Kind: "AXIS"}
	if validateInstanceConstraintReferences(c) == nil {
		t.Fatal("axis from another body silently frozen")
	}
	c.AngleAxis.InstanceID = "b"
	if err := validateInstanceConstraintReferences(c); err != nil {
		t.Fatal(err)
	}
	c.AngleAxis.Kind = "POINT"
	if validateInstanceConstraintReferences(c) == nil {
		t.Fatal("directionless point axis accepted")
	}
}

func TestDatumSupportAvailabilityChecksKindAndAxis(t *testing.T) {
	part := newPartModel()
	if !datumAssemblyReferenceExists(part, AssemblyGeometryRef{Kind: "PLANE", GeometryID: "datum-xy"}) {
		t.Fatal("existing plane missing")
	}
	for _, ref := range []AssemblyGeometryRef{{Kind: "AXIS", GeometryID: "datum-xy"}, {Kind: "PLANE", GeometryID: "removed-datum"}, {Kind: "AXIS", GeometryID: "axis-system-default", Axis: "camera"}} {
		if datumAssemblyReferenceExists(part, ref) {
			t.Fatalf("invalid support accepted: %+v", ref)
		}
	}
}

func TestAngleQuantityEditPreservesReferenceIntent(t *testing.T) {
	c := AssemblyConstraint{ID: "angle", Kind: "ANGLE", AngleRelation: "DIRECTED", ReverseAngleAxis: true, First: AssemblyGeometryRef{InstanceID: "a", Kind: "PLANE"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "PLANE"}, AngleAxis: &AssemblyGeometryRef{InstanceID: "b", Kind: "AXIS", GeometryID: "stable-axis"}}
	raw, _ := json.Marshal(ProductModel{Constraints: []AssemblyConstraint{c}})
	next, _, err := applyEditAssemblyConstraint(raw, json.RawMessage(`{"constraintId":"angle","value":1}`))
	if err != nil {
		t.Fatal(err)
	}
	var model ProductModel
	_ = json.Unmarshal(next, &model)
	changed := model.Constraints[0]
	if !changed.ReverseAngleAxis || changed.AngleRelation != "DIRECTED" || !reflect.DeepEqual(changed.AngleAxis, c.AngleAxis) {
		t.Fatal("quantity edit lost stable angle intent")
	}
}

func TestRestoredFailureKeepsBrokenAndInactiveEvidence(t *testing.T) {
	failure := &assemblySolveFailure{status: "INCONSISTENT", code: "ASSEMBLY_CONFLICT", diagnostic: "inconsistent constraints"}
	model := ProductModel{Constraints: []AssemblyConstraint{{ID: "active", EvaluationStatus: modelcore.AssemblyConstraintNotUpdated}, {ID: "broken", EvaluationStatus: modelcore.AssemblyConstraintBroken}, {ID: "inactive", Suppressed: true, EvaluationStatus: modelcore.AssemblyConstraintVerified}}}
	if acceptAssemblyEvaluationFailure(&model, failure, false) == nil {
		t.Fatal("ordinary command accepted an inconsistent model")
	}
	if err := acceptAssemblyEvaluationFailure(&model, failure, true); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintImpossible || model.Constraints[1].EvaluationStatus != modelcore.AssemblyConstraintBroken || model.Constraints[2].EvaluationStatus != modelcore.AssemblyConstraintVerified {
		t.Fatal("failure evidence lost orthogonal state")
	}
	retryable := *failure
	retryable.retryable = true
	if acceptAssemblyEvaluationFailure(&model, &retryable, true) == nil {
		t.Fatal("transient failure was recorded as deterministic history")
	}
}
