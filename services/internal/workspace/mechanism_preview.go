package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/valuecopy"
)

// A command preview is not a run, publishing plan, or committable assembly
// candidate. Only the selected mechanism's explicitly adopted equations enter.
type MechanismPreviewRequest struct {
	RequestID      string         `json:"requestId,omitempty"`
	BaseRevisionID string         `json:"baseRevisionId"`
	Mechanism      Mechanism      `json:"mechanism"`
	DraftJointID   string         `json:"draftJointId,omitempty"`
	Drivers        []MotionDriver `json:"-"`
	NominalProduct *ProductModel  `json:"-"` // trusted application source, includes nonmember environment
}
type MechanismPreview struct {
	BaseRevisionID string                 `json:"baseRevisionId"`
	Mechanism      Mechanism              `json:"mechanism"`
	InstancePoses  []MechanismPreviewPose `json:"instancePoses"`
}
type MechanismPreviewPose struct {
	InstanceID string `json:"instanceId"`
	InstancePose
}

func mechanismProductInput(model ProductModel, m Mechanism) (ProductModel, error) {
	input, err := mechanismPoseModel(model, m)
	if err != nil {
		return input, err
	}
	for _, id := range m.SupplementalConstraintIDs {
		i := slices.IndexFunc(model.Constraints, func(c AssemblyConstraint) bool { return c.ID == id })
		if i < 0 {
			return input, fmt.Errorf("%w: missing supplemental relationship %s", ErrValidation, id)
		}
		for _, j := range m.Joints {
			for _, src := range j.Sources {
				if src.ConstraintID == id {
					return input, fmt.Errorf("%w: duplicate source equation %s", ErrValidation, id)
				}
			}
		}
		input.Constraints = append(input.Constraints, model.Constraints[i])
	}
	return input, nil
}

