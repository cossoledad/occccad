package geometry

import (
	"context"
	"fmt"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"math"
)

type MeasurementElement struct {
	InputID string
	Kind    string
	LocalID uint64
	Mode    string
}
type MeasurementResult struct {
	Complete     bool                  `json:"complete"`
	Diagnostic   string                `json:"diagnostic,omitempty"`
	GeometryType string                `json:"geometryType,omitempty"`
	Values       map[string]float64    `json:"values"`
	Anchors      map[string][3]float64 `json:"anchors"`
	KernelBuild  string                `json:"kernelBuild"`
}

func (c *Client) MeasureGeometry(ctx context.Context, id, kind string, inputs []AnalysisGeometry, operands [][]MeasurementElement) (MeasurementResult, error) {
	req := &workerv1.MeasureGeometryRequest{RequestId: id, Kind: kind}
	for _, g := range inputs {
		req.Geometry = append(req.Geometry, &workerv1.AnalysisGeometry{Id: g.ID, GeometryId: g.GeometryID, Brep: artifactProto(g.BRep), Pose: protoPose(g.Pose)})
	}
	for _, op := range operands {
		p := &workerv1.MeasurementOperand{}
		for _, e := range op {
			p.Elements = append(p.Elements, &workerv1.MeasurementElement{InputId: e.InputID, Kind: e.Kind, LocalId: e.LocalID, Mode: e.Mode})
		}
		req.Operands = append(req.Operands, p)
	}
	reply, err := c.worker.MeasureGeometry(ctx, req)
	if err != nil {
		return MeasurementResult{}, fmt.Errorf("measurement: %w", err)
	}
	return validateMeasurementResult(reply, kind)
}
func validateMeasurementResult(reply *workerv1.MeasureGeometryResponse, kind string) (MeasurementResult, error) {
	bad := func() (MeasurementResult, error) {
		return MeasurementResult{}, fmt.Errorf("invalid measurement response")
	}
	if reply == nil || reply.KernelBuild == "" || len(reply.Values) > 32 || len(reply.Anchors) > 16 {
		return bad()
	}
	out := MeasurementResult{Complete: reply.Complete, Diagnostic: reply.Diagnostic, GeometryType: reply.GeometryType, KernelBuild: reply.KernelBuild, Values: reply.Values, Anchors: map[string][3]float64{}}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	for key, v := range reply.Values {
		if !finite(v) {
			return bad()
		}
		switch key {
		case "distanceMm", "lengthMm", "perimeterMm", "areaMm2", "volumeMm3", "radiusMm", "diameterMm", "majorRadiusMm", "minorRadiusMm", "sizeXMm", "sizeYMm", "sizeZMm", "arcAngleRad":
			if v < 0 {
				return bad()
			}
		case "angleRad":
			if v < 0 || v > math.Pi {
				return bad()
			}
		case "xMm", "yMm", "zMm", "deltaXMm", "deltaYMm", "deltaZMm":
		default:
			return bad()
		}
	}
	for k, p := range reply.Anchors {
		if p == nil || !finite(p.X) || !finite(p.Y) || !finite(p.Z) {
			return bad()
		}
		out.Anchors[k] = [3]float64{p.X, p.Y, p.Z}
	}
	if !out.Complete {
		if out.Diagnostic == "" || len(out.Values) > 0 || len(out.Anchors) > 0 {
			return bad()
		}
		return out, nil
	}
	if out.Diagnostic != "" || len(out.Values) == 0 {
		return bad()
	}
	if kind == "BETWEEN" {
		d, ok := out.Values["distanceMm"]
		a, aok := out.Anchors["first"]
		b, bok := out.Anchors["second"]
		if !ok || d < 0 || !aok || !bok || math.Abs(math.Hypot(math.Hypot(a[0]-b[0], a[1]-b[1]), a[2]-b[2])-d) > 1e-5*math.Max(1, d) {
			return bad()
		}
	}
	if kind == "ANGLE_POINTS" {
		a, ok := out.Values["angleRad"]
		_, v := out.Anchors["vertex"]
		_, first := out.Anchors["first"]
		_, second := out.Anchors["second"]
		if !ok || !v || !first || !second || a < 0 || a > math.Pi {
			return bad()
		}
	}
	return out, nil
}
