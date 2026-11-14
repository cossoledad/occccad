package workspace

import (
	"context"
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
	"testing"
)

func performanceSketch(circleCount int) PartModel {
	model := newPartModel()
	sketch := Feature{ID: "perf-sketch", Type: "SKETCH", Plane: "XY", Sketch: &SketchFeature{
		SchemaVersion: SketchSchemaVersion, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"},
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

func executionPlanWorkload(count int) (PartModel, *modelcore.EvaluationManifest) {
	m := newPartModel()
	m.Bodies[0].GeometryKey = "accepted-body"
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("sketch-%d", i)
		m.Features = append(m.Features, Feature{ID: id, BodyID: m.ActiveBodyID, Type: "SKETCH", Plane: "XY", Sketch: &SketchFeature{SchemaVersion: SketchSchemaVersion,
			Support:  SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY", Status: "CONNECTED", OrientationRule: sketchSupportOrientationRule},
			Entities: []SketchEntity{{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 10, Y: 10}}}}})
	}
	normalizePartModel(&m)
	_, prior, err := buildPartEvaluation(m, "revision", "model", nil, nil)
	if err != nil {
		panic(err)
	}
	return m, &prior
}

func TestPartExecutionPlanLimitsUnconsumedSketchAndRejectsColdReuse(t *testing.T) {
	m, prior := executionPlanWorkload(1000)
	next := clonePartModel(m)
	next.Features[999].Sketch.Entities[0].End.X = 11
	ctx, err := preparePartExecution(context.Background(), m, next, prior, []modelcore.DependencyKey{"feature:sketch-999"})
	if err != nil {
		t.Fatal(err)
	}
	if needsPartBody(ctx, m.ActiveBodyID) || !needsPartNode(ctx, "feature:sketch-999") || needsPartNode(ctx, "feature:sketch-0") {
		t.Fatal("incorrect execution slice")
	}
	if !reusableSketchInputs(ctx, next.Features[999]) {
		t.Fatal("unchanged datum support not reused")
	}
	cold, err := preparePartExecution(WithColdEvaluation(context.Background()), m, next, prior, nil)
	if err != nil || executionPlan(cold) != nil {
		t.Fatal("cold evaluation borrowed incremental plan", err)
	}
}

func BenchmarkPartExecutionPlan(b *testing.B) {
	for _, n := range []int{20, 100, 300, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			m, prior := executionPlanWorkload(n)
			next := clonePartModel(m)
			next.Features[n-1].Sketch.Entities[0].End.X = 11
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := preparePartExecution(context.Background(), m, next, prior, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestPartExecutionPlanReevaluatesRemovedTerminalSolid(t *testing.T) {
	m, _ := executionPlanWorkload(2)
	m.Features = append(m.Features, Feature{ID: "tip", BodyID: m.ActiveBodyID, Type: "LINEAR_EXTRUDE", Profile: "sketch-1", Operation: "ADD", Length: 10})
	normalizePartModel(&m)
	_, prior, err := buildPartEvaluation(m, "previous", "hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	next := clonePartModel(m)
	next.Features = next.Features[:len(next.Features)-1]
	var parameters []modelcore.ParameterDefinition
	for _, p := range next.Parameters {
		if p.OwnerFeatureID != "tip" {
			parameters = append(parameters, p)
		}
	}
	next.Parameters = parameters
	ctx, err := preparePartExecution(context.Background(), m, next, &prior, []modelcore.DependencyKey{"feature:tip"})
	if err != nil || !needsPartBody(ctx, m.ActiveBodyID) {
		t.Fatal("removed final feature left stale solid", err)
	}
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
