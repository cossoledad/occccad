package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"time"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const typeAdoptMechanismPose = "occccad://product/kinematics/adopt-pose"

// Empty target selects Product. The source identifies an accepted mechanism,
// never arbitrary browser poses. Resume explicitly restores that same mechanism.
type MechanismApplicationRequest struct {
	BaseRevisionID     string   `json:"baseRevisionId"`
	MechanismID        string   `json:"mechanismId,omitempty"`
	SourceMechanismID  string   `json:"sourceMechanismId,omitempty"`
	SourceMechanismIDs []string `json:"sourceMechanismIds,omitempty"`
	Resume             bool     `json:"resume,omitempty"`
}
type MechanismApplication struct {
	BaseRevisionID string                 `json:"baseRevisionId"`
	Mechanism      *Mechanism             `json:"mechanism,omitempty"`
	InstancePoses  []MechanismPreviewPose `json:"instancePoses"`
	Product        ProductModel           `json:"-"`
}

func (service *Service) PrepareMechanismApplication(ctx context.Context, documentID string, req MechanismApplicationRequest) (MechanismApplication, error) {
	ctx, cancel := context.WithTimeout(withAssemblyReadCache(ctx), 30*time.Second)
	defer cancel()
	out := MechanismApplication{BaseRevisionID: req.BaseRevisionID, InstancePoses: []MechanismPreviewPose{}}
	view, err := service.GetDocument(ctx, documentID)
	if err != nil {
		return out, err
	}
	if view.Product == nil || req.BaseRevisionID == "" || req.BaseRevisionID != view.Document.VersionID {
		return out, fmt.Errorf("%w: KINEMATICS_BASE_REVISION_CHANGED", ErrValidation)
	}
	next, err := cloneAssemblyProduct(*view.Product)
	if err != nil {
		return out, err
	}
	source := req.SourceMechanismID
	if req.Resume {
		if req.MechanismID == "" || (source != "" && source != req.MechanismID) {
			return out, fmt.Errorf("%w: invalid application resume", ErrValidation)
		}
		source = req.MechanismID
	}
	sources := append([]string(nil), req.SourceMechanismIDs...)
	if source != "" && !slices.Contains(sources, source) {
		sources = append(sources, source)
	}
	if len(sources) > 16 {
		return out, fmt.Errorf("%w: application source scope limit", ErrValidation)
	}
	seen := map[string]bool{}
	for _, source := range sources {
		if source == "" || seen[source] {
			return out, fmt.Errorf("%w: duplicate application source", ErrValidation)
		}
		seen[source] = true
		i := slices.IndexFunc(next.Kinematics.Mechanisms, func(m Mechanism) bool { return m.ID == source })
		if i < 0 {
			return out, fmt.Errorf("%w: missing source mechanism", ErrValidation)
		}
		m := next.Kinematics.Mechanisms[i]
		if len(m.UnitIDs) > 0 && len(m.Poses) != len(m.UnitIDs) {
			return out, fmt.Errorf("%w: source mechanism has no accepted pose", ErrValidation)
		}
		for n, v := range next.Instances {
			if slices.Contains(m.UnitIDs, v.ID) {
				p, ok := m.Poses[v.ID]
				if !ok || !validMotionPose(p) {
					return out, fmt.Errorf("%w: invalid source pose", ErrValidation)
				}
				next.Instances[n].Translation, next.Instances[n].Rotation = p.Translation, p.Rotation
			}
		}
	}
	if req.MechanismID != "" {
		i := slices.IndexFunc(next.Kinematics.Mechanisms, func(m Mechanism) bool { return m.ID == req.MechanismID })
		if i < 0 {
			return out, fmt.Errorf("%w: missing target mechanism", ErrValidation)
		}
		m := next.Kinematics.Mechanisms[i]
		m.Poses = map[string]InstancePose{}
		for _, v := range next.Instances {
			if slices.Contains(m.UnitIDs, v.ID) {
				m.Poses[v.ID] = InstancePose{Translation: v.Translation, Rotation: v.Rotation}
			}
		}
		result := MechanismPreview{InstancePoses: []MechanismPreviewPose{}}
		if len(m.Joints) == 0 && len(m.UnitIDs) == 0 {
			m.Evaluation = &MechanismEvaluation{}
			result.Mechanism = m
			for _, v := range next.Instances {
				result.InstancePoses = append(result.InstancePoses, MechanismPreviewPose{InstanceID: v.ID, InstancePose: InstancePose{Translation: v.Translation, Rotation: v.Rotation}})
			}
		} else {
			result, err = service.PreviewMechanism(ctx, documentID, MechanismPreviewRequest{BaseRevisionID: req.BaseRevisionID, Mechanism: m, NominalProduct: &next})
		}
		e := err
		if e != nil {
			return out, e
		}
		out.Mechanism = &result.Mechanism
		out.InstancePoses = result.InstancePoses
		// Entering changes only the target mechanism's accepted baseline.
		next = *view.Product
		next.Kinematics.Mechanisms = append([]Mechanism(nil), view.Product.Kinematics.Mechanisms...)
		next.Kinematics.Mechanisms[i] = result.Mechanism
		if mechanismAcceptanceEqual(view.Product.Kinematics.Mechanisms[i], result.Mechanism) {
			next.Kinematics.Mechanisms[i] = view.Product.Kinematics.Mechanisms[i]
		}
	} else {
		if len(next.Constraints) > 0 {
			frozen, e := service.FreezeAssemblyInput(ctx, documentID, req.BaseRevisionID, next)
			if e != nil {
				return out, e
			}
			equations, e := interactionFrozenConstraints(frozen)
			if e != nil {
				return out, e
			}
			if len(frozen.RelativeFixUpdates) > 0 {
				return out, fmt.Errorf("%w: Product relative Fix awaits an explicit update", ErrValidation)
			}
			covered := map[string]bool{}
			for _, c := range equations {
				covered[c.ID] = true
				if c.GroupID != "" {
					covered[c.GroupID] = true
				}
			}
			for _, c := range frozen.Definitions {
				if !c.Suppressed && c.Mode != "MEASURED" && !covered[c.ID] {
					return out, fmt.Errorf("%w: missing destination constraint %s", ErrValidation, c.ID)
				}
			}
			frozen.Constraints = equations
			result, e := service.worker.SolveAssemblyWithOptions(ctx, "mechanism-exit/"+req.BaseRevisionID, frozen.Bodies, frozen.Geometry, equations, geometry.AssemblySolveOptions{SolverProfile: &frozen.SolverProfile})
			if e != nil {
				return out, e
			}
			if result.Status != "CONVERGED" {
				code, retry := assemblySolveFailureDetails(result.Status)
				return out, &assemblySolveFailure{status: result.Status, code: code, retryable: retry, phase: "EXIT_MECHANISM", diagnostic: result.Diagnostic}
			}
			if e = validateAssemblyPreference(result); e != nil {
				return out, e
			}
			if ok, why := conflictIndependentWitness(frozen, result); !ok {
				return out, fmt.Errorf("%w: application witness: %s", ErrValidation, why)
			}
			if len(result.Bodies) != len(next.Instances) {
				return out, fmt.Errorf("%w: incomplete application frame", ErrValidation)
			}
			for _, b := range result.Bodies {
				i := slices.IndexFunc(next.Instances, func(v ProductInstance) bool { return v.ID == b.ID })
				if i < 0 || !validMotionPose(InstancePose(b.Pose)) {
					return out, fmt.Errorf("%w: invalid application frame", ErrValidation)
				}
				next.Instances[i].Translation, next.Instances[i].Rotation = b.Pose.Translation, b.Pose.Rotation
			}
		}
		for _, v := range next.Instances {
			out.InstancePoses = append(out.InstancePoses, MechanismPreviewPose{InstanceID: v.ID, InstancePose: InstancePose{Translation: v.Translation, Rotation: v.Rotation}})
		}
	}
	for i, v := range next.Instances {
		old := slices.IndexFunc(view.Product.Instances, func(p ProductInstance) bool { return p.ID == v.ID })
		if old >= 0 && conflictPoseClose(geometry.AssemblyPose{Translation: v.Translation, Rotation: v.Rotation}, geometry.AssemblyPose{Translation: view.Product.Instances[old].Translation, Rotation: view.Product.Instances[old].Rotation}, 1e-7, 1e-8) {
			next.Instances[i].Translation, next.Instances[i].Rotation = view.Product.Instances[old].Translation, view.Product.Instances[old].Rotation
		}
	}
	current, e := service.GetDocument(ctx, documentID)
	if e != nil {
		return out, e
	}
	if current.Document.VersionID != req.BaseRevisionID {
		return out, fmt.Errorf("%w: application base changed during solve", ErrValidation)
	}
	out.Product = next
	return out, nil
}

