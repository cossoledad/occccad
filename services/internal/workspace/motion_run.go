package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/geometry"
	"google.golang.org/grpc/codes"
)

type MotionFrame struct {
	KeyframeID     string                               `json:"keyframeId,omitempty"`
	TimeSeconds    float64                              `json:"timeSeconds"`
	DriverValue    float64                              `json:"driverValue"` // canonical rad/mm
	UnitPoses      map[string]InstancePose              `json:"unitPoses"`
	Coordinates    map[string]float64                   `json:"coordinates"`
	AngleBranches  []geometry.AssemblySolvedAngleBranch `json:"angleBranches,omitempty"`
	KinematicValid bool                                 `json:"kinematicValid"`
	HardIterations uint64                               `json:"hardIterations"`
}
type MotionFailure struct {
	TimeSeconds    float64                 `json:"timeSeconds"`
	DriverValue    float64                 `json:"driverValue"`
	Code           string                  `json:"code"`
	Detail         string                  `json:"detail"`
	Solve          *geometry.AssemblySolve `json:"solve,omitempty"`
	ReplayObjectID string                  `json:"replayObjectId,omitempty"`
}
type MotionRun struct {
	Schema      int            `json:"schema"`
	Snapshot    MotionSnapshot `json:"snapshot"`
	Status      string         `json:"status"`
	Frames      []MotionFrame  `json:"frames"`
	Failure     *MotionFailure `json:"failure,omitempty"`
	SolverBuild string         `json:"solverBuild"`
	SolveCalls  int            `json:"solveCalls"`
	Completed   bool           `json:"completed"`
	ElapsedMS   float64        `json:"elapsedMs"`
}

func motionLimits(m Mechanism, coords map[string]float64) error {
	for _, j := range m.Joints {
		q, ok := coords[j.ID]
		if !ok {
			continue
		}
		if j.Lower != nil {
			v, _ := motionValue(*j.Lower, j.Kind == "REVOLUTE")
			if q < v-1e-8 {
				return fmt.Errorf("LIMIT_REACHED: joint %s lower limit", j.ID)
			}
		}
		if j.Upper != nil {
			v, _ := motionValue(*j.Upper, j.Kind == "REVOLUTE")
			if q > v+1e-8 {
				return fmt.Errorf("LIMIT_REACHED: joint %s upper limit", j.ID)
			}
		}
	}
	return nil
}
func motionDof(r geometry.AssemblySolve) (uint64, uint64) {
	var dof, gauge uint64
	for _, c := range r.Components {
		dof += c.RelativeDof
		gauge += c.GaugeDof
	}
	return dof, gauge
}

// Frozen paths and interactive trials share mechanismMotionSolver. Private
// continuation steps never leak into Frames after an ultimately rejected target.
func (service *Service) RunMotionStudy(parent context.Context, s MotionSnapshot, onFrame func(int) error) (out MotionRun, err error) {
	started := time.Now()
	out = MotionRun{Schema: 1, Snapshot: s, Status: "RUNNING", Frames: []MotionFrame{}}
	defer func() { out.ElapsedMS = float64(time.Since(started).Microseconds()) / 1000 }()
	if s.Schema != 1 || s.Policy != motionPolicy || s.Digest != motionDigest(s) ||
		s.Study.BudgetMS < 100 || s.Study.BudgetMS > 120000 || s.Study.Frames < 1 || s.Study.Frames > 500 || len(s.Equations.Bodies) > 64 {
		return out, fmt.Errorf("%w: invalid motion snapshot", ErrValidation)
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(s.Study.BudgetMS)*time.Millisecond)
	defer cancel()
	var engine *mechanismMotionSolver
	failure := func(code, detail string, t, q float64) {
		if parent.Err() != nil {
			code = "CANCELED"
		} else if ctx.Err() != nil {
			code = "BUDGET_EXHAUSTED"
		}
		if engine != nil {
			out.SolveCalls = engine.calls
			out.SolverBuild = engine.solverBuild
			if code == "KINEMATIC_FAILED" {
				switch {
				case engine.transport == codes.InvalidArgument || engine.last != nil && engine.last.Status == "INVALID_MODEL":
					code = "INPUT_ERROR"
				case engine.transport != codes.OK && engine.transport != codes.Unknown:
					code = "EXECUTION_FAILURE"
				case strings.HasPrefix(detail, "UNSUPPORTED_DOF"):
					code = "UNSUPPORTED_DOF"
				case strings.HasPrefix(detail, "INPUT_ERROR"):
					code = "INPUT_ERROR"
				case strings.HasPrefix(detail, "BUDGET_EXHAUSTED") || engine.calls >= 2048:
					code = "BUDGET_EXHAUSTED"
				}
			}
		}
		out.Status = code
		out.Failure = &MotionFailure{TimeSeconds: t, DriverValue: q, Code: code, Detail: detail}
		if engine == nil {
			return
		}
		out.Failure.Solve = engine.last
		if _, e := json.Marshal(engine.last); e != nil {
			out.Failure.Solve = nil
			out.Failure.Detail += "; non-finite diagnostics retained in replay"
		}
		if len(engine.replay) > 0 && service.artifacts != nil {
			saveCtx, c := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
			defer c()
			object, e := service.artifacts.Put(saveCtx, artifactstore.Kind("ASSEMBLY_REPLAY"), "application/json", bytes.NewReader(engine.replay))
			if e == nil {
				out.Failure.ReplayObjectID = object.ID
			}
		}
	}
	engine, err = newMechanismMotionSolver(ctx, service, s)
	if err != nil {
		failure("KINEMATIC_FAILED", err.Error(), 0, 0)
		return out, nil
	}
	reference := 0.0
	if s.Study.Reference != nil {
		reference, err = motionValue(*s.Study.Reference, true)
		if err != nil {
			return out, err
		}
	}
	targets, err := motionPathTargets(s.Study)
	if err != nil {
		return out, err
	}
	if len(s.Study.Keyframes) > 0 && math.Abs(engine.state.Frame.DriverValue-targets[0].Value-reference) > 1e-7 {
		failure("INPUT_ERROR", "initial keyframe does not match captured configuration", 0, targets[0].Value)
		return out, nil
	}
	for _, target := range targets {
		frame, e := engine.Advance(ctx, reference+target.Value)
		if e != nil {
			code := "KINEMATIC_FAILED"
			if strings.HasPrefix(e.Error(), "LIMIT_REACHED") {
				code = "LIMIT_REACHED"
			}
			failure(code, e.Error(), target.Time, target.Value)
			return out, nil
		}
		frame.TimeSeconds = target.Time
		frame.DriverValue = target.Value
		frame.KeyframeID = target.KeyframeID
		out.Frames = append(out.Frames, frame)
		if onFrame != nil {
			if e = onFrame(len(out.Frames)); e != nil {
				failure("EXECUTION_FAILURE", e.Error(), target.Time, target.Value)
				return out, nil
			}
		}
	}
	out.Status = "COMPLETED"
	out.Completed = true
	out.SolveCalls = engine.calls
	out.SolverBuild = engine.solverBuild
	return out, nil
}

