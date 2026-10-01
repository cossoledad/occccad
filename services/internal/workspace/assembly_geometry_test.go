package workspace

import (
	"github.com/occccad/occccad/internal/geometry"
	"math"
	"testing"
)

func TestAssemblySupportConstraintEligibilityPreservesTrimmedDomain(t *testing.T) {
	start, trimmed, full := .25, 1.75, .25+2*math.Pi
	value := geometry.AssemblyGeometry{Kind: "CIRCLE", Origin: [3]float64{1, 2, 3}, Direction: [3]float64{0, 0, 1}, XDirection: [3]float64{1, 0, 0}, Radius: 6, ParameterStart: &start, ParameterEnd: &trimmed}
	if eligible, code, diagnostic := assemblySupportConstraintEligibility(value, ""); eligible || code != "ASSEMBLY_SUPPORT_REQUIRES_UNDERLYING_CIRCLE" || diagnostic == "" {
		t.Fatal("trimmed circle silently treated as full support")
	}
	for _, role := range []string{"underlying-circle", "circle-center", "circle-axis", "circle-plane"} {
		derived, err := assemblyDerivedGeometry(value, role)
		if err != nil {
			t.Fatal(err)
		}
		if eligible, code, diagnostic := assemblySupportConstraintEligibility(derived, role); !eligible || code != "" || diagnostic != "" {
			t.Fatal("explicit valid derived support blocked", role)
		}
	}
	value.ParameterEnd = &full
	if eligible, _, _ := assemblySupportConstraintEligibility(value, ""); !eligible {
		t.Fatal("full circle rejected")
	}
	// The exact same versioned solver profile cutoff as command compilation.
	boundary := full - defaultAssemblySolverProfile().AngleTolerance/2
	value.ParameterEnd = &boundary
	if eligible, _, _ := assemblySupportConstraintEligibility(value, ""); !eligible {
		t.Fatal("full-circle profile boundary drifted")
	}
	if unresolved := (AssemblySupportInspection{}); unresolved.ConstraintEligible {
		t.Fatal("unresolved inspection must fail closed")
	}
	if eligible, _, _ := assemblySupportConstraintEligibility(geometry.AssemblyGeometry{}, ""); eligible {
		t.Fatal("unknown descriptor cannot open a constraint command")
	}
}

