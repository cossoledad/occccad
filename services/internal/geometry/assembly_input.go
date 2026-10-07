package geometry

import (
	"fmt"
	"sort"
	"strconv"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/valuecopy"
)

type AssemblyGeometryKey struct{ BodyID, ID string }

// CompileAssemblyInput allocates a solve-private namespace by identity, never
// by coordinates. Already compiled tables are idempotent across group/RPC/replay.
func CompileAssemblyInput(values []AssemblyGeometry, constraints []AssemblyConstraint) ([]AssemblyGeometry, []AssemblyConstraint, map[AssemblyGeometryKey]string, error) {
	return compileAssemblyInput(values, constraints, true)
}
func privateGeometryIndex(id string) (int, bool) {
	if len(id) < 7 || id[0] != 'g' {
		return 0, false
	}
	if len(id) > 7 {
		if id[1] < '1' || id[1] > '9' {
			return 0, false
		}
		for i := 2; i < len(id); i++ {
			if id[i] < '0' || id[i] > '9' {
				return 0, false
			}
		}
		n, err := strconv.Atoi(id[1:])
		return n, err == nil && n >= 1000000
	}
	n := 0
	for i := 1; i < 7; i++ {
		if id[i] < '0' || id[i] > '9' {
			return 0, false
		}
		n = n*10 + int(id[i]-'0')
	}
	return n, true
}
func privateGeometryID(index int) string {
	if index >= 1000000 {
		return "g" + strconv.Itoa(index)
	}
	id := [7]byte{'g', '0', '0', '0', '0', '0', '0'}
	for i := 6; i > 0; i-- {
		id[i] += byte(index % 10)
		index /= 10
	}
	return string(id[:])
}
func compileAssemblyInput(values []AssemblyGeometry, constraints []AssemblyConstraint, mapping bool) ([]AssemblyGeometry, []AssemblyConstraint, map[AssemblyGeometryKey]string, error) {
	values = append([]AssemblyGeometry(nil), values...)
	constraints = append([]AssemblyConstraint(nil), constraints...)
	canonical := true
	for i, g := range values {
		index, ok := privateGeometryIndex(g.ID)
		if !ok || index != i {
			canonical = false
			break
		}
	}
	if canonical {
		for _, g := range values {
			if g.BodyID == "" {
				return nil, nil, nil, fmt.Errorf("empty assembly geometry body identity")
			}
		}
		for _, c := range constraints {
			for _, ref := range []struct{ body, id string }{{c.FirstBodyID, c.FirstGeometryID}, {c.SecondBodyID, c.SecondGeometryID}, {c.AngleReferenceBodyID, c.AngleReferenceGeometryID}} {
				if ref.id == "" {
					continue
				}
				index, ok := privateGeometryIndex(ref.id)
				if !ok || index >= len(values) || values[index].BodyID != ref.body {
					return nil, nil, nil, fmt.Errorf("missing assembly geometry endpoint for body %s", ref.body)
				}
			}
		}
		var ids map[AssemblyGeometryKey]string
		if mapping {
			ids = make(map[AssemblyGeometryKey]string, len(values))
			for _, g := range values {
				ids[AssemblyGeometryKey{g.BodyID, g.ID}] = g.ID
			}
		}
		return values, constraints, ids, nil
	}
	if !canonical {
		sort.Slice(values, func(i, j int) bool {
			if values[i].BodyID != values[j].BodyID {
				return values[i].BodyID < values[j].BodyID
			}
			return values[i].ID < values[j].ID
		})
	}
	ids := make(map[AssemblyGeometryKey]string, len(values))
	for i := range values {
		g := &values[i]
		key := AssemblyGeometryKey{g.BodyID, g.ID}
		if key.ID == "" || key.BodyID == "" {
			return nil, nil, nil, fmt.Errorf("empty assembly geometry identity")
		}
		if _, exists := ids[key]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate assembly geometry identity within body %s", g.BodyID)
		}
		g.ID = privateGeometryID(i)
		ids[key] = g.ID
	}
	bind := func(body string, id *string) error {
		if *id == "" {
			return nil
		}
		next, ok := ids[AssemblyGeometryKey{body, *id}]
		if !ok {
			return fmt.Errorf("missing assembly geometry endpoint for body %s", body)
		}
		*id = next
		return nil
	}
	for i := range constraints {
		c := &constraints[i]
		for _, ref := range []struct {
			body string
			id   *string
		}{{c.FirstBodyID, &c.FirstGeometryID}, {c.SecondBodyID, &c.SecondGeometryID}, {c.AngleReferenceBodyID, &c.AngleReferenceGeometryID}} {
			if err := bind(ref.body, ref.id); err != nil {
				return nil, nil, nil, err
			}
		}
	}
	return values, constraints, ids, nil
}

