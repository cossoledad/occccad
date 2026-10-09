package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/geometry"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MotionFrame struct {
	TimeSeconds    float64                              `json:"timeSeconds"`
	DriverValue    float64                              `json:"driverValue"` // canonical rad/mm
	UnitPoses      map[string]InstancePose              `json:"unitPoses"`
	Coordinates    map[string]float64                   `json:"coordinates"`
	AngleBranches  []geometry.AssemblySolvedAngleBranch `json:"angleBranches,omitempty"`
	KinematicValid bool                                 `json:"kinematicValid"`
	HardIterations uint64                               `json:"hardIterations"`
	DMU            *geometry.InterferenceReport         `json:"dmu,omitempty"`
	DMUConclusion  string                               `json:"dmuConclusion"`
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
func (service *Service) motionDMU(ctx context.Context, s MotionSnapshot, f *MotionFrame) error {
	f.DMUConclusion = "NOT_CHECKED"
	if !s.Study.CheckDMU {
		return nil
	}
	inputs := []geometry.AnalysisGeometry{}
	for _, g := range s.GeometryUnits {
		p, ok := f.UnitPoses[g.MotionUnitID]
		if !ok {
			return fmt.Errorf("missing rigid unit for DMU")
		}
		inputs = append(inputs, geometry.AnalysisGeometry{ID: g.ID, GeometryID: g.GeometryID, BRep: g.BRep, Pose: geometry.AssemblyPose(composeInstancePose(p, g.RelativePose))})
	}
	gap, _ := motionValue(s.Study.Clearance, false)
	report, err := service.worker.AnalyzeInterference(ctx, "motion/"+s.Digest, inputs, s.Pairs, gap, 1e-7)
	if err != nil {
		f.DMUConclusion = "INCONCLUSIVE"
		return err
	}
	f.DMU = &report
	f.DMUConclusion = "PASS"
	for _, p := range report.Pairs {
		if !p.ClearanceSatisfied || p.Classification == "PENETRATION" || p.Classification == "CONTAINMENT" {
			f.DMUConclusion = "VIOLATION"
		}
	}
	if !report.Complete {
		f.DMUConclusion = "INCONCLUSIVE"
		return fmt.Errorf("DMU has incomplete pairs")
	}
	return nil
}

