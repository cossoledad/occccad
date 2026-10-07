package geometry

import (
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

func replayBenchmarkInput() ([]AssemblyBody, []AssemblyGeometry, []AssemblyConstraint) {
	var bodies []AssemblyBody
	var values []AssemblyGeometry
	var constraints []AssemblyConstraint
	for i := 0; i < 32; i++ {
		id := fmt.Sprintf("body-%02d", i)
		g := fmt.Sprintf("%s/geometry-%02d", strings.Repeat("persistent-selection/evidence/", 256), i)
		bodies = append(bodies, AssemblyBody{ID: id, Pose: AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}})
		values = append(values, AssemblyGeometry{ID: g, BodyID: id, Kind: "POINT", LengthUnit: "mm"})
		if i > 0 {
			constraints = append(constraints, AssemblyConstraint{ID: fmt.Sprintf("constraint-%02d", i), Kind: "COINCIDENT", FirstBodyID: id, FirstGeometryID: g, SecondBodyID: bodies[i-1].ID, SecondGeometryID: values[i-1].ID})
		}
	}
	return bodies, values, constraints
}
func BenchmarkAssemblyRPC(b *testing.B) {
	bodies, values, constraints := replayBenchmarkInput()
	r, err := CompileAssemblyRequest("fixture", bodies, values, constraints, AssemblySolveOptions{})
	if err != nil {
		b.Fatal(err)
	}
	defer b.ReportMetric(float64(proto.Size(r)), "rpc-bytes")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := CompileAssemblyRequest("fixture", bodies, values, constraints, AssemblySolveOptions{})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := proto.Marshal(r); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkAssemblyReplay(b *testing.B) {
	bodies, values, constraints := replayBenchmarkInput()
	r, err := CompileAssemblyRequest("fixture", bodies, values, constraints, AssemblySolveOptions{})
	if err != nil {
		b.Fatal(err)
	}
	raw, err := makeAssemblyReplay(r, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer b.ReportMetric(float64(len(raw)), "file-bytes")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := makeAssemblyReplay(r, nil, nil); err != nil {
			b.Fatal(err)
		}
	}
}
