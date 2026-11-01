package workspace

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestProductParametersLifecycleExpressionHistoryAndIsolation(t *testing.T) {
	model := ProductModel{Instances: []ProductInstance{{ID: "instance-a", Name: "Part.1", ReferencedDocumentID: "part", ReferencedVersionID: "part-revision"}}}
	raw, _ := json.Marshal(model)
	q, _ := modelcore.NewQuantity(12, "mm")
	parameter := modelcore.ParameterDefinition{ParameterID: "base", Key: "base", Label: "Base", Lifecycle: "USER", ValueType: modelcore.ValueQuantity, Dimension: q.Dimension, DisplayUnit: "mm", Role: "DESIGN_INPUT", Source: modelcore.ValueSource{Literal: &q}}
	payload, _ := json.Marshal(createUserParameterPayload{Parameter: parameter})
	next, changes, err := applyCreateUserParameter(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	before, err := productHistoryValues(raw, changes)
	if err != nil {
		t.Fatal(err)
	}
	after, err := productHistoryValues(next, changes)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyProductHistoryValues(next, before)
	if err != nil {
		t.Fatal(err)
	}
	redone, err := applyProductHistoryValues(restored, after)
	if err != nil {
		t.Fatal(err)
	}
	var reopened ProductModel
	if err := json.Unmarshal(redone, &reopened); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened.Instances, model.Instances) || len(reopened.Parameters) != 1 || reopened.Parameters[0].EvaluatedValue.SIValue != q.SIValue {
		t.Fatal("parameters lost Product data or history", string(redone))
	}
	parameter.ParameterID, parameter.Key = "derived", "derived"
	payload, _ = json.Marshal(createUserParameterPayload{Parameter: parameter})
	next, _, err = applyCreateUserParameter(redone, payload)
	if err != nil {
		t.Fatal(err)
	}
	expression, err := modelcore.CompileExpression("base * 2", map[string]modelcore.ParameterBinding{"base": {ParameterID: "base", Dimension: q.Dimension}}, q.Dimension, "mm")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ = json.Marshal(parameterSourcePayload{ParameterID: "derived", Source: modelcore.ValueSource{Expression: &expression}})
	next, _, err = applyParameterSource(next, payload)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, manifest, err := prepareProductDocument("revision", next)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.NodeResults) < 3 {
		t.Fatal("Product parameter dependencies missing", manifest)
	}
	_ = json.Unmarshal(next, &reopened)
	if math.Abs(reopened.Parameters[1].EvaluatedValue.SIValue-.024) > 1e-12 {
		t.Fatal("expression not evaluated", reopened.Parameters)
	}
	payload, _ = json.Marshal(deleteParameterPayload{ParameterID: "base"})
	if _, _, err := applyDeleteParameter(next, payload); err == nil {
		t.Fatal("deleted parameter used by formula")
	}
}

func TestProductParameterCycleRejected(t *testing.T) {
	q, _ := modelcore.NewQuantity(1, "mm")
	parameters := []modelcore.ParameterDefinition{}
	for _, id := range []string{"a", "b"} {
		parameters = append(parameters, modelcore.ParameterDefinition{ParameterID: id, Key: id, ValueType: modelcore.ValueQuantity, Dimension: q.Dimension, Source: modelcore.ValueSource{Literal: &q}})
	}
	bindings := map[string]modelcore.ParameterBinding{"a": {ParameterID: "a", Dimension: q.Dimension}, "b": {ParameterID: "b", Dimension: q.Dimension}}
	for i, source := range []string{"b", "a"} {
		expression, err := modelcore.CompileExpression(source, bindings, q.Dimension, "mm")
		if err != nil {
			t.Fatal(err)
		}
		parameters[i].Source = modelcore.ValueSource{Expression: &expression}
	}
	if _, err := resolveParameterDefinitions(parameters); err == nil {
		t.Fatal("cycle accepted")
	}
}
