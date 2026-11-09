package geometry

import (
	"context"
	"fmt"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"math"
)

type CurvatureSample struct {
	Position [3]float64 `json:"position"`
	Normal   [3]float64 `json:"normal"`
	Minimum  float64    `json:"minimum"`
	Maximum  float64    `json:"maximum"`
	Mean     float64    `json:"mean"`
	Gaussian float64    `json:"gaussian"`
	Surface  bool       `json:"surface"`
}
type CurvatureResult struct {
	Complete         bool              `json:"complete"`
	Diagnostic       string            `json:"diagnostic,omitempty"`
	GeometryType     string            `json:"geometryType"`
	Samples          []CurvatureSample `json:"samples"`
	UndefinedSamples uint32            `json:"undefinedSamples"`
	KernelBuild      string            `json:"kernelBuild"`
}

func (c *Client) AnalyzeCurvature(ctx context.Context, id string, inputs []AnalysisGeometry, operands [][]MeasurementElement, count uint32) (CurvatureResult, error) {
	request := measurementRequest(id, "CURVATURE", inputs, operands)
	request.SampleCount = count
	reply, err := c.worker.MeasureGeometry(ctx, request)
	if err != nil {
		return CurvatureResult{}, fmt.Errorf("curvature: %w", err)
	}
	return validateCurvatureResult(reply)
}
func validateCurvatureResult(reply *workerv1.MeasureGeometryResponse) (CurvatureResult, error) {
	bad := func() (CurvatureResult, error) { return CurvatureResult{}, fmt.Errorf("invalid curvature response") }
	if reply == nil || reply.KernelBuild == "" || len(reply.Curvature) > 8192 || len(reply.Values) > 0 || len(reply.Anchors) > 0 || reply.UndefinedSamples > 65536 {
		return bad()
	}
	out := CurvatureResult{Complete: reply.Complete, Diagnostic: reply.Diagnostic, GeometryType: reply.GeometryType, UndefinedSamples: reply.UndefinedSamples, KernelBuild: reply.KernelBuild, Samples: []CurvatureSample{}}
	if !reply.Complete {
		if reply.Diagnostic == "" || len(reply.Curvature) != 0 {
			return bad()
		}
		return out, nil
	}
	if reply.Diagnostic != "" || len(reply.Curvature) == 0 {
		return bad()
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	for _, p := range reply.Curvature {
		if p == nil || p.Position == nil || p.Normal == nil {
			return bad()
		}
		for _, v := range []float64{p.Position.X, p.Position.Y, p.Position.Z, p.Normal.X, p.Normal.Y, p.Normal.Z, p.Minimum, p.Maximum, p.Mean, p.Gaussian} {
			if !finite(v) {
				return bad()
			}
		}
		if p.Minimum > p.Maximum || math.Abs(p.Mean-(p.Minimum+p.Maximum)/2) > 1e-9*math.Max(1, math.Abs(p.Mean)) || p.Surface && math.Abs(p.Gaussian-p.Minimum*p.Maximum) > 1e-9*math.Max(1, math.Abs(p.Gaussian)) || !p.Surface && (p.Minimum < 0 || p.Minimum != p.Maximum || p.Gaussian != 0) {
			return bad()
		}
		norm := math.Hypot(math.Hypot(p.Normal.X, p.Normal.Y), p.Normal.Z)
		if (p.Surface || p.Maximum > 1e-10) && math.Abs(norm-1) > 1e-7 {
			return bad()
		}
		out.Samples = append(out.Samples, CurvatureSample{Position: [3]float64{p.Position.X, p.Position.Y, p.Position.Z}, Normal: [3]float64{p.Normal.X, p.Normal.Y, p.Normal.Z}, Minimum: p.Minimum, Maximum: p.Maximum, Mean: p.Mean, Gaussian: p.Gaussian, Surface: p.Surface})
	}
	return out, nil
}
