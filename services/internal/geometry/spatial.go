package geometry

import (
	"context"
	"fmt"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"math"
)

type SpatialOptions struct {
	Kind                       string       `json:"kind"`
	SourceIDs                  []string     `json:"sourceIds"`
	TargetIDs                  []string     `json:"targetIds,omitempty"`
	Frame                      AssemblyPose `json:"frame"`
	SectionType                string       `json:"sectionType,omitempty"`
	Size                       [3]float64   `json:"size"`
	Minimum, Maximum, Accuracy float64
}
type SpatialBodyResult struct {
	ID         string       `json:"id"`
	Complete   bool         `json:"complete"`
	Diagnostic string       `json:"diagnostic,omitempty"`
	Segments   [][3]float64 `json:"segments,omitempty"`
	Triangles  [][3]float64 `json:"triangles,omitempty"`
	Regions    []uint32     `json:"regions,omitempty"`
	AreaMM2    float64      `json:"areaMm2"`
	Centroid   [3]float64   `json:"centroid"`
}
type SpatialGeometryReport struct {
	Bodies      []SpatialBodyResult `json:"bodies"`
	Complete    bool                `json:"complete"`
	KernelBuild string              `json:"kernelBuild"`
}

func (c *Client) AnalyzeSpatialGeometry(ctx context.Context, id string, inputs []AnalysisGeometry, o SpatialOptions) (SpatialGeometryReport, error) {
	request := &workerv1.AnalyzeSpatialGeometryRequest{RequestId: id, Kind: o.Kind, SourceIds: o.SourceIDs, TargetIds: o.TargetIDs, Frame: protoPose(o.Frame), SectionType: o.SectionType, Size: &workerv1.Vec3{X: o.Size[0], Y: o.Size[1], Z: o.Size[2]}, MinimumMm: o.Minimum, MaximumMm: o.Maximum, AccuracyMm: o.Accuracy}
	for _, g := range inputs {
		request.Geometry = append(request.Geometry, &workerv1.AnalysisGeometry{Id: g.ID, GeometryId: g.GeometryID, Brep: artifactProto(g.BRep), Pose: protoPose(g.Pose)})
	}
	response, err := c.worker.AnalyzeSpatialGeometry(ctx, request)
	if err != nil {
		return SpatialGeometryReport{}, fmt.Errorf("spatial analysis: %w", err)
	}
	result := SpatialGeometryReport{Complete: response.Complete, KernelBuild: response.KernelBuild, Bodies: []SpatialBodyResult{}}
	complete := true
	if len(response.Bodies) != len(o.SourceIDs) || response.KernelBuild == "" {
		return result, fmt.Errorf("invalid spatial response cardinality")
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	for i, b := range response.Bodies {
		if b.Id != o.SourceIDs[i] || len(b.Segments)%2 != 0 || len(b.Segments) > 32768 || len(b.Triangles)%3 != 0 || len(b.Triangles) > 90000 || o.Kind == "BAND" && len(b.Regions) != len(b.Triangles)/3 || !finite(b.AreaMm2) || b.AreaMm2 < 0 {
			return result, fmt.Errorf("invalid spatial result identity or geometry")
		}
		value := SpatialBodyResult{ID: b.Id, Complete: b.Complete, Diagnostic: b.Diagnostic, AreaMM2: b.AreaMm2, Regions: b.Regions}
		for _, group := range []struct {
			src []*workerv1.Vec3
			dst *[][3]float64
		}{{b.Segments, &value.Segments}, {b.Triangles, &value.Triangles}} {
			for _, p := range group.src {
				if p == nil || !finite(p.X) || !finite(p.Y) || !finite(p.Z) {
					return result, fmt.Errorf("invalid spatial result point")
				}
				*group.dst = append(*group.dst, [3]float64{p.X, p.Y, p.Z})
			}
		}
		if b.Centroid == nil || !finite(b.Centroid.X) || !finite(b.Centroid.Y) || !finite(b.Centroid.Z) {
			return result, fmt.Errorf("invalid spatial centroid")
		}
		value.Centroid = [3]float64{b.Centroid.X, b.Centroid.Y, b.Centroid.Z}
		for _, region := range b.Regions {
			if region > 2 {
				return result, fmt.Errorf("invalid distance band region")
			}
		}
		if !b.Complete {
			complete = false
			if len(b.Triangles) > 0 || len(b.Segments) > 0 {
				return result, fmt.Errorf("uncertified spatial geometry")
			}
		}
		result.Bodies = append(result.Bodies, value)
	}
	if complete != response.Complete {
		return result, fmt.Errorf("inconsistent spatial completion")
	}
	return result, nil
}
