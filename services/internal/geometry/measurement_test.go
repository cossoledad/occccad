package geometry

import (
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"math"
	"testing"
)

func TestMeasurementResponseRejectsInconsistentAuthority(t *testing.T) {
	valid := func() *workerv1.MeasureGeometryResponse {
		return &workerv1.MeasureGeometryResponse{Complete: true, KernelBuild: "kernel", Values: map[string]float64{"distanceMm": 5}, Anchors: map[string]*workerv1.Vec3{"first": {}, "second": {X: 3, Y: 4}}}
	}
	if _, e := validateMeasurementResult(valid(), "BETWEEN"); e != nil {
		t.Fatal(e)
	}
	for name, corrupt := range map[string]func(*workerv1.MeasureGeometryResponse){"missing witness": func(r *workerv1.MeasureGeometryResponse) { delete(r.Anchors, "second") }, "wrong witness": func(r *workerv1.MeasureGeometryResponse) { r.Values["distanceMm"] = 6 }, "nonfinite": func(r *workerv1.MeasureGeometryResponse) { r.Anchors["second"].Z = math.NaN() }, "partial failure": func(r *workerv1.MeasureGeometryResponse) { r.Complete = false; r.Diagnostic = "cancelled" }, "missing version": func(r *workerv1.MeasureGeometryResponse) { r.KernelBuild = "" }} {
		t.Run(name, func(t *testing.T) {
			r := valid()
			corrupt(r)
			if _, e := validateMeasurementResult(r, "BETWEEN"); e == nil {
				t.Fatal("accepted corrupt measurement")
			}
		})
	}
}
