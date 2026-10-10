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
	c := AssemblyConstraint{ID: "formal-axis", Kind: "COINCIDENT", DirectionRelation: "SAME"}
	baseline := semanticConstraint(c)
	c.DirectionRelation = "OPPOSITE"
	joint := func(id, kind string) MechanismJoint {
		return MechanismJoint{ID: id, Name: id, Kind: kind, Sources: []JointSource{{ConstraintID: c.ID, Baseline: baseline}, {ConstraintID: "deleted-source", Baseline: "old"}}}
	}
	k := KinematicsDefinitions{Mechanisms: []Mechanism{{ID: "m1", Name: "original", Joints: []MechanismJoint{joint("j1", "REVOLUTE")}}, {ID: "m2", Name: "imported", Joints: []MechanismJoint{joint("j2", "PRISMATIC")}}}, Associations: []MotionAssemblyMapping{{MechanismID: "m1", JointID: "j1", Role: "axis", ConstraintID: c.ID}}}
	tree := kinematicsStructure("product", "revision", "product", k, []AssemblyConstraint{c})
	seen := map[string]bool{}
	changed, deleted, references := 0, 0, 0
	var walk func(DocumentStructureNode)
	walk = func(n DocumentStructureNode) {
		if seen[n.ID] {
			t.Fatalf("duplicate tree node %s", n.ID)
		}
		seen[n.ID] = true
		if n.Kind == "JOINT_SOURCE" {
			if len(n.Capabilities) > 0 {
				t.Fatal("reference has mutation capability")
			}
			if n.EntityID == c.ID {
				references++
			}
		}
		if n.ResolutionStatus == "SOURCE_CHANGED" {
			changed++
		}
		if n.ResolutionStatus == "SOURCE_DELETED" {
			deleted++
		}
		if n.Kind == "JOINT_EXPANSION" && n.EntityID == "j2/轴向定位" {
			t.Fatal("prismatic projection incorrectly freezes axial coordinate")
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(tree)
	if references != 3 || changed != 2 || deleted != 2 {
		t.Fatalf("source projection %d %d %d", references, changed, deleted)
	}
}

func TestMotionApplicationProjectionNamedSupports(t *testing.T) {
	first := JointEndpoint{InstanceID: "shaft", Axis: &AssemblyGeometryRef{InstanceID: "shaft", Kind: "AXIS"}, Plane: &AssemblyGeometryRef{InstanceID: "shaft", Kind: "PLANE"}}
	second := JointEndpoint{InstanceID: "rotor", Axis: &AssemblyGeometryRef{InstanceID: "rotor", Kind: "AXIS"}, Plane: &AssemblyGeometryRef{InstanceID: "rotor", Kind: "PLANE"}}
	k := KinematicsDefinitions{Mechanisms: []Mechanism{{ID: "m", Name: "机构", Joints: []MechanismJoint{{ID: "ground", Name: "固定件", Kind: "GROUND", First: JointEndpoint{InstanceID: "shaft"}}, {ID: "joint", Name: "旋转接合", Kind: "REVOLUTE", First: first, Second: &second}}}}}
	tree := kinematicsStructure("product", "revision", "product", k, nil, []ProductInstance{{ID: "shaft", Name: "圆柱.1"}, {ID: "rotor", Name: "螺旋桨.1"}})
	joints := tree.Children[0].Children
	if joints[0].Name != "固定件 (圆柱.1)" || joints[1].Name != "旋转接合 (圆柱.1 ↔ 螺旋桨.1)" {
		t.Fatalf("joint endpoint presentation missing: %+v", joints)
	}
	count := 0
	for _, n := range joints[1].Children {
		if n.Kind == "JOINT_SUPPORT" {
			count++
			if len(n.Capabilities) != 0 || n.PresentationRole != "INPUT_REFERENCE" {
				t.Fatal("derived support became editable")
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