// CompileAssemblyRequest is the shared production/replay numerical adapter.
func CompileAssemblyRequest(requestID string, bodies []AssemblyBody, values []AssemblyGeometry, constraints []AssemblyConstraint, options AssemblySolveOptions) (*workerv1.SolveAssemblyRequest, error) {
	values, constraints, _, err := compileAssemblyInput(values, constraints, false)
	if err != nil {
		return nil, err
	}
	// The encoder creates its own messages; detach the few borrowed slice/pointer fields.
	r := assemblySolveRequest(requestID, bodies, values, constraints, options)
	r.AffectedBodyIds = append([]string(nil), r.AffectedBodyIds...)
	if r.SolveIntent != nil {
		r.SolveIntent.MovingBodyIds = append([]string(nil), r.SolveIntent.MovingBodyIds...)
		r.SolveIntent.ReferenceBodyIds = append([]string(nil), r.SolveIntent.ReferenceBodyIds...)
	}
	if r.SolverProfile != nil && r.SolverProfile.MaxPreferenceIterations != nil {
		x := *r.SolverProfile.MaxPreferenceIterations
		r.SolverProfile.MaxPreferenceIterations = &x
	}
	for _, g := range r.Geometry {
		if g.ParameterStart != nil {
			x := *g.ParameterStart
			g.ParameterStart = &x
		}
		if g.ParameterEnd != nil {
			x := *g.ParameterEnd
			g.ParameterEnd = &x
		}
	}
	return r, nil
}

