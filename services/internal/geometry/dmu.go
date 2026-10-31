package geometry

import (
	"context"
	"fmt"
	"math"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
)

type AnalysisGeometry struct {
	ID         string            `json:"id"`
	GeometryID string            `json:"geometryId"`
	BRep       ArtifactReference `json:"brep"`
	Pose       AssemblyPose      `json:"pose"`
}
type AnalysisPair struct {
	FirstID  string `json:"firstId"`
	SecondID string `json:"secondId"`
}
type InterferencePair struct {
	FirstID            string       `json:"firstId"`
	SecondID           string       `json:"secondId"`
	Classification     string       `json:"classification"`
	DistanceMM         float64      `json:"distanceMm"`
	CommonVolumeMM3    float64      `json:"commonVolumeMm3"`
	FirstWitness       [3]float64   `json:"firstWitness"`
	SecondWitness      [3]float64   `json:"secondWitness"`
	Complete           bool         `json:"complete"`
	ClearanceSatisfied bool         `json:"clearanceSatisfied"`
	Diagnostic         string       `json:"diagnostic,omitempty"`
	CommonTested       bool         `json:"commonTested"`
	DistanceExact      bool         `json:"distanceExact"`
	WitnessKind        string       `json:"witnessKind"`
	DetailStatus       string       `json:"detailStatus"`
	Containment        string       `json:"containment,omitempty"`
	SectionPoints      [][3]float64 `json:"sectionPoints,omitempty"`
	OverlapTriangles   [][3]float64 `json:"overlapTriangles,omitempty"`
}
type InterferenceReport struct {
	Pairs       []InterferencePair `json:"pairs"`
	Complete    bool               `json:"complete"`
	KernelBuild string             `json:"kernelBuild"`
}

type InterferenceOptions struct{ ThresholdOnly, Details bool }

