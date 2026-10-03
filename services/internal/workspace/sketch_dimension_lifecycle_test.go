package workspace

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/modelcore"
	"testing"
)

func dimensionLifecycleModel(t *testing.T) PartModel {
	t.Helper()
	m := newPartModel()
	f := dimensionalSketchFeature("life")
	f.Sketch.Constraints = f.Sketch.Constraints[:1]
	m.Features = append(m.Features, f)
	normalizePartModel(&m)
	if err := validateAndResolvePartParameters(&m); err != nil {
		t.Fatal(err)
	}
	return m
}
func dimensionLifecycleEdit(t *testing.T, m PartModel, op SketchOperation) (PartModel, modelcore.ChangeSet, error) {
	t.Helper()
	data, _ := json.Marshal(m)
	payload, _ := json.Marshal(editSketchPayload{SketchID: "life", Operations: []SketchOperation{op}})
	next, changes, err := workspaceCommandRegistry.Apply("PART", data, modelcore.DomainCommand{CommandID: "edit-life", TypeURI: typeEditSketch, SchemaVersion: 1, Payload: payload})
	var result PartModel
	if err == nil {
		err = json.Unmarshal(next, &result)
	}
	return result, changes, err
}
func TestSketchReferenceLifecyclePreservesSourceAndExplicitRestore(t *testing.T) {
	m := dimensionLifecycleModel(t)
	id := m.Parameters[0].ParameterID
	original, _ := json.Marshal(m.Parameters[0].Source)
	c := m.Features[0].Sketch.Constraints[0]
	c.Reference = true
	ref, changes, err := dimensionLifecycleEdit(t, m, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: c.ID, Constraint: &c, ParameterKey: "gap"})
	if err != nil {
		t.Fatal(err)
	}
	if ref.Parameters[0].Role != "MEASURED" || ref.Parameters[0].ParameterID != id || ref.Parameters[0].Key != "gap" {
		t.Fatal("lost identity/reference role/rename")
	}
	source, _ := json.Marshal(ref.Parameters[0].Source)
	if string(source) != string(original) {
		t.Fatal("lost driving source")
	}
	found := false
	for _, change := range changes.Changes {
		if change.Target.SlotID == "parameter.entity" {
			found = true
		}
	}
	if !found {
		t.Fatal("metadata missing from reversible changes")
	}
	ref.Features[0].Sketch.Entities[1].Point.X = 29
	refreshSketchReferenceMeasurements(&ref)
	if *ref.Features[0].Sketch.Constraints[0].Value != 29 || ref.Parameters[0].EvaluatedValue.SIValue != .029 {
		t.Fatal("measurement stale")
	}
	c = ref.Features[0].Sketch.Constraints[0]
	c.Reference = false
	if _, _, err = dimensionLifecycleEdit(t, ref, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: c.ID, Constraint: &c}); err == nil {
		t.Fatal("implicit restore accepted")
	}
	restored, _, err := dimensionLifecycleEdit(t, ref, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: c.ID, Constraint: &c, RestoreMode: "ORIGINAL"})
	if err != nil {
		t.Fatal(err)
	}
	if *restored.Features[0].Sketch.Constraints[0].Value != 15 {
		t.Fatal("original source not restored")
	}
	restored, _, err = dimensionLifecycleEdit(t, ref, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: c.ID, Constraint: &c, RestoreMode: "MEASUREMENT"})
	if err != nil {
		t.Fatal(err)
	}
	if *restored.Features[0].Sketch.Constraints[0].Value != 29 {
		t.Fatal("explicit measurement not used")
	}
	ref.Features[0].Sketch.Entities[1].Suppressed = true
	refreshSketchReferenceMeasurements(&ref)
	if ref.Parameters[0].EvaluatedValue != nil || ref.Features[0].Sketch.Constraints[0].Value != nil {
		t.Fatal("unmeasurable retained value")
	}
}
func TestSketchDimensionExpressionAndReferenceDependency(t *testing.T) {
	m := dimensionLifecycleModel(t)
	c := m.Features[0].Sketch.Constraints[0]
	source := "10 mm + 5 mm"
	edited, _, err := dimensionLifecycleEdit(t, m, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: c.ID, Constraint: &c, ParameterSource: &source, ParameterKey: "width"})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Parameters[0].Source.Expression == nil {
		t.Fatal("formula flattened")
	}
	value := 20.
	if _, _, err = dimensionLifecycleEdit(t, edited, SketchOperation{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: c.ID, Value: &value}); err == nil {
		t.Fatal("formula silently replaced")
	}
	quantity, _ := modelcore.NewQuantity(1, "mm")
	expression, err := modelcore.CompileExpression("width", map[string]modelcore.ParameterBinding{"width": {ParameterID: edited.Parameters[0].ParameterID, Dimension: modelcore.LengthDimension}}, modelcore.LengthDimension)
	if err != nil {
		t.Fatal(err)
	}
	edited.Parameters = append(edited.Parameters, modelcore.ParameterDefinition{ParameterID: "dependent", Key: "dependent", ValueType: modelcore.ValueQuantity, Role: "INPUT", Dimension: modelcore.LengthDimension, DisplayUnit: "mm", Source: modelcore.ValueSource{Expression: &expression}, EvaluatedValue: &quantity})
	c.Reference = true
	if _, _, err = dimensionLifecycleEdit(t, edited, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: c.ID, Constraint: &c}); err == nil {
		t.Fatal("reference measurement feedback dependency accepted")
	}
}
func TestSketchSignedDimensionsAndExactLineMeasurement(t *testing.T) {
	entities := map[string]SketchEntity{"a": {ID: "a", Kind: "POINT", Point: &SketchPoint2{X: 30, Y: 8}}, "b": {ID: "b", Kind: "POINT", Point: &SketchPoint2{X: 12, Y: 8}}}
	refs := []SketchGeometryRef{{Target: "ENTITY", EntityID: "a", SubElement: "POINT"}, {Target: "ENTITY", EntityID: "b", SubElement: "POINT"}}
	for kind, want := range map[string]float64{"HORIZONTAL_DISTANCE": -18, "VERTICAL_DISTANCE": 0, "DISTANCE": 18} {
		got, ok := measureSketchConstraint(SketchConstraint{Kind: kind, References: refs}, entities)
		if !ok || got != want {
			t.Fatalf("%s=%v,%v", kind, got, ok)
		}
	}
	if !validSketchDimensionValue("HORIZONTAL_DISTANCE", -18) || validSketchDimensionValue("RADIUS", -18) {
		t.Fatal("dimension sign semantics")
	}
}

