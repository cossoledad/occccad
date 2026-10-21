package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

type AutomationAssemblyVerificationInput struct {
	RevisionID            string               `json:"revisionId"`
	MovingInstanceID      string               `json:"movingInstanceId"`
	ReferenceInstanceID   string               `json:"referenceInstanceId"`
	FirstAxis             *AssemblyGeometryRef `json:"firstAxis,omitempty"`
	SecondAxis            *AssemblyGeometryRef `json:"secondAxis,omitempty"`
	FirstPlane            *AssemblyGeometryRef `json:"firstPlane,omitempty"`
	SecondPlane           *AssemblyGeometryRef `json:"secondPlane,omitempty"`
	OffsetMM              *float64             `json:"offsetMM,omitempty"`
	DirectionRelation     string               `json:"directionRelation,omitempty"`
	AxisDirectionRelation string               `json:"axisDirectionRelation,omitempty"`
	TargetDOF             *uint64              `json:"targetDOF,omitempty"`
}
type AutomationAssemblyVerification struct {
	Passed            bool                           `json:"passed"`
	RevisionID        string                         `json:"revisionId"`
	CurrentHead       bool                           `json:"currentHead"`
	SolverStatus      string                         `json:"solverStatus"`
	SolverBuild       string                         `json:"solverBuild,omitempty"`
	ManifestDigest    string                         `json:"manifestDigest,omitempty"`
	LengthToleranceMM float64                        `json:"lengthToleranceMM"`
	AngleToleranceRad float64                        `json:"angleToleranceRad"`
	AxisDistanceMM    float64                        `json:"axisDistanceMM"`
	AxisAngleRad      float64                        `json:"axisAngleRad"`
	AxisDirectionDot  float64                        `json:"axisDirectionDot"`
	PlaneOffsetMM     float64                        `json:"planeOffsetMM"`
	OffsetConvention  string                         `json:"offsetConvention"`
	PlaneDirectionDot float64                        `json:"planeDirectionDot"`
	RelativeDOF       uint64                         `json:"relativeDOF"`
	GaugeDOF          uint64                         `json:"gaugeDOF"`
	TranslationDOF    uint64                         `json:"translationDOF"`
	RotationDOF       uint64                         `json:"rotationDOF"`
	RotationAxis      *geometry.AssemblyScrewFreedom `json:"rotationAxis,omitempty"`
	Failures          []string                       `json:"failures"`
	References        []AssemblyGeometryRef          `json:"references,omitempty"`
}

func automationDot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func automationSub(a, b [3]float64) [3]float64 {
	return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}
func automationNorm(a [3]float64) float64 { return math.Sqrt(automationDot(a, a)) }
func automationCross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func automationAngle(a, b [3]float64, unoriented bool) float64 {
	v := automationDot(a, b)
	if unoriented {
		v = math.Abs(v)
	}
	return math.Atan2(automationNorm(automationCross(a, b)), v)
}

