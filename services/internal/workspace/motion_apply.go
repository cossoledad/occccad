package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const typeApplyMotionFrame = "occccad://product/kinematics/apply-frame"

type MotionApplyRequest struct {
	JobID       string            `json:"jobId"`
	FrameIndex  int               `json:"frameIndex"`
	LockAngle   bool              `json:"lockAngle"`
	Resolutions map[string]string `json:"resolutions,omitempty"` // SUPPRESS | REPLACE, always explicit
	PlanDigest  string            `json:"planDigest,omitempty"`
}
type MotionApplyItem struct {
	JointID      string `json:"jointId,omitempty"`
	Role         string `json:"role"`
	ConstraintID string `json:"constraintId"`
	Action       string `json:"action"`
	Detail       string `json:"detail,omitempty"`
}
type MotionApplyPlan struct {
	BaseRevisionID   string            `json:"baseRevisionId"`
	Digest           string            `json:"digest"`
	Ready            bool              `json:"ready"`
	Items            []MotionApplyItem `json:"items"`
	PoseChanges      []string          `json:"poseChanges"`
	SolverStatus     string            `json:"solverStatus,omitempty"`
	DegreesOfFreedom uint64            `json:"degreesOfFreedom"`
	Candidate        ProductModel      `json:"-"`
}
type motionApplyPayload struct {
	Model ProductModel `json:"model"`
}

func semanticMotionReference(v AssemblyGeometryRef) AssemblyGeometryRef {
	v.Resolution = nil
	v.PublicationResolution = nil
	v.GeometryKey = ""
	v.TopologyID = 0
	if v.InstancePath != nil {
		p := *v.InstancePath
		p.Canonical = ""
		p.Display = ""
		p.Segments = append([]InstancePathSegment(nil), p.Segments...)
		for i := range p.Segments {
			p.Segments[i].InstanceName = ""
			if i == 0 {
				p.Segments[i].OwnerVersionID = ""
			}
		}
		v.InstancePath = &p
	}
	return v
}

