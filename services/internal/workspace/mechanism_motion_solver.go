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

// One numerical path serves command trials and recorded vector segments. The
// checkpoint contains only fully qualified complete states, never private steps.
type mechanismMotionState struct {
	Equations           mechanismEquations
	Frame               MotionFrame
	Drivers             []MotionDriver
	Qualified           bool
	PhysicalConstraints []geometry.AssemblyConstraint
}
type mechanismMotionSolver struct {
	service            *Service
	snapshot           MotionSnapshot
	state              mechanismMotionState
	physicalComponents []geometry.AssemblyComponentDof
	solverBuild        string
	calls              int
	last               *geometry.AssemblySolve
	transport          codes.Code
	replay             []byte
}

func newMechanismMotionSolver(ctx context.Context, service *Service, s MotionSnapshot) (*mechanismMotionSolver, error) {
	eq, err := valuecopy.Clone(s.Equations)
	if err != nil {
		return nil, err
	}
	e := &mechanismMotionSolver{service: service, snapshot: s, state: mechanismMotionState{Equations: eq}}
	r, err := e.solve(ctx, eq.Constraints, nil)
	if err != nil {
		return e, err
	}
	e.physicalComponents = r.Components
	poses, err := e.poses(r)
	if err != nil {
		return e, err
	}
	coordinates, err := verifyMechanismJoints(s.Mechanism, poses, s.Mechanism.Coordinates, nil)
	if err != nil {
		return e, err
	}
	if err = motionLimits(s.Mechanism, coordinates); err != nil {
		return e, err
	}
	e.publish(r, MotionFrame{UnitPoses: poses, Coordinates: coordinates, CommandValues: commandValues(s.Mechanism, s.Drivers, coordinates), AngleBranches: r.AngleBranches, KinematicValid: true, HardIterations: r.Iterations})
	if len(s.Study.ActiveDriverIDs) > 0 {
		if err = e.SelectDrivers(ctx, s.Study.ActiveDriverIDs); err != nil {
			return e, err
		}
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
func (e *mechanismMotionSolver) Advance(ctx context.Context, targets map[string]float64) (MotionFrame, error) {
	if !e.state.Qualified || len(targets) != len(e.state.Drivers) {
		return MotionFrame{}, fmt.Errorf("QUALIFICATION_REQUIRED: select a complete independent drive set")
	}
	for _, d := range e.state.Drivers {
		q, ok := targets[d.CoordinateID]
		if !ok || !finiteMotion(q) || !finiteMotion(q-e.state.Frame.Coordinates[d.CoordinateID]) {
			return MotionFrame{}, fmt.Errorf("INPUT_ERROR: incomplete or non-finite target vector")
		}
		_, coordinate, _ := driverCoordinate(e.snapshot.Mechanism, d)
		if err := motionCoordinateLimit(coordinate, q); err != nil {
			return MotionFrame{}, err
		}
	}
	before, err := valuecopy.Clone(e.state)
	if err != nil {
		return MotionFrame{}, err
	}
	f, err := e.advance(ctx, targets, 0)
	if err != nil {
		e.state = before
		return MotionFrame{}, err
	}
	return f, nil
}

// The rank difference of physical and driven tangent spaces proves independent
// coverage. Inactive physical components are held at their accepted pose only.
func (e *mechanismMotionSolver) SelectDrivers(ctx context.Context, ids []string) (err error) {
	before, err := valuecopy.Clone(e.state)
	if err != nil {
		return err
	}
	original, err := valuecopy.Clone(e.snapshot)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			e.state = before
			e.snapshot = original
		}
	}()
	ds, err := snapshotDrivers(e.snapshot, ids)
	if err != nil {
		return err
	}
	if len(ds) == 0 {
		return fmt.Errorf("QUALIFICATION_REQUIRED: select commands")
	}
	for _, d := range ds {
		j, c, er := driverCoordinate(e.snapshot.Mechanism, d)
		if er != nil {
			return er
		}
		if d.Reference == nil || d.ReferenceDigest != commandCoordinateBasis(j, c.ID) {
			return fmt.Errorf("INPUT_ERROR: COMMAND_REFERENCE_REVALIDATION_REQUIRED")
		}
		if c.Lower == nil || c.Upper == nil {
			return fmt.Errorf("INPUT_ERROR: driven coordinate requires both limits")
		}
	}
	cs := append([]geometry.AssemblyConstraint(nil), e.state.Equations.Constraints...)
	for _, component := range e.physicalComponents {
		active := false
		for _, d := range ds {
			j, _, _ := driverCoordinate(e.snapshot.Mechanism, d)
			active = active || slices.Contains(component.BodyIDs, j.First.InstanceID) || slices.Contains(component.BodyIDs, j.Second.InstanceID)
		}
		if !active {
			for _, id := range component.BodyIDs {
				p := geometry.AssemblyPose(before.Frame.UnitPoses[id])
				cs = append(cs, geometry.AssemblyConstraint{ID: "mechanism/inactive/" + id, Kind: "FIX", FirstBodyID: id, FixedPose: &p})
			}
		}
	}
	physical, err := e.solve(ctx, cs, nil)
	if err != nil {
		return err
	}
	dof, gauge := motionDof(physical)
	if gauge != 0 || dof == 0 {
		return fmt.Errorf("UNSUPPORTED_DOF: active physical relative DOF=%d gauge=%d", dof, gauge)
	}
	e.state.Drivers = ds
	e.state.PhysicalConstraints = cs
	e.state.Qualified = true
	targets := map[string]float64{}
	for _, d := range ds {
		targets[d.CoordinateID] = before.Frame.Coordinates[d.CoordinateID]
	}
	driven := e.coordinateConstraints(targets)
	r, err := e.solve(ctx, append(append([]geometry.AssemblyConstraint(nil), cs...), driven...), nil)
	if err != nil {
		return err
	}
	remaining, gauge := motionDof(r)
	if remaining != 0 || gauge != 0 || dof-remaining != uint64(len(ds)) {
		return fmt.Errorf("UNSUPPORTED_DOF: independent physical DOF=%d driven DOF=%d gauge=%d selected=%d", dof, remaining, gauge, len(ds))
	}
	p, err := e.poses(r)
	if err != nil {
		return err
	}
	coordinates, err := verifyMechanismJoints(e.snapshot.Mechanism, p, before.Frame.Coordinates, targets)
	if err != nil {
		return err
	}
	for id, pose := range before.Frame.UnitPoses {
		if !conflictPoseClose(geometry.AssemblyPose(p[id]), geometry.AssemblyPose(pose), e.snapshot.SolverProfile.LengthTolerance, e.snapshot.SolverProfile.AngleTolerance) {
			return fmt.Errorf("LOCAL_SOLVE_FAILED: drive selection moved checkpoint")
		}
	}
	e.snapshot.Study.ActiveDriverIDs = append([]string(nil), ids...)
	e.snapshot.Study.References = commandReferences(ds)
	e.snapshot.Digest = motionDigest(e.snapshot)
	e.publish(r, MotionFrame{UnitPoses: p, Coordinates: coordinates, CommandValues: commandValues(e.snapshot.Mechanism, e.snapshot.Drivers, coordinates), AngleBranches: r.AngleBranches, KinematicValid: true, HardIterations: r.Iterations})
	return nil
}
func (e *mechanismMotionSolver) coordinateConstraints(targets map[string]float64) []geometry.AssemblyConstraint {
	cs := []geometry.AssemblyConstraint{}
	for _, driver := range e.state.Drivers {
		j, c, _ := driverCoordinate(e.snapshot.Mechanism, driver)
		d := mechanismCoordinateEquation(e.snapshot.Mechanism.ID, j, c)
		zero, _ := motionValue(c.Zero, c.angular())
		physical := zero + float64(c.Direction)*targets[c.ID]
		if c.angular() {
			d.Value = wrappedMotionAngle(physical)
			prior := zero + float64(c.Direction)*e.state.Frame.Coordinates[c.ID]
			wrapped := math.Remainder(prior, 2*math.Pi)
			d.AngleBranchState = &geometry.AssemblyAngleBranchState{WrappedAngle: wrapped, UnwrappedAngle: prior, Winding: int64(math.Round((prior - wrapped) / (2 * math.Pi)))}
		} else {
			d.Value = -physical
		}
		cs = append(cs, d)
	}
	return cs
}