func (service *Service) PreviewMechanism(ctx context.Context, documentID string, req MechanismPreviewRequest) (_ MechanismPreview, returnErr error) {
	ctx, cancel := context.WithTimeout(withAssemblyReadCache(ctx), 20*time.Second)
	defer cancel()
	out := MechanismPreview{BaseRevisionID: req.BaseRevisionID}
	view, err := service.GetDocument(ctx, documentID)
	if err != nil {
		return out, err
	}
	if view.Product == nil || req.BaseRevisionID == "" || view.Document.VersionID != req.BaseRevisionID {
		return out, fmt.Errorf("%w: Product revision changed", ErrValidation)
	}
	if len(view.Product.Instances) > 64 || len(req.Mechanism.Joints) > 128 {
		return out, fmt.Errorf("%w: mechanism preview size limit", ErrValidation)
	}
	if len(req.RequestID) > 128 {
		return out, fmt.Errorf("%w: requestId size", ErrValidation)
	}
	if req.RequestID == "" {
		req.RequestID = uuid.NewString()
	}
	diagnostic := service.newOperationDiagnostic(ctx, documentID, CommandRequest{Type: "MECHANISM_PREVIEW", RequestID: req.RequestID, Kinematics: &KinematicsDefinitions{Mechanisms: []Mechanism{req.Mechanism}}}, "MECHANISM_PREVIEW")
	if diagnostic != nil {
		diagnostic.BaseRevisionID = req.BaseRevisionID
		diagnostic.BaseModel, _ = json.Marshal(view.Product)
		diagnostic.stage("COMPILE_MECHANISM")
	}
	defer func() { service.finishOperationDiagnostic(ctx, diagnostic, nil, nil, &returnErr) }()
	m, err := valuecopy.Clone(req.Mechanism)
	if err != nil {
		return out, err
	}
	// The definition service owns capture, even for a direct preview request.
	// New rigid/fixed baselines cannot be supplied as a fabricated client witness.
	for i, j := range m.Joints {
		if j.Kind != "RIGID" && j.Kind != "GROUND" {
			continue
		}
		m.Joints[i].Constraints = nil
		for _, saved := range view.Product.Kinematics.Mechanisms {
			if saved.ID != m.ID {
				continue
			}
			for _, prior := range saved.Joints {
				if prior.ID == j.ID && prior.Kind == j.Kind && prior.First.InstanceID == j.First.InstanceID && (prior.Second == nil && j.Second == nil || prior.Second != nil && j.Second != nil && prior.Second.InstanceID == j.Second.InstanceID) {
					m.Joints[i].Constraints = prior.Constraints
				}
			}
		}
	}
	// The sole unfinished joint may preview axis alignment before planes exist.
	var partial *MechanismJoint
	complete := make([]MechanismJoint, 0, len(m.Joints))
	for _, j := range m.Joints {
		if j.ID == req.DraftJointID && (j.Kind == "REVOLUTE" || j.Kind == "PRISMATIC") && j.Second != nil && (j.First.Plane == nil || j.Second.Plane == nil) {
			if partial != nil || j.First.Axis == nil || j.Second.Axis == nil || j.First.InstanceID == j.Second.InstanceID || !slices.Contains([]string{"REVOLUTE", "PRISMATIC"}, j.Kind) {
				return out, fmt.Errorf("%w: incomplete joint axes", ErrValidation)
			}
			copy := j
			partial = &copy
		} else {
			complete = append(complete, j)
		}
	}
	m.Joints = complete
	// Unit membership also includes the unfinished joint, so it is not frozen.
	m.UnitIDs = append([]string(nil), req.Mechanism.UnitIDs...)
	for _, j := range req.Mechanism.Joints {
		m.UnitIDs = append(m.UnitIDs, j.First.InstanceID)
		if j.Second != nil {
			m.UnitIDs = append(m.UnitIDs, j.Second.InstanceID)
		}
	}
	slices.Sort(m.UnitIDs)
	m.UnitIDs = slices.Compact(m.UnitIDs)
	if len(m.Poses) > 0 {
		m.Poses = mechanismMemberPoses(m, m.Poses)
		for _, v := range view.Product.Instances {
			if slices.Contains(m.UnitIDs, v.ID) {
				if _, ok := m.Poses[v.ID]; !ok {
					m.Poses[v.ID] = InstancePose{Translation: v.Translation, Rotation: v.Rotation}
				}
			}
		}
	}
	validation := req.Mechanism
	validation.UnitIDs = m.UnitIDs
	validation.Poses = m.Poses
	for _, id := range m.UnitIDs {
		if !slices.ContainsFunc(view.Product.Instances, func(v ProductInstance) bool { return v.ID == id }) {
			return out, fmt.Errorf("%w: missing motion unit", ErrValidation)
		}
	}
	if err = validateKinematics(*view.Product, KinematicsDefinitions{Mechanisms: []Mechanism{validation}}); err != nil {
		return out, err
	}

	nominal := *view.Product
	if req.NominalProduct != nil {
		nominal = *req.NominalProduct
	}
	// A new prismatic definition can start with perpendicular world axes. Its
	// projected orientation angle is undefined there. First obtain a qualified
	// axis-alignment draft using the same retained physical relations; use that
	// draft as the construction baseline for the complete definition. Neither
	// stage publishes a Revision or a complete-joint witness on its own.
	for _, joint := range m.Joints {
		if joint.ID != req.DraftJointID || joint.Kind != "PRISMATIC" || joint.Second == nil || joint.First.Plane == nil || joint.Second.Plane == nil {
			continue
		}
		alignment, e := valuecopy.Clone(m)
		if e != nil {
			return out, e
		}
		for i := range alignment.Joints {
			if alignment.Joints[i].ID == joint.ID {
				alignment.Joints[i].First.Plane = nil
				alignment.Joints[i].Second.Plane = nil
			}
		}
		seed, e := service.PreviewMechanism(ctx, documentID, MechanismPreviewRequest{BaseRevisionID: req.BaseRevisionID, Mechanism: alignment, DraftJointID: joint.ID, Drivers: []MotionDriver{}, NominalProduct: req.NominalProduct})
		if e != nil {
			return out, e
		}
		m.Poses = seed.Mechanism.Poses
	}
	input, err := mechanismProductInput(nominal, m)
	if err != nil {
		return out, err
	}
	if m, err = service.resolveMechanismGeometry(ctx, input, m, true); err != nil {
		return out, err
	}
	pendingRigid := false
	qualified := m
	qualified.Joints = nil
	for _, j := range m.Joints {
		if j.Kind == "RIGID" && len(j.Constraints) == 0 {
			pendingRigid = true
		} else {
			qualified.Joints = append(qualified.Joints, j)
		}
	}
	if pendingRigid {
		first, e := service.PreviewMechanism(ctx, documentID, MechanismPreviewRequest{BaseRevisionID: req.BaseRevisionID, Mechanism: qualified, Drivers: []MotionDriver{}, NominalProduct: req.NominalProduct})
		if e != nil {
			return out, e
		}
		m.Poses = first.Mechanism.Poses
		input, e = mechanismProductInput(nominal, m)
		if e != nil {
			return out, e
		}
	}
	if m, err = service.bindMechanismConstraints(ctx, input, m); err != nil {
		return out, err
	}
	appendMechanismConstraints(&input, m)
	frozen, err := service.FreezeAssemblyInput(ctx, documentID, req.BaseRevisionID, input)
	if err != nil {
		return out, err
	}
	equations, err := compileMechanismEquations(frozen, m, MotionStudy{}, false)
	if err != nil {
		return out, err
	}
	if partial != nil {
		resolver := newAssemblySupportResolver(ctx, service, &input)
		refs := []*AssemblyGeometryRef{partial.First.Axis, partial.Second.Axis}
		ends := []JointEndpoint{partial.First, *partial.Second}
		ids := []string{"preview/axis/a", "preview/axis/b"}
		for i, ref := range refs {
			if ref.InstanceID != ends[i].InstanceID || !slices.ContainsFunc(input.Instances, func(v ProductInstance) bool { return v.ID == ref.InstanceID }) {
				return out, fmt.Errorf("%w: axis belongs to another motion unit", ErrValidation)
			}
			bound := *ref
			if err = service.bindAssemblyPick(ctx, &input, &bound); err != nil {
				return out, err
			}
			value, e := resolver.resolve(bound)
			if e != nil {
				return out, e
			}
			if value.Kind != "AXIS" {
				return out, fmt.Errorf("%w: joint requires axial support", ErrValidation)
			}
			value.ID = ids[i]
			equations.Geometry = append(equations.Geometry, value)
		}
		direction := "SAME"
		if partial.Kind == "PRISMATIC" && partial.AxisDirection == "OPPOSITE" {
			direction = "OPPOSITE"
		}
		equations.Constraints = append(equations.Constraints, geometry.AssemblyConstraint{ID: "preview/axis", Kind: "CONCENTRIC", FirstBodyID: ends[0].InstanceID, FirstGeometryID: ids[0], SecondBodyID: ends[1].InstanceID, SecondGeometryID: ids[1], DirectionRelation: direction})
	}
	profile := defaultAssemblySolverProfile()
	var intent *geometry.AssemblySolveIntent
	previewJoint := partial
	for i := range m.Joints {
		if m.Joints[i].ID == req.DraftJointID && m.Joints[i].Second != nil {
			previewJoint = &m.Joints[i]
		}
	}
	if previewJoint != nil {
		intent = &geometry.AssemblySolveIntent{MovingBodyIDs: []string{previewJoint.First.InstanceID}, ReferenceBodyIDs: []string{previewJoint.Second.InstanceID}, PreferencePolicy: "MOVE_FIRST_MINIMIZE_REFERENCE"}
	}
	var capture func([]byte, error)
	if diagnostic != nil {
		diagnostic.stage("SOLVE_MECHANISM")
		capture = func(data []byte, e error) {
			if e == nil {
				diagnostic.recordAssemblyAttempt(data)
			}
		}
	}
	result, err := service.worker.SolveAssemblyWithOptions(ctx, "mechanism-preview/"+req.RequestID, equations.Bodies, equations.Geometry, equations.Constraints, geometry.AssemblySolveOptions{SolverProfile: &profile, Intent: intent, CaptureReplay: capture})
	if err != nil {
		return out, err
	}
	manifest := AssemblySolveManifest{Bodies: equations.Bodies, Geometry: equations.Geometry, Constraints: equations.Constraints, SolverProfile: profile}
	if result.Status != "CONVERGED" {
		code, retryable := assemblySolveFailureDetails(result.Status)
		return out, &assemblySolveFailure{status: result.Status, code: code, retryable: retryable, phase: "SOLVE_MECHANISM", diagnostic: result.Diagnostic}
	}
	if err = validateAssemblyPreference(result); err != nil {
		return out, &assemblySolveFailure{status: "PREFERENCE_NOT_CONVERGED", code: "ASSEMBLY_PREFERENCE_NOT_CONVERGED", phase: "SOLVE_MECHANISM", diagnostic: err.Error()}
	}
	if ok, why := conflictIndependentWitness(manifest, result); !ok {
		return out, fmt.Errorf("%w: mechanism preview witness: %s", ErrValidation, why)
	}
	poses := map[string]InstancePose{}
	for _, body := range result.Bodies {
		p := InstancePose(body.Pose)
		if !validMotionPose(p) {
			return out, fmt.Errorf("%w: invalid preview pose", ErrValidation)
		}
		poses[body.ID] = p
	}
	for _, body := range equations.Bodies {
		p, ok := poses[body.ID]
		if !ok {
			return out, fmt.Errorf("%w: incomplete preview frame", ErrValidation)
		}
		out.InstancePoses = append(out.InstancePoses, MechanismPreviewPose{InstanceID: body.ID, InstancePose: p})
	}
	// Capture each new coordinate's zero at the first physically accepted
	// configuration; no later read, trial or playback recaptures it.
	for ji, j := range m.Joints {
		for ci, c := range j.Coordinates {
			known := false
			for _, old := range view.Product.Kinematics.Mechanisms {
				if old.ID == m.ID && mechanismHasCoordinate(old, c.ID) {
					known = true
				}
			}
			if !known {
				raw, err := motionJointCoordinate(j, c, poses, nil)
				if err != nil {
					return out, err
				}
				zero, _ := motionValue(c.Zero, c.angular())
				m.Joints[ji].Coordinates[ci].Zero = MotionQuantity{Value: zero + float64(c.Direction)*raw, Unit: c.unit()}
			}
		}
	}
	coordinates, err := verifyMechanismJoints(m, poses, m.Coordinates, nil)
	if err != nil {
		return out, fmt.Errorf("%w: %v", ErrValidation, err)
	}

	if err = motionLimits(m, coordinates); err != nil {
		return out, err
	}
	m.Coordinates = coordinates
	current, err := service.GetDocument(ctx, documentID)
	if err != nil {
		return out, err
	}
	if current.Document.VersionID != req.BaseRevisionID {
		return out, fmt.Errorf("%w: Product revision changed during preview", ErrValidation)
	}
	m.Poses = mechanismMemberPoses(m, poses)
	if partial == nil {
		relative, gauge := motionDof(result)
		m.Evaluation = &MechanismEvaluation{RelativeDOF: relative, GaugeDOF: gauge}
		drivers := req.Drivers
		if drivers == nil {
			drivers = view.Product.Kinematics.Drivers
		}
		for _, d := range drivers {
			if d.MechanismID == m.ID {
				if !mechanismHasCoordinate(m, d.CoordinateID) {
					return out, fmt.Errorf("%w: command joint missing", ErrValidation)
				}
				m.Evaluation.DriverCount++
			}
		}
		// Saved commands are candidates. Only an executing Session selects and
		// evaluates hard coordinate equations; definition evaluation keeps physical DOF.

	} else {
		m.Evaluation = nil
	}
	out.Mechanism = m
	if partial != nil {
		out.Mechanism.Joints = append(out.Mechanism.Joints, *partial)
	}
	return out, nil
}
