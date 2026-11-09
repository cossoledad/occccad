//go:build integration

package control

import (
	"context"
	"github.com/google/uuid"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc"
	"testing"
)

func TestSurfaceShapeQualityThroughRouter(t *testing.T) {
	routed := 0
	f := newCompositionControlFixture(t, grpc.WithUnaryInterceptor(func(ctx context.Context, method string, request, reply any, conn *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
		if r, ok := request.(*workerv1.MeasureGeometryRequest); ok && r.Kind == "CURVATURE" {
			routed++
		}
		return invoke(ctx, method, request, reply, conn, options...)
	}))
	ctx := t.Context()
	v, err := f.service.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PART", Name: "Surface quality Router"})
	if err != nil {
		t.Fatal(err)
	}
	apply := func(feature workspace.Feature) {
		t.Helper()
		v, err = f.service.ApplyCommand(ctx, v.Document.ID, workspace.CommandRequest{ActorID: f.actor, RequestID: uuid.NewString(), VersionID: v.Document.VersionID, Type: "CREATE_GEOMETRY_FEATURE", Feature: &feature})
		if err != nil {
			t.Fatal(err)
		}
	}
	ids := []string{}
	for _, y := range []float64{0, 10} {
		apply(workspace.Feature{Type: "GEOMETRY", Geometry: &workspace.GeometricFeatureDefinition{Kind: "LINE", Curve: &workspace.GeometryCurveDefinition{Points: []workspace.GeometryControlPoint{{ID: uuid.NewString(), Position: [3]float64{0, y, 0}}, {ID: uuid.NewString(), Position: [3]float64{10, y, 0}}}}}})
		ids = append(ids, v.Part.Features[len(v.Part.Features)-1].ID)
	}
	inputs := []workspace.GeometricInput{{Kind: "RESULT", FeatureID: ids[0], ResultID: "result"}, {Kind: "RESULT", FeatureID: ids[1], ResultID: "result"}}
	plane := workspace.GeometricInput{Kind: "PLANE", PlaneID: "datum-xy"}
	apply(workspace.Feature{Type: "GEOMETRY", Geometry: &workspace.GeometricFeatureDefinition{Kind: "SURFACE_BLEND", Inputs: inputs, Surface: &workspace.GeometrySurfaceDefinition{Tolerance: 1e-6, Boundaries: []workspace.GeometryBoundaryControl{{ID: uuid.NewString(), Boundary: inputs[0], Support: &plane, Continuity: "G2", Tension: 1}}}}})
	feature := v.Part.Features[len(v.Part.Features)-1]
	head := v.Document.VersionID
	report, err := f.service.AnalyzeShapeQuality(ctx, v.Document.ID, workspace.ShapeQualityRequest{RevisionID: head, Input: workspace.GeometricInput{Kind: "RESULT", FeatureID: feature.ID, ResultID: "result"}, SampleCount: 64})
	if err != nil || !report.Result.Complete || len(report.Result.Samples) != 64 {
		t.Fatal(report, err)
	}
	if routed != 1 {
		t.Fatal("curvature did not traverse Router", routed)
	}
	after, err := f.service.GetDocument(ctx, v.Document.ID)
	if err != nil || after.Document.VersionID != head {
		t.Fatal("read only analysis mutated history", err)
	}
}
