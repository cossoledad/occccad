package geometry

import (
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/protobuf/proto"
	"math"
	"testing"
)

func TestCurvatureResponseTrustBoundary(t *testing.T) {
	valid := &workerv1.MeasureGeometryResponse{Complete: true, GeometryType: "SURFACE", KernelBuild: "test", Curvature: []*workerv1.CurvatureSample{{Position: &workerv1.Vec3{}, Normal: &workerv1.Vec3{Z: 1}, Minimum: -.2, Maximum: 0, Mean: -.1, Gaussian: 0, Surface: true}}}
	if _, err := validateCurvatureResult(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*workerv1.MeasureGeometryResponse){func(r *workerv1.MeasureGeometryResponse) { r.Curvature[0].Mean = math.NaN() }, func(r *workerv1.MeasureGeometryResponse) { r.Curvature[0].Gaussian = 1 }, func(r *workerv1.MeasureGeometryResponse) { r.Curvature[0].Normal.Z = 0 }, func(r *workerv1.MeasureGeometryResponse) { r.Complete = false; r.Diagnostic = "failed" }} {
		next := proto.Clone(valid).(*workerv1.MeasureGeometryResponse)
		mutate(next)
		if _, err := validateCurvatureResult(next); err == nil {
			t.Fatal("accepted malformed curvature response")
		}
	}
	if _, err := validateMeasurementResult(valid, "ITEM"); err == nil {
		t.Fatal("analysis field leaked into measurement contract")
	}
}