func semanticConstraint(c AssemblyConstraint) string {
	if c.Mode == "" {
		c.Mode = "DRIVING"
	}
	if c.Kind != "DISTANCE" {
		c.DistanceRelation = ""
	}
	c.ID = ""
	c.ConnectionID = ""
	c.Name = ""
	c.Visible = nil
	c.EvaluationStatus = ""
	c.EvaluationSummary = ""
	c.EvaluationFailure = nil
	c.MeasuredValue = nil
	c.First = semanticMotionReference(c.First)
	if c.Second != nil {
		v := semanticMotionReference(*c.Second)
		c.Second = &v
	}
	if c.AngleAxis != nil {
		v := semanticMotionReference(*c.AngleAxis)
		c.AngleAxis = &v
	}
	source := any(nil)
	if c.QuantityParameter != nil {
		source = c.QuantityParameter.Source
	}
	c.QuantityParameter = nil
	return motionSemanticDigest(struct {
		Definition     AssemblyConstraint
		QuantitySource any
	}{c, source})
}
func motionSemanticDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func jointDefinitionDigest(j MechanismJoint) string {
	endpoint := func(e JointEndpoint) JointEndpoint {
		if e.Axis != nil {
			v := semanticMotionReference(*e.Axis)
			e.Axis = &v
			e.Frame = InstancePose{}
		}
		if e.Plane != nil {
			v := semanticMotionReference(*e.Plane)
			e.Plane = &v
		}
		return e
	}
	j.First = endpoint(j.First)
	if j.Second != nil {
		second := endpoint(*j.Second)
		j.Second = &second
	}
	relations := []struct{ ID, Definition string }{}
	for _, c := range j.Constraints {
		relations = append(relations, struct{ ID, Definition string }{c.ID, semanticConstraint(c)})
	}
	j.Constraints = nil
	j.Sources = nil
	return motionSemanticDigest(struct {
		Joint     MechanismJoint
		Relations any
	}{j, relations})
}
func (service *Service) readMotionResult(ctx context.Context, doc, actor, jobID string) (MotionRun, error) {
	var out MotionRun
	var objectID string
	if service.artifacts == nil {
		return out, fmt.Errorf("%w: artifact service unavailable", ErrValidation)
	}
	if e := service.database.QueryRow(ctx, `SELECT result_object_id::text FROM occccad.jobs WHERE id=$1 AND document_id=$2 AND requested_by_user_id=$3 AND job_type='MOTION_STUDY' AND state IN ('SUCCEEDED','CANCELED') AND result_object_id IS NOT NULL`, jobID, doc, actor).Scan(&objectID); e != nil {
		return out, fmt.Errorf("%w: accepted motion result unavailable", ErrValidation)
	}
	o, r, e := service.artifacts.Open(ctx, objectID)
	if e != nil {
		return out, e
	}
	defer r.Close()
	b, e := io.ReadAll(io.LimitReader(r, (16<<20)+1))
	if e != nil {
		return out, e
	}
	h := sha256.Sum256(b)
	if len(b) > 16<<20 || string(o.Kind) != "MOTION_RUN" || o.SHA256 != hex.EncodeToString(h[:]) {
		return out, fmt.Errorf("%w: motion artifact integrity", ErrValidation)
	}
	e = json.Unmarshal(b, &out)
	if e != nil {
		return out, e
	}
	if out.Snapshot.DocumentID != doc || out.Snapshot.Digest != motionDigest(out.Snapshot) {
		return out, fmt.Errorf("%w: frozen input integrity", ErrValidation)
	}
	return out, nil
}
func (service *Service) PlanMotionApply(ctx context.Context, doc, actor, base string, req MotionApplyRequest) (MotionApplyPlan, error) {
	run, e := service.readMotionResult(ctx, doc, actor, req.JobID)
	if e != nil {
		return MotionApplyPlan{}, e
	}
	view, e := service.GetDocument(ctx, doc)
	if e != nil {
		return MotionApplyPlan{}, e
	}
	if view.Product == nil || view.Document.VersionID != base {
		return MotionApplyPlan{}, fmt.Errorf("%w: MOTION_APPLY_BASE_CHANGED", ErrValidation)
	}
	return service.planMotionApply(ctx, doc, base, *view.Product, run, req)
}
func (service *Service) planMotionApply(ctx context.Context, doc, base string, model ProductModel, run MotionRun, req MotionApplyRequest) (MotionApplyPlan, error) {
	if len(req.Resolutions) > 0 {
		return MotionApplyPlan{}, fmt.Errorf("%w: conversion only adds mechanism relationships; edit existing assembly constraints separately", ErrValidation)
	}
	p := MotionApplyPlan{BaseRevisionID: base, Items: []MotionApplyItem{}, PoseChanges: []string{}}
	if req.FrameIndex < 0 || req.FrameIndex >= len(run.Frames) || !run.Frames[req.FrameIndex].KinematicValid {
		return p, fmt.Errorf("%w: select a valid mechanism frame", ErrValidation)
	}
	frame := run.Frames[req.FrameIndex]
	mid := run.Snapshot.Mechanism.ID
	mi := slices.IndexFunc(model.Kinematics.Mechanisms, func(m Mechanism) bool { return m.ID == mid })
	if mi < 0 {
		return p, fmt.Errorf("%w: mechanism deleted", ErrValidation)
	}
	mechanism, e := service.resolveMechanismGeometry(ctx, model, model.Kinematics.Mechanisms[mi], false)
	if e != nil {
		return p, e
	}
	if len(mechanism.Joints) != len(run.Snapshot.Mechanism.Joints) || !reflect.DeepEqual(mechanism.UnitIDs, run.Snapshot.Mechanism.UnitIDs) || !reflect.DeepEqual(mechanism.SupplementalConstraintIDs, run.Snapshot.Mechanism.SupplementalConstraintIDs) {
		return p, fmt.Errorf("%w: frozen mechanism changed; run again", ErrValidation)
	}
	for i, j := range mechanism.Joints {
		if jointDefinitionDigest(j) != jointDefinitionDigest(run.Snapshot.Mechanism.Joints[i]) {
			return p, fmt.Errorf("%w: frozen joint changed; run again", ErrValidation)
		}
	}
	old := run.Snapshot.View.Product
	if old == nil || len(old.Instances) != len(model.Instances) {
		return p, fmt.Errorf("%w: occurrence snapshot changed", ErrValidation)
	}
	next, e := cloneAssemblyProduct(model)
	if e != nil {
		return p, e
	}
	for i, v := range next.Instances {
		oi := slices.IndexFunc(old.Instances, func(o ProductInstance) bool { return o.ID == v.ID })
		pose, ok := frame.UnitPoses[v.ID]
		if oi < 0 || old.Instances[oi].ReferencedDocumentID != v.ReferencedDocumentID || old.Instances[oi].ReferencedVersionID != v.ReferencedVersionID || !ok || !validMotionPose(pose) {
			return p, fmt.Errorf("%w: occurrence/source snapshot changed", ErrValidation)
		}
		if v.Translation != pose.Translation || v.Rotation != pose.Rotation {
			p.PoseChanges = append(p.PoseChanges, v.ID)
		}
		next.Instances[i].Translation, next.Instances[i].Rotation = pose.Translation, pose.Rotation
	}
	newMappings := []MotionAssemblyMapping{}
	proposed := map[string]bool{}
	add := func(j MechanismJoint, role string, c AssemblyConstraint) error {
		b, _ := json.Marshal(c)
		var detached AssemblyConstraint
		if e := json.Unmarshal(b, &detached); e != nil {
			return e
		}
		c = detached
		c.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(doc+"/"+mid+"/"+j.ID+"/"+role)).String()
		c.Name = j.Name + " / " + role
		c.QuantityParameter = nil // new Product identity has its own scoped Quantity
		c.Mode = "DRIVING"
		c.DefinitionVersion = 2
		if err := canonicalAssemblyDefinition(&c, "", ""); err != nil {
			return err
		}
		if err := service.validatePublicAssemblySupports(ctx, &next, &c); err != nil {
			return err
		}
		c.ConnectionID = j.ID
		var previous *MotionAssemblyMapping
		for _, a := range model.Kinematics.Associations {
			if a.MechanismID == mid && a.JointID == j.ID && a.Role == role {
				copy := a
				previous = &copy
				c.ID = a.ConstraintID
				break
			}
		}
		idx := slices.IndexFunc(next.Constraints, func(v AssemblyConstraint) bool { return v.ID == c.ID })
		action := "ADD"
		if idx >= 0 {
			prior := next.Constraints[idx]
			c.QuantityParameter = prior.QuantityParameter
			c.DefinitionVersion = prior.DefinitionVersion
			action = "REUSE"
			if semanticConstraint(c) != semanticConstraint(prior) {
				action = "UPDATE"
			}
			if previous != nil && semanticConstraint(prior) != previous.ConstraintBaseline && semanticConstraint(c) != semanticConstraint(prior) {
				p.Items = append(p.Items, MotionApplyItem{j.ID, role, c.ID, "CONFLICT", "正式约束自上次关联基线后已修改；请在装配设计中处理该关系后重新规划"})
				return nil
			}
			if action == "REUSE" {
				c = prior
			}
			next.Constraints[idx] = c
		} else {
			next.Constraints = append(next.Constraints, c)
			idx = len(next.Constraints) - 1
		}
		empty := ""
		if e := func() error {
			if action == "REUSE" {
				return nil
			}
			return editAssemblyQuantity(&next, idx, &empty, "", c.Value)
		}(); e != nil {
			if _, supported := assemblyQuantitySpecification(c); supported {
				return e
			}
			if e = editAssemblyQuantity(&next, idx, nil, "", c.Value); e != nil {
				return e
			}
		}
		c = next.Constraints[idx]
		if e := validateInstanceConstraintReferences(c); e != nil {
			return e
		}
		proposed[c.ID] = true
		p.Items = append(p.Items, MotionApplyItem{j.ID, role, c.ID, action, ""})
		newMappings = append(newMappings, MotionAssemblyMapping{mid, j.ID, role, c.ID, jointDefinitionDigest(j), semanticConstraint(c)})
		return nil
	}
	for _, j := range mechanism.Joints {
		if j.Kind == "GROUND" {
			pose := frame.UnitPoses[j.First.InstanceID]
			if e = add(j, "ground", AssemblyConstraint{Kind: "FIX", FixMode: "SPACE", First: AssemblyGeometryRef{InstanceID: j.First.InstanceID, Kind: "BODY"}, FixedPose: &pose}); e != nil {
				return p, e
			}
			continue
		}
		if j.Second == nil || j.First.Axis == nil || j.First.Plane == nil || j.Second.Axis == nil || j.Second.Plane == nil {
			return p, fmt.Errorf("%w: publish requires persistent axis/plane supports", ErrValidation)
		}
		if len(j.Constraints) != 2 {
			return p, fmt.Errorf("%w: missing persisted Revolute constraints", ErrValidation)
		}
		for _, c := range j.Constraints {
			role := "axis"
			if c.Kind == "DISTANCE" {
				role = "axial-location"
			}
			if e = add(j, role, c); e != nil {
				return p, e
			}
		}
		a, b := *j.First.Axis, *j.Second.Axis
		if req.LockAngle {
			if j.First.CapturedX == nil || j.Second.CapturedX == nil {
				return p, fmt.Errorf("%w: captured angle baseline missing", ErrValidation)
			}
			a, b = *j.First.Axis, *j.Second.Axis
			a.CapturedDirection = j.First.CapturedX
			b.CapturedDirection = j.Second.CapturedX
			axis := *j.First.Axis
			value := 0.
			if j.Kind == "REVOLUTE" {
				q, er := motionJointCoordinate(j, frame.UnitPoses, nil)
				if er != nil {
					return p, er
				}
				zero, _ := motionValue(j.Zero, true)
				value = math.Mod(zero+float64(j.Direction)*q, 2*math.Pi)
				if value < 0 {
					value += 2 * math.Pi
				}
			}
			role := "orientation"
			if j.Kind == "REVOLUTE" {
				role = "angle-lock"
			}
			if e = add(j, role, AssemblyConstraint{Kind: "ANGLE", First: a, Second: &b, AngleAxis: &axis, AngleRelation: "DIRECTED", Value: value}); e != nil {
				return p, e
			}
		}
	}
	next.Kinematics.Associations = slices.DeleteFunc(next.Kinematics.Associations, func(a MotionAssemblyMapping) bool {
		return a.MechanismID == mid && (proposed[a.ConstraintID])
	})
	next.Kinematics.Associations = append(next.Kinematics.Associations, newMappings...)
	frozen, e := service.FreezeAssemblyInput(ctx, doc, base, next)
	if e != nil {
		return p, e
	}
	equations, e := interactionFrozenConstraints(frozen)
	if e != nil {
		return p, e
	}
	covered := map[string]bool{}
	for _, c := range equations {
		covered[c.ID] = true
		if c.GroupID != "" {
			covered[c.GroupID] = true
		}
	}
	for _, c := range next.Constraints {
		if !c.Suppressed && c.Mode != "MEASURED" && !covered[c.ID] {
			p.Items = append(p.Items, MotionApplyItem{Role: "existing", ConstraintID: c.ID, Action: "CONFLICT", Detail: "几何无效或完整方程未编译"})
		}
	}
	geo := map[string]geometry.AssemblyGeometry{}
	for _, g := range frozen.Geometry {
		geo[g.ID] = g
	}
	poses := map[string]geometry.AssemblyPose{}
	for id, v := range frame.UnitPoses {
		poses[id] = geometry.AssemblyPose(v)
	}
	for _, c := range equations {
		if c.Mode == "MEASURED" {
			continue
		}
		known, ok, why := conflictGeometryRelation(c, geo, poses, frozen.SolverProfile.LengthTolerance, frozen.SolverProfile.AngleTolerance)
		if !known || !ok {
			id := c.ID
			if c.GroupID != "" {
				id = c.GroupID
			}
			p.Items = append(p.Items, MotionApplyItem{Role: "existing", ConstraintID: id, Action: "CONFLICT", Detail: why})
		}
	}
	p.Ready = !slices.ContainsFunc(p.Items, func(i MotionApplyItem) bool { return i.Action == "CONFLICT" })
	if p.Ready {
		solveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		r, er := service.worker.SolveAssemblyWithOptions(solveCtx, "motion-apply/"+req.JobID, frozen.Bodies, frozen.Geometry, equations, geometry.AssemblySolveOptions{SolverProfile: &frozen.SolverProfile})
		if er != nil {
			return p, er
		}
		p.SolverStatus = r.Status
		for _, c := range r.Components {
			p.DegreesOfFreedom += c.RelativeDof + c.GaugeDof
		}
		if r.Status != "CONVERGED" {
			p.Ready = false
			p.Items = append(p.Items, MotionApplyItem{Role: "solver", Action: "CONFLICT", Detail: r.Status + ": " + r.Diagnostic})
		} else if ok, why := conflictIndependentWitness(frozen, r); !ok {
			return p, fmt.Errorf("%w: full assembly witness: %s", ErrValidation, why)
		}
		for _, b := range r.Bodies {
			if !conflictPoseClose(b.Pose, poses[b.ID], frozen.SolverProfile.LengthTolerance, frozen.SolverProfile.AngleTolerance) {
				return p, fmt.Errorf("%w: formal solve moved away from selected frame", ErrValidation)
			}
		}
	}
	sort.Strings(p.PoseChanges)
	p.Candidate = next
	p.Digest = motionSemanticDigest(struct {
		Base    string
		Request MotionApplyRequest
		Model   ProductModel
		Items   []MotionApplyItem
	}{base, MotionApplyRequest{JobID: req.JobID, FrameIndex: req.FrameIndex, LockAngle: req.LockAngle, Resolutions: req.Resolutions}, next, p.Items})
	return p, nil
}
func applyMotionFrame(raw, payload json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var before ProductModel
	var p motionApplyPayload
	if e := json.Unmarshal(raw, &before); e != nil {
		return nil, modelcore.ChangeSet{}, e
	}
	if e := json.Unmarshal(payload, &p); e != nil {
		return nil, modelcore.ChangeSet{}, e
	}
	next := p.Model
	set := modelcore.ChangeSet{}
	add := func(kind modelcore.ChangeKind, id, slot string, a, b any) {
		c, _ := modelcore.NewChange(kind, modelcore.PropertyAddress{EntityID: id, SlotID: slot}, a, b)
		set.Changes = append(set.Changes, c)
	}
	for _, c := range before.Constraints {
		if slices.IndexFunc(next.Constraints, func(v AssemblyConstraint) bool { return v.ID == c.ID }) < 0 {
			add(modelcore.ChangeDelete, c.ID, "assembly-constraint.entity", c, nil)
			set.ImpactSeeds = append(set.ImpactSeeds, modelcore.DependencyKey("assembly-constraint:"+c.ID))
		}
	}
	for _, c := range next.Constraints {
		i := slices.IndexFunc(before.Constraints, func(v AssemblyConstraint) bool { return v.ID == c.ID })
		if i < 0 {
			add(modelcore.ChangeCreate, c.ID, "assembly-constraint.entity", nil, c)
		} else if !reflect.DeepEqual(before.Constraints[i], c) {
			add(modelcore.ChangeUpdate, c.ID, "assembly-constraint.entity", before.Constraints[i], c)
		}
		set.ImpactSeeds = append(set.ImpactSeeds, modelcore.DependencyKey("assembly-constraint:"+c.ID))
	}
	for i, v := range next.Instances {
		old := before.Instances[i]
		a, b := InstancePose{Translation: old.Translation, Rotation: old.Rotation}, InstancePose{Translation: v.Translation, Rotation: v.Rotation}
		if a != b {
			add(modelcore.ChangeUpdate, v.ID, "instance.pose", a, b)
			set.ImpactSeeds = append(set.ImpactSeeds, modelcore.DependencyKey("instance:"+v.ID))
		}
	}
	add(modelcore.ChangeUpdate, "product", "product.kinematics", before.Kinematics, next.Kinematics)
	b, e := json.Marshal(next)
	return b, set, e
}
