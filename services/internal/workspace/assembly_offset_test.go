package workspace

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func TestOffsetPreviewIdentityRejectsChangedIntent(t *testing.T) {
	base := editAssemblyConstraintPayload{ConstraintID: "offset", Value: -2, DirectionRelation: "SAME", DistanceRelation: selectedPlaneNormalV1,
		First: &AssemblyGeometryRef{InstanceID: "a", Kind: "PLANE", GeometryID: "datum-xy"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "PLANE", GeometryID: "datum-xy"}}
	for _, change := range []func(*editAssemblyConstraintPayload){
		func(p *editAssemblyConstraintPayload) { p.ConstraintID = "other" },
		func(p *editAssemblyConstraintPayload) { p.First, p.Second = p.Second, p.First },
		func(p *editAssemblyConstraintPayload) { p.DistanceRelation = "UNSIGNED" },
		func(p *editAssemblyConstraintPayload) { p.DirectionRelation = "OPPOSITE" },
		func(p *editAssemblyConstraintPayload) { v := "MEASURED"; p.Mode = &v },
		func(p *editAssemblyConstraintPayload) { p.Value = 3 },
		func(p *editAssemblyConstraintPayload) { v := "Base + 1 mm"; p.QuantityExpression = &v },
		func(p *editAssemblyConstraintPayload) { p.OffsetKey = "Renamed" },
		func(p *editAssemblyConstraintPayload) { v := "Base + 1 mm"; p.QuantityExpression = &v },
		func(p *editAssemblyConstraintPayload) { p.QuantityKey = "Renamed" },
	} {
		raw, _ := json.Marshal(base)
		prepared := preparedDomainMutation{actorID: "actor", headRevision: "head", headSequence: 3, command: modelcore.DomainCommand{TypeURI: typeEditAssemblyConstraint, Payload: raw}}
		var cache interactionCandidateCache
		cache.put(interactionCandidate{id: "preview", documentID: "product", actorID: "actor", headRevision: "head", headSequence: 3, commandType: typeEditAssemblyConstraint, payloadDigest: modelcore.ValueDigest(raw), expiresAt: time.Now().Add(time.Minute)})
		changed := base
		change(&changed)
		prepared.command.Payload, _ = json.Marshal(changed)
		if _, accepted := cache.take("preview", "product", prepared); accepted {
			t.Fatal("changed Offset intent reused old candidate", changed)
		}
	}
}

func TestOffsetQuantityStableBindingAndAtomicFailure(t *testing.T) {
	model := ProductModel{}
	add := func(id, key string, value float64, expression *string) {
		t.Helper()
		raw, _ := json.Marshal(model)
		payload, _ := json.Marshal(addAssemblyConstraintPayload{Constraint: AssemblyConstraint{ID: id, Kind: "DISTANCE", Value: value, DistanceRelation: selectedPlaneNormalV1, First: AssemblyGeometryRef{InstanceID: "a", Kind: "PLANE"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "PLANE"}}, QuantityKey: key, QuantityExpression: expression})
		next, _, err := applyAddAssemblyConstraint(raw, payload)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(next, &model); err != nil {
			t.Fatal(err)
		}
	}
	add("base", "Base", -2, nil)
	expression := "Base + 5 mm"
	add("derived", "Derived", 0, &expression)
	if model.Constraints[1].Value != 3 {
		t.Fatal(model.Constraints)
	}
	edit := func(payload editAssemblyConstraintPayload) (json.RawMessage, modelcore.ChangeSet, error) {
		raw, _ := json.Marshal(model)
		p, _ := json.Marshal(payload)
		return applyEditAssemblyConstraint(raw, p)
	}
	next, changes, err := edit(editAssemblyConstraintPayload{ConstraintID: "base", Value: 4, QuantityKey: "Renamed", DistanceRelation: selectedPlaneNormalV1})
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(next, &model); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[1].Value != 9 || model.Constraints[1].QuantityParameter.Source.Expression.CheckedAST.Left.ParameterID != "offset:base" {
		t.Fatal(model)
	}
	if model.Constraints[1].QuantityParameter.Source.Expression.SourceText != "(Renamed + 5 mm)" {
		t.Fatal(model.Constraints[1].QuantityParameter)
	}
	// Real entity compensation, not a mock Undo state machine.
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
	if err = resolveAssemblyQuantities(&undo); err != nil || undo.Constraints[0].Value != -2 || undo.Constraints[1].Value != 3 {
		t.Fatal(undo, err)
	}
	for _, bad := range []string{"Renamed + 2 deg", "Derived + 1 mm", "1 mm / 0"} {
		_, set, e := edit(editAssemblyConstraintPayload{ConstraintID: "base", Value: 4, QuantityExpression: &bad, DistanceRelation: selectedPlaneNormalV1})
		if e == nil || len(set.Changes) != 0 {
			t.Fatalf("invalid expression committed: %s %v", bad, e)
		}
	}
	negative := "-2 mm"
	positive := "2 mm"
	accepted, _, e := edit(editAssemblyConstraintPayload{ConstraintID: "base", Value: -4, QuantityExpression: &positive, DistanceRelation: "UNSIGNED"})
	if e != nil {
		t.Fatal("valid expression incorrectly checked stale numeric projection", e)
	}
	var positiveModel ProductModel
	_ = json.Unmarshal(accepted, &positiveModel)
	if positiveModel.Constraints[0].Value != 2 {
		t.Fatal("expression was not authoritative", positiveModel)
	}
	raw, set, e := edit(editAssemblyConstraintPayload{ConstraintID: "base", QuantityExpression: &negative, DistanceRelation: "UNSIGNED"})
	if !errors.Is(e, ErrValidation) || raw != nil || len(set.Changes) != 0 {
		t.Fatal("unsigned negative expression was accepted", e)
	}
}