// AssemblyInputFromRequest reverses the production adapter without geometry resolution.
// Protobuf optional presence and integer branch winding are retained exactly.
func AssemblyInputFromRequest(r *workerv1.SolveAssemblyRequest) ([]AssemblyBody, []AssemblyGeometry, []AssemblyConstraint, AssemblySolveOptions, error) {
	vec := func(v *workerv1.Vec3) [3]float64 { return [3]float64{v.GetX(), v.GetY(), v.GetZ()} }
	quat := func(v *workerv1.Quaternion) [4]float64 { return [4]float64{v.GetX(), v.GetY(), v.GetZ(), v.GetW()} }
	pose := func(v *workerv1.RigidPose) AssemblyPose {
		return AssemblyPose{Translation: vec(v.GetTranslation()), Rotation: quat(v.GetRotation())}
	}
	var bodies []AssemblyBody
	var values []AssemblyGeometry
	var constraints []AssemblyConstraint
	for _, b := range r.Bodies {
		v := AssemblyBody{ID: b.Id, Pose: pose(b.InitialPose)}
		if b.InitialGuess != nil {
			p := pose(b.InitialGuess)
			v.InitialGuess = &p
		}
		bodies = append(bodies, v)
	}
	for _, g := range r.Geometry {
		values = append(values, AssemblyGeometry{ID: g.Id, BodyID: g.BodyId, Kind: g.Kind, Origin: vec(g.Origin), Direction: vec(g.Direction), Radius: g.Radius, HalfAngle: g.HalfAngle, ConeLeaf: g.ConeLeaf, MaterialSide: g.MaterialSide, Rotation: quat(g.Rotation), XDirection: vec(g.XDirection), ParameterStart: g.ParameterStart, ParameterEnd: g.ParameterEnd, LengthUnit: g.LengthUnit})
	}
	for _, c := range r.Constraints {
		v := AssemblyConstraint{ID: c.Id, ConnectionID: c.ConnectionId, GroupID: c.GroupId, Kind: c.Kind, Mode: c.Mode, FirstBodyID: c.First.GetBodyId(), FirstGeometryID: c.First.GetGeometryId(), SecondBodyID: c.Second.GetBodyId(), SecondGeometryID: c.Second.GetGeometryId(), AngleReferenceBodyID: c.AngleReference.GetBodyId(), AngleReferenceGeometryID: c.AngleReference.GetGeometryId(), ReverseAngleReference: c.ReverseAngleReference, Value: c.Value, DirectionRelation: c.DirectionRelation, DistanceRelation: c.DistanceRelation, ContactKind: c.ContactKind, ContactSide: c.ContactSide, ContactBranch: c.ContactBranch}
		if c.FixedPose != nil {
			p := pose(c.FixedPose)
			v.FixedPose = &p
		}
		if c.AngleReferenceDirection != nil {
			p := vec(c.AngleReferenceDirection)
			v.AngleReferenceDirection = &p
		}
		if c.SpatialAngleBranchDirection != nil {
			p := vec(c.SpatialAngleBranchDirection)
			v.SpatialAngleBranchDirection = &p
		}
		if c.AngleBranchState != nil {
			v.AngleBranchState = &AssemblyAngleBranchState{WrappedAngle: c.AngleBranchState.WrappedAngle, UnwrappedAngle: c.AngleBranchState.UnwrappedAngle, Winding: c.AngleBranchState.Winding}
		}
		constraints = append(constraints, v)
	}
	opts := AssemblySolveOptions{AffectedBodyIDs: append([]string(nil), r.AffectedBodyIds...), DisableConflictProbes: r.DisableConflictProbes}
	if d := r.DebugOptions; d != nil {
		opts.Debug = &AssemblyDebugOptions{CaptureEvaluations: d.CaptureEvaluations, CaptureMatrices: d.CaptureMatrices, ByteBudget: d.ByteBudget}
	}
	if i := r.SolveIntent; i != nil {
		opts.Intent = &AssemblySolveIntent{MovingBodyIDs: append([]string(nil), i.MovingBodyIds...), ReferenceBodyIDs: append([]string(nil), i.ReferenceBodyIds...), PreferencePolicy: i.PreferencePolicy}
	}
	if t := r.DragTarget; t != nil {
		if len(t.TranslationComponents) != 3 || len(t.RotationComponents) != 3 || len(t.HoldTranslationComponents) != 3 || len(t.HoldRotationComponents) != 3 {
			return nil, nil, nil, opts, fmt.Errorf("invalid drag component masks")
		}
		opts.DragTarget = &AssemblyDragTarget{BodyID: t.BodyId, LocalGrabPoint: vec(t.LocalGrabPoint), TargetPose: pose(t.TargetPose), FrameRotation: quat(t.FrameRotation), TargetSequence: t.TargetSequence}
		copy(opts.DragTarget.TranslationComponents[:], t.TranslationComponents)
		copy(opts.DragTarget.RotationComponents[:], t.RotationComponents)
		copy(opts.DragTarget.HoldTranslationComponents[:], t.HoldTranslationComponents)
		copy(opts.DragTarget.HoldRotationComponents[:], t.HoldRotationComponents)
	}
	if p := r.SolverProfile; p != nil {
		opts.SolverProfile = &AssemblySolverProfile{SchemaVersion: p.SchemaVersion, MaxIterations: p.MaxIterations, LengthTolerance: p.LengthTolerance, AngleTolerance: p.AngleTolerance, ClassificationLengthTolerance: p.ClassificationLengthTolerance, ClassificationAngleTolerance: p.ClassificationAngleTolerance, TranslationStepTolerance: p.TranslationStepTolerance, RotationStepTolerance: p.RotationStepTolerance, DegeneracyTolerance: p.DegeneracyTolerance, FiniteDifferenceStep: p.FiniteDifferenceStep, InitialDamping: p.InitialDamping, RankTolerance: p.RankTolerance, TranslationFiniteDifferenceStep: p.TranslationFiniteDifferenceStep, RotationFiniteDifferenceStep: p.RotationFiniteDifferenceStep, RankAbsoluteTolerance: p.RankAbsoluteTolerance, RankRelativeTolerance: p.RankRelativeTolerance, GradientTolerance: p.GradientTolerance, MotionLengthScale: p.MotionLengthScale, MotionAngleScale: p.MotionAngleScale, PreferenceTolerance: p.PreferenceTolerance, ObjectiveTolerance: p.ObjectiveTolerance, MaxPreferenceIterations: p.MaxPreferenceIterations, MaxConflictProbes: p.MaxConflictProbes, VerifyAnalyticJacobians: p.VerifyAnalyticJacobians, JacobianCheckTolerance: p.JacobianCheckTolerance}
	}
	values, err := valuecopy.Clone(values)
	if err != nil {
		return nil, nil, nil, opts, err
	}
	opts, err = valuecopy.Clone(opts)
	if err != nil {
		return nil, nil, nil, opts, err
	}
	return bodies, values, constraints, opts, nil
}