func TestSketchDimensionAtomicRenameFormulaHistoryAndColdRead(t *testing.T) {
	m := dimensionLifecycleModel(t)
	c := m.Features[0].Sketch.Constraints[0]
	source := "2 * 9 mm"
	edited, changes, err := dimensionLifecycleEdit(t, m, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: c.ID, Constraint: &c, ParameterSource: &source, ParameterKey: "width"})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(edited)
	current, err := modelValues("PART", after, changes)
	if err != nil {
		t.Fatal(err)
	}
	inverse, err := changes.Compensate(current)
	if err != nil {
		t.Fatal(err)
	}
	restoredJSON, err := applyModelValues("PART", after, inverse)
	if err != nil {
		t.Fatal(err)
	}
	var restored PartModel
	if err = json.Unmarshal(restoredJSON, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Parameters[0].Key != m.Parameters[0].Key || restored.Parameters[0].Source.Expression != nil {
		t.Fatal("undo lost definition")
	}
	redoValues := map[modelcore.PropertyAddress]json.RawMessage{}
	for _, change := range changes.Changes {
		redoValues[change.Target] = change.After
	}
	redoneJSON, err := applyModelValues("PART", restoredJSON, redoValues)
	if err != nil {
		t.Fatal(err)
	}
	var cold PartModel
	if err = json.Unmarshal(redoneJSON, &cold); err != nil {
		t.Fatal(err)
	}
	if err = validateAndResolvePartParameters(&cold); err != nil {
		t.Fatal(err)
	}
	if cold.Parameters[0].ParameterID != m.Parameters[0].ParameterID || cold.Parameters[0].Key != "width" || cold.Parameters[0].Source.Expression == nil || *cold.Features[0].Sketch.Constraints[0].Value != 18 {
		t.Fatal("redo/cold read lost stable formula")
	}
}

