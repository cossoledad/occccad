package workspace

import (
	"math"
	"testing"
)

func TestAssemblySnapHintsUseExactBoundaryAndTransformOnce(t *testing.T) {
	pose := InstancePose{Translation: [3]float64{10, 20, 30}, Rotation: [4]float64{0, 0, math.Sqrt(.5), math.Sqrt(.5)}}
	hints := assemblySnapHints(TopologyElementProperties{GeometryType: "LINE", Properties: map[string]any{"origin": [3]float64{1, 2, 3}, "direction": [3]float64{1, 0, 0}, "firstParameter": 2.0, "lastParameter": 6.0}}, pose)
	for _, v := range []*[3]float64{hints.EndFirst, hints.Center, hints.EndLast} {
		if v == nil {
			t.Fatal("missing exact line bounds")
		}
	}
	want := [][3]float64{{8, 23, 33}, {8, 25, 33}, {8, 27, 33}}
	for i, v := range []*[3]float64{hints.EndFirst, hints.Center, hints.EndLast} {
		for k := range v {
			if math.Abs(v[k]-want[i][k]) > 1e-10 {
				t.Fatal("double/missing transform", i, v)
			}
		}
	}
	plane := assemblySnapHints(TopologyElementProperties{GeometryType: "PLANE", Properties: map[string]any{"snapCenter": [3]float64{1, 2, 3}, "snapBoundaryDirection": [3]float64{1, 0, 0}}}, pose)
	if plane.BoundaryDirection == nil || math.Abs((*plane.BoundaryDirection)[1]-1) > 1e-10 || math.Abs((*plane.Center)[0]-8) > 1e-10 {
		t.Fatal(plane)
	}
	fallback := assemblySnapHints(TopologyElementProperties{GeometryType: "PLANE", Properties: map[string]any{}}, pose)
	if fallback.BoundaryDirection != nil || fallback.EndFirst != nil {
		t.Fatal("do not invent precision", fallback)
	}
}
