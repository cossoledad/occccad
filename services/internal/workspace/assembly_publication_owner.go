package workspace

import (
	"context"
	"fmt"
	"slices"

	"github.com/occccad/occccad/internal/modelcore"
)

// Preserve the local public address while resolving its actual moving body.
// Product publication descriptors are in Product coordinates; the numerical
// solver consumes body-local descriptors and applies the body's pose once.
func (s *Service) resolveLocalAssemblyPublication(ctx context.Context, model *ProductModel, ref *AssemblyGeometryRef, variants map[string]ContextVariantSnapshot) (Publication, error) {
	if ref.PublicationRef.OwnerPath != nil {
		return Publication{}, fmt.Errorf("%w: PUBLICATION_OWNER_INVALID", ErrValidation)
	}
	if variants == nil && len(model.ContextBindings) > 0 {
		var err error
		variants, err = s.contextVariantsForProductModel(ctx, *model)
		if err != nil {
			return Publication{}, err
		}
	}
	index := slices.IndexFunc(model.Publications, func(p ProductPublication) bool { return p.ID == ref.PublicationRef.PublicationID })
	if index < 0 {
		return Publication{}, fmt.Errorf("%w: PUBLICATION_MISSING", ErrValidation)
	}
	// Re-evaluate only this interface against the supplied candidate and accepted
	// component revisions. This never loads the owning Product's Head model.
	candidate := *model
	candidate.Publications = []ProductPublication{model.Publications[index]}
	s.resolveProductPublications(ctx, &candidate, variants)
	p := candidate.Publications[0]
	if p.Target.ParameterID != "" || len(p.Target.InstancePath.Segments) == 0 {
		return Publication{}, fmt.Errorf("%w: PUBLICATION_CONSTRAINT_GEOMETRY_REQUIRED", ErrValidation)
	}
	if p.Resolution.Status != "CONNECTED" {
		return Publication{}, fmt.Errorf("%w: %s", ErrValidation, p.Resolution.DiagnosticCode)
	}
	segment := p.Target.InstancePath.Segments[0]
	instance := slices.IndexFunc(model.Instances, func(i ProductInstance) bool { return i.ID == segment.InstanceID })
	if instance < 0 {
		return Publication{}, fmt.Errorf("%w: PUBLICATION_MOTION_INSTANCE_MISSING", ErrValidation)
	}
	port := Publication{ID: p.ID, Name: p.Name, Type: p.Type, CompatibilityVersion: p.CompatibilityVersion, Contract: p.Contract, Resolution: p.Resolution}
	if !publicationReferenceCompatible(*ref.PublicationRef, port) {
		return Publication{}, fmt.Errorf("%w: PUBLICATION_CONTRACT_INCOMPATIBLE", ErrValidation)
	}
	i := model.Instances[instance]
	inverse := inverseRelativePose(InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, InstancePose{Translation: i.Translation, Rotation: normalizedInstanceRotation(i.Rotation)})
	port = publicationThroughRigidPose(port, inverse)
	if err := applyPublicationDescriptor(ref, port); err != nil {
		return Publication{}, err
	}
	ref.InstanceID = i.ID
	path := appendInstancePath(InstancePath{RootDocumentID: p.Target.InstancePath.RootDocumentID}, segment)
	ref.InstancePath = &path
	ref.GeometryKey, ref.TopologyID = "", 0
	ref.Resolution = nil
	ref.PublicationRef.PersistentSelection = nil
	ref.PublicationRef.SelectionSourceVersionID = ""
	resolution := port.Resolution
	ref.PublicationResolution = &resolution
	return port, nil
}

func localPublicationUsed(model ProductModel, id string) bool {
	for _, c := range model.Constraints {
		for _, ref := range []*AssemblyGeometryRef{&c.First, c.Second, c.AngleAxis} {
			if ref != nil && ref.PublicationRef != nil && ref.PublicationRef.Scope == "LOCAL" && ref.PublicationRef.PublicationID == id {
				return true
			}
		}
	}
	return false
}

// A changed public target invalidates its consumers without moving components.
func invalidateLocalPublicationConsumers(model *ProductModel, id string) []modelcore.ModelChange {
	var changes []modelcore.ModelChange
	for i := range model.Constraints {
		c := &model.Constraints[i]
		before := *c
		affected := false
		for _, ref := range []*AssemblyGeometryRef{&c.First, c.Second, c.AngleAxis} {
			if ref != nil && ref.PublicationRef != nil && ref.PublicationRef.Scope == "LOCAL" && ref.PublicationRef.PublicationID == id {
				affected = true
			}
		}
		if affected {
			c.EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
			c.EvaluationSummary = "PUBLICATION_TARGET_CHANGED"
			c.MeasuredValue = nil
			c.EvaluationFailure = nil
			change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: c.ID, SlotID: "assembly-constraint.entity"}, before, *c)
			changes = append(changes, change)
		}
	}
	return changes
}

// Publishing changes interfaces, never component poses. Only retargeted
// consumers need their accepted support snapshots refreshed before persistence.
func publicationDefinitionOnly(kind string) bool {
	return kind == typeCreateProductPublication || kind == typeEditProductPublication || kind == typeRedirectProductPublication || kind == typeDeleteProductPublication
}
func (s *Service) refreshPublicationEditConsumers(ctx context.Context, model *ProductModel) error {
	excluded := map[string]bool{}
	for _, c := range model.Constraints {
		excluded[c.ID] = c.EvaluationSummary != "PUBLICATION_TARGET_CHANGED"
	}
	return s.resolveAssemblySupports(ctx, model, excluded)
}
