package workspace

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/modelcore"
	"math"
	"reflect"
	"testing"
)

func TestMechanismInteractionTransportsQualifiedContinuousCoordinates(t *testing.T) {
	snapshot := motionFixture(t, false, "REVOLUTE")
	m := snapshot.Mechanism
	m.Poses = map[string]InstancePose{}
	original := ProductModel{}
	for _, body := range snapshot.Equations.Bodies {
		p := InstancePose(body.Pose)
		m.Poses[body.ID] = p
		original.Instances = append(original.Instances, ProductInstance{ID: body.ID, Translation: p.Translation, Rotation: p.Rotation})
	}
	m.Coordinates = map[string]float64{"drive": math.Pi / 3}
	original.Kinematics.Mechanisms = []Mechanism{m}
	solved, err := cloneAssemblyProduct(original)
	if err != nil {
		t.Fatal(err)
	}
	q := 2*math.Pi + 70*math.Pi/180
	solved.Instances[1].Rotation = [4]float64{0, 0, math.Sin(q / 2), math.Cos(q / 2)}
	checkpoint := map[string]float64{"drive": q}
	candidate, err := mechanismInteractionCandidate(original, solved, "m", "crank", checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	accepted := candidate.Kinematics.Mechanisms[0]
	if math.Abs(accepted.Coordinates["drive"]-q) > 1e-8 || !reflect.DeepEqual(candidate.Instances, original.Instances) || original.Kinematics.Mechanisms[0].Coordinates["drive"] != math.Pi/3 {
		t.Fatal("drag lost qualified winding or modified Product/base state", candidate)
	}
	witness := accepted
	witness.Coordinates = map[string]float64{"drive": q}
	if !mechanismAcceptanceEqual(accepted, witness) {
		t.Fatal("final acceptance rejected the qualified coordinate checkpoint")
	}
	checkpoint["drive"] = 0
	if math.Abs(accepted.Coordinates["drive"]-q) > 1e-8 {
		t.Fatal("candidate retained mutable session coordinates")
	}
	solved.Instances[1].Translation[2] = 1
	if _, err = mechanismInteractionCandidate(original, solved, "m", "crank", checkpoint); err == nil {
		t.Fatal("candidate accepted an unsatisfied joint closure")
	}
}

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
	next, err := mechanismInteractionCandidate(model, input, "m", "b", nil)
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