func TestAssemblyExactGeometryPreservesUnitsRolesAndNestedPose(t *testing.T) {
	fixtures := []TopologyElementProperties{
		{GeometryType: "SPHERE", Properties: map[string]any{"center": [3]float64{11, 23, -7}, "radius": 12.5, "materialSide": int64(-1), "lengthUnit": "mm"}},
		{GeometryType: "CONE", Properties: map[string]any{"apex": [3]float64{11, 23, -7}, "axis": [3]float64{0, 0, 1}, "semiAngle": -math.Pi / 6, "coneLeaf": int64(-1), "materialSide": int64(1), "lengthUnit": "mm"}},
		{GeometryType: "CIRCLE", Properties: map[string]any{"center": [3]float64{11, 23, -7}, "normal": [3]float64{0, 0, 1}, "xDirection": [3]float64{1, 0, 0}, "radius": 12.5, "firstParameter": 0.25, "lastParameter": 1.75, "lengthUnit": "mm"}},
	}
	roles := []string{"sphere-center", "cone-axis", "circle-center"}
	pose := InstancePose{Translation: [3]float64{5, -4, 3}, Rotation: [4]float64{0, 0, math.Sqrt(.5), math.Sqrt(.5)}}
	for i, p := range fixtures {
		t.Run(p.GeometryType, func(t *testing.T) {
			base := geometry.AssemblyGeometry{ID: "stable-source:role", BodyID: "occurrence-in-owning-product"}
			v, err := assemblyGeometryFromProperties(base, p)
			if err != nil {
				t.Fatal(err)
			}
			got := assemblyGeometryInBody(v, pose)
			expected := [3]float64{-18, 7, -4}
			for j := range expected {
				if math.Abs(got.Origin[j]-expected[j]) > 1e-12 {
					t.Fatalf("transformed twice or wrong frame: %+v", got)
				}
			}
			if got.ID != base.ID || got.BodyID != base.BodyID || got.Radius != v.Radius || got.HalfAngle != v.HalfAngle || got.ConeLeaf != v.ConeLeaf || got.MaterialSide != v.MaterialSide || got.LengthUnit != "mm" {
				t.Fatalf("value/provenance transformed incorrectly: %+v", got)
			}
			derived, err := assemblyDerivedGeometry(v, roles[i])
			if err != nil {
				t.Fatal(err)
			}
			if derived.ID != base.ID || derived.BodyID != base.BodyID || derived.Origin != v.Origin {
				t.Fatal("derived support lost source or origin")
			}
			if p.GeometryType == "CIRCLE" {
				if *got.ParameterStart != .25 || *got.ParameterEnd != 1.75 {
					t.Fatal("trimmed domain lost")
				}
				if math.Abs(got.XDirection[1]-1) > 1e-12 {
					t.Fatal("circle orientation not transformed")
				}
			}
		})
	}
}
func TestAssemblyExactGeometryRejectsMissingOrInvalidAnalyticDescriptors(t *testing.T) {
	cases := []TopologyElementProperties{
		{GeometryType: "SPHERE", Properties: map[string]any{"center": [3]float64{}, "materialSide": int64(1)}},
		{GeometryType: "3", Properties: map[string]any{"origin": [3]float64{}}},
		{GeometryType: "BSPLINE_SURFACE", Properties: map[string]any{"origin": [3]float64{}}},
		{GeometryType: "CONE", Properties: map[string]any{"apex": [3]float64{}, "axis": [3]float64{0, 0, 1}, "semiAngle": math.Pi / 2, "coneLeaf": int64(1), "materialSide": int64(1)}},
		{GeometryType: "SPHERE", Properties: map[string]any{"center": [3]float64{}, "radius": 1., "materialSide": int64(1), "lengthUnit": "m"}},
		{GeometryType: "CIRCLE", Properties: map[string]any{"center": [3]float64{}, "normal": [3]float64{0, 0, 1}, "radius": 2.}},
	}
	for i, p := range cases {
		if _, err := assemblyGeometryFromProperties(geometry.AssemblyGeometry{}, p); err == nil {
			t.Fatalf("invalid descriptor %d silently accepted", i)
		}
	}
}
func TestAssemblyExactFrameAndExplicitSubelements(t *testing.T) {
	system := AxisSystem{ID: "stable-frame", Origin: [3]float64{4, 5, 6}, XDirection: [3]float64{0, 1, 0}, YDirection: [3]float64{-1, 0, 0}, ZDirection: [3]float64{0, 0, 1}}
	frame, err := assemblyDatumFrame(geometry.AssemblyGeometry{ID: system.ID, BodyID: "rigid-occurrence"}, system)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"frame-origin", "frame-axis-x", "frame-axis-y", "frame-axis-z", "frame-plane-xy", "frame-plane-yz", "frame-plane-zx"} {
		got, err := assemblyDerivedGeometry(frame, role)
		if err != nil {
			t.Fatal(err)
		}
		if got.Origin != system.Origin || got.ID != system.ID {
			t.Fatal("frame role changed stable identity")
		}
		if role == "frame-axis-x" && math.Abs(got.Direction[1]-1) > 1e-12 {
			t.Fatal("frame X axis is not its persistent orientation")
		}
	}
	pose := InstancePose{Translation: [3]float64{3, 2, 1}, Rotation: [4]float64{0, 0, math.Sqrt(.5), math.Sqrt(.5)}}
	world := assemblyGeometryInBody(frame, pose)
	x, err := assemblyDerivedGeometry(world, "frame-axis-x")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(x.Direction[0]+1) > 1e-12 {
		t.Fatal("Frame quaternion was not composed exactly once")
	}
	if _, err := assemblyDerivedGeometry(frame, "sphere-center"); err == nil {
		t.Fatal("implicit Frame/Surface cast accepted")
	}
	system.ZDirection = [3]float64{0, 0, -1}
	if _, err := assemblyDatumFrame(frame, system); err == nil {
		t.Fatal("reflected Frame accepted as rotation")
	}
}

