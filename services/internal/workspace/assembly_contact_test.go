package workspace

import (
	"github.com/occccad/occccad/internal/geometry"
	"math"
	"testing"
)

func TestAssemblyContactStaticInfeasibilityIsNotNumericalFailureOrSuppression(t *testing.T) {
	c := geometry.AssemblyConstraint{Kind: "CONTACT", ContactKind: "FACE", ContactSide: "INTERNAL", ContactBranch: 1}
	a := geometry.AssemblyGeometry{Kind: "SPHERE", Radius: 2, MaterialSide: 1}
	b := a
	b.Radius = 3
	if got := contactDefinitionInfeasibility(c, a, b); got != "CONTACT_FACE_RADIUS_MISMATCH" {
		t.Fatal(got)
	}
	b.Radius = 2
	if got := contactDefinitionInfeasibility(c, a, b); got != "" {
		t.Fatal(got)
	}
	c.ContactSide = "EXTERNAL"
	if got := contactDefinitionInfeasibility(c, a, b); got != "CONTACT_MATERIAL_SIDE_MISMATCH" {
		t.Fatal(got)
	}
	b.MaterialSide = -1
	if got := contactDefinitionInfeasibility(c, a, b); got != "" {
		t.Fatal(got)
	}
	c.ContactKind = "RING"
	b.Kind = "CIRCLE"
	b.Radius = 3
	if got := contactDefinitionInfeasibility(c, a, b); got != "CONTACT_CIRCLE_EXCEEDS_SPHERE" {
		t.Fatal(got)
	}
	c.ContactKind = "POINT"
	a.Kind = "PLANE"
	b.Kind = "SPHERE"
	b.MaterialSide = 1
	if got := contactDefinitionInfeasibility(c, a, b); got != "" {
		t.Fatal(got)
	}
	c.ContactBranch = -1
	if got := contactDefinitionInfeasibility(c, a, b); got != "CONTACT_PLANE_MATERIAL_BRANCH_MISMATCH" {
		t.Fatal(got)
	}
}

func TestCylinderContactIndependentWitness(t *testing.T) {
	for _, tc := range []struct {
		name, kind, side        string
		material                int32
		radius, distance, axial float64
		rotation                [4]float64
		known, satisfied        bool
	}{
		{"internal line", "LINE", "INTERNAL", 1, 4, 8, 0, [4]float64{0, 0, 0, 1}, true, true},
		{"external line", "LINE", "EXTERNAL", 1, 4, 16, 0, [4]float64{0, 0, 0, 1}, true, true},
		{"cavity reverses internal branch", "LINE", "INTERNAL", -1, 4, 16, 0, [4]float64{0, 0, 0, 1}, true, true},
		{"cavity reverses external branch", "LINE", "EXTERNAL", -1, 4, 8, 0, [4]float64{0, 0, 0, 1}, true, true},
		{"axis parameterization and axial slide", "LINE", "INTERNAL", 1, 4, 8, 25, [4]float64{1, 0, 0, 0}, true, true},
		{"wrong radial distance", "LINE", "INTERNAL", 1, 4, 9, 0, [4]float64{0, 0, 0, 1}, true, false},
		{"wrong material branch", "LINE", "INTERNAL", -1, 4, 8, 0, [4]float64{0, 0, 0, 1}, true, false},
		{"tilted axes", "LINE", "INTERNAL", 1, 4, 8, 0, [4]float64{0.6, 0, 0, 0.8}, true, false},
		{"coincident faces", "FACE", "INTERNAL", 1, 12, 0, 5, [4]float64{0, 0, 0, 1}, true, true},
		{"external face material", "FACE", "EXTERNAL", -1, 12, 0, 0, [4]float64{0, 0, 0, 1}, true, true},
		{"face radius mismatch", "FACE", "INTERNAL", 1, 4, 0, 0, [4]float64{0, 0, 0, 1}, true, false},
		{"face material mismatch", "FACE", "EXTERNAL", 1, 12, 0, 0, [4]float64{0, 0, 0, 1}, true, false},
		{"line degenerates to face", "LINE", "INTERNAL", 1, 12, 0, 0, [4]float64{0, 0, 0, 1}, true, false},
		{"unsupported contact stays unknown", "POINT", "INTERNAL", 1, 4, 8, 0, [4]float64{0, 0, 0, 1}, false, false},
		{"invalid radius stays unknown", "LINE", "INTERNAL", 1, 0, 12, 0, [4]float64{0, 0, 0, 1}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := conflictFixture(t, 0)
			m.Geometry[0].Kind, m.Geometry[1].Kind = "CYLINDER", "CYLINDER"
			m.Geometry[0].Direction, m.Geometry[1].Direction = [3]float64{0, 0, 1}, [3]float64{0, 0, 1}
			m.Geometry[0].Radius, m.Geometry[1].Radius = 12, tc.radius
			m.Geometry[0].MaterialSide, m.Geometry[1].MaterialSide = 1, tc.material
			c := m.Constraints[0]
			c.Kind, c.ContactKind, c.ContactSide = "CONTACT", tc.kind, tc.side
			result := geometry.AssemblySolve{Bodies: append([]geometry.AssemblyBody(nil), m.Bodies...)}
			result.Bodies[1].Pose = geometry.AssemblyPose{Translation: [3]float64{tc.distance, 0, tc.axial}, Rotation: tc.rotation}
			poses := map[string]geometry.AssemblyPose{"a": result.Bodies[0].Pose, "b": result.Bodies[1].Pose}
			geo := map[string]geometry.AssemblyGeometry{m.Geometry[0].ID: m.Geometry[0], m.Geometry[1].ID: m.Geometry[1]}
			known, satisfied, why := conflictGeometryRelation(c, geo, poses, m.SolverProfile.LengthTolerance, m.SolverProfile.AngleTolerance)
			if known != tc.known || satisfied != tc.satisfied {
				t.Fatal(known, satisfied, why)
			}
			m.Constraints = []geometry.AssemblyConstraint{c}
			if ok, why := conflictIndependentWitness(m, result); ok != tc.satisfied {
				t.Fatal("final-pose witness", ok, why)
			}
		})
	}
}

func TestConflictQuaternionDistanceNearIdentity(t *testing.T) {
	// The real cylinder-contact solve returned this same quaternion twice.
	// Its norm squared is 1 - 1 ulp; acos(dot(q,q)) incorrectly reports 2.98e-8.
	q := [4]float64{-0.46642289011201976, 0.2798537340672115, 3.5358433335162044e-17, 0.839125482337528}
	negative := [4]float64{-q[0], -q[1], -q[2], -q[3]}
	for _, other := range [][4]float64{q, negative} {
		if angle := conflictQuaternionAngle(q, other); angle != 0 {
			t.Fatal("identical rotations rejected", angle)
		}
		if !conflictPoseClose(geometry.AssemblyPose{Rotation: q}, geometry.AssemblyPose{Rotation: other}, 1e-7, 1e-8) {
			t.Fatal("same selected frame rejected")
		}
	}
	for _, angle := range []float64{1e-9, 1e-7, math.Pi / 2, math.Pi} {
		other := [4]float64{math.Sin(angle / 2), 0, 0, math.Cos(angle / 2)}
		got := conflictQuaternionAngle([4]float64{0, 0, 0, 1}, other)
		if math.Abs(got-angle) > 1e-15 {
			t.Fatal("rotation distance", angle, got)
		}
		if conflictPoseClose(geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}, geometry.AssemblyPose{Rotation: other}, 1e-7, 1e-8) != (angle <= 1e-8) {
			t.Fatal("angular acceptance tolerance changed", angle)
		}
	}
}