func (c *Client) AnalyzeInterference(ctx context.Context, id string, inputs []AnalysisGeometry, pairs []AnalysisPair, clearance, tolerance float64, options ...InterferenceOptions) (InterferenceReport, error) {
	request := &workerv1.AnalyzeInterferenceRequest{RequestId: id, ClearanceMm: clearance, ToleranceMm: tolerance}
	if len(options) > 0 {
		request.ThresholdOnly = options[0].ThresholdOnly
		request.Details = options[0].Details
	}
	for _, v := range inputs {
		request.Geometry = append(request.Geometry, &workerv1.AnalysisGeometry{Id: v.ID, GeometryId: v.GeometryID, Brep: artifactProto(v.BRep), Pose: protoPose(v.Pose)})
	}
	for _, v := range pairs {
		request.Pairs = append(request.Pairs, &workerv1.AnalysisPair{FirstId: v.FirstID, SecondId: v.SecondID})
	}
	response, err := c.worker.AnalyzeInterference(ctx, request)
	if err != nil {
		return InterferenceReport{}, fmt.Errorf("analyze interference: %w", err)
	}
	result := InterferenceReport{Complete: response.Complete, KernelBuild: response.KernelBuild, Pairs: []InterferencePair{}}
	if len(response.Pairs) != len(pairs) {
		return result, fmt.Errorf("incomplete interference response cardinality")
	}
	complete := true
	for i, v := range response.Pairs {
		if v.FirstId != pairs[i].FirstID || v.SecondId != pairs[i].SecondID {
			return result, fmt.Errorf("interference response identity mismatch")
		}

		if v.Complete {
			if v.FirstWitness == nil || v.SecondWitness == nil {
				return result, fmt.Errorf("missing complete DMU witnesses")
			}
			for _, point := range []*workerv1.Vec3{v.FirstWitness, v.SecondWitness} {
				for _, value := range []float64{point.X, point.Y, point.Z} {
					if math.IsNaN(value) || math.IsInf(value, 0) {
						return result, fmt.Errorf("invalid complete DMU witness")
					}
				}
			}
			switch v.Classification {
			case "SEPARATED", "CONTACT", "PENETRATION", "CONTAINMENT":
			default:
				return result, fmt.Errorf("invalid complete DMU classification")
			}
			if math.IsNaN(v.DistanceMm) || math.IsInf(v.DistanceMm, 0) || v.DistanceMm < 0 || math.IsNaN(v.CommonVolumeMm3) || math.IsInf(v.CommonVolumeMm3, 0) || v.CommonVolumeMm3 < 0 {
				return result, fmt.Errorf("invalid DMU measurements")
			}
			if v.ClearanceSatisfied && (v.Classification == "PENETRATION" || v.Classification == "CONTAINMENT" || v.DistanceMm < clearance) {
				return result, fmt.Errorf("inconsistent DMU clearance result")
			}
		} else {
			complete = false
			if v.Classification != "INCONCLUSIVE" || v.ClearanceSatisfied {
				return result, fmt.Errorf("invalid incomplete DMU result")
			}
		}
		pair := InterferencePair{FirstID: v.FirstId, SecondID: v.SecondId, Classification: v.Classification,
			Complete: v.Complete, ClearanceSatisfied: v.ClearanceSatisfied, Diagnostic: v.Diagnostic, CommonTested: v.CommonTested, DistanceExact: v.DistanceExact,
			WitnessKind: v.WitnessKind, DetailStatus: v.DetailStatus, Containment: v.Containment}
		// Unfinished numeric intermediates are not certified measurements and may
		// contain NaN. Keep the diagnostic and completed pairs JSON-persistable.
		if v.Complete {
			pair.DistanceMM, pair.CommonVolumeMM3 = v.DistanceMm, v.CommonVolumeMm3
			pair.FirstWitness = [3]float64{v.FirstWitness.X, v.FirstWitness.Y, v.FirstWitness.Z}
			pair.SecondWitness = [3]float64{v.SecondWitness.X, v.SecondWitness.Y, v.SecondWitness.Z}
		}
		if v.Complete && !v.DistanceExact && (v.Classification != "SEPARATED" || !v.ClearanceSatisfied || !request.ThresholdOnly || request.Details || v.WitnessKind != "NONE" || v.CommonTested || v.CommonVolumeMm3 != 0) {
			return result, fmt.Errorf("invalid conservative interference proof")
		}
		if v.Complete && v.DistanceExact && (v.Classification == "SEPARATED" && v.WitnessKind != "DISTANCE" || v.Classification == "CONTACT" && v.WitnessKind != "CONTACT") {
			return result, fmt.Errorf("missing exact interference witness semantics")
		}
		switch v.DetailStatus {
		case "NOT_REQUESTED", "COMPLETE", "UNAVAILABLE", "LIMIT":
		default:
			return result, fmt.Errorf("invalid interference detail status")
		}
		if v.DetailStatus != "NOT_REQUESTED" && !request.Details {
			return result, fmt.Errorf("unsolicited interference detail")
		}
		if v.Complete && v.Classification == "CONTAINMENT" && v.Containment != "FIRST_INSIDE_SECOND" && v.Containment != "SECOND_INSIDE_FIRST" && v.Containment != "COINCIDENT" {
			return result, fmt.Errorf("missing containment direction")
		}
		if v.WitnessKind != "NONE" && v.WitnessKind != "DISTANCE" && v.WitnessKind != "CONTACT" {
			return result, fmt.Errorf("invalid interference witness kind")
		}
		if v.WitnessKind != "NONE" && (!v.DistanceExact || !v.Complete || v.Classification == "PENETRATION" || v.Classification == "CONTAINMENT") {
			return result, fmt.Errorf("invalid interference witness evidence")
		}
		if len(v.SectionPoints)%2 != 0 || len(v.SectionPoints) > 32768 || len(v.OverlapTriangles)%3 != 0 || len(v.OverlapTriangles) > 90000 {
			return result, fmt.Errorf("invalid interference graphics limits")
		}
		if (len(v.SectionPoints) > 0 || len(v.OverlapTriangles) > 0) && (!v.Complete || v.DetailStatus != "COMPLETE") {
			return result, fmt.Errorf("uncertified interference graphics")
		}
		for _, group := range []struct {
			src []*workerv1.Vec3
			dst *[][3]float64
		}{{v.SectionPoints, &pair.SectionPoints}, {v.OverlapTriangles, &pair.OverlapTriangles}} {
			for _, p := range group.src {
				if p == nil || math.IsNaN(p.X) || math.IsInf(p.X, 0) || math.IsNaN(p.Y) || math.IsInf(p.Y, 0) || math.IsNaN(p.Z) || math.IsInf(p.Z, 0) {
					return result, fmt.Errorf("invalid interference graphic point")
				}
				*group.dst = append(*group.dst, [3]float64{p.X, p.Y, p.Z})
			}
		}
		result.Pairs = append(result.Pairs, pair)
	}
	if response.KernelBuild == "" || response.Complete != complete {
		return result, fmt.Errorf("inconsistent DMU completion metadata")
	}
	return result, nil
}
