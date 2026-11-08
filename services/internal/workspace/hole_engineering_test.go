package workspace

import (
	"math"
	"strings"
	"testing"
)

func TestEngineeringHoleDepthAndAnnotationBoundaries(t *testing.T) {
	f := Feature{Type: "HOLE", Diameter: 2, Depth: 5, Extent: "BLIND", DrillTipAngle: 120, DepthConvention: "TIP"}
	if e := validateHoleDimensions(f); e != nil {
		t.Fatal(e)
	}
	if math.Abs(holeCylinderDepth(f)-(5-1/math.Tan(math.Pi/3))) > 1e-12 {
		t.Fatal("tip depth convention")
	}
	f.Thread = &HoleThreadDefinition{Mode: "ANNOTATION", Designation: "M3x0.5", Hand: "RIGHT", MajorDiameter: 3, Pitch: .5, Depth: 4}
	if e := validateHoleDimensions(f); e != nil {
		t.Fatal(e)
	}
	f.Thread.Mode = "GEOMETRY"
	if e := validateHoleDimensions(f); e == nil || !strings.Contains(e.Error(), "MODELED_THREAD_UNSUPPORTED") {
		t.Fatal("annotation mistaken for geometry", e)
	}
	f.Thread.Mode = "ANNOTATION"
	f.Thread.Depth = 5
	if e := validateHoleDimensions(f); e == nil {
		t.Fatal("annotation extends into drill cone")
	}
	f.Thread = nil
	f.Depth = .1
	if e := validateHoleDimensions(f); e == nil {
		t.Fatal("tip larger than total depth accepted")
	}
	f.Depth = 5
	f.Extent = "THROUGH_ALL"
	if e := validateHoleDimensions(f); e == nil {
		t.Fatal("through hole has a finite cone")
	}
}
func TestEngineeringHolePositionSetHasFormalUniqueMembers(t *testing.T) {
	a := PatternPointReference{SketchID: "s", Reference: SketchGeometryRef{Target: "ENTITY", EntityID: "a", SubElement: "POINT"}}
	b := a
	b.Reference.EntityID = "b"
	f := Feature{HolePoints: []PatternPointReference{a, b}}
	if e := validateHolePositionSet(f); e != nil {
		t.Fatal(e)
	}
	f.HolePoints = append(f.HolePoints, a)
	if e := validateHolePositionSet(f); e == nil {
		t.Fatal("duplicate formal position accepted")
	}
	f.HolePoints = []PatternPointReference{b}
	f.HolePoint = &a
	if e := validateHolePositionSet(f); e == nil {
		t.Fatal("two sources of position truth")
	}
}
