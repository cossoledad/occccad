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
type QualityBoundaryEdge struct {
	EdgeID uint64       `json:"edgeId"`
	Points [][3]float64 `json:"points"`
}
type QualityGapCandidate struct {
	FirstEdge  uint64     `json:"firstEdge"`
	SecondEdge uint64     `json:"secondEdge"`
	First      [3]float64 `json:"first"`
	Second     [3]float64 `json:"second"`
	DistanceMM float64    `json:"distanceMm"`
}
type CurvatureResult struct {
	BoundaryEdges    []QualityBoundaryEdge `json:"boundaryEdges"`
	GapCandidates    []QualityGapCandidate `json:"gapCandidates"`
	BoundaryComplete bool                  `json:"boundaryComplete"`
	Complete         bool                  `json:"complete"`
	Diagnostic       string                `json:"diagnostic,omitempty"`
	GeometryType     string                `json:"geometryType"`
	Samples          []CurvatureSample     `json:"samples"`
	UndefinedSamples uint32                `json:"undefinedSamples"`
	KernelBuild      string                `json:"kernelBuild"`
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
	if len(reply.BoundaryEdges) > 256 || len(reply.GapCandidates) > 256 {
		return bad()
	}
	seen := map[uint64]bool{}
	for _, edge := range reply.BoundaryEdges {
		if edge == nil || edge.EdgeId == 0 || seen[edge.EdgeId] || len(edge.Points) < 2 || len(edge.Points) > 64 {
			return bad()
		}
		seen[edge.EdgeId] = true
		v := QualityBoundaryEdge{EdgeID: edge.EdgeId, Points: [][3]float64{}}
		for _, point := range edge.Points {
			if point == nil || !finite(point.X) || !finite(point.Y) || !finite(point.Z) {
				return bad()
			}
			v.Points = append(v.Points, [3]float64{point.X, point.Y, point.Z})
		}
		out.BoundaryEdges = append(out.BoundaryEdges, v)
	}
	for _, gap := range reply.GapCandidates {
		if gap == nil || gap.First == nil || gap.Second == nil || !seen[gap.FirstEdge] || !seen[gap.SecondEdge] || gap.FirstEdge == gap.SecondEdge || !finite(gap.DistanceMm) || gap.DistanceMm <= 1e-7 || gap.DistanceMm > .01 {
			return bad()
		}
		for _, p := range []*workerv1.Vec3{gap.First, gap.Second} {
			if !finite(p.X) || !finite(p.Y) || !finite(p.Z) {
				return bad()
			}
		}
		out.GapCandidates = append(out.GapCandidates, QualityGapCandidate{gap.FirstEdge, gap.SecondEdge, [3]float64{gap.First.X, gap.First.Y, gap.First.Z}, [3]float64{gap.Second.X, gap.Second.Y, gap.Second.Z}, gap.DistanceMm})
		measured := math.Hypot(math.Hypot(gap.First.X-gap.Second.X, gap.First.Y-gap.Second.Y), gap.First.Z-gap.Second.Z)
		if math.Abs(measured-gap.DistanceMm) > 1e-7 {
			return bad()
		}
	}
	out.BoundaryComplete = reply.BoundaryComplete
	return out, nil
}
