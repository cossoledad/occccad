package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func validateLocalInput(v *ContextInputLocalValue) error {
	fail := func() error { return fmt.Errorf("%w: CONTEXT_LOCAL_INPUT_UNSUPPORTED", ErrValidation) }
	if v == nil {
		return fail()
	}
	for _, a := range [][3]float64{v.Origin, v.Direction, v.XDirection} {
		for _, n := range a {
			if math.IsNaN(n) || math.IsInf(n, 0) {
				return fail()
			}
		}
	}
	if v.Type == "PARAMETER" {
		if v.Value == nil || math.IsNaN(v.Value.SIValue) || math.IsInf(v.Value.SIValue, 0) {
			return fail()
		}
		return nil
	}
	if v.Type == "POINT" && v.GeometryKind == "POINT" {
		return nil
	}
	if v.Type == "PLANE" {
		if _, _, _, e := validatedSupportFrame(v.Origin, v.XDirection, v.Direction); e != nil {
			return fail()
		}
		return nil
	}
	norm := func(a [3]float64) float64 { return math.Sqrt(a[0]*a[0] + a[1]*a[1] + a[2]*a[2]) }
	if math.Abs(norm(v.Direction)-1) > 1e-7 {
		return fail()
	}
	if v.Type == "AXIS" {
		return nil
	}
	if v.Type != "CURVE" || (v.GeometryKind != "LINE" && v.GeometryKind != "CIRCLE") || v.ParameterStart == nil || v.ParameterEnd == nil {
		return fail()
	}
	a, b := *v.ParameterStart, *v.ParameterEnd
	if math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) || b <= a {
		return fail()
	}
	if v.GeometryKind == "CIRCLE" {
		if v.Radius <= 1e-7 || math.IsNaN(v.Radius) || math.IsInf(v.Radius, 0) {
			return fail()
		}
		if _, _, _, e := validatedSupportFrame(v.Origin, v.XDirection, v.Direction); e != nil {
			return fail()
		}
	}
	return nil
}

// A local assignment has no dormant external reference that could later be read
// by update, projection or dependency analysis. Saved modules may be rebased
// through deeper paths; direct-receiver scope belongs to mutation validation.
func validateContextAssignment(b ContextBinding) error {
	if b.Local == nil {
		return nil
	}
	if len(b.OwningInstancePath.Segments) == 0 {
		return fmt.Errorf("%w: CONTEXT_DIRECT_RECEIVER_REQUIRED", ErrValidation)
	}
	if (b.SourceInstancePath.RootDocumentID != "" || b.SourceInstancePath.Canonical != "" || b.SourceInstancePath.Display != "" || len(b.SourceInstancePath.Segments) != 0) || b.Publication != (PublicationRef{}) || b.ReferenceMode != "" || b.Transform != (InstancePose{}) || !reflect.DeepEqual(b.Resolution, PublicationResolution{}) || b.Accepted.SourceRevisionID != "" || b.Accepted.SourceDigest != "" {
		return fmt.Errorf("%w: CONTEXT_ASSIGNMENT_CONFLICT", ErrValidation)
	}
	return validateLocalInput(b.Local)
}

