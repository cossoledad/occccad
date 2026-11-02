package workspace

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/modelcore"
	"math"
	"testing"
)

func TestAssemblyQuantitiesShareDocumentParameterNamespaceAndHistory(t *testing.T) {
	q, _ := modelcore.NewQuantity(20, "mm")
	model := ProductModel{Parameters: []modelcore.ParameterDefinition{{ParameterID: "width", Key: "width", Label: "Width", Lifecycle: "USER", ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension, DisplayUnit: "mm", Source: modelcore.ValueSource{Literal: &q}}}, Constraints: []AssemblyConstraint{{ID: "distance", Kind: "DISTANCE", DistanceRelation: selectedPlaneNormalV1, First: AssemblyGeometryRef{InstanceID: "a", Kind: "PLANE"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "PLANE"}}}}
	expression := "width + 5 mm"
	if err := editAssemblyQuantity(&model, 0, &expression, "", 0); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[0].Value != 25 {
		t.Fatal(model.Constraints[0])
	}
	raw, _ := json.Marshal(model)
	d, err := readParameterDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Parameters) != 2 || d.Parameters[1].OwnerFeatureID != "distance" || d.Parameters[1].Lifecycle != "ASSEMBLY_REQUIRED" {
		t.Fatal(d.Parameters)
	}
	alias := d.Parameters[1].Key
	payload, _ := json.Marshal(editParameterPayload{ParameterID: "width", Key: "span", Source: modelcore.ValueSource{Literal: &q}})
	renamed, _, err := applyEditParameter(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	var current ProductModel
	_ = json.Unmarshal(renamed, &current)
	if len(current.Parameters) != 1 || current.Constraints[0].QuantityParameter.Source.Expression.SourceText != "(span + 5 mm)" {
		t.Fatal(string(renamed))
	}
	if len(productParameterDefinitions(current)) != 2 {
		t.Fatal("duplicate parameter storage")
	}
	// Editing through the generic parameter command changes the same owned slot.
	literal, _ := modelcore.NewQuantity(40, "mm")
	p, _ := json.Marshal(editParameterPayload{ParameterID: "offset:distance", Key: "clearance", Source: modelcore.ValueSource{Literal: &literal}})
	next, changes, err := applyEditParameter(renamed, p)
	if err != nil {
		t.Fatal(err)
	}
	current = ProductModel{}
	_ = json.Unmarshal(next, &current)
	if current.Constraints[0].Value != 40 || current.Constraints[0].QuantityParameter.Key != "clearance" {
		t.Fatal(string(next))
	}
	if productParameterChangesOnly(current, changes) {
		t.Fatal("driving parameter edits must recompute the assembly")
	}
	values, err := modelValues("PRODUCT", next, changes)
	if err != nil {
		t.Fatal(err)
	}
	undo, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PRODUCT", next, undo)
	if err != nil {
		t.Fatal(err)
	}
	current = ProductModel{}
	_ = json.Unmarshal(restored, &current)
	if current.Constraints[0].Value != 25 || current.Constraints[0].QuantityParameter.Key != alias {
		t.Fatal(string(restored))
	}
	current.Constraints[0].Mode = "MEASURED"
	v := 12.0
	current.Constraints[0].MeasuredValue = &v
	current.Constraints[0].EvaluationStatus = modelcore.AssemblyConstraintVerified
	projected := productParameterDefinitions(current)[1]
	if projected.Role != "MEASURED" || projected.EvaluatedValue == nil || math.Abs(projected.EvaluatedValue.SIValue-.012) > 1e-12 {
		t.Fatal(projected)
	}
	raw, _ = json.Marshal(current)
	if _, _, err = applyEditParameter(raw, p); err == nil {
		t.Fatal("measured quantity source must remain read-only")
	}
}

func TestAssemblyParameterNamespacePreservesProducedValuesAndBlocksStaleInputs(t *testing.T) {
	q, _ := modelcore.NewQuantity(10, "mm")
	model := ProductModel{Parameters: []modelcore.ParameterDefinition{{ParameterID: "measured", Key: "measured_area_width", ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension, DisplayUnit: "mm", Role: "OUTPUT", CurrencyStatus: "CURRENT", EvaluatedValue: &q, Source: modelcore.ValueSource{Produced: &modelcore.ParameterProducer{Kind: "MEASUREMENT", EntityID: "measurement", OutputSlot: "distance"}}}}, Constraints: []AssemblyConstraint{{ID: "offset", Kind: "DISTANCE", First: AssemblyGeometryRef{InstanceID: "a", Kind: "POINT"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "POINT"}}}}
	expression := "measured_area_width + 2 mm"
	if err := editAssemblyQuantity(&model, 0, &expression, "", 0); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[0].Value != 12 || model.Parameters[0].ParameterID != "measured" {
		t.Fatal(model)
	}
	model.Parameters[0].CurrencyStatus = "STALE"
	model.Parameters[0].EvaluatedValue = nil
	if err := resolveAssemblyQuantities(&model); err != nil {
		t.Fatal(err)
	}
	if model.Constraints[0].QuantityParameter.EvaluatedValue != nil || model.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated || model.Constraints[0].Value != 12 {
		t.Fatal(model.Constraints[0])
	}
}