// authorizeAutomationAssemblySources checks all immutable definitions that the
// shared production compiler may consume. It never reads B-Rep or mutable Heads.
func (s *Service) authorizeAutomationAssemblySources(ctx context.Context, acl CommandAuthorizer, actor string, root ModelSnapshot) error {
	seen := map[string]bool{}
	var visit func(ModelSnapshot, int) error
	visit = func(snapshot ModelSnapshot, depth int) error {
		if depth > instancePathMaxDepth || len(seen) > maxManifestBodies {
			return fmt.Errorf("%w: assembly observation budget exceeded", ErrValidation)
		}
		key := snapshot.DocumentID + "@" + snapshot.RevisionID
		if seen[key] {
			return nil
		}
		seen[key] = true
		if snapshot.DocumentType != "PRODUCT" {
			return nil
		}
		var product ProductModel
		if err := json.Unmarshal(snapshot.Model, &product); err != nil {
			return err
		}
		for _, instance := range product.Instances {
			if _, err := acl.RequireDocument(ctx, instance.ReferencedDocumentID, actor, access.RoleViewer); err != nil {
				return err
			}
			child, err := s.ReadModelSnapshot(ctx, instance.ReferencedDocumentID, instance.ReferencedVersionID)
			if err != nil {
				return err
			}
			if err = visit(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(root, 0)
}

func (s *Service) automationAssemblySupport(ctx context.Context, acl CommandAuthorizer, actor, root, revision string, product *ProductModel, reference AssemblyGeometryRef) (geometry.AssemblyGeometry, AssemblyGeometryRef, error) {
	if reference.InstanceID == "" || reference.CapturedFrame != nil || reference.CapturedDirection != nil || reference.GeometryKey != "" || reference.TopologyID != 0 {
		return geometry.AssemblyGeometry{}, reference, fmt.Errorf("%w: formal source-derived assembly support required", ErrValidation)
	}
	path := InstancePath{RootDocumentID: root, Segments: []InstancePathSegment{{InstanceID: reference.InstanceID}}}
	if reference.InstancePath != nil {
		if reference.InstancePath.RootDocumentID != "" && reference.InstancePath.RootDocumentID != root {
			return geometry.AssemblyGeometry{}, reference, fmt.Errorf("%w: assembly root mismatch", ErrValidation)
		}
		path.Segments = nil
		for _, segment := range reference.InstancePath.Segments {
			path.Segments = append(path.Segments, InstancePathSegment{InstanceID: segment.InstanceID})
		}
	}
	occurrence, err := s.ReadAutomationOccurrence(ctx, acl, actor, root, revision, path)
	if err != nil {
		return geometry.AssemblyGeometry{}, reference, err
	}
	if occurrence.ContextVariantRequired {
		return geometry.AssemblyGeometry{}, reference, fmt.Errorf("%w: OCCURRENCE_CONTEXT_VARIANT_UNAVAILABLE", ErrValidation)
	}
	if reference.SourceVersionID != "" {
		if _, err := s.ReadModelSnapshot(ctx, occurrence.SourceDocumentID, reference.SourceVersionID); err != nil {
			return geometry.AssemblyGeometry{}, reference, fmt.Errorf("%w: assembly source revision does not belong to resolved occurrence", ErrValidation)
		}
	}
	reference.InstancePath = &occurrence.Path
	if reference.PublicationRef != nil {
		var publications []Publication
		if occurrence.SourceDocumentType == "PART" {
			part, err := decodeAssemblyPartModel(occurrence.Definition.Model)
			if err != nil {
				return geometry.AssemblyGeometry{}, reference, err
			}
			publications = part.Publications
		} else {
			var nested ProductModel
			if err := json.Unmarshal(occurrence.Definition.Model, &nested); err != nil {
				return geometry.AssemblyGeometry{}, reference, err
			}
			for _, p := range nested.Publications {
				publications = append(publications, Publication{ID: p.ID, Type: p.Type, CompatibilityVersion: p.CompatibilityVersion, Resolution: p.Resolution})
			}
		}
		found := false
		for _, p := range publications {
			if publicationReferenceCompatible(*reference.PublicationRef, p) && p.Resolution.Status == "CONNECTED" {
				resolved, err := s.readAutomationPublication(ctx, occurrence, p.ID, map[string]bool{})
				if err != nil {
					return geometry.AssemblyGeometry{}, reference, err
				}
				r := resolved.Resolution
				reference.PublicationResolution = &r
				reference.SourceVersionID = occurrence.SourceRevisionID
				found = true
				break
			}
		}
		if !found {
			return geometry.AssemblyGeometry{}, reference, fmt.Errorf("%w: requested Publication is unavailable or incompatible", ErrValidation)
		}
	}
	value, err := newAssemblySupportResolver(ctx, s, product).resolve(reference)
	if err != nil {
		return value, reference, err
	}
	for _, instance := range product.Instances {
		if instance.ID == reference.InstanceID {
			return assemblyGeometryInBody(value, InstancePose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)}), reference, nil
		}
	}
	return value, reference, ErrNotFound
}

// VerifyAutomationAssembly is read-only. It checks the persisted poses before
// running the real compiler/solver; a diagnostic solve's corrected pose cannot
// make an unassembled document appear complete.
func (s *Service) VerifyAutomationAssembly(ctx context.Context, acl CommandAuthorizer, actor, document string, input AutomationAssemblyVerificationInput) (AutomationAssemblyVerification, error) {
	profile := defaultAssemblySolverProfile()
	out := AutomationAssemblyVerification{RevisionID: input.RevisionID, LengthToleranceMM: profile.LengthTolerance, AngleToleranceRad: profile.AngleTolerance, OffsetConvention: selectedPlaneNormalV1, Failures: []string{}}
	if input.RevisionID == "" || input.MovingInstanceID == "" || input.ReferenceInstanceID == "" || input.MovingInstanceID == input.ReferenceInstanceID {
		return out, fmt.Errorf("%w: exact Product revision and two distinct motion units required", ErrValidation)
	}
	if _, err := acl.RequireDocument(ctx, document, actor, access.RoleViewer); err != nil {
		return out, err
	}
	snapshot, err := s.ReadModelSnapshot(ctx, document, input.RevisionID)
	if err != nil {
		return out, err
	}
	if snapshot.DocumentType != "PRODUCT" {
		return out, fmt.Errorf("%w: assembly verification requires a Product", ErrValidation)
	}
	if err = s.authorizeAutomationAssemblySources(ctx, acl, actor, snapshot); err != nil {
		return out, err
	}
	var product ProductModel
	if err = json.Unmarshal(snapshot.Model, &product); err != nil {
		return out, err
	}
	ctx = withAssemblyReadCache(ctx)
	axisPairs, planePairs := [][2]AssemblyGeometryRef{}, [][2]AssemblyGeometryRef{}
	axisDefinitions := []AssemblyConstraint{}
	planeDefinitions := []AssemblyConstraint{}
	for _, c := range product.Constraints {
		if c.Suppressed || c.Second == nil || c.EvaluationStatus != modelcore.AssemblyConstraintVerified {
			continue
		}
		first, second := c.First, *c.Second
		if first.InstanceID == input.ReferenceInstanceID && second.InstanceID == input.MovingInstanceID {
			first, second = second, first
		} else if first.InstanceID != input.MovingInstanceID || second.InstanceID != input.ReferenceInstanceID {
			continue
		}
		a, _, e := s.automationAssemblySupport(ctx, acl, actor, document, input.RevisionID, &product, first)
		if e != nil {
			return out, e
		}
		b, _, e := s.automationAssemblySupport(ctx, acl, actor, document, input.RevisionID, &product, second)
		if e != nil {
			return out, e
		}
		if (a.Kind == "AXIS" || a.Kind == "CYLINDER") && (b.Kind == "AXIS" || b.Kind == "CYLINDER") && (c.Kind == "CONCENTRIC" || c.Kind == "COINCIDENT") {
			axisPairs = append(axisPairs, [2]AssemblyGeometryRef{first, second})
			axisDefinitions = append(axisDefinitions, c)
		}
		if a.Kind == "PLANE" && b.Kind == "PLANE" && (c.Kind == "COINCIDENT" || c.Kind == "DISTANCE") {
			planePairs = append(planePairs, [2]AssemblyGeometryRef{first, second})
			planeDefinitions = append(planeDefinitions, c)
		}
	}
	if input.FirstAxis == nil || input.SecondAxis == nil {
		if len(axisPairs) == 1 {
			input.FirstAxis = &axisPairs[0][0]
			input.SecondAxis = &axisPairs[0][1]
		} else {
			out.Failures = append(out.Failures, "AXIS_SUPPORT_MISSING_OR_AMBIGUOUS")
		}
	}
	if input.FirstPlane == nil || input.SecondPlane == nil {
		if len(planePairs) == 1 {
			input.FirstPlane = &planePairs[0][0]
			input.SecondPlane = &planePairs[0][1]
		} else {
			out.Failures = append(out.Failures, "END_PLANE_SUPPORT_MISSING_OR_AMBIGUOUS")
		}
	}
	if len(out.Failures) > 0 {
		return out, nil
	}
	if input.AxisDirectionRelation == "" && len(axisDefinitions) == 1 {
		input.AxisDirectionRelation = axisDefinitions[0].DirectionRelation
	}
	if input.AxisDirectionRelation != "" && input.AxisDirectionRelation != "UNORIENTED" && input.AxisDirectionRelation != "SAME" && input.AxisDirectionRelation != "OPPOSITE" {
		return out, fmt.Errorf("%w: invalid axial direction relation", ErrValidation)
	}
	if input.FirstAxis.InstanceID != input.MovingInstanceID || input.FirstPlane.InstanceID != input.MovingInstanceID || input.SecondAxis.InstanceID != input.ReferenceInstanceID || input.SecondPlane.InstanceID != input.ReferenceInstanceID {
		return out, fmt.Errorf("%w: first supports must belong to moving and second supports to reference motion unit", ErrValidation)
	}
	if len(planeDefinitions) == 1 {
		if input.DirectionRelation == "" {
			input.DirectionRelation = planeDefinitions[0].DirectionRelation
		}
		if input.OffsetMM == nil {
			v := planeDefinitions[0].Value
			if planeDefinitions[0].Kind == "COINCIDENT" {
				v = 0
			} else if planeDefinitions[0].DistanceRelation != selectedPlaneNormalV1 {
				out.Failures = append(out.Failures, "EXPLICIT_SELECTED_PLANE_OFFSET_REQUIRED")
				return out, nil
			}
			input.OffsetMM = &v
		}
	}
	if input.DirectionRelation != "SAME" && input.DirectionRelation != "OPPOSITE" {
		out.Failures = append(out.Failures, "END_PLANE_DIRECTION_REQUIRED")
		return out, nil
	}
	if input.OffsetMM == nil || math.IsNaN(*input.OffsetMM) || math.IsInf(*input.OffsetMM, 0) {
		return out, fmt.Errorf("%w: explicit finite end-plane offset required", ErrValidation)
	}
	refs := []AssemblyGeometryRef{*input.FirstAxis, *input.SecondAxis, *input.FirstPlane, *input.SecondPlane}
	values := []geometry.AssemblyGeometry{}
	for _, ref := range refs {
		value, accepted, e := s.automationAssemblySupport(ctx, acl, actor, document, input.RevisionID, &product, ref)
		if e != nil {
			return out, e
		}
		values = append(values, value)
		out.References = append(out.References, accepted)
	}
	a, b, p, q := values[0], values[1], values[2], values[3]
	if (a.Kind != "AXIS" && a.Kind != "CYLINDER") || (b.Kind != "AXIS" && b.Kind != "CYLINDER") || p.Kind != "PLANE" || q.Kind != "PLANE" {
		return out, fmt.Errorf("%w: expected exact axial and planar supports", ErrValidation)
	}
	out.AxisDistanceMM = automationNorm(automationCross(automationSub(a.Origin, b.Origin), b.Direction))
	out.AxisAngleRad = automationAngle(a.Direction, b.Direction, true)
	out.AxisDirectionDot = automationDot(a.Direction, b.Direction)
	if (input.AxisDirectionRelation == "SAME" && out.AxisDirectionDot < 0) || (input.AxisDirectionRelation == "OPPOSITE" && out.AxisDirectionDot > 0) {
		out.Failures = append(out.Failures, "PERSISTED_AXIS_DIRECTION_MISMATCH")
	}
	out.PlaneOffsetMM = automationDot(automationSub(p.Origin, q.Origin), p.Direction)
	out.PlaneDirectionDot = automationDot(p.Direction, q.Direction)
	if out.AxisDistanceMM > profile.LengthTolerance || out.AxisAngleRad > profile.AngleTolerance {
		out.Failures = append(out.Failures, "PERSISTED_AXES_NOT_COAXIAL")
	}
	if math.Abs(out.PlaneOffsetMM-*input.OffsetMM) > profile.LengthTolerance {
		out.Failures = append(out.Failures, "PERSISTED_END_PLANE_OFFSET_MISMATCH")
	}
	wantDot := 1.0
	if input.DirectionRelation == "OPPOSITE" {
		wantDot = -1
	}
	if automationAngle(p.Direction, [3]float64{wantDot * q.Direction[0], wantDot * q.Direction[1], wantDot * q.Direction[2]}, false) > profile.AngleTolerance {
		out.Failures = append(out.Failures, "PERSISTED_END_PLANE_DIRECTION_MISMATCH")
	}
	manifest, err := s.FreezeAssemblyInput(ctx, document, input.RevisionID, product)
	if err != nil {
		return out, err
	}
	out.ManifestDigest = manifest.Digest
	if s.worker == nil {
		return out, fmt.Errorf("%w: assembly solver unavailable", ErrValidation)
	}
	solveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := s.solveFrozenManifest(solveCtx, "automation/verify/"+manifest.Digest, manifest, nil)
	if err != nil {
		return out, err
	}
	out.SolverStatus, out.SolverBuild = result.Status, result.SolverBuild
	if result.Status != "CONVERGED" || len(result.UnsatisfiedConstraintIDs) > 0 {
		out.Failures = append(out.Failures, "SOLVER_NOT_CONVERGED")
	}
	target := uint64(1)
	if input.TargetDOF != nil {
		target = *input.TargetDOF
	}
	if target > 6 {
		return out, fmt.Errorf("%w: target DOF must be 0..6", ErrValidation)
	}
	found := false
	for _, component := range result.Components {
		hasMoving, hasReference := false, false
		for _, id := range component.BodyIDs {
			hasMoving = hasMoving || id == input.MovingInstanceID
			hasReference = hasReference || id == input.ReferenceInstanceID
		}
		if !hasMoving || !hasReference {
			continue
		}
		found = true
		out.RelativeDOF, out.GaugeDOF = component.RelativeDof, component.GaugeDof
		if !component.Solved || component.RelativeDof != target || component.GaugeDof != 0 {
			out.Failures = append(out.Failures, "TARGET_COMPONENT_DOF_MISMATCH")
		}
		movingFreedom := false
		for _, f := range component.Freedoms {
			if f.BodyID != input.MovingInstanceID {
				continue
			}
			movingFreedom = true
			out.TranslationDOF, out.RotationDOF = f.TranslationDof, f.RotationDof
			if target == 1 {
				if f.TranslationDof != 0 || f.RotationDof != 1 || len(f.Rotations) != 1 {
					out.Failures = append(out.Failures, "TARGET_ROTATION_FREEDOM_MISSING")
				} else {
					axis := f.Rotations[0]
					out.RotationAxis = &axis
					if math.Abs(axis.Pitch) > profile.LengthTolerance || automationAngle(axis.Direction, b.Direction, true) > profile.AngleTolerance || automationNorm(automationCross(automationSub(axis.AxisPoint, b.Origin), b.Direction)) > profile.LengthTolerance {
						out.Failures = append(out.Failures, "FREE_ROTATION_AXIS_MISMATCH")
					}
				}
			}
		}
		if !movingFreedom {
			out.Failures = append(out.Failures, "MOVING_BODY_FREEDOM_MISSING")
		}
	}
	if !found {
		out.Failures = append(out.Failures, "TARGET_COMPONENT_MISSING")
	}
	// If the solver had to repair a nominal pose, the saved model has not met
	// the goal even when the proposed solution happens to meet it.
	for _, solved := range result.Bodies {
		for _, stored := range manifest.Bodies {
			if solved.ID != stored.ID {
				continue
			}
			if automationNorm(automationSub(solved.Pose.Translation, stored.Pose.Translation)) > 10*profile.LengthTolerance {
				out.Failures = append(out.Failures, "PERSISTED_POSE_REQUIRES_SOLVE")
			}
		}
	}
	head, err := s.ReadModelSnapshot(ctx, document, "")
	if err != nil {
		return out, err
	}
	out.CurrentHead = head.RevisionID == input.RevisionID
	if !out.CurrentHead {
		out.Failures = append(out.Failures, "ROOT_HEAD_CHANGED")
	}
	out.Passed = len(out.Failures) == 0
	return out, nil
}
