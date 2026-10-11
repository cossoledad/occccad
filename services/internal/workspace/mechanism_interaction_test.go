package workspace

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/modelcore"
	"reflect"
	"testing"
)

func TestMechanismInteractionPreservesProductAndHistory(t *testing.T) {
	pose := func(x float64) InstancePose {
		return InstancePose{Translation: [3]float64{x, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}}
	}
	model := ProductModel{Instances: []ProductInstance{{ID: "a", Translation: pose(100).Translation, Rotation: pose(0).Rotation}, {ID: "b", Translation: pose(80).Translation, Rotation: pose(0).Rotation}}, Constraints: []AssemblyConstraint{{ID: "product-fix", Kind: "FIX", First: AssemblyGeometryRef{Kind: "BODY", InstanceID: "a"}}}, Kinematics: KinematicsDefinitions{Mechanisms: []Mechanism{{ID: "m", Name: "M", UnitIDs: []string{"a"}, Poses: map[string]InstancePose{"a": pose(10)}, Joints: []MechanismJoint{{ID: "g", Name: "Ground", Kind: "GROUND", Direction: 1, First: JointEndpoint{InstanceID: "a", Frame: pose(0)}, Zero: MotionQuantity{Unit: "deg"}, Constraints: []AssemblyConstraint{{ID: "mechanism-fix", Kind: "FIX"}}}}}}}}
	input, err := mechanismInteractionModel(model, "m", nil)
	if err != nil {
		t.Fatal(err)
	}
	if input.Instances[0].Translation != pose(10).Translation || len(input.Constraints) != 1 || input.Constraints[0].ID != "mechanism-fix" {
		t.Fatal("Product pose/equations leaked into mechanism drag", input)
	}
	input.Instances[1].Translation = pose(20).Translation
	next, err := mechanismInteractionCandidate(model, input, "m", "b")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(next.Instances, model.Instances) || !reflect.DeepEqual(next.Constraints, model.Constraints) {
		t.Fatal("mechanism drag modified Product truth")
	}
	m := next.Kinematics.Mechanisms[0]
	if len(m.UnitIDs) != 2 || m.Poses["b"] != pose(20) || len(model.Kinematics.Mechanisms[0].UnitIDs) != 1 {
		t.Fatal("unit ownership/copy failed", m)
	}
	raw, _ := json.Marshal(model)
	payload, _ := json.Marshal(next.Kinematics)
	persisted, changes, err := applyKinematicsDefinition(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	var accepted ProductModel
	json.Unmarshal(persisted, &accepted)
	if !kinematicsOnlyHistory(model, accepted, changes) {
		t.Fatal("move history escaped kinematics slot")
	}
	before := map[modelcore.PropertyAddress]json.RawMessage{}
	for _, c := range changes.Changes {
		before[c.Target] = c.Before
	}
	restored, err := applyModelValues("PRODUCT", persisted, before)
	if err != nil {
		t.Fatal(err)
	}
	var original ProductModel
	json.Unmarshal(restored, &original)
	if !reflect.DeepEqual(original.Kinematics, model.Kinematics) {
		t.Fatal("undo did not restore mechanism ownership and poses")
	}
	if _, err = mechanismInteractionModel(model, "deleted", nil); err == nil {
		t.Fatal("missing mechanism accepted")
	}
}
