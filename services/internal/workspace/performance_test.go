package workspace

import (
	"fmt"
	"testing"
)

func performanceSketch(circleCount int) PartModel {
	model := newPartModel()
	sketch := Feature{ID: "perf-sketch", Type: "SKETCH", Plane: "XY", Sketch: &SketchFeature{
		SchemaVersion: 1, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"},
		Entities: []SketchEntity{}, Constraints: []SketchConstraint{}, Solve: SketchSolveState{Status: "UNDER_CONSTRAINED"},
	}}
	for index := 0; index < circleCount; index++ {
		x, y := float64(index%20)*20, float64(index/20)*20
		sketch.Sketch.Entities = append(sketch.Sketch.Entities, SketchEntity{ID: fmt.Sprintf("circle-%d", index),
			Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{X: x, Y: y}, Radius: 5})
	}
	model.Features = append(model.Features, sketch)
	return model
}

func BenchmarkProfileBuilder200Circles(benchmark *testing.B) {
	model := performanceSketch(200)
	sketch := model.Features[0]
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for index := 0; index < benchmark.N; index++ {
		if _, err := buildProfileRegions(sketch); err != nil {
			benchmark.Fatal(err)
		}
	}
}

func BenchmarkVisualizationManifest200Circles(benchmark *testing.B) {
	model := performanceSketch(200)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for index := 0; index < benchmark.N; index++ {
		_ = visualizationManifest(model)
	}
}
