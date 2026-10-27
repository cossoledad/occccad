package workspace

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/modelcore"
	"testing"
)

func TestContextFeatureEditPreservesUnchangedPatternDefinition(t *testing.T) {
	original := Feature{ID: "pattern", Type: "SOLID_PATTERN", Name: "Pattern.1", Pattern: &FeaturePattern{PatternDefinition: PatternDefinition{Count: 3, Spacing: 10, Angle: 45, Phase: 2, Origin: [3]float64{1, 2, 3}, Direction: [3]float64{1, 0, 0}}}}
	base := PartModel{Features: []Feature{original}}
	projected := clonePartModel(base)
	projected.Features[0].Pattern.Count = 5
	projected.Features[0].Pattern.Spacing = 20
	projected.Features[0].Pattern.Origin = [3]float64{10, 20, 30}
	projected.Features[0].Pattern.Direction = [3]float64{0, 1, 0}
	draft := clonePartModel(projected).Features[0]
	draft.Name = "Renamed"
	draft.Pattern.Angle = 60
	retainBaseFeatureValues(base, projected, &draft)
	if draft.Name != "Renamed" || draft.Pattern.Angle != 60 || draft.Pattern.Count != 3 || draft.Pattern.Spacing != 10 || draft.Pattern.Origin != original.Pattern.Origin || draft.Pattern.Direction != original.Pattern.Direction {
		t.Fatalf("accepted projection leaked into base definition: %+v", draft.Pattern)
	}
	if projected.Features[0].Pattern.Count != 5 {
		t.Fatal("normalization mutated the editor's accepted projection")
	}
	draft.Pattern.Spacing = 23
	retainBaseFeatureValues(base, projected, &draft)
	if draft.Pattern.Spacing != 23 {
		t.Fatal("explicit edit was lost")
	}
}

func TestContextInputWriteGuardCoversManagedParametersAndDimensions(t *testing.T) {
	for _, slot := range []string{"length", "diameter", "constraint.value"} {
		t.Run(slot, func(t *testing.T) {
			q, _ := modelcore.NewQuantity(10, "mm")
			base := PartModel{Parameters: []modelcore.ParameterDefinition{{ParameterID: "managed", OwnerFeatureID: "feature", PropertySlot: slot, Source: modelcore.ValueSource{Literal: &q}}}, ContextInputs: []ContextInput{{ID: "port", Name: "Linked dimension", Target: ContextInputTarget{Kind: "PARAMETER", TargetID: "managed"}}}}
			bindings := []ContextBinding{{ContextInputID: "port"}}
			if slot == "constraint.value" { // datum-managed values share the same guard
				base.ContextInputs[0].Target = ContextInputTarget{Kind: "DATUM", TargetID: "feature"}
			}
			raw, _ := json.Marshal(base)
			candidate := clonePartModel(base)
			candidate.Parameters[0].Key = "renamed"
			next, _ := json.Marshal(candidate)
			if err := validateContextInputWrites(raw, next, bindings); err != nil {
				t.Fatal("rename must stay editable", err)
			}
			q2, _ := modelcore.NewQuantity(20, "mm")
			candidate.Parameters[0].Source = modelcore.ValueSource{Literal: &q2}
			next, _ = json.Marshal(candidate)
			if err := validateContextInputWrites(raw, next, bindings); err == nil {
				t.Fatal("managed source write bypassed binding")
			}
			if err := validateContextInputWrites(raw, next, nil); err != nil {
				t.Fatal("independent defaults must remain editable", err)
			}
		})
	}
}

func TestContextDimensionRenameDoesNotEchoAcceptedValue(t *testing.T) {
	a, b := 10., 20.
	base := PartModel{Features: []Feature{{ID: "sketch", Sketch: &SketchFeature{Constraints: []SketchConstraint{{ID: "dimension", Value: &a}}}}}}
	projected := clonePartModel(base)
	projected.Features[0].Sketch.Constraints[0].Value = &b
	draft := projected.Features[0].Sketch.Constraints[0]
	operations := []SketchOperation{{Type: "UPDATE_CONSTRAINT", ConstraintID: "dimension", Constraint: &draft, ParameterKey: "renamed"}}
	normalized := retainBaseSketchValues(base, projected, "sketch", operations)
	if *normalized[0].Constraint.Value != 10 || normalized[0].ParameterKey != "renamed" || *draft.Value != 20 {
		t.Fatal("dimension echo normalization changed intent or caller")
	}
}

func TestContextInputProtectionAllowsStableAliasRewrite(t *testing.T) {
	names := map[string]modelcore.ParameterBinding{"pitch": {ParameterID: "stable-pitch", Dimension: modelcore.LengthDimension}}
	expression, err := modelcore.CompileExpression("pitch/2", names, modelcore.LengthDimension, "mm")
	if err != nil {
		t.Fatal(err)
	}
	a := modelcore.ValueSource{Expression: &expression}
	renamed := expression
	renamed.SourceText = "renamed_pitch / 2"
	b := modelcore.ValueSource{Expression: &renamed}
	if !contextSourceEqual(a, b) {
		t.Fatal("display alias rewrite blocked unrelated edit")
	}
	renamed.Reads = []modelcore.DependencyKey{"another-source"}
	if contextSourceEqual(a, b) {
		t.Fatal("changed source dependency bypassed protection")
	}
}