func TestAssemblyExactPublicationPreservesAnalyticAndFrameContracts(t *testing.T) {
	start, end := 0.3, 1.7
	fixtures := []PublicationResolution{
		{Status: "CONNECTED", GeometryKind: "SPHERE", Origin: [3]float64{30, 20, 10}, Radius: 7, MaterialSide: -1, LengthUnit: "mm"},
		{Status: "CONNECTED", GeometryKind: "CONE", Origin: [3]float64{30, 20, 10}, ZDirection: [3]float64{0, 1, 0}, HalfAngle: math.Pi / 6, ConeLeaf: -1, MaterialSide: 1, LengthUnit: "mm"},
		{Status: "CONNECTED", GeometryKind: "CIRCLE", Origin: [3]float64{30, 20, 10}, ZDirection: [3]float64{0, 1, 0}, XDirection: [3]float64{1, 0, 0}, Radius: 7, ParameterStart: &start, ParameterEnd: &end, LengthUnit: "mm"},
	}
	for _, r := range fixtures {
		kind := "FACE"
		if r.GeometryKind == "CIRCLE" {
			kind = "EDGE"
		}
		v, err := assemblyGeometryFromPublication(geometry.AssemblyGeometry{Kind: kind, ID: "publication:occurrence:frozen-version"}, r)
		if err != nil {
			t.Fatal(err)
		}
		if v.Kind != r.GeometryKind || v.Origin != r.Origin || v.MaterialSide != r.MaterialSide || v.ConeLeaf != r.ConeLeaf || v.Radius != r.Radius {
			t.Fatalf("frozen Publication changed: %+v", v)
		}
		pose := InstancePose{Translation: [3]float64{5, -4, 3}, Rotation: [4]float64{0, 0, math.Sqrt(.5), math.Sqrt(.5)}}
		forwarded := publicationThroughRigidPose(Publication{Type: "SURFACE", Resolution: r}, pose)
		forwardedValue, err := assemblyGeometryFromPublication(geometry.AssemblyGeometry{Kind: kind}, forwarded.Resolution)
		if err != nil {
			t.Fatal(err)
		}
		expected := [3]float64{-15, 26, 13}
		for i := range expected {
			if math.Abs(forwardedValue.Origin[i]-expected[i]) > 1e-12 {
				t.Fatal("Product forwarded Publication transformed zero/two times")
			}
		}
		if forwardedValue.Radius != r.Radius || forwardedValue.ConeLeaf != r.ConeLeaf || forwardedValue.MaterialSide != r.MaterialSide {
			t.Fatal("forwarding changed radius or physical side/leaf")
		}
	}
	var reference AssemblyGeometryRef
	frame := Publication{Type: "FRAME", Resolution: PublicationResolution{Status: "CONNECTED", GeometryKind: "FRAME", Origin: [3]float64{1, 2, 3}, XDirection: [3]float64{1, 0, 0}, YDirection: [3]float64{0, 1, 0}, ZDirection: [3]float64{0, 0, 1}}}
	if err := applyPublicationDescriptor(&reference, frame); err != nil {
		t.Fatal(err)
	}
	if reference.Kind != "FRAME" {
		t.Fatal("FRAME Publication was silently lowered to POINT")
	}
	legacy := AssemblyGeometryRef{Kind: "POINT"}
	if err := applyPublicationDescriptor(&legacy, frame); err != nil || legacy.Kind != "POINT" {
		t.Fatal("old POINT projection of Frame was reinterpreted")
	}
	value, err := assemblyGeometryFromPublication(geometry.AssemblyGeometry{Kind: reference.Kind}, frame.Resolution)
	if err != nil || value.Kind != "FRAME" {
		t.Fatalf("full FRAME unavailable: %v", err)
	}
	missing := PublicationResolution{GeometryKind: "CIRCLE", Radius: 7, ZDirection: [3]float64{0, 0, 1}}
	if _, err := assemblyGeometryFromPublication(geometry.AssemblyGeometry{Kind: "EDGE"}, missing); err == nil {
		t.Fatal("old Circle without frozen orientation/domain was invented")
	}
}

func TestAssemblySupportResolverDistinguishesForwardedPublications(t *testing.T) {
	product := ProductModel{Instances: []ProductInstance{{ID: "rigid-product-occurrence", ReferencedDocumentID: "child-product", ReferencedVersionID: "accepted-child-revision"}}}
	resolver := newAssemblySupportResolver(t.Context(), &Service{}, &product)
	first := AssemblyGeometryRef{InstanceID: product.Instances[0].ID, Kind: "POINT", GeometryID: "axis-system-default", PublicationRef: &PublicationRef{PublicationID: "forwarding-a"}, PublicationResolution: &PublicationResolution{Status: "CONNECTED", Origin: [3]float64{1, 2, 3}}}
	second := first
	second.PublicationRef = &PublicationRef{PublicationID: "forwarding-b"}
	second.PublicationResolution = &PublicationResolution{Status: "CONNECTED", Origin: [3]float64{11, 12, 13}}
	a, err := resolver.resolve(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := resolver.resolve(second)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || a.Origin == b.Origin || b.Origin != second.PublicationResolution.Origin {
		t.Fatal("stable forwarded Publication IDs collided in descriptor cache")
	}
	if _, err := resolver.part(&ProductInstance{ReferencedDocumentID: "missing-accepted-source"}); err == nil {
		t.Fatal("source without accepted revision fell back to latest Head")
	}
	var service *Service
	if _, err := service.InspectAssemblySupports(t.Context(), "product", nil); err == nil {
		t.Fatal("empty inspection accepted")
	}
	if _, err := service.InspectAssemblySupports(t.Context(), "product", make([]AssemblyGeometryRef, 257)); err == nil {
		t.Fatal("unbounded inspection accepted")
	}
}
