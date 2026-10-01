package workspace

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestAssemblyAngleQuantityStableReferenceAtomicityAndCompensation(t *testing.T) {
	raw := json.RawMessage(`{"instances":[],"constraints":[]}`)
	add := func(id, key string, value float64, expression *string) {
		t.Helper()
		p, _ := json.Marshal(addAssemblyConstraintPayload{Constraint: AssemblyConstraint{ID: id, Kind: "ANGLE", AngleRelation: "FREE", Value: value,
			First: AssemblyGeometryRef{InstanceID: "a", Kind: "AXIS"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "AXIS"}}, QuantityExpression: expression, QuantityKey: key})
		next, _, err := applyAddAssemblyConstraint(raw, p)
		if err != nil {
			t.Fatal(err)
		}
		raw = next
	}
	add("base", "Base", math.Pi/2, nil)
	expression := "Base + 30 deg"
	add("dependent", "Dependent", 0, &expression)
	var original ProductModel
	_ = json.Unmarshal(raw, &original)
	if math.Abs(original.Constraints[1].Value-2*math.Pi/3) > 1e-12 {
		t.Fatal(original)
	}
	edit := func(p editAssemblyConstraintPayload) (json.RawMessage, modelcore.ChangeSet, error) {
		payload, _ := json.Marshal(p)
		return applyEditAssemblyConstraint(raw, payload)
	}
	next, changes, err := edit(editAssemblyConstraintPayload{ConstraintID: "base", Value: math.Pi / 3, QuantityKey: "Renamed"})
	if err != nil {
		t.Fatal(err)
	}
	var changed ProductModel
	_ = json.Unmarshal(next, &changed)
	c := changed.Constraints[1]
	if math.Abs(c.Value-math.Pi/2) > 1e-12 || c.OffsetParameter != nil || c.DefinitionVersion != 2 || c.QuantityParameter == nil ||
		c.QuantityParameter.Source.Expression.CheckedAST.Left.ParameterID != "angle:base" ||
		math.Abs(c.QuantityParameter.Source.Expression.CheckedAST.Right.Quantity.SIValue-math.Pi/6) > 1e-12 ||
		!strings.HasPrefix(c.QuantityParameter.Source.Expression.SourceText, "(Renamed + ") || !strings.HasSuffix(c.QuantityParameter.Source.Expression.SourceText, " deg)") {
		t.Fatal("stable angle binding or canonical definition lost", c, c.QuantityParameter.Source.Expression)
	}
	values, err := modelValues("PRODUCT", next, changes)
	if err != nil {
		t.Fatal(err)
	}
	desired, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PRODUCT", next, desired)
	if err != nil {
		t.Fatal(err)
	}
	var undo ProductModel
	_ = json.Unmarshal(restored, &undo)
	if err := resolveAssemblyQuantities(&undo); err != nil || math.Abs(undo.Constraints[0].Value-math.Pi/2) > 1e-12 || math.Abs(undo.Constraints[1].Value-2*math.Pi/3) > 1e-12 {
		t.Fatal("actual entity compensation failed", undo, err)
	}
	raw = next
	for _, bad := range []string{"1 mm", "Dependent + 1 deg", "361 deg", "-1 deg", "1 rad / 0"} {
		candidate, set, err := edit(editAssemblyConstraintPayload{ConstraintID: "base", QuantityExpression: &bad})
		if err == nil || candidate != nil || len(set.Changes) != 0 {
			t.Fatal("invalid angle expression was committed", bad, err)
		}
	}
	valid := "60 deg"
	accepted, _, err := edit(editAssemblyConstraintPayload{ConstraintID: "base", Value: -999, QuantityExpression: &valid})
	if err != nil {
		t.Fatal("stale numeric projection overruled a valid Quantity", err)
	}
	var validModel ProductModel
	_ = json.Unmarshal(accepted, &validModel)
	if math.Abs(validModel.Constraints[0].Value-math.Pi/3) > 1e-12 {
		t.Fatal(validModel)
	}
}