func (s *Service) prepareContextInputMaintenance(ctx context.Context, doc string, raw json.RawMessage, r CommandRequest) (string, any, error) {
	var model ProductModel
	if e := json.Unmarshal(raw, &model); e != nil {
		return "", nil, e
	}
	i := slices.IndexFunc(model.ContextBindings, func(b ContextBinding) bool { return b.ID == r.ContextBindingID })
	if i < 0 {
		return "", nil, fmt.Errorf("%w: CONTEXT_INPUT_ASSIGNMENT_MISSING", ErrValidation)
	}
	b := model.ContextBindings[i]
	if len(b.OwningInstancePath.Segments) != 1 || b.OwningInstancePath.RootDocumentID != doc {
		return "", nil, fmt.Errorf("%w: CONTEXT_DIRECT_RECEIVER_REQUIRED", ErrValidation)
	}
	path, e := s.refreshProductInstancePath(ctx, model, b.OwningInstancePath)
	if e != nil {
		return "", nil, e
	}
	if r.Type == "RESTORE_CONTEXT_INPUT_DEFAULT" || r.Type == "DELETE_CONTEXT_BINDING" {
		return typeDeleteContextBinding, contextBindingDeletePayload{ContextBindingID: b.ID}, nil
	}
	if b.Local != nil {
		return "", nil, fmt.Errorf("%w: CONTEXT_INPUT_ALREADY_LOCAL", ErrValidation)
	}
	variants, e := s.contextVariantsForProductModel(ctx, model)
	if e != nil {
		return "", nil, e
	}
	cv, ok := variants[path.Canonical]
	if !ok || cv.Part == nil || cv.Status != "READY" {
		return "", nil, fmt.Errorf("%w: CONTEXT_INPUT_ACCEPTED_RESULT_UNAVAILABLE", ErrValidation)
	}
	var input *ContextInput
	for j := range cv.Part.ContextInputs {
		if cv.Part.ContextInputs[j].ID == b.ContextInputID {
			input = &cv.Part.ContextInputs[j]
		}
	}
	if input == nil {
		return "", nil, fmt.Errorf("%w: CONTEXT_INPUT_ASSIGNMENT_MISSING", ErrValidation)
	}
	local := &ContextInputLocalValue{Type: input.Type}
	switch input.Type {
	case "PARAMETER":
		for _, p := range cv.Part.Parameters {
			if p.ParameterID == input.Target.TargetID && p.EvaluatedValue != nil {
				v := *p.EvaluatedValue
				local.Value = &v
			}
		}
	case "PLANE":
		for _, p := range cv.Part.DatumPlanes {
			if p.ID == input.Target.TargetID {
				local.Origin, local.Direction, local.XDirection = p.Origin, p.Normal, p.UDirection
			}
		}
	case "AXIS":
		for _, p := range cv.Part.DatumAxes {
			if p.ID == input.Target.TargetID {
				local.Origin, local.Direction = p.Origin, p.Direction
			}
		}
	case "POINT", "CURVE":
		source, err := publicationProjectionSource(b.Resolution, b.Transform)
		if err != nil {
			return "", nil, err
		}
		local.GeometryKind, local.Origin, local.Direction, local.XDirection = source.GeometryType, source.Origin, source.Direction, source.XDirection
		local.ParameterStart, local.ParameterEnd = source.ParameterStart, source.ParameterEnd
		if source.RadiusMM != nil {
			local.Radius = *source.RadiusMM
		}
	default:
		return "", nil, fmt.Errorf("%w: CONTEXT_LOCAL_INPUT_UNSUPPORTED", ErrValidation)
	}
	if e := validateLocalInput(local); e != nil {
		return "", nil, e
	}
	b.Local = local
	b.SourceInstancePath = InstancePath{Segments: []InstancePathSegment{}}
	b.Publication = PublicationRef{}
	b.ReferenceMode = ""
	b.Transform = InstancePose{}
	b.Resolution = PublicationResolution{}
	b.OwningInstancePath = path
	b.Accepted = ContextBindingResolutionSnapshot{RootProductRevisionID: r.VersionID, OwningRevisionID: path.Segments[0].ResolvedVersionID, ContractDigest: resolvedDigest(input.Contract), Status: "CONNECTED"}
	return typeEditContextBinding, contextBindingPayload{Binding: b}, nil
}

func materializeLocalInput(m *PartModel, input ContextInput, value *ContextInputLocalValue) error {
	if e := validateLocalInput(value); e != nil {
		return e
	}
	if value.Type != input.Type {
		return fmt.Errorf("%w: CONTEXT_LOCAL_INPUT_INCOMPATIBLE", ErrValidation)
	}
	switch input.Target.Kind {
	case "PARAMETER":
		for i := range m.Parameters {
			p := &m.Parameters[i]
			if p.ParameterID == input.Target.TargetID {
				if input.Contract.Dimension == nil || value.Value.Dimension != *input.Contract.Dimension {
					return fmt.Errorf("%w: CONTEXT_LOCAL_INPUT_INCOMPATIBLE", ErrValidation)
				}
				v := *value.Value
				p.Source = modelcore.ValueSource{Literal: &v}
				return nil
			}
		}
	case "DATUM":
		for i := range m.DatumPlanes {
			p := &m.DatumPlanes[i]
			if value.Type == "PLANE" && p.ID == input.Target.TargetID {
				p.Origin, p.Normal, p.UDirection = value.Origin, value.Direction, value.XDirection
				p.Definition = nil
				return nil
			}
		}
		for i := range m.DatumAxes {
			p := &m.DatumAxes[i]
			if value.Type == "AXIS" && p.ID == input.Target.TargetID {
				p.Origin, p.Direction = value.Origin, value.Direction
				p.Definition = nil
				return nil
			}
		}
	case "SKETCH_EXTERNAL_GEOMETRY":
		if e := installContextInput(m, input); e != nil {
			return e
		}
		for i := range m.Features {
			f := &m.Features[i]
			if f.ID != input.Target.TargetID || f.Sketch == nil {
				continue
			}
			for j := range f.Sketch.ExternalGeometry {
				g := &f.Sketch.ExternalGeometry[j]
				if g.ContextInputID == input.ID {
					g.LocalInput = value
					g.SourceVersionID = "LOCAL_INPUT"
					g.SourceDocumentID = ""
					g.ContextReferenceID = ""
					g.PersistentSelection = modelcore.PersistentSelection{}
					g.DependencySnapshot = nil
					return nil
				}
			}
		}
	}
	return fmt.Errorf("%w: CONTEXT_INPUT_ASSIGNMENT_MISSING", ErrValidation)
}