func TestSketchDimensionRenameReformatsBoundExpressions(t *testing.T) {
	m := dimensionLifecycleModel(t)
	c := m.Features[0].Sketch.Constraints[0]
	binding := modelcore.ParameterBinding{ParameterID: m.Parameters[0].ParameterID, Dimension: modelcore.LengthDimension}
	expression, err := modelcore.CompileExpression(m.Parameters[0].Key+" * 2", map[string]modelcore.ParameterBinding{m.Parameters[0].Key: binding}, modelcore.LengthDimension)
	if err != nil {
		t.Fatal(err)
	}
	m.Parameters = append(m.Parameters, modelcore.ParameterDefinition{ParameterID: "dependent", Key: "double_width", ValueType: modelcore.ValueQuantity, Role: "INPUT", Dimension: modelcore.LengthDimension, DisplayUnit: "mm", Source: modelcore.ValueSource{Expression: &expression}})
	edited, _, err := dimensionLifecycleEdit(t, m, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: c.ID, Constraint: &c, ParameterKey: "span"})
	if err != nil {
		t.Fatal(err)
	}
	var dependent modelcore.ParameterDefinition
	for _, parameter := range edited.Parameters {
		if parameter.ParameterID == "dependent" {
			dependent = parameter
		}
	}
	if dependent.Source.Expression == nil || dependent.Source.Expression.SourceText != "(span * 2)" || len(dependent.Source.Expression.Reads) != 1 || dependent.Source.Expression.Reads[0] != modelcore.DependencyKey("parameter:"+binding.ParameterID) || dependent.EvaluatedValue == nil || dependent.EvaluatedValue.SIValue != .03 {
		t.Fatalf("rename broke AST binding: %#v", dependent)
	}
}

