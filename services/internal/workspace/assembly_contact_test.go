package workspace

import (
	"github.com/occccad/occccad/internal/geometry"
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