func (s *Service) projectLocalInput(ctx context.Context, request string, m PartModel, sketch, id string, v *ContextInputLocalValue) (*SketchExternalGeometrySnapshot, error) {
	if e := validateLocalInput(v); e != nil {
		return nil, e
	}
	for _, f := range m.Features {
		if f.ID != sketch || f.Sketch == nil {
			continue
		}
		o, x, n, ok := supportFrame(m, f.Sketch.Support)
		if !ok {
			break
		}
		// Reuse the exact self-contained analytic path; no source locator or document
		// is supplied to the Worker, including for a formerly published BREP curve.
		source := geometry.ExternalProjectionSource{SourceKind: "SKETCH_ENTITY", GeometryType: v.GeometryKind, Origin: v.Origin, Direction: v.Direction, XDirection: v.XDirection, ParameterStart: v.ParameterStart, ParameterEnd: v.ParameterEnd, EvidenceDigest: resolvedDigest(v)}
		if v.Radius > 0 {
			radius := v.Radius
			source.RadiusMM = &radius
		}
		projected, e := s.projectExternalGeometry(ctx, request, sketch, contextExternalID(id), source, geometry.ExternalProjectionFrame{Origin: o, XDirection: x, Normal: n})
		if e != nil {
			return nil, e
		}
		if projected.Status != "CONNECTED" {
			return nil, fmt.Errorf("%w: %s: %s", ErrValidation, projected.DiagnosticCode, projected.Diagnostic)
		}
		return externalProjectionSnapshot(projected), nil
	}
	return nil, fmt.Errorf("%w: CONTEXT_INPUT_ACCEPTED_RESULT_UNAVAILABLE", ErrValidation)
}

// Both Preview and Commit (including adoption) check the complete, accepted
// candidate, while unrelated pre-existing assembly definitions retain policy.
func assignmentMutation(r CommandRequest) bool {
	return r.Type == "DELETE_CONTEXT_BINDING" || r.Type == "LOCALIZE_CONTEXT_INPUT" || r.Type == "RESTORE_CONTEXT_INPUT_DEFAULT" || r.Type == "EDIT_CONTEXT_BINDING" && r.SourceInstancePath != nil
}
func (s *Service) checkInputAssignmentCandidate(ctx context.Context, m *ProductModel, before ProductModel, r CommandRequest) error {
	if !assignmentMutation(r) {
		return nil
	}
	variants, e := s.contextVariantsForProductModel(ctx, *m)
	if e != nil {
		return e
	}
	owner := ""
	for _, b := range before.ContextBindings {
		if b.ID == r.ContextBindingID && len(b.OwningInstancePath.Segments) == 1 {
			owner = b.OwningInstancePath.Segments[0].InstanceID
		}
	}
	if owner == "" {
		return fmt.Errorf("%w: CONTEXT_DIRECT_RECEIVER_REQUIRED", ErrValidation)
	}
	for _, i := range m.Instances {
		if i.ID == owner {
			if e := s.requireReadyReferencedPart(ctx, i.ReferencedDocumentID, i.ReferencedVersionID); e != nil {
				return e
			}
		}
	}

	if e := validateReplacementConstraints(*m, owner); e != nil {
		return e
	}
	s.resolveProductPublications(ctx, m, variants)
	for _, p := range m.Publications {
		if len(p.Target.InstancePath.Segments) > 0 && p.Target.InstancePath.Segments[0].InstanceID == owner && p.Resolution.Status != "CONNECTED" {
			return fmt.Errorf("%w: CONTEXT_INPUT_FORWARD_FAILED", ErrValidation)
		}
	}
	return nil
}

func appendInputAssignmentChanges(before json.RawMessage, after ProductModel, set modelcore.ChangeSet) (modelcore.ChangeSet, error) {
	raw, e := json.Marshal(updateReferencesPayload{Model: after})
	if e != nil {
		return set, e
	}
	_, resolved, e := applyUpdateReferences(before, raw)
	if e != nil {
		return set, e
	}
	for _, change := range resolved.Changes {
		index := slices.IndexFunc(set.Changes, func(c modelcore.ModelChange) bool { return c.Target == change.Target })
		if index >= 0 {
			set.Changes[index] = change
		} else {
			set.Changes = append(set.Changes, change)
		}
	}
	set.ImpactSeeds = append(set.ImpactSeeds, resolved.ImpactSeeds...)
	return set, nil
}

func validateContextAssignmentEdit(b ContextBinding) error {
	if b.Local != nil && len(b.OwningInstancePath.Segments) != 1 {
		return fmt.Errorf("%w: CONTEXT_DIRECT_RECEIVER_REQUIRED", ErrValidation)
	}
	return validateContextAssignment(b)
}
