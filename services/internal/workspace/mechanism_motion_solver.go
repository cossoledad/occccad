package workspace

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/valuecopy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// One numerical path serves command trials and recorded/law segments. The
// checkpoint contains only fully qualified complete states, never private steps.
type mechanismMotionState struct {
	Equations mechanismEquations
	Frame     MotionFrame
}
type mechanismMotionSolver struct {
	service     *Service
	snapshot    MotionSnapshot
	state       mechanismMotionState
	driver      MechanismJoint
	solverBuild string
	calls       int
	last        *geometry.AssemblySolve
	transport   codes.Code
	replay      []byte
}

func newMechanismMotionSolver(ctx context.Context, service *Service, s MotionSnapshot) (*mechanismMotionSolver, error) {
	eq, err := valuecopy.Clone(s.Equations)
	if err != nil {
		return nil, err
	}
	e := &mechanismMotionSolver{service: service, snapshot: s, state: mechanismMotionState{Equations: eq}}
	for _, j := range s.Mechanism.Joints {
		if j.ID == s.Study.DriverJointID {
			e.driver = j
		}
	}
	if e.driver.Second == nil {
		return e, fmt.Errorf("INPUT_ERROR: missing active coordinate")
	}
	r, err := e.solve(ctx, eq.Constraints, nil)
	if err != nil {
		return e, err
	}
	dof, gauge := motionDof(r)
	if dof != 1 || gauge != 0 {
		return e, fmt.Errorf("UNSUPPORTED_DOF: grounded relative DOF=%d gauge=%d; require 1/0", dof, gauge)
	}
	poses, err := e.poses(r)
	if err != nil {
		return e, err
	}
	var prior *float64
	if value, ok := s.Mechanism.Coordinates[e.driver.ID]; ok {
		prior = &value
	}
	q, err := motionJointCoordinate(e.driver, poses, prior)
	if err != nil {
		return e, err
	}
	coords, err := verifyMechanismJoints(s.Mechanism, poses, s.Mechanism.Coordinates, e.driver.ID, q)
	if err != nil {
		return e, err
	}
	if err = motionLimits(s.Mechanism, coords); err != nil {
		return e, err
	}
	e.publish(r, MotionFrame{DriverValue: q, UnitPoses: poses, Coordinates: coords, AngleBranches: r.AngleBranches, KinematicValid: true, HardIterations: r.Iterations})
	// Probe this selected coordinate at the same configuration. Other saved
	// commands are measured outputs, not extra hard equations.
	if _, err = e.Advance(ctx, q); err != nil {
		return e, err
	}
	return e, nil
}
func (e *mechanismMotionSolver) solve(ctx context.Context, cs []geometry.AssemblyConstraint, target *geometry.AssemblyDragTarget) (geometry.AssemblySolve, error) {
	if err := ctx.Err(); err != nil {
		return geometry.AssemblySolve{}, err
	}
	if e.calls >= 2048 {
		return geometry.AssemblySolve{}, fmt.Errorf("BUDGET_EXHAUSTED: continuation solve count")
	}
	e.calls++
	eq := e.state.Equations
	r, err := e.service.worker.SolveAssemblyWithOptions(ctx, fmt.Sprintf("mechanism/%s/%d", e.snapshot.Digest, e.calls), eq.Bodies, eq.Geometry, cs, geometry.AssemblySolveOptions{
		SolverProfile: &e.snapshot.SolverProfile, DragTarget: target, DisableConflictProbes: true,
		CaptureReplay: func(b []byte, err error) {
			if err == nil {
				e.replay = b
			}
		},
	})
	e.last = &r
	e.transport = status.Code(err)
	if err != nil {
		return r, err
	}
	if r.SolverBuild != assemblySolverBuildPolicy || e.solverBuild != "" && e.solverBuild != r.SolverBuild {
		e.transport = codes.Internal
		return r, status.Error(codes.Internal, "solver build policy missing or changed")
	}
	e.solverBuild = r.SolverBuild
	if !motionHardQualified(r) {
		return r, fmt.Errorf("%s: %s", r.Status, r.Diagnostic)
	}
	manifest := AssemblySolveManifest{Bodies: eq.Bodies, Geometry: eq.Geometry, Constraints: cs, SolverProfile: e.snapshot.SolverProfile}
	if ok, why := conflictIndependentWitness(manifest, r); !ok {
		return r, fmt.Errorf("hard witness: %s", why)
	}
	for _, c := range cs {
		if c.Kind != "FIX" {
			continue
		}
		found := false
		for _, b := range r.Bodies {
			if b.ID == c.FirstBodyID {
				found = c.FixedPose != nil && conflictPoseClose(b.Pose, *c.FixedPose, e.snapshot.SolverProfile.LengthTolerance, e.snapshot.SolverProfile.AngleTolerance)
			}
		}
		if !found {
			return r, fmt.Errorf("fixed unit pose residual")
		}
	}
	return r, nil
}
func (e *mechanismMotionSolver) poses(r geometry.AssemblySolve) (map[string]InstancePose, error) {
	for _, b := range r.AngleBranches {
		if !finiteMotion(b.State.WrappedAngle) || !finiteMotion(b.State.UnwrappedAngle) {
			return nil, fmt.Errorf("invalid solved angle branch")
		}
	}
	p := map[string]InstancePose{}
	for _, b := range r.Bodies {
		pose := InstancePose(b.Pose)
		if !validMotionPose(pose) {
			return nil, fmt.Errorf("invalid solved pose")
		}
		if _, duplicate := p[b.ID]; duplicate {
			return nil, fmt.Errorf("duplicate solved unit")
		}
		p[b.ID] = pose
	}
	if len(p) != len(e.state.Equations.Bodies) {
		return nil, fmt.Errorf("incomplete solved units")
	}
	for _, b := range e.state.Equations.Bodies {
		if _, ok := p[b.ID]; !ok {
			return nil, fmt.Errorf("missing solved unit")
		}
	}
	return p, nil
}
func (e *mechanismMotionSolver) publish(r geometry.AssemblySolve, f MotionFrame) {
	e.state.Frame = f
	for i := range e.state.Equations.Bodies {
		b := &e.state.Equations.Bodies[i]
		guess := geometry.AssemblyPose(f.UnitPoses[b.ID])
		b.InitialGuess = &guess
	}
	for i := range e.state.Equations.Constraints {
		c := &e.state.Equations.Constraints[i]
		for _, b := range r.AngleBranches {
			if b.ConstraintID == c.ID {
				v := b.State
				c.AngleBranchState = &v
			}
		}
		for _, b := range r.AlignmentBranches {
			if b.ConstraintID == c.ID {
				c.DirectionRelation = b.Direction
			}
		}
		for _, b := range r.DistanceBranches {
			if b.ConstraintID == c.ID {
				c.DistanceRelation = b.Side
			}
		}
	}
}
func (e *mechanismMotionSolver) Advance(ctx context.Context, target float64) (MotionFrame, error) {
	if !finiteMotion(target) || !finiteMotion(target-e.state.Frame.DriverValue) {
		return MotionFrame{}, fmt.Errorf("INPUT_ERROR: coordinate overflow")
	}
	before, err := valuecopy.Clone(e.state)
	if err != nil {
		return MotionFrame{}, err
	}
	f, err := e.advance(ctx, target, 0)
	if err != nil {
		e.state = before
		return MotionFrame{}, err
	}
	return f, nil
}