func TestAssemblyAngleQuantityMeasurementActivationPreservesDefinition(t *testing.T) {
	expression := "360 deg"
	payload, _ := json.Marshal(addAssemblyConstraintPayload{Constraint: AssemblyConstraint{ID: "turn", Kind: "ANGLE", AngleRelation: "FREE",
		First: AssemblyGeometryRef{InstanceID: "a", Kind: "AXIS"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "AXIS"}}, QuantityExpression: &expression})
	raw, _, err := applyAddAssemblyConstraint(json.RawMessage(`{"instances":[],"constraints":[]}`), payload)
	if err != nil {
		t.Fatal(err)
	}
	var initial ProductModel
	_ = json.Unmarshal(raw, &initial)
	parameter, _ := json.Marshal(initial.Constraints[0].QuantityParameter)
	if initial.Constraints[0].Value != 0 || math.Abs(initial.Constraints[0].QuantityParameter.EvaluatedValue.SIValue-2*math.Pi) > 1e-12 {
		t.Fatal("360 static normalization lost source Quantity", initial)
	}
	for _, mode := range []string{"MEASURED", "DRIVING"} {
		payload, _ = json.Marshal(assemblyConstraintStatePayload{ConstraintIDs: []string{"turn"}, Mode: &mode})
		raw, _, err = applyAssemblyConstraintState(raw, payload)
		if err != nil {
			t.Fatal(err)
		}
		for _, suppressed := range []bool{true, false} {
			payload, _ = json.Marshal(assemblyConstraintStatePayload{ConstraintIDs: []string{"turn"}, Suppressed: &suppressed})
			raw, _, err = applyAssemblyConstraintState(raw, payload)
			if err != nil {
				t.Fatal(err)
			}
		}
		var model ProductModel
		_ = json.Unmarshal(raw, &model)
		applyAssemblyMeasurements(&model, geometry.AssemblySolve{EquationResiduals: []geometry.AssemblyEquationResidual{{ConstraintID: "turn", EquationID: "turn/equation/SPATIAL_ANGLE", NormalizedValue: .7}}}, defaultAssemblySolverProfile())
		c := model.Constraints[0]
		frozen, _ := json.Marshal(c.QuantityParameter)
		if c.Mode != mode || c.Suppressed || c.Value != 0 || string(frozen) != string(parameter) {
			t.Fatal("activation/mode modified driving Quantity", c)
		}
		if mode == "MEASURED" && (c.MeasuredValue == nil || math.Abs(*c.MeasuredValue-.7) > 1e-12) {
			t.Fatal(c)
		}
		if mode == "DRIVING" && c.MeasuredValue != nil {
			t.Fatal("Driving retained stale measured result", c)
		}
		applyAssemblyMeasurements(&model, geometry.AssemblySolve{}, defaultAssemblySolverProfile())
		if model.Constraints[0].MeasuredValue != nil {
			t.Fatal("missing measurement retained stale value")
		}
	}
}