func TestSketchDimensionDeletePrunesParameterAndHistoryRestores(t *testing.T) {
	for _, reference := range []bool{false, true} {
		m := dimensionLifecycleModel(t)
		m.Features[0].Sketch.Constraints[0].Reference = reference
		ensureFeatureParameters(&m)
		id := m.Parameters[0].ParameterID
		deleted, changes, err := dimensionLifecycleEdit(t, m, SketchOperation{Type: "DELETE_CONSTRAINT", ConstraintID: "distance"})
		if err != nil {
			t.Fatal(err)
		}
		if len(deleted.Parameters) != 0 || len(deleted.Features[0].Sketch.Constraints) != 0 {
			t.Fatal("delete left orphan constraint parameter")
		}
		data, _ := json.Marshal(deleted)
		current, err := modelValues("PART", data, changes)
		if err != nil {
			t.Fatal(err)
		}
		undo, err := changes.Compensate(current)
		if err != nil {
			t.Fatal(err)
		}
		restoredJSON, err := applyModelValues("PART", data, undo)
		if err != nil {
			t.Fatal(err)
		}
		var restored PartModel
		_ = json.Unmarshal(restoredJSON, &restored)
		if len(restored.Parameters) != 1 || restored.Parameters[0].ParameterID != id || restored.Features[0].Sketch.Constraints[0].Reference != reference {
			t.Fatal("undo lost dimension mode or stable parameter")
		}
		redo := map[modelcore.PropertyAddress]json.RawMessage{}
		for _, change := range changes.Changes {
			redo[change.Target] = change.After
		}
		redoneJSON, err := applyModelValues("PART", restoredJSON, redo)
		if err != nil {
			t.Fatal(err)
		}
		var cold PartModel
		if err = json.Unmarshal(redoneJSON, &cold); err != nil {
			t.Fatal(err)
		}
		if err = validateAndResolvePartParameters(&cold); err != nil {
			t.Fatal(err)
		}
		if len(cold.Parameters) != 0 || len(cold.Features[0].Sketch.Constraints) != 0 {
			t.Fatal("redo/cold read revived deleted definition")
		}
	}
}
func TestSketchDimensionDeleteRejectsFormulaDependencyAtomically(t *testing.T) {
	m := dimensionLifecycleModel(t)
	binding := modelcore.ParameterBinding{ParameterID: m.Parameters[0].ParameterID, Dimension: modelcore.LengthDimension}
	expression, err := modelcore.CompileExpression(m.Parameters[0].Key, map[string]modelcore.ParameterBinding{m.Parameters[0].Key: binding}, modelcore.LengthDimension)
	if err != nil {
		t.Fatal(err)
	}
	m.Parameters = append(m.Parameters, modelcore.ParameterDefinition{ParameterID: "dependent", Key: "dependent", ValueType: modelcore.ValueQuantity, Role: "INPUT", Dimension: modelcore.LengthDimension, DisplayUnit: "mm", Source: modelcore.ValueSource{Expression: &expression}})
	before, _ := json.Marshal(m)
	if _, _, err = dimensionLifecycleEdit(t, m, SketchOperation{Type: "DELETE_CONSTRAINT", ConstraintID: "distance"}); err == nil {
		t.Fatal("delete left dangling expression source")
	}
	after, _ := json.Marshal(m)
	if string(before) != string(after) {
		t.Fatal("rejected edit mutated model")
	}
}
func TestSketchGeneratedRelationDeleteHasExplicitImpact(t *testing.T) {
	sketch := SketchFeature{Constraints: []SketchConstraint{{ID: "mirror", Kind: "MIRROR", Internal: true}, {ID: "join", Kind: "COINCIDENT", Internal: true}}}
	if err := deleteSketchConstraint(&sketch, SketchOperation{ConstraintID: "mirror"}); err != nil {
		t.Fatal(err)
	}
	if len(sketch.Constraints) != 1 || sketch.Constraints[0].ID != "join" {
		t.Fatal("unlink silently dropped connections")
	}
	if err := deleteSketchConstraint(&sketch, SketchOperation{ConstraintID: "join"}); err == nil {
		t.Fatal("generated relation released without explicit impact")
	}
	if err := deleteSketchConstraint(&sketch, SketchOperation{ConstraintID: "join", DetachConstraintIDs: []string{"join"}}); err != nil {
		t.Fatal(err)
	}
}