// Change the active coordinate at this very checkpoint. Keep the physical
// equations, nominal poses, warm state and winding; probe the new hard equation
// at its measured current value before publishing a different input identity.
func (e *mechanismMotionSolver) SelectDriver(ctx context.Context, d MotionDriver) error {
	m := e.snapshot.Mechanism
	i := slices.IndexFunc(m.Joints, func(j MechanismJoint) bool { return j.ID == d.JointID && j.Kind == "REVOLUTE" })
	if d.MechanismID != m.ID || i < 0 || d.Reference == nil || d.ReferenceDigest != commandCoordinateBasis(m.Joints[i]) {
		return fmt.Errorf("INPUT_ERROR: COMMAND_REFERENCE_REVALIDATION_REQUIRED")
	}
	before, err := valuecopy.Clone(e.state.Frame)
	if err != nil {
		return err
	}
	e.driver = m.Joints[i]
	prior := before.Coordinates[e.driver.ID]
	q, err := motionJointCoordinate(e.driver, before.UnitPoses, &prior)
	if err != nil {
		return err
	}
	e.state.Equations.Driver = mechanismCoordinateEquation(m.ID, e.driver)
	e.state.Frame.DriverValue = q
	e.snapshot.Study.DriverID, e.snapshot.Study.DriverJointID = d.ID, d.JointID
	e.snapshot.Study.Reference = d.Reference
	e.snapshot.Equations, err = valuecopy.Clone(e.state.Equations)
	if err != nil {
		return err
	}
	e.snapshot.Mechanism.Poses = mechanismMemberPoses(m, before.UnitPoses)
	e.snapshot.Mechanism.Coordinates, _ = valuecopy.Clone(before.Coordinates)
	e.snapshot.Digest = motionDigest(e.snapshot)
	if _, err = e.Advance(ctx, q); err != nil {
		return err
	}
	for id, p := range before.UnitPoses {
		if !conflictPoseClose(geometry.AssemblyPose(p), geometry.AssemblyPose(e.state.Frame.UnitPoses[id]), e.snapshot.SolverProfile.LengthTolerance, e.snapshot.SolverProfile.AngleTolerance) {
			return fmt.Errorf("LOCAL_SOLVE_FAILED: command switch changed its checkpoint")
		}
	}
	e.snapshot.Equations, err = valuecopy.Clone(e.state.Equations)
	if err != nil {
		return err
	}
	e.snapshot.Mechanism.Poses = mechanismMemberPoses(m, before.UnitPoses)
	e.snapshot.Mechanism.Coordinates, _ = valuecopy.Clone(before.Coordinates)
	e.snapshot.Digest = motionDigest(e.snapshot)
	return nil
}
func (e *mechanismMotionSolver) advance(ctx context.Context, target float64, depth int) (MotionFrame, error) {
	if err := ctx.Err(); err != nil {
		return MotionFrame{}, err
	}
	current := e.state.Frame.DriverValue
	step := target - current
	if math.Abs(step) > math.Pi/36 {
		if depth >= 10 {
			return MotionFrame{}, fmt.Errorf("BUDGET_EXHAUSTED: continuation step size")
		}
		if _, err := e.advance(ctx, current+step/2, depth+1); err != nil {
			return MotionFrame{}, err
		}
		return e.advance(ctx, target, depth+1)
	}
	d := e.state.Equations.Driver
	zero, _ := motionValue(e.driver.Zero, true)
	d.Value = math.Mod(zero+float64(e.driver.Direction)*target, 2*math.Pi)
	if d.Value < 0 {
		d.Value += 2 * math.Pi
	}
	prior := zero + float64(e.driver.Direction)*current
	wrapped := math.Remainder(prior, 2*math.Pi)
	d.AngleBranchState = &geometry.AssemblyAngleBranchState{WrappedAngle: wrapped, UnwrappedAngle: prior, Winding: int64(math.Round((prior - wrapped) / (2 * math.Pi)))}
	cs := append(append([]geometry.AssemblyConstraint(nil), e.state.Equations.Constraints...), d)
	r, err := e.solve(ctx, cs, nil)
	var p map[string]InstancePose
	var coordinates map[string]float64
	if err == nil {
		dof, gauge := motionDof(r)
		if dof != 0 || gauge != 0 {
			err = fmt.Errorf("UNSUPPORTED_DOF: driven relative DOF=%d gauge=%d", dof, gauge)
		}
	}
	if err == nil {
		p, err = e.poses(r)
	}
	if err == nil {
		coordinates, err = verifyMechanismJoints(e.snapshot.Mechanism, p, e.state.Frame.Coordinates, e.driver.ID, target)
	}
	if err == nil {
		if err = motionLimits(e.snapshot.Mechanism, coordinates); err != nil {
			return MotionFrame{}, err
		}
	}
	if err != nil {
		transport := status.Code(err)
		if depth >= 10 || math.Abs(step) < 1e-8 || ctx.Err() != nil || e.calls >= 2048 || r.Status == "INVALID_MODEL" ||
			transport == codes.InvalidArgument || transport == codes.Unavailable || transport == codes.Internal || strings.HasPrefix(err.Error(), "UNSUPPORTED_DOF") {
			return MotionFrame{}, err
		}
		if _, err = e.advance(ctx, current+step/2, depth+1); err != nil {
			return MotionFrame{}, err
		}
		return e.advance(ctx, target, depth+1)
	}
	f := MotionFrame{DriverValue: target, UnitPoses: p, Coordinates: coordinates, AngleBranches: r.AngleBranches, KinematicValid: true, HardIterations: r.Iterations}
	e.publish(r, f)
	return f, nil
}
func (e *mechanismMotionSolver) Drag(ctx context.Context, target geometry.AssemblyDragTarget) (MotionFrame, error) {
	if !validInteractionTarget(target) {
		return MotionFrame{}, fmt.Errorf("INPUT_ERROR: invalid spatial target")
	}
	if _, ok := e.state.Frame.UnitPoses[target.BodyID]; !ok {
		return MotionFrame{}, fmt.Errorf("INPUT_ERROR: spatial target outside mechanism")
	}
	r, err := e.solve(ctx, e.state.Equations.Constraints, &target)
	if err != nil {
		return MotionFrame{}, err
	}
	if r.Interaction == nil || !r.Interaction.TargetConverged || !r.Interaction.EligibleForCommit {
		return MotionFrame{}, fmt.Errorf("TARGET_UNCONFIRMED: spatial preference")
	}
	p, err := e.poses(r)
	if err != nil {
		return MotionFrame{}, err
	}
	coordinates, err := verifyMechanismJoints(e.snapshot.Mechanism, p, e.state.Frame.Coordinates, "", 0)
	if err != nil {
		return MotionFrame{}, err
	}
	if err = motionLimits(e.snapshot.Mechanism, coordinates); err != nil {
		return MotionFrame{}, err
	}
	f := MotionFrame{DriverValue: coordinates[e.driver.ID], UnitPoses: p, Coordinates: coordinates, AngleBranches: r.AngleBranches, KinematicValid: true, HardIterations: r.Iterations}
	e.publish(r, f)
	return f, nil
}