// Auto-select only a unique qualified basis. A bounded ambiguity search leaves
// selection to the user when the candidate space is too large to prove unique.
func (e *mechanismMotionSolver) UniqueDriveSet(ctx context.Context) ([]string, error) {
	ds := e.snapshot.Drivers
	if len(ds) == 0 || len(ds) > 7 {
		return nil, nil
	}
	before, err := valuecopy.Clone(e.state)
	if err != nil {
		return nil, err
	}
	snapshot, err := valuecopy.Clone(e.snapshot)
	if err != nil {
		return nil, err
	}
	defer func() { e.state = before; e.snapshot = snapshot }()
	var unique []string
	for mask := 1; mask < 1<<len(ds); mask++ {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		ids := []string{}
		for i, d := range ds {
			if mask&(1<<i) != 0 {
				ids = append(ids, d.ID)
			}
		}
		e.state, _ = valuecopy.Clone(before)
		e.snapshot, _ = valuecopy.Clone(snapshot)
		if er := e.SelectDrivers(ctx, ids); er == nil {
			if unique != nil {
				return nil, nil
			}
			unique = ids
		} else if e.transport == codes.Unavailable || e.transport == codes.Internal {
			return nil, er
		}
	}
	return unique, nil
}
func (e *mechanismMotionSolver) advance(ctx context.Context, targets map[string]float64, depth int) (MotionFrame, error) {
	if err := ctx.Err(); err != nil {
		return MotionFrame{}, err
	}
	large := false
	midpoint := map[string]float64{}
	maxStep := 0.0
	for _, driver := range e.state.Drivers {
		_, c, _ := driverCoordinate(e.snapshot.Mechanism, driver)
		current := e.state.Frame.Coordinates[c.ID]
		step := targets[c.ID] - current
		scale := e.snapshot.LinearStepMM
		if c.angular() {
			scale = math.Pi / 36
		}
		large = large || math.Abs(step) > scale
		maxStep = math.Max(maxStep, math.Abs(step)/scale)
		midpoint[c.ID] = current + step/2
	}
	if large {
		if depth >= 12 {
			return MotionFrame{}, fmt.Errorf("BUDGET_EXHAUSTED: mixed coordinate continuation")
		}
		if _, err := e.advance(ctx, midpoint, depth+1); err != nil {
			return MotionFrame{}, err
		}
		return e.advance(ctx, targets, depth+1)
	}
	cs := append(append([]geometry.AssemblyConstraint(nil), e.state.PhysicalConstraints...), e.coordinateConstraints(targets)...)
	r, err := e.solve(ctx, cs, nil)
	var p map[string]InstancePose
	var coordinates map[string]float64
	if err == nil {
		dof, gauge := motionDof(r)
		if dof != 0 || gauge != 0 {
			err = fmt.Errorf("UNSUPPORTED_DOF: driven DOF=%d gauge=%d", dof, gauge)
		}
	}
	if err == nil {
		p, err = e.poses(r)
	}
	if err == nil {
		coordinates, err = verifyMechanismJoints(e.snapshot.Mechanism, p, e.state.Frame.Coordinates, targets)
	}
	if err == nil {
		// A passive spherical/planar connection has no scalar angle with which
		// to detect a finite branch jump. Check the full body poses as well and
		// let the same bounded subdivision resolve legitimate large motion.
		for id, pose := range p {
			prior := e.state.Frame.UnitPoses[id]
			dot := math.Abs(pose.Rotation[0]*prior.Rotation[0] + pose.Rotation[1]*prior.Rotation[1] + pose.Rotation[2]*prior.Rotation[2] + pose.Rotation[3]*prior.Rotation[3])
			if dot < math.Cos(math.Pi/4) || conflictNorm(conflictSub(pose.Translation, prior.Translation)) > 10*e.snapshot.LinearStepMM {
				err = fmt.Errorf("LOCAL_SOLVE_FAILED: body %s branch discontinuity", id)
				break
			}
		}
	}
	if err == nil {
		if err = motionLimits(e.snapshot.Mechanism, coordinates); err != nil {
			return MotionFrame{}, err
		}
	}
	if err != nil {
		transport := status.Code(err)
		if depth >= 12 || maxStep < 1e-8 || ctx.Err() != nil || e.calls >= 2048 || r.Status == "INVALID_MODEL" || transport == codes.InvalidArgument || transport == codes.Unavailable || transport == codes.Internal || strings.HasPrefix(err.Error(), "UNSUPPORTED_DOF") {
			return MotionFrame{}, err
		}
		if _, err = e.advance(ctx, midpoint, depth+1); err != nil {
			return MotionFrame{}, err
		}
		return e.advance(ctx, targets, depth+1)
	}
	f := MotionFrame{UnitPoses: p, Coordinates: coordinates, CommandValues: commandValues(e.snapshot.Mechanism, e.snapshot.Drivers, coordinates), AngleBranches: r.AngleBranches, KinematicValid: true, HardIterations: r.Iterations}
	e.publish(r, f)
	return f, nil
}
func (e *mechanismMotionSolver) Drag(ctx context.Context, target geometry.AssemblyDragTarget) (MotionFrame, error) {
	if !e.state.Qualified {
		return MotionFrame{}, fmt.Errorf("QUALIFICATION_REQUIRED: select commands")
	}
	if !validInteractionTarget(target) {
		return MotionFrame{}, fmt.Errorf("INPUT_ERROR: invalid spatial target")
	}
	if _, ok := e.state.Frame.UnitPoses[target.BodyID]; !ok {
		return MotionFrame{}, fmt.Errorf("INPUT_ERROR: spatial target outside mechanism")
	}
	r, err := e.solve(ctx, e.state.PhysicalConstraints, &target)
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
	coordinates, err := verifyMechanismJoints(e.snapshot.Mechanism, p, e.state.Frame.Coordinates, nil)
	if err != nil {
		return MotionFrame{}, err
	}
	if err = motionLimits(e.snapshot.Mechanism, coordinates); err != nil {
		return MotionFrame{}, err
	}
	f := MotionFrame{CommandValues: commandValues(e.snapshot.Mechanism, e.snapshot.Drivers, coordinates), UnitPoses: p, Coordinates: coordinates, AngleBranches: r.AngleBranches, KinematicValid: true, HardIterations: r.Iterations}
	before, _ := valuecopy.Clone(e.state)
	e.publish(r, f)
	if err = e.SelectDrivers(ctx, e.snapshot.Study.ActiveDriverIDs); err != nil {
		e.state = before
		return MotionFrame{}, err
	}
	// The probe qualifies coordinates, while the gesture's own target evidence
	// remains the interaction outcome consumed by viewport finalization.
	e.last = &r
	return e.state.Frame, nil
}
