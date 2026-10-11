package workspace

import (
	"github.com/occccad/occccad/internal/geometry"
	"math"
	"testing"
)

func TestMotionGeometricFrameAndCapturedDatum(t *testing.T) {
	axis := geometry.AssemblyGeometry{Kind: "AXIS", Origin: [3]float64{2, 3, 0}, Direction: [3]float64{0, 0, -1}}
	plane := geometry.AssemblyGeometry{Kind: "PLANE", Origin: [3]float64{0, 0, 7}, Direction: [3]float64{0, 0, 1}}
	frame, e := motionFrameFromSupports(axis, plane, [3]float64{1, 0, 0})
	if e != nil || !validMotionPose(frame) || frame.Translation != [3]float64{2, 3, 7} {
		t.Fatal(frame, e)
	}
	if conflictNorm(conflictSub(rotateByPose(frame, [3]float64{0, 0, 1}), axis.Direction)) > 1e-12 {
		t.Fatal("axis sign lost")
	}
	bad := plane
	bad.Direction = [3]float64{1, 0, 0}
	if _, e = motionFrameFromSupports(axis, bad, [3]float64{1, 0, 0}); e == nil {
		t.Fatal("oblique locating plane accepted")
	}
	if _, e = motionFrameFromSupports(axis, plane, [3]float64{0, 0, 1}); e == nil {
		t.Fatal("planes invented angular zero")
	}
	ref := AssemblyGeometryRef{InstanceID: "unit", Kind: "AXIS", GeometryID: "stable-axis"}
	baseline := assemblyReferenceKey(ref)
	x := [3]float64{1, 0, 0}
	ref.CapturedDirection = &x
	if assemblyReferenceKey(ref) == baseline {
		t.Fatal("cache conflated supporting axis and captured transverse datum")
	}
	v, e := capturedAssemblyDirection(axis, ref)
	if e != nil || v.Direction != x {
		t.Fatal(v, e)
	}
	x = [3]float64{math.NaN(), 0, 0}
	if _, e = capturedAssemblyDirection(axis, ref); e == nil {
		t.Fatal("invalid captured datum admitted")
	}
}
func TestMotionAssociationSemanticBaseline(t *testing.T) {
	c := AssemblyConstraint{ID: "constraint", Kind: "COINCIDENT", Family: "Coincidence", Subtype: "axis-axis", Mode: "DRIVING", DefinitionVersion: 2, First: AssemblyGeometryRef{InstanceID: "unit", Kind: "AXIS", GeometryID: "datum"}, DirectionRelation: "SAME"}
	baseline := semanticConstraint(c)
	c.EvaluationStatus = "VERIFIED"
	c.EvaluationSummary = "solve evidence"
	c.DistanceRelation = "UNSIGNED"
	c.Name = "new display name"
	if semanticConstraint(c) != baseline {
		t.Fatal("derived solve data caused a two-sided conflict")
	}
	c.DirectionRelation = "OPPOSITE"
	if semanticConstraint(c) == baseline {
		t.Fatal("real formal definition change ignored")
	}
	j := MechanismJoint{ID: "numeric", First: jointEnd("unit", 0, 0)}
	d := jointDefinitionDigest(j)
	j.First.Frame.Translation[0] = 1
	if jointDefinitionDigest(j) == d {
		t.Fatal("explicit local frame definition ignored")
	}
	j.First.Axis = &AssemblyGeometryRef{InstanceID: "unit", Kind: "AXIS", GeometryID: "datum"}
	d = jointDefinitionDigest(j)
	j.First.Frame.Translation[0] = 2
	if jointDefinitionDigest(j) != d {
		t.Fatal("derived frame became a second truth")
	}
}