type motionPathTarget struct {
	Time, Value float64
	KeyframeID  string
}

func motionPathTargets(s MotionStudy) ([]motionPathTarget, error) {
	if len(s.Keyframes) == 0 {
		start, err := motionValue(s.Start, true)
		if err != nil {
			return nil, err
		}
		end, err := motionValue(s.End, true)
		if err != nil {
			return nil, err
		}
		targets := make([]motionPathTarget, s.Frames)
		for i := range targets {
			fraction := 0.0
			if s.Frames > 1 {
				fraction = float64(i) / float64(s.Frames-1)
			}
			targets[i] = motionPathTarget{Time: fraction * s.DurationSeconds, Value: start + fraction*(end-start)}
		}
		return targets, nil
	}
	duration := s.Keyframes[len(s.Keyframes)-1].TimeSeconds
	times := make(map[float64]string, len(s.Keyframes)+s.Frames)
	for _, key := range s.Keyframes {
		times[key.TimeSeconds] = key.ID
	}
	if duration > 0 {
		for i := 0; i < s.Frames; i++ {
			t := duration * float64(i) / float64(max(1, s.Frames-1))
			if _, ok := times[t]; !ok {
				times[t] = ""
			}
		}
	}
	ordered := make([]float64, 0, len(times))
	for t := range times {
		ordered = append(ordered, t)
	}
	slices.Sort(ordered)
	targets := make([]motionPathTarget, 0, len(ordered))
	segment := 0
	for _, t := range ordered {
		for segment+1 < len(s.Keyframes) && s.Keyframes[segment+1].TimeSeconds <= t {
			segment++
		}
		key := s.Keyframes[segment]
		value, err := motionValue(key.Value, true)
		if err != nil {
			return nil, err
		}
		if segment+1 < len(s.Keyframes) {
			next := s.Keyframes[segment+1]
			end, err := motionValue(next.Value, true)
			if err != nil {
				return nil, err
			}
			value += (end - value) * (t - key.TimeSeconds) / (next.TimeSeconds - key.TimeSeconds)
		}
		targets = append(targets, motionPathTarget{Time: t, Value: value, KeyframeID: times[t]})
	}
	return targets, nil
}

// Assembly preference convergence is separate from mechanism hard feasibility.
// MAX_ITERATIONS is admitted only with a complete independently checked witness.
func motionHardQualified(r geometry.AssemblySolve) bool {
	if r.Status != "CONVERGED" && r.Status != "MAX_ITERATIONS" {
		return false
	}
	if len(r.Components) == 0 || !finiteMotion(r.NormalizedResidual) || len(r.UnsatisfiedConstraintIDs) > 0 {
		return false
	}
	for _, component := range r.Components {
		if !component.Solved {
			return false
		}
	}
	for _, v := range r.EquationResiduals {
		if !finiteMotion(v.NormalizedValue) || math.Abs(v.NormalizedValue) > 1 {
			return false
		}
	}
	return true
}