func mechanismAcceptanceEqual(a, b Mechanism) bool {
	if len(a.Coordinates) != len(b.Coordinates) {
		return false
	}
	for id, q := range a.Coordinates {
		v, ok := b.Coordinates[id]
		if !ok || math.Abs(q-v) > 1e-8 {
			return false
		}
	}
	if a.ID != b.ID || a.Name != b.Name || !reflect.DeepEqual(a.UnitIDs, b.UnitIDs) || len(a.Joints) != len(b.Joints) || len(a.Poses) != len(b.Poses) || !reflect.DeepEqual(a.Evaluation, b.Evaluation) {
		return false
	}
	for _, j := range a.Joints {
		i := slices.IndexFunc(b.Joints, func(v MechanismJoint) bool { return v.ID == j.ID })
		if i < 0 || j.Name != b.Joints[i].Name || jointDefinitionDigest(j) != jointDefinitionDigest(b.Joints[i]) {
			return false
		}
	}
	for id, p := range a.Poses {
		q, ok := b.Poses[id]
		if !ok || !conflictPoseClose(geometry.AssemblyPose(p), geometry.AssemblyPose(q), 1e-7, 1e-8) {
			return false
		}
	}
	return true
}

// An audited no-op has a frozen receipt even though it creates no business transaction.
type ApplicationCommandReceipt struct {
	Sequence  uint64 `json:"sequence"`
	VersionID string `json:"versionId"`
	ActorID   string `json:"actorId"`
}

func applicationAuditDigest(raw json.RawMessage) string {
	var audit map[string]json.RawMessage
	if json.Unmarshal(raw, &audit) != nil {
		return ""
	}
	delete(audit, "applicationReceipt")
	request, _ := json.Marshal(audit)
	return modelcore.ValueDigest(request)
}