func TestMotionApplicationProjectionReferencesUnique(t *testing.T) {
	k := KinematicsDefinitions{Mechanisms: []Mechanism{{ID: "m", Name: "mechanism", Joints: []MechanismJoint{{ID: "j", Name: "rotation", Kind: "REVOLUTE"}}}}, Studies: []MotionStudy{{ID: "s", Name: "simulation", MechanismID: "m"}}, Associations: []MotionAssemblyMapping{{MechanismID: "m", JointID: "j", Role: "axis", ConstraintID: "formal-axis"}}}
	tree := kinematicsStructure("p", "r", "p", k, []AssemblyConstraint{{ID: "formal-axis", Name: "axis"}})
	seen := map[string]bool{}
	var walk func(DocumentStructureNode)
	walk = func(n DocumentStructureNode) {
		if seen[n.ID] {
			t.Fatal("duplicate", n.ID)
		}
		seen[n.ID] = true
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(tree)
	if len(tree.Children) != 2 || tree.Children[1].Kind != "MOTION_STUDY" || tree.Children[1].Capabilities[0] != "EDIT" {
		t.Fatal("simulation is not an independently editable application object", tree)
	}
	for _, n := range tree.Children[0].Children {
		if n.Kind == "MOTION_STUDY" {
			t.Fatal("simulation is incorrectly owned by a mechanism")
		}
	}
}

func TestMotionApplicationProjectionNamedSupports(t *testing.T) {
	first := JointEndpoint{InstanceID: "shaft", Axis: &AssemblyGeometryRef{InstanceID: "shaft", Kind: "AXIS"}, Plane: &AssemblyGeometryRef{InstanceID: "shaft", Kind: "PLANE"}}
	second := JointEndpoint{InstanceID: "rotor", Axis: &AssemblyGeometryRef{InstanceID: "rotor", Kind: "AXIS"}, Plane: &AssemblyGeometryRef{InstanceID: "rotor", Kind: "PLANE"}}
	k := KinematicsDefinitions{Mechanisms: []Mechanism{{ID: "m", Name: "机构", Joints: []MechanismJoint{{ID: "ground", Name: "固定件", Kind: "GROUND", First: JointEndpoint{InstanceID: "shaft"}}, {ID: "joint", Name: "旋转接合", Kind: "REVOLUTE", First: first, Second: &second, Constraints: []AssemblyConstraint{{ID: "joint/axis", EvaluationStatus: "VERIFIED"}, {ID: "joint/axial-location", EvaluationStatus: "VERIFIED"}}}}}}}
	tree := kinematicsStructure("product", "revision", "product", k, nil, []ProductInstance{{ID: "shaft", Name: "圆柱.1"}, {ID: "rotor", Name: "螺旋桨.1"}})
	joints := tree.Children[0].Children
	if joints[0].Name != "固定件 (圆柱.1)" || joints[1].Name != "旋转接合" {
		t.Fatalf("joint endpoint presentation missing: %+v", joints)
	}
	if len(joints[1].Children) != 2 || joints[1].Children[0].Name != "同心" || joints[1].Children[1].Name != "偏移" {
		t.Fatalf("joint relations: %+v", joints[1].Children)
	}
	count := 0
	for _, relation := range joints[1].Children {
		if relation.EvaluationStatus != "VERIFIED" {
			t.Fatal("persistent constraint status was lost", relation)
		}
		if relation.Kind != "JOINT_EXPANSION" || len(relation.Capabilities) != 1 || relation.Capabilities[0] != "EDIT" {
			t.Fatal("joint relation became an independent definition")
		}
		for _, n := range relation.Children {
			if n.Kind == "JOINT_SUPPORT" {
				count++
				if len(n.Capabilities) != 0 || n.PresentationRole != "INPUT_REFERENCE" {
					t.Fatal("derived support became editable")
				}
			}
		}
	}
	if count != 4 {
		t.Fatalf("expected four support references, got %d", count)
	}
	if k.Mechanisms[0].Joints[1].Name != "旋转接合" {
		t.Fatal("presentation changed persistent name")
	}
}