func TestAssemblyQuantityLegacyReadAndExplicitEditMigration(t *testing.T) {
	model := ProductModel{Constraints: []AssemblyConstraint{{ID: "gap", Kind: "DISTANCE", Value: 4,
		First: AssemblyGeometryRef{InstanceID: "a", Kind: "POINT"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "POINT"}}}}
	if err := editAssemblyQuantity(&model, 0, nil, "Gap", 4); err != nil {
		t.Fatal(err)
	}
	old := &model.Constraints[0]
	old.OffsetParameter, old.QuantityParameter, old.DefinitionVersion = old.QuantityParameter, nil, 0
	before, _ := json.Marshal(model)
	var cold ProductModel
	_ = json.Unmarshal(before, &cold)
	if err := resolveAssemblyQuantities(&cold); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(cold)
	if string(after) != string(before) {
		t.Fatal("ordinary read rewrote historical parameter semantics")
	}
	payload, _ := json.Marshal(editAssemblyConstraintPayload{ConstraintID: "gap", Value: 5})
	next, changes, err := applyEditAssemblyConstraint(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	var edited ProductModel
	_ = json.Unmarshal(next, &edited)
	c := edited.Constraints[0]
	if c.OffsetParameter != nil || c.QuantityParameter == nil || c.QuantityParameter.ParameterID != "offset:gap" || c.DefinitionVersion != 2 || c.Value != 5 {
		t.Fatal(c)
	}
	values, err := modelValues("PRODUCT", next, changes)
	if err != nil {
		t.Fatal(err)
	}
	desired, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PRODUCT", next, desired)
	if err != nil {
		t.Fatal(err)
	}
	var undo ProductModel
	_ = json.Unmarshal(restored, &undo)
	if undo.Constraints[0].QuantityParameter != nil || undo.Constraints[0].OffsetParameter == nil || undo.Constraints[0].Value != 4 {
		t.Fatal("compensation did not restore legacy definition", undo)
	}
}

func TestAssemblyQuantityConflictingInputsAndUnsupportedRelations(t *testing.T) {
	first, second := "2 mm", "3 mm"
	if _, _, err := assemblyQuantityInputs("DISTANCE", &first, "", &second, ""); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
	if _, _, err := assemblyQuantityInputs("DISTANCE", nil, "A", nil, "B"); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
	if _, _, err := assemblyQuantityInputs("ANGLE", nil, "", &first, ""); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
	if expr, key, err := assemblyQuantityInputs("DISTANCE", &first, "A", &first, "A"); err != nil || *expr != first || key != "A" {
		t.Fatal(expr, key, err)
	}
	for _, relation := range []string{"PARALLEL", "PERPENDICULAR"} {
		model := ProductModel{Constraints: []AssemblyConstraint{{ID: "relation", Kind: "ANGLE", AngleRelation: relation}}}
		if err := editAssemblyQuantity(&model, 0, &first, "", 0); !errors.Is(err, ErrValidation) {
			t.Fatal("nonquantity relation accepted expression", relation, err)
		}
	}
}

func TestAssemblyQuantitiesShareSIAndStableCrossDimensionReferences(t *testing.T) {
	model := ProductModel{Instances: []ProductInstance{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}, Constraints: []AssemblyConstraint{
		{ID: "span", Kind: "DISTANCE", First: AssemblyGeometryRef{InstanceID: "a", Kind: "POINT"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "POINT"}},
		{ID: "angle", Kind: "ANGLE", AngleRelation: "FREE", First: AssemblyGeometryRef{InstanceID: "a", Kind: "AXIS"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "AXIS"}},
	}}
	length := "2 m + 10 mm"
	if err := editAssemblyQuantity(&model, 0, &length, "Span", 0); err != nil {
		t.Fatal(err)
	}
	angle := "Span / 1 m * 90 deg"
	if err := editAssemblyQuantity(&model, 1, &angle, "Rotation", 0); err != nil {
		t.Fatal(err)
	}
	if math.Abs(model.Constraints[0].Value-2010) > 1e-10 ||
		math.Abs(model.Constraints[0].QuantityParameter.EvaluatedValue.SIValue-2.01) > 1e-12 ||
		math.Abs(model.Constraints[1].Value-2.01*math.Pi/2) > 1e-12 ||
		model.Constraints[1].QuantityParameter.Source.Expression.CheckedAST.Left.Left.ParameterID != "offset:span" {
		t.Fatal("SI Quantity to mm/rad compilation or stable cross-family dependency incorrect", model)
	}
	if err := editAssemblyQuantity(&model, 0, nil, "RenamedSpan", 2010); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(model.Constraints[1].QuantityParameter.Source.Expression.SourceText, "RenamedSpan") ||
		math.Abs(model.Constraints[1].Value-2.01*math.Pi/2) > 1e-12 {
		t.Fatal(model)
	}
	// The graph must retain the cross-family parameter edge, not just numeric values.
	graph, _, err := buildProductEvaluation(model, "revision", "hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	closure := graph.DirtyClosure([]modelcore.DependencyKey{"parameter:offset:span"})
	found := false
	for _, key := range closure {
		if key == "parameter:angle:angle" {
			found = true
		}
	}
	if !found {
		t.Fatal("stable cross-family parameter dependency missing", closure)
	}
}
