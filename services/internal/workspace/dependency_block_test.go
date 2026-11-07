package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestDependencyBlockMeasurementNamesOwnersAndProtectsCandidate(t *testing.T) {
	output := "parameter:measurement:output:angleRad"
	driving := modelcore.ParameterDefinition{ParameterID: "angle", Source: modelcore.ValueSource{Expression: &modelcore.TypedExpression{Reads: []modelcore.DependencyKey{modelcore.DependencyKey("parameter:" + output)}}}}
	product := ProductModel{Instances: []ProductInstance{}, Measurements: []Measurement{{ID: "measurement", Name: "测量.1"}}, Parameters: []modelcore.ParameterDefinition{{ParameterID: output, Source: modelcore.ValueSource{Produced: &modelcore.ParameterProducer{Kind: "MEASUREMENT", EntityID: "measurement", OutputSlot: "angleRad"}}}}, Constraints: []AssemblyConstraint{{ID: "constraint", Name: "Angle.3", Kind: "ANGLE", QuantityParameter: &driving}}}
	raw, _ := json.Marshal(product)
	before := append([]byte(nil), raw...)
	_, changes, err := applyMeasurements(raw, json.RawMessage("[]"))
	var blocked *DependencyBlockedError
	if !errors.As(err, &blocked) || !errors.Is(err, ErrValidation) || blocked.Code() != "DEPENDENCY_IN_USE" || blocked.Retryable() {
		t.Fatalf("missing domain block: %v", err)
	}
	if len(changes.Changes) != 0 || !bytes.Equal(raw, before) {
		t.Fatal("blocked deletion changed the candidate")
	}
	if blocked.Block.Subject.Name != "测量.1" || blocked.Block.Total != 1 || blocked.Block.Consumers[0].Kind != "ASSEMBLY_CONSTRAINT" || blocked.Block.Consumers[0].ID != "constraint" || blocked.Block.Consumers[0].Name != "Angle.3" {
		t.Fatalf("opaque identities escaped instead of labels: %+v", blocked.Block)
	}
}
func TestDependencyBlockSharedParametersPublicationsAndLimits(t *testing.T) {
	d := &parameterDocument{Parameters: []modelcore.ParameterDefinition{{ParameterID: "consumer", Key: "width", Source: modelcore.ValueSource{Expression: &modelcore.TypedExpression{Reads: []modelcore.DependencyKey{"parameter:source"}}}}}, Publications: []Publication{{ID: "pub", Name: "安装角", Target: PublicationTarget{ParameterID: "source"}}}}
	uses := d.parameterConsumers(map[modelcore.DependencyKey]bool{"parameter:source": true})
	if len(uses) != 2 {
		t.Fatal(uses)
	}
	if dependencyBlocked(DependencyObject{Kind: "PARAMETER", ID: "source", Name: "angle"}, nil) != nil {
		t.Fatal("unreferenced parameter blocked")
	}
	var refs []DependencyObject
	for i := 0; i < 40; i++ {
		refs = append(refs, DependencyObject{Kind: "PARAMETER", ID: fmt.Sprint(i), Name: "参数"})
	}
	refs = append(refs, refs...)
	err := dependencyBlocked(DependencyObject{Kind: "MEASUREMENT", ID: "m", Name: "测量"}, refs).(*DependencyBlockedError)
	if err.Block.Total != 40 || len(err.Block.Consumers) != 32 {
		t.Fatal("duplicate or unbounded blocker list", err.Block)
	}
}

func TestDependencyBlockUserParameterAndLocalPublicationDeletion(t *testing.T) {
	parameter := modelcore.ParameterDefinition{ParameterID: "source", Key: "mount_angle", Lifecycle: "USER"}
	consumer := modelcore.ParameterDefinition{ParameterID: "consumer", Key: "target_angle", Source: modelcore.ValueSource{Expression: &modelcore.TypedExpression{Reads: []modelcore.DependencyKey{"parameter:source"}}}}
	product := ProductModel{Instances: []ProductInstance{}, Parameters: []modelcore.ParameterDefinition{parameter, consumer}}
	raw, _ := json.Marshal(product)
	next, changes, err := applyDeleteParameter(raw, json.RawMessage(`{"parameterId":"source"}`))
	var blocked *DependencyBlockedError
	if !errors.As(err, &blocked) || next != nil || len(changes.Changes) != 0 {
		t.Fatalf("parameter removal was not atomically blocked: %v", err)
	}
	if blocked.Block.Subject.Name != "mount_angle" || blocked.Block.Consumers[0] != (DependencyObject{Kind: "PARAMETER", ID: "consumer", Name: "target_angle"}) {
		t.Fatalf("parameter removal lost its named consumer: %+v", blocked.Block)
	}
	product.Publications = []ProductPublication{{ID: "port", Name: "安装面"}}
	product.Constraints = []AssemblyConstraint{{ID: "coincident", Name: "Coincident.1", First: AssemblyGeometryRef{PublicationRef: &PublicationRef{Scope: "LOCAL", PublicationID: "port"}}}}
	raw, _ = json.Marshal(product)
	next, changes, err = applyDeleteProductPublication(raw, json.RawMessage(`{"publicationId":"port"}`))
	if !errors.As(err, &blocked) || next != nil || len(changes.Changes) != 0 {
		t.Fatalf("publication removal was not atomically blocked: %v", err)
	}
	if blocked.Block.Subject.Name != "安装面" || blocked.Block.Consumers[0] != (DependencyObject{Kind: "ASSEMBLY_CONSTRAINT", ID: "coincident", Name: "Coincident.1"}) {
		t.Fatalf("publication removal lost its named consumer: %+v", blocked.Block)
	}
}
