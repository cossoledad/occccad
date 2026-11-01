package workspace

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/occccad/occccad/internal/geometry"
)

// Transient session policy; never part of Product, Mechanism or MotionStudy.
type MechanismClashPolicy struct {
	Mode        string  `json:"mode"`
	ClearanceMM float64 `json:"clearanceMm"`
}
type MechanismClashResult struct {
	Mode    string                   `json:"mode"`
	Stopped bool                     `json:"stopped"`
	Sampled bool                     `json:"sampled"`
	Report  StaticInterferenceReport `json:"report"`
}
type frozenInterference struct {
	singleBody bool
	report     StaticInterferenceReport
	inputs     []geometry.AnalysisGeometry
	pairs      []geometry.AnalysisPair
	roots      map[string]InstancePose
}

var errMechanismClash = errors.New("mechanism collision stop")

func validClashPolicy(p MechanismClashPolicy) bool {
	return (p.Mode == "OFF" || p.Mode == "PROMPT" || p.Mode == "STOP") && !math.IsNaN(p.ClearanceMM) && !math.IsInf(p.ClearanceMM, 0) && p.ClearanceMM >= 0
}
func clashViolation(r StaticInterferenceReport) bool {
	for _, p := range r.Pairs {
		if p.Complete && (p.Classification == "PENETRATION" || p.Classification == "CONTAINMENT" || !p.ClearanceSatisfied) {
			return true
		}
	}
	return false
}

// The prepared scope retains missing inputs. They cannot disappear between samples.
func (s *Service) checkFrozenInterference(ctx context.Context, frozen *frozenInterference, poses map[string]InstancePose, clearance float64, mechanism string, sequence uint64, displayDetails bool, previous ...*StaticInterferenceReport) (StaticInterferenceReport, error) {
	out := frozen.report
	out.Source = "MECHANISM"
	out.MechanismID = mechanism
	out.FrameSequence = sequence
	out.ClearanceMM = clearance
	out.Bodies = append([]StaticInterferenceBody{}, frozen.report.Bodies...)
	out.Pairs = append([]geometry.InterferencePair{}, frozen.report.Pairs...)
	inputs := append([]geometry.AnalysisGeometry{}, frozen.inputs...)
	placed := map[string]InstancePose{}
	for i := range out.Bodies {
		body := &out.Bodies[i]
		if len(body.InstancePath.Segments) == 0 {
			return out, fmt.Errorf("%w: missing collision occurrence", ErrValidation)
		}
		id := body.InstancePath.Segments[0].InstanceID
		pose, ok := poses[id]
		base, found := frozen.roots[id]
		if !ok || !found {
			return out, fmt.Errorf("%w: incomplete collision frame", ErrValidation)
		}
		body.Pose = composeInstancePose(pose, inverseRelativePose(body.Pose, base))
		placed[body.ID] = body.Pose
	}
	for i := range inputs {
		inputs[i].Pose = geometry.AssemblyPose(placed[inputs[i].ID])
	}
	out.InputDigest = resolvedDigest(struct {
		Basis, Mechanism string
		Poses            map[string]InstancePose
		Clearance        float64
	}{frozen.report.InputDigest, mechanism, poses, clearance})
	out.CheckedPairs = 0
	out.Complete = false
	if len(previous) > 0 && previous[0] != nil && previous[0].InputDigest == out.InputDigest && previous[0].FrameSequence == sequence {
		// The last STOP probe can already be the exact final checkpoint. Reuse
		// only that same frozen input/pose/sequence, never a nearby sample.
		out.Pairs = append([]geometry.InterferencePair{}, previous[0].Pairs...)
		out.CheckedPairs, out.Complete, out.KernelBuild = previous[0].CheckedPairs, previous[0].Complete, previous[0].KernelBuild
	} else if len(frozen.pairs) > 0 {
		report, err := s.worker.AnalyzeInterference(ctx, newID("mechanism-clash"), inputs, frozen.pairs, clearance, out.ToleranceMM, geometry.InterferenceOptions{ThresholdOnly: true})
		if err != nil {
			return out, err
		}
		out.KernelBuild = report.KernelBuild
		byID := map[geometry.AnalysisPair]geometry.InterferencePair{}
		for _, p := range report.Pairs {
			byID[geometry.AnalysisPair{FirstID: p.FirstID, SecondID: p.SecondID}] = p
		}
		for i, p := range out.Pairs {
			if value, ok := byID[geometry.AnalysisPair{FirstID: p.FirstID, SecondID: p.SecondID}]; ok {
				out.Pairs[i] = value
				if value.Complete {
					out.CheckedPairs++
				}
			}
		}
		out.Complete = report.Complete && len(out.Issues) == 0 && len(out.Pairs) > 0 && out.CheckedPairs == len(out.Pairs)
	}
	// Continuation probes stay numerical. Only the final displayed checkpoint
	// requests Common/Section graphics, using these same frozen BReps and poses.
	// Graphics failure must not change the numerical stop/accept decision.
	if displayDetails {
		byID := map[string]geometry.AnalysisGeometry{}
		for _, input := range inputs {
			byID[input.ID] = input
		}
		for i := range out.Pairs {
			p := &out.Pairs[i]
			if !p.Complete || (p.Classification != "PENETRATION" && p.Classification != "CONTAINMENT" && p.Classification != "CONTACT") {
				continue
			}
			p.DetailStatus = "UNAVAILABLE"
			// Details is a single-pair Worker contract. Send only its two exact
			// inputs; unrelated occurrences never need reloading for graphics.
			pair := geometry.AnalysisPair{FirstID: p.FirstID, SecondID: p.SecondID}
			details, err := s.worker.AnalyzeInterference(ctx, newID("mechanism-clash-display"), []geometry.AnalysisGeometry{byID[p.FirstID], byID[p.SecondID]}, []geometry.AnalysisPair{pair}, clearance, out.ToleranceMM, geometry.InterferenceOptions{Details: true})
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			if err == nil && len(details.Pairs) == 1 {
				d := details.Pairs[0]
				if d.Complete && d.FirstID == p.FirstID && d.SecondID == p.SecondID && d.Classification == p.Classification {
					p.DetailStatus, p.SectionPoints, p.OverlapTriangles = d.DetailStatus, d.SectionPoints, d.OverlapTriangles
				}
			}
		}
	}

	return out, nil
}