func TestOffsetCompilePreservesRolesAndExactPlaneGate(t *testing.T) {
	c := AssemblyConstraint{ID: "offset", Kind: "DISTANCE", Value: -4, DirectionRelation: "UNORIENTED", DistanceRelation: selectedPlaneNormalV1}
	v := geometry.AssemblyConstraint{FirstBodyID: "moving", SecondBodyID: "reference", Value: c.Value, DirectionRelation: c.DirectionRelation}
	if err := compileAssemblyOffset(c, &v, geometry.AssemblyGeometry{Kind: "PLANE"}, geometry.AssemblyGeometry{Kind: "PLANE"}); err != nil {
		t.Fatal(err)
	}
	if v.FirstBodyID != "moving" || v.SecondBodyID != "reference" || v.Value != -4 || v.DirectionRelation != "UNORIENTED" || v.DistanceRelation != selectedPlaneNormalV1 {
		t.Fatal(v)
	}
	if err := compileAssemblyOffset(c, &v, geometry.AssemblyGeometry{Kind: "POINT"}, geometry.AssemblyGeometry{Kind: "AXIS"}); !errors.Is(err, ErrValidation) {
		t.Fatal(err)
	}
}

func TestOffsetModeExpressionAndSuppressionCompensate(t *testing.T) {
	expression := "-2 mm"
	raw, p := json.RawMessage(`{"instances":[],"constraints":[]}`), addAssemblyConstraintPayload{Constraint: AssemblyConstraint{ID: "offset", Kind: "DISTANCE", Value: -2, DistanceRelation: selectedPlaneNormalV1, First: AssemblyGeometryRef{InstanceID: "a", Kind: "PLANE"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "PLANE"}}, QuantityExpression: &expression}
	payload, _ := json.Marshal(p)
	raw, _, err := applyAddAssemblyConstraint(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	measured := "MEASURED"
	edit, _ := json.Marshal(editAssemblyConstraintPayload{ConstraintID: "offset", Value: -2, DistanceRelation: selectedPlaneNormalV1, Mode: &measured})
	raw, _, err = applyEditAssemblyConstraint(raw, edit)
	if err != nil {
		t.Fatal(err)
	}
	for _, suppressed := range []bool{true, false} {
		payload, _ = json.Marshal(assemblyConstraintStatePayload{ConstraintIDs: []string{"offset"}, Suppressed: &suppressed})
		raw, _, err = applyAssemblyConstraintState(raw, payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	var model ProductModel
	_ = json.Unmarshal(raw, &model)
	if model.Constraints[0].Mode != "MEASURED" || model.Constraints[0].Value != -2 || model.Constraints[0].QuantityParameter.Source.Expression == nil {
		t.Fatal(model)
	}
	applyAssemblyMeasurements(&model, geometry.AssemblySolve{EquationResiduals: []geometry.AssemblyEquationResidual{{ConstraintID: "offset", EquationID: "offset/equation/PLANE_DISTANCE", NormalizedValue: -3}}}, defaultAssemblySolverProfile())
	if model.Constraints[0].MeasuredValue == nil || math.Abs(*model.Constraints[0].MeasuredValue+5) > 1e-12 || model.Constraints[0].Value != -2 {
		t.Fatal(model)
	}
	manifest, err := newAssemblySolveManifest("product", "revision", "hash", []geometry.AssemblyBody{{ID: "a", Pose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}}}, nil, nil, nil, nil, nil, model.Constraints)
	if err != nil {
		t.Fatal(err)
	}
	frozen, _ := json.Marshal(manifest.Definitions)
	model.Constraints[0].QuantityParameter.Source.Expression.SourceText = "changed"
	after, _ := json.Marshal(manifest.Definitions)
	if string(after) != string(frozen) {
		t.Fatal("manifest retained caller-owned Offset AST")
	}
	// Experimental policies are unsupported, never reinterpreted as current.
	manifest.SolverBuildPolicy = "assembly-offset-selected-plane-v8"
	if !errors.Is(validateAssemblySolveManifest(manifest), ErrValidation) {
		t.Fatal("old policy accepted")
	}
	manifest.SolverBuildPolicy = assemblySolverBuildPolicy
	manifest.Definitions[0].DefinitionVersion = 0
	if !errors.Is(validateAssemblySolveManifest(manifest), ErrValidation) {
		t.Fatal("old definition accepted")
	}
}
