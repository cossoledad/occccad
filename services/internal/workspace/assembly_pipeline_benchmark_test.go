package workspace

import (
	"fmt"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
)

func assemblyBenchmarkInput() ([]geometry.AssemblyBody, []geometry.AssemblyGeometry, []geometry.AssemblyConstraint) {
	var bodies []geometry.AssemblyBody
	var values []geometry.AssemblyGeometry
	var constraints []geometry.AssemblyConstraint
	for i := 0; i < 32; i++ {
		id := fmt.Sprintf("body-%02d", i)
		g := fmt.Sprintf("%s/geometry-%02d", strings.Repeat("persistent-selection/evidence/", 256), i)
		bodies = append(bodies, geometry.AssemblyBody{ID: id, Pose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}, InitialGuess: &geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}})
		values = append(values, geometry.AssemblyGeometry{ID: g, BodyID: id, Kind: "POINT", LengthUnit: "mm"})
		if i > 0 {
			constraints = append(constraints, geometry.AssemblyConstraint{ID: fmt.Sprintf("constraint-%02d", i), Kind: "COINCIDENT", FirstBodyID: id, FirstGeometryID: g, SecondBodyID: bodies[i-1].ID, SecondGeometryID: values[i-1].ID})
		}
	}
	return bodies, values, constraints
}
func BenchmarkAssemblyFreeze(b *testing.B) {
	bodies, values, constraints := assemblyBenchmarkInput()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := newAssemblySolveManifest("product", "revision", "model", bodies, values, constraints, nil, nil, nil); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkAssemblyDigest(b *testing.B) {
	bodies, values, constraints := assemblyBenchmarkInput()
	m, err := newAssemblySolveManifest("product", "revision", "model", bodies, values, constraints, nil, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		assemblyManifestDigest(m)
	}
}
func BenchmarkAssemblyReferenceKey(b *testing.B) {
	path := InstancePath{RootDocumentID: "document-0"}
	ids, names := []string{}, []string{}
	for i := 0; i < 16; i++ {
		id, name := fmt.Sprintf("occurrence-%02d", i), fmt.Sprintf("Component %02d", i)
		if i == 15 {
			id = "body"
		}
		path.Segments = append(path.Segments, InstancePathSegment{OwnerDocumentID: fmt.Sprintf("document-%d", i), OwnerVersionID: fmt.Sprintf("revision-%d", i), InstanceID: id, InstanceName: name, ReferencedDocumentID: fmt.Sprintf("document-%d", i+1), ResolvedVersionID: fmt.Sprintf("revision-%d", i+1)})
		ids, names = append(ids, id), append(names, name)
	}
	path.Canonical, path.Display = strings.Join(ids, "/"), strings.Join(names, "/")
	ref := AssemblyGeometryRef{InstanceID: "body", Kind: "PLANE", GeometryID: "plane", SourceVersionID: "revision-16", InstancePath: &path}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		assemblyReferenceKey(ref)
	}
}
