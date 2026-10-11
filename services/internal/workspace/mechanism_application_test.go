package workspace

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestMechanismMemberOwnershipAndNominal(t *testing.T) {
	pose := func(x float64) InstancePose {
		return InstancePose{Translation: [3]float64{x, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}}
	}
	model := ProductModel{Instances: []ProductInstance{{ID: "member", Translation: pose(100).Translation, Rotation: pose(100).Rotation}, {ID: "other", Translation: pose(77).Translation, Rotation: pose(77).Rotation}}, Constraints: []AssemblyConstraint{{ID: "product-fix", Kind: "FIX"}}}
	m := Mechanism{ID: "m", Name: "m", UnitIDs: []string{"member"}, Poses: map[string]InstancePose{"member": pose(10)}}
	input, err := mechanismPoseModel(model, m)
	if err != nil {
		t.Fatal(err)
	}
	if input.Instances[0].Translation != pose(10).Translation || input.Instances[1].Translation != pose(77).Translation || len(input.Constraints) != 0 {
		t.Fatal("independent input did not merge current nonmember poses", input)
	}
	if len(model.Constraints) != 1 || model.Instances[0].Translation != pose(100).Translation {
		t.Fatal("compiler mutated Product truth")
	}
	got := mechanismMemberPoses(m, map[string]InstancePose{"member": pose(20), "other": pose(1)})
	if len(got) != 1 || got["member"] != pose(20) {
		t.Fatal("historical frame leaked nonmember ownership", got)
	}
	m.Poses = map[string]InstancePose{"wrong": pose(0)}
	if _, err = mechanismPoseModel(model, m); err == nil {
		t.Fatal("missing member pose admitted")
	}
}

func TestConstraintNamesSurviveDeletionAndHistory(t *testing.T) {
	model := ProductModel{Instances: []ProductInstance{{ID: "a", Name: "Part.1"}, {ID: "b", Name: "Part.2"}}}
	raw, _ := json.Marshal(model)
	create := func(id string) (ProductModel, modelcore.ChangeSet) {
		t.Helper()
		second := AssemblyGeometryRef{Kind: "BODY", InstanceID: "b"}
		payload, _ := json.Marshal(addAssemblyConstraintPayload{Constraint: AssemblyConstraint{ID: id, Kind: "COINCIDENT", First: AssemblyGeometryRef{Kind: "BODY", InstanceID: "a"}, Second: &second}})
		next, set, err := applyAddAssemblyConstraint(raw, payload)
		if err != nil {
			t.Fatal(err)
		}
		var m ProductModel
		json.Unmarshal(next, &m)
		raw = next
		return m, set
	}
	first, _ := create("opaque-first")
	if first.Constraints[0].Name != "重合.1" {
		t.Fatal(first.Constraints[0].Name)
	}
	first.Constraints = nil
	raw, _ = json.Marshal(first)
	second, set := create("opaque-second")
	if second.Constraints[0].Name != "重合.2" {
		t.Fatal("deleted identity was renumbered", second.Constraints)
	}
	values, err := modelValues("PRODUCT", raw, set)
	if err != nil {
		t.Fatal(err)
	}
	counter := modelcore.PropertyAddress{EntityID: "product", SlotID: "product.constraint-name-counters"}
	if string(values[counter]) != "{\"重合\":2}" {
		t.Fatal("naming counter missing from current history values", values)
	}
	before := map[modelcore.PropertyAddress]json.RawMessage{}
	for _, c := range set.Changes {
		before[c.Target] = c.Before
	}
	restored, err := applyModelValues("PRODUCT", raw, before)
	if err != nil {
		t.Fatal(err)
	}
	var m ProductModel
	json.Unmarshal(restored, &m)
	if len(m.Constraints) != 0 || m.ConstraintNameCounters["重合"] != 1 {
		t.Fatal("counter and relation do not compensate together", m)
	}
	label := constraintDisplayName(second.Constraints[0], func(id string) string {
		if id == "a" {
			return "Part.1"
		}
		return "Part.2"
	})
	if label != "重合.2 (Part.1,Part.2)" {
		t.Fatal(label)
	}
	custom := second.Constraints[0]
	custom.Name = "轴定位"
	if constraintDisplayName(custom, func(id string) string { return id }) != "轴定位 (a,b)" {
		t.Fatal("custom name discarded")
	}
}

func TestMotionLawReferencesAndStableJointDigest(t *testing.T) {
	a := MechanismJoint{ID: "j", Name: "旋转.1", Kind: "REVOLUTE", First: jointEnd("a", 0, 0), Second: jointEndPointer("b", 0, 0), Direction: 1, Zero: MotionQuantity{Unit: "deg"}}
	b := a
	b.Name = "renamed"
	if jointDefinitionDigest(a) != jointDefinitionDigest(b) {
		t.Fatal("display rename invalidated a frozen definition")
	}
	k := KinematicsDefinitions{Laws: []MotionLaw{{ID: "law", Name: "linear", MechanismID: "m", DriverID: "d", Start: MotionQuantity{Unit: "deg"}, End: MotionQuantity{Value: 90, Unit: "deg"}, DurationSeconds: 2}}, Studies: []MotionStudy{{ID: "s", MechanismID: "m", LawID: "law", Frames: 9, BudgetMS: 1000}}}
	if err := resolveMotionLaws(&k); err != nil {
		t.Fatal(err)
	}
	s := k.Studies[0]
	if s.DriverID != "d" || s.End.Value != 90 || s.DurationSeconds != 2 {
		t.Fatal("snapshot ignored the referenced law", s)
	}
	k.Studies[0].LawID = "deleted"
	if resolveMotionLaws(&k) == nil {
		t.Fatal("deleted law accepted")
	}
	if !reflect.DeepEqual(a.First, b.First) {
		t.Fatal("digest mutated definition")
	}
}

func jointEndPointer(id string, x, y float64) *JointEndpoint { v := jointEnd(id, x, y); return &v }
