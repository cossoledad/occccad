package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/geometry"
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
	m := req.Mechanism
	// The sole unfinished joint may preview axis alignment before planes exist.
	var partial *MechanismJoint
	complete := make([]MechanismJoint, 0, len(m.Joints))
	for _, j := range m.Joints {
		if j.ID == req.DraftJointID && j.Kind != "GROUND" && j.Second != nil && (j.First.Plane == nil || j.Second.Plane == nil) {
			if partial != nil || j.First.Axis == nil || j.Second.Axis == nil || j.First.InstanceID == j.Second.InstanceID || !slices.Contains([]string{"REVOLUTE"}, j.Kind) {
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
	input, err := mechanismProductInput(nominal, m)
	if err != nil {
		return out, err
	}
	if m, err = service.resolveMechanismGeometry(ctx, input, m, true); err != nil {
		return out, err
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
		equations.Constraints = append(equations.Constraints, geometry.AssemblyConstraint{ID: "preview/axis", Kind: "CONCENTRIC", FirstBodyID: ends[0].InstanceID, FirstGeometryID: ids[0], SecondBodyID: ends[1].InstanceID, SecondGeometryID: ids[1], DirectionRelation: "SAME"})
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
	if _, err = verifyMechanismJoints(m, poses, nil, "", 0); err != nil {
		return out, fmt.Errorf("%w: %v", ErrValidation, err)
	}
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
		m.Evaluation = &MechanismEvaluation{RelativeDOF: relative, GaugeDOF: gauge, DrivenDOF: relative}
		drivers := req.Drivers
		if drivers == nil {
			drivers = view.Product.Kinematics.Drivers
		}
		probe := equations
		for _, d := range drivers {
			if d.MechanismID == m.ID {
				j := slices.IndexFunc(m.Joints, func(j MechanismJoint) bool { return j.ID == d.JointID && j.Kind == "REVOLUTE" })
				if j < 0 {
					return out, fmt.Errorf("%w: command joint missing", ErrValidation)
				}
				q, e := motionJointCoordinate(m.Joints[j], poses, nil)
				if e != nil {
					return out, e
				}
				compiled, e := compileMechanismEquations(frozen, m, MotionStudy{DriverJointID: d.JointID}, true)
				if e != nil {
					return out, e
				}
				coordinate := compiled.Driver
				zero, _ := motionValue(m.Joints[j].Zero, true)
				coordinate.Value = math.Mod(zero+float64(m.Joints[j].Direction)*q, 2*math.Pi)
				if coordinate.Value < 0 {
					coordinate.Value += 2 * math.Pi
				}
				probe.Constraints = append(probe.Constraints, coordinate)
				m.Evaluation.DriverCount++
			}
		}
		if m.Evaluation.DriverCount > 0 {
			for i := range probe.Bodies {
				probe.Bodies[i].Pose = geometry.AssemblyPose(poses[probe.Bodies[i].ID])
				probe.Bodies[i].InitialGuess = nil
			}
			r, e := service.worker.SolveAssemblyWithOptions(ctx, "mechanism-commands/"+req.RequestID, probe.Bodies, probe.Geometry, probe.Constraints, geometry.AssemblySolveOptions{SolverProfile: &profile})
			if e != nil {
				return out, e
			}
			witness := AssemblySolveManifest{Bodies: probe.Bodies, Geometry: probe.Geometry, Constraints: probe.Constraints, SolverProfile: profile}
			if r.Status != "CONVERGED" {
				return out, fmt.Errorf("%w: command coverage solve: %s", ErrValidation, r.Status)
			}
			if e = validateAssemblyPreference(r); e != nil {
				return out, e
			}
			if ok, why := conflictIndependentWitness(witness, r); !ok {
				return out, fmt.Errorf("%w: command coverage witness: %s", ErrValidation, why)
			}
			m.Evaluation.DrivenDOF, _ = motionDof(r)
		}
	} else {
		m.Evaluation = nil
	}
	out.Mechanism = m
	if partial != nil {
		out.Mechanism.Joints = append(out.Mechanism.Joints, *partial)
	}
	return out, nil
}