// Runs only frozen inputs. Qualified poses are transient; no business-model writes.
func (service *Service) RunMotionStudy(parent context.Context, s MotionSnapshot, onFrame func(int) error) (out MotionRun, err error) {
	started := time.Now()
	out = MotionRun{Schema: 1, Snapshot: s, Status: "RUNNING", Frames: []MotionFrame{}}
	defer func() { out.ElapsedMS = float64(time.Since(started).Microseconds()) / 1000 }()
	if s.Schema != 1 || s.Policy != motionPolicy || s.Digest != motionDigest(s) || s.Study.BudgetMS < 100 || s.Study.BudgetMS > 120000 || s.Study.Frames < 1 || s.Study.Frames > 500 || len(s.Equations.Bodies) > 64 {
		return out, fmt.Errorf("%w: invalid motion snapshot", ErrValidation)
	}
	// A per-run copy keeps the frozen baseline and concurrent jobs independent.
	b, _ := json.Marshal(s.Equations)
	var equations mechanismEquations
	if err = json.Unmarshal(b, &equations); err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(s.Study.BudgetMS)*time.Millisecond)
	defer cancel()
	var last *geometry.AssemblySolve
	var lastTransport codes.Code
	var replay []byte
	failure := func(code, detail string, t, q float64) {
		if parent.Err() != nil {
			code = "CANCELED"
		} else if ctx.Err() != nil {
			code = "BUDGET_EXHAUSTED"
		}

		if code == "KINEMATIC_FAILED" {
			if lastTransport == codes.InvalidArgument || (last != nil && last.Status == "INVALID_MODEL") {
				code = "INPUT_ERROR"
			} else if lastTransport != codes.OK && lastTransport != codes.Unknown {
				code = "EXECUTION_FAILURE"
			} else if strings.HasPrefix(detail, "UNSUPPORTED_DOF") {
				code = "UNSUPPORTED_DOF"
			}
		}
		if out.SolveCalls >= 2048 {
			code = "BUDGET_EXHAUSTED"
		}
		out.Status = code
		out.Failure = &MotionFailure{TimeSeconds: t, DriverValue: q, Code: code, Detail: detail, Solve: last}
		if _, e := json.Marshal(last); e != nil {
			out.Failure.Solve = nil
			out.Failure.Detail += "; non-finite solver diagnostics retained only in replay"
		}
		if len(replay) > 0 && service.artifacts != nil {
			saveCtx, c := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
			defer c()
			o, e := service.artifacts.Put(saveCtx, artifactstore.Kind("ASSEMBLY_REPLAY"), "application/json", bytes.NewReader(replay))
			if e == nil {
				out.Failure.ReplayObjectID = o.ID
			}
		}
	}
	solve := func(constraints []geometry.AssemblyConstraint) (geometry.AssemblySolve, error) {
		if out.SolveCalls >= 2048 {
			return geometry.AssemblySolve{}, fmt.Errorf("continuation solve budget exhausted")
		}
		out.SolveCalls++
		r, e := service.worker.SolveAssemblyWithOptions(ctx, fmt.Sprintf("motion/%s/%d", s.Digest, out.SolveCalls), equations.Bodies, equations.Geometry, constraints, geometry.AssemblySolveOptions{SolverProfile: &s.SolverProfile, DisableConflictProbes: true, CaptureReplay: func(b []byte, e error) {
			if e == nil {
				replay = b
			}
		}})
		last = &r
		lastTransport = status.Code(e)
		if e != nil {
			return r, e
		}
		if r.SolverBuild == "" {
			lastTransport = codes.Internal
			return r, status.Error(codes.Internal, "solver build missing")
		}
		if out.SolverBuild == "" {
			out.SolverBuild = r.SolverBuild
		} else if out.SolverBuild != r.SolverBuild {
			lastTransport = codes.Internal
			return r, status.Error(codes.Internal, "solver build changed during study")
		}
		if !motionHardQualified(r) {
			return r, fmt.Errorf("%s: %s", r.Status, r.Diagnostic)
		}

		manifest := AssemblySolveManifest{Bodies: equations.Bodies, Geometry: equations.Geometry, Constraints: constraints, SolverProfile: s.SolverProfile}
		if r.Status != "CONVERGED" {
			if ok, why := conflictIndependentWitness(manifest, r); !ok {
				return r, fmt.Errorf("hard witness: %s", why)
			}
		}
		for _, c := range constraints {
			if c.Kind == "FIX" {
				mp := map[string]geometry.AssemblyPose{}
				for _, b := range r.Bodies {
					mp[b.ID] = b.Pose
				}
				if c.FixedPose == nil || !conflictPoseClose(mp[c.FirstBodyID], *c.FixedPose, s.SolverProfile.LengthTolerance, s.SolverProfile.AngleTolerance) {
					return r, fmt.Errorf("fixed unit pose residual")
				}
			}
		}
		return r, nil
	}
	poses := func(r geometry.AssemblySolve) (map[string]InstancePose, error) {
		for _, branch := range r.AngleBranches {
			if !finiteMotion(branch.State.WrappedAngle) || !finiteMotion(branch.State.UnwrappedAngle) {
				return nil, fmt.Errorf("invalid solved angle branch")
			}
		}
		p := map[string]InstancePose{}
		for _, v := range r.Bodies {
			pose := InstancePose(v.Pose)
			if !validMotionPose(pose) {
				return nil, fmt.Errorf("invalid solved pose")
			}
			p[v.ID] = pose
		}
		for _, body := range equations.Bodies {
			if _, ok := p[body.ID]; !ok {
				return nil, fmt.Errorf("missing solved unit")
			}
		}
		return p, nil
	}
	if s.CurrentOnly {
		p := map[string]InstancePose{}
		for _, v := range equations.Bodies {
			p[v.ID] = InstancePose(v.Pose)
		}
		f := MotionFrame{UnitPoses: p, DMUConclusion: "NOT_CHECKED"}
		e := service.motionDMU(ctx, s, &f)
		out.Frames = append(out.Frames, f)
		if e != nil {
			failure("DMU_UNFINISHED", e.Error(), 0, 0)
		} else {
			out.Status = "COMPLETED"
			out.Completed = true
		}
		return out, nil
	}
	baseline, e := solve(equations.Constraints)
	if e != nil {
		failure("KINEMATIC_FAILED", e.Error(), 0, 0)
		return out, nil
	}
	dof, gauge := motionDof(baseline)
	if dof != 1 || gauge != 0 {
		failure("UNSUPPORTED_DOF", fmt.Sprintf("grounded relative DOF=%d gauge=%d; require 1/0", dof, gauge), 0, 0)
		return out, nil
	}
	p, e := poses(baseline)
	if e != nil {
		failure("KINEMATIC_FAILED", e.Error(), 0, 0)
		return out, nil
	}
	driver := MechanismJoint{}
	for _, j := range s.Mechanism.Joints {
		if j.ID == s.Study.DriverJointID {
			driver = j
		}
	}
	if driver.Second == nil {
		return out, fmt.Errorf("invalid driver")
	}
	current, e := motionJointCoordinate(driver, p, nil)
	if e != nil {
		return out, e
	}
	coords, e := verifyMechanismJoints(s.Mechanism, p, nil, driver.ID, current)
	if e != nil {
		failure("KINEMATIC_FAILED", e.Error(), 0, current)
		return out, nil
	}
	if e = motionLimits(s.Mechanism, coords); e != nil {
		failure("LIMIT_REACHED", e.Error(), 0, current)
		return out, nil
	}
	update := func(r geometry.AssemblySolve, p map[string]InstancePose) {
		for i := range equations.Bodies {
			guess := geometry.AssemblyPose(p[equations.Bodies[i].ID])
			equations.Bodies[i].InitialGuess = &guess
		}
		for i := range equations.Constraints {
			c := &equations.Constraints[i]
			for _, v := range r.AngleBranches {
				if v.ConstraintID == c.ID {
					state := v.State
					c.AngleBranchState = &state
				}
			}
			for _, v := range r.AlignmentBranches {
				if v.ConstraintID == c.ID {
					c.DirectionRelation = v.Direction
				}
			}
			for _, v := range r.DistanceBranches {
				if v.ConstraintID == c.ID {
					c.DistanceRelation = v.Side
				}
			}
		}
	}
	update(baseline, p)
	zero, _ := motionValue(driver.Zero, driver.Kind == "REVOLUTE")
	var advance func(float64, int) (MotionFrame, error)
	advance = func(target float64, depth int) (MotionFrame, error) {
		if e := ctx.Err(); e != nil {
			return MotionFrame{}, e
		}
		step := target - current
		if driver.Kind == "REVOLUTE" && math.Abs(step) > math.Pi/12 && depth < 10 {
			if _, e := advance(current+step/2, depth+1); e != nil {
				return MotionFrame{}, e
			}
			return advance(target, depth+1)
		}
		d := equations.Driver
		d.Value = zero + float64(driver.Direction)*target
		if driver.Kind == "REVOLUTE" {
			d.Value = math.Mod(d.Value, 2*math.Pi)
			if d.Value < 0 {
				d.Value += 2 * math.Pi
			}
		}
		if driver.Kind == "REVOLUTE" {
			prior := zero + float64(driver.Direction)*current
			wrapped := math.Remainder(prior, 2*math.Pi)
			d.AngleBranchState = &geometry.AssemblyAngleBranchState{WrappedAngle: wrapped, UnwrappedAngle: prior, Winding: int64(math.Round((prior - wrapped) / (2 * math.Pi)))}
		}
		cs := append(append([]geometry.AssemblyConstraint(nil), equations.Constraints...), d)
		r, e := solve(cs)
		var next map[string]InstancePose
		var nc map[string]float64
		if e == nil {
			rd, g := motionDof(r)
			if rd != 0 || g != 0 {
				e = fmt.Errorf("UNSUPPORTED_DOF: driven relative DOF=%d gauge=%d; require 0/0", rd, g)
			}
		}
		if e == nil {
			next, e = poses(r)
		}
		if e == nil {
			nc, e = verifyMechanismJoints(s.Mechanism, next, coords, driver.ID, target)
		}
		if e == nil {
			if e = motionLimits(s.Mechanism, nc); e != nil {
				return MotionFrame{}, e
			}
		}

		if e != nil {
			transport := status.Code(e)
			if r.Status == "INVALID_MODEL" || transport == codes.InvalidArgument || transport == codes.Unavailable || transport == codes.Internal || out.SolveCalls >= 2048 {
				return MotionFrame{}, e
			}
			if depth >= 10 || math.Abs(step) < 1e-8 || ctx.Err() != nil || strings.HasPrefix(e.Error(), "UNSUPPORTED_DOF") {
				return MotionFrame{}, e
			}
			if _, e = advance(current+step/2, depth+1); e != nil {
				return MotionFrame{}, e
			}
			return advance(target, depth+1)
		}
		current = target
		coords = nc
		p = next
		update(r, p)
		return MotionFrame{DriverValue: target, UnitPoses: next, Coordinates: nc, AngleBranches: r.AngleBranches, KinematicValid: true, HardIterations: r.Iterations, DMUConclusion: "NOT_CHECKED"}, nil
	}
	start, _ := motionValue(s.Study.Start, driver.Kind == "REVOLUTE")
	end, _ := motionValue(s.Study.End, driver.Kind == "REVOLUTE")
	for i := 0; i < s.Study.Frames; i++ {
		fraction := float64(i) / float64(s.Study.Frames-1)
		t := fraction * s.Study.DurationSeconds
		q := start + fraction*(end-start)
		f, e := advance(q, 0)
		if e != nil {
			code := "KINEMATIC_FAILED"
			if strings.HasPrefix(e.Error(), "LIMIT_REACHED") {
				code = "LIMIT_REACHED"
			}
			failure(code, e.Error(), t, q)
			return out, nil
		}
		f.TimeSeconds = t
		e = service.motionDMU(ctx, s, &f)
		out.Frames = append(out.Frames, f)
		if e != nil {
			failure("DMU_UNFINISHED", e.Error(), t, q)
			return out, nil
		}
		if onFrame != nil {
			if e = onFrame(len(out.Frames)); e != nil {
				failure("EXECUTION_FAILURE", e.Error(), t, q)
				return out, nil
			}
		}
	}
	out.Status = "COMPLETED"
	out.Completed = true
	return out, nil
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