func TestSketchLogicalConstraintAtomicDefinitionEditing(t *testing.T) {
	m := dimensionLifecycleModel(t)
	m.Features[0].Sketch.Constraints = []SketchConstraint{{ID: "logic", Kind: "HORIZONTAL", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "line-a", SubElement: "WHOLE"}}}}
	ensureFeatureParameters(&m)
	replacement := m.Features[0].Sketch.Constraints[0]
	replacement.Kind = "PARALLEL"
	replacement.References = append(replacement.References, SketchGeometryRef{Target: "ENTITY", EntityID: "line-b", SubElement: "WHOLE"})
	replacement.Suppressed = true
	edited, changes, err := dimensionLifecycleEdit(t, m, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: replacement.ID, Constraint: &replacement})
	if err != nil {
		t.Fatal(err)
	}
	if len(edited.Parameters) != 0 || edited.Features[0].Sketch.Constraints[0].ID != "logic" || edited.Features[0].Sketch.Constraints[0].Kind != "PARALLEL" || !edited.Features[0].Sketch.Constraints[0].Suppressed {
		t.Fatal("logical editor lost stable definition")
	}
	data, _ := json.Marshal(edited)
	current, _ := modelValues("PART", data, changes)
	undo, err := changes.Compensate(current)
	if err != nil {
		t.Fatal(err)
	}
	restoredJSON, err := applyModelValues("PART", data, undo)
	if err != nil {
		t.Fatal(err)
	}
	var restored PartModel
	_ = json.Unmarshal(restoredJSON, &restored)
	if restored.Features[0].Sketch.Constraints[0].Kind != "HORIZONTAL" || restored.Features[0].Sketch.Constraints[0].Suppressed {
		t.Fatal("logical Undo lost definition")
	}
	value := 1.
	replacement.Kind = "DISTANCE"
	replacement.Value = &value
	replacement.Unit = "mm"
	if _, _, err = dimensionLifecycleEdit(t, m, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: "logic", Constraint: &replacement}); err == nil {
		t.Fatal("logic silently converted to dimension")
	}
	replacement = m.Features[0].Sketch.Constraints[0]
	replacement.Internal = true
	if _, _, err = dimensionLifecycleEdit(t, m, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: "logic", Constraint: &replacement}); err == nil {
		t.Fatal("editor changed generated ownership")
	}
}

func TestSketchLogicalEditorRejectsInvalidDisabledDefinition(t *testing.T) {
	m := dimensionLifecycleModel(t)
	m.Features[0].Sketch.Constraints = []SketchConstraint{{ID: "logic", Kind: "HORIZONTAL", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "line-a", SubElement: "WHOLE"}}}}
	ensureFeatureParameters(&m)
	c := m.Features[0].Sketch.Constraints[0]
	c.Kind = "UNKNOWN"
	c.Suppressed = true
	if _, _, err := dimensionLifecycleEdit(t, m, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: c.ID, Constraint: &c}); err == nil {
		t.Fatal("disabled unknown constraint accepted")
	}
	c.Kind = "PARALLEL"
	c.References = []SketchGeometryRef{{Target: "ENTITY", EntityID: "point-a", SubElement: "POINT"}, {Target: "ENTITY", EntityID: "line-a", SubElement: "WHOLE"}}
	if _, _, err := dimensionLifecycleEdit(t, m, SketchOperation{Type: "UPDATE_CONSTRAINT", ConstraintID: c.ID, Constraint: &c}); err == nil {
		t.Fatal("disabled incompatible geometry accepted")
	}
}

func TestReferenceSourceCannotBypassLifecycleThroughParameterCommands(t *testing.T) {
	m := dimensionLifecycleModel(t)
	m.Features[0].Sketch.Constraints[0].Reference = true
	ensureFeatureParameters(&m)
	data, _ := json.Marshal(m)
	quantity, _ := modelcore.NewQuantity(99, "mm")
	source := modelcore.ValueSource{Literal: &quantity}
	payload, _ := json.Marshal(parameterSourcePayload{ParameterID: m.Parameters[0].ParameterID, Source: source})
	if _, _, err := applyParameterSource(data, payload); err == nil {
		t.Fatal("generic source command edited reference driver")
	}
	editPayload, _ := json.Marshal(editParameterPayload{ParameterID: m.Parameters[0].ParameterID, Key: "renamed_reference", Source: source})
	if _, _, err := applyEditParameter(data, editPayload); err == nil {
		t.Fatal("combined parameter editor replaced reference source")
	}
	editPayload, _ = json.Marshal(editParameterPayload{ParameterID: m.Parameters[0].ParameterID, Key: "renamed_reference", Source: m.Parameters[0].Source})
	if _, _, err := applyEditParameter(data, editPayload); err != nil {
		t.Fatal("reference alias with preserved source should remain editable:", err)
	}
}
