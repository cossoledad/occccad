package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

type assemblyResolvedPart struct{ model PartModel }
type assemblySupportResolver struct {
	ctx      context.Context
	service  *Service
	model    *ProductModel
	parts    map[string]assemblyResolvedPart
	geometry map[string]geometry.AssemblyGeometry
}

type AssemblySupportInspection struct {
	Reference                AssemblyGeometryRef        `json:"reference"`
	ExactType                string                     `json:"exactType,omitempty"`
	Descriptor               *geometry.AssemblyGeometry `json:"descriptor,omitempty"`
	Status                   string                     `json:"status"`
	DiagnosticCode           string                     `json:"diagnosticCode,omitempty"`
	Diagnostic               string                     `json:"diagnostic,omitempty"`
	ConstraintEligible       bool                       `json:"constraintEligible"`
	ConstraintDiagnosticCode string                     `json:"constraintDiagnosticCode,omitempty"`
	ConstraintDiagnostic     string                     `json:"constraintDiagnostic,omitempty"`
}
type AssemblySupportInspectionResult struct {
	DocumentID string                      `json:"documentId"`
	VersionID  string                      `json:"versionId"`
	Supports   []AssemblySupportInspection `json:"supports"`
}

// Inspection binds transient picks without saving a definition, producing a
// Preview/Revision, changing navigation, or invoking an assembly solve. It uses
// exactly the descriptor compiler consumed by the authoritative solve path.
func (service *Service) InspectAssemblySupports(ctx context.Context, documentID string, references []AssemblyGeometryRef) (AssemblySupportInspectionResult, error) {
	if len(references) == 0 || len(references) > 256 {
		return AssemblySupportInspectionResult{}, fmt.Errorf("%w: support inspection requires 1–256 references", ErrValidation)
	}
	ctx = withAssemblyReadCache(ctx)
	view, err := service.GetDocument(ctx, documentID)
	if err != nil {
		return AssemblySupportInspectionResult{}, err
	}
	if view.Product == nil {
		return AssemblySupportInspectionResult{}, fmt.Errorf("%w: assembly supports require a Product", ErrValidation)
	}
	result := AssemblySupportInspectionResult{DocumentID: documentID, VersionID: view.Document.VersionID, Supports: make([]AssemblySupportInspection, 0, len(references))}
	resolver := newAssemblySupportResolver(ctx, service, view.Product)
	for _, reference := range references {
		item := AssemblySupportInspection{Reference: reference, Status: "RESOLVED"}
		bound := reference
		err = service.bindAssemblyPick(ctx, view.Product, &bound)
		if err == nil && bound.SourceVersionID == "" && bound.Kind != "BODY" {
			var source *ProductInstance
			source, _, err = service.assemblyReferenceOccurrence(ctx, *view.Product, bound)
			if err == nil {
				bound.SourceVersionID = source.ReferencedVersionID
			}
		}
		if err == nil {
			var value geometry.AssemblyGeometry
			value, err = resolver.resolve(bound)
			if err == nil {
				item.Reference = bound
				item.ExactType = value.Kind
				item.Descriptor = &value
				item.ConstraintEligible, item.ConstraintDiagnosticCode, item.ConstraintDiagnostic = assemblySupportConstraintEligibility(value, bound.DerivedRole)
			}
		}
		if err != nil {
			if !errors.Is(err, ErrValidation) && !errors.Is(err, ErrNotFound) {
				return AssemblySupportInspectionResult{}, err
			}
			item.Status = "BROKEN"
			item.DiagnosticCode = "ASSEMBLY_SUPPORT_UNAVAILABLE"
			item.Diagnostic = err.Error()
		}
		result.Supports = append(result.Supports, item)
	}
	return result, nil
}

func newAssemblySupportResolver(ctx context.Context, service *Service, model *ProductModel) *assemblySupportResolver {
	return &assemblySupportResolver{ctx: ctx, service: service, model: model, parts: map[string]assemblyResolvedPart{}, geometry: map[string]geometry.AssemblyGeometry{}}
}
func (resolver *assemblySupportResolver) part(instance *ProductInstance) (assemblyResolvedPart, error) {
	if instance.ReferencedVersionID == "" {
		return assemblyResolvedPart{}, fmt.Errorf("%w: accepted instance revision is required", ErrValidation)
	}
	key := instance.ReferencedDocumentID + ":" + instance.ReferencedVersionID
	if cached, ok := resolver.parts[key]; ok {
		return cached, nil
	}
	var kind string
	var raw []byte
	if err := resolver.service.database.QueryRow(resolver.ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id WHERE d.id=$1 AND v.id=$2`, instance.ReferencedDocumentID, instance.ReferencedVersionID).Scan(&kind, &raw); err != nil {
		return assemblyResolvedPart{}, err
	}
	if kind != "PART" {
		return assemblyResolvedPart{}, fmt.Errorf("%w: precise source must be a Part leaf", ErrValidation)
	}
	part, err := decodeAssemblyPartModel(raw)
	if err != nil {
		return assemblyResolvedPart{}, err
	}
	value := assemblyResolvedPart{model: part}
	resolver.parts[key] = value
	return value, nil
}
func (resolver *assemblySupportResolver) resolve(reference AssemblyGeometryRef) (geometry.AssemblyGeometry, error) {
	service, ctx, model := resolver.service, resolver.ctx, resolver.model

	instance, localPose, occurrenceErr := service.assemblyReferenceOccurrence(ctx, *model, reference)
	if occurrenceErr != nil {
		return geometry.AssemblyGeometry{}, occurrenceErr
	}
	if reference.PublicationRef != nil && (reference.PublicationResolution == nil || reference.PublicationResolution.Status != "CONNECTED") {
		return geometry.AssemblyGeometry{}, fmt.Errorf("%w: assembly Publication endpoint is not connected", ErrValidation)
	}
	if reference.Kind == "BODY" {
		return geometry.AssemblyGeometry{ID: reference.InstanceID + ":BODY", BodyID: reference.InstanceID, Kind: "BODY", LengthUnit: "mm"}, nil
	}
	persistentKey := ""
	if reference.PersistentSelection != nil {
		encoded, _ := json.Marshal(reference.PersistentSelection)
		persistentKey = string(encoded)
	}
	pathKey, _ := json.Marshal(reference.InstancePath)
	publicationKey, _ := json.Marshal(reference.PublicationRef)
	key := reference.InstanceID + ":" + string(pathKey) + ":" + reference.Kind + ":" + reference.GeometryID + ":" + reference.Axis + ":" + reference.DerivedRole + ":" + persistentKey + ":" + string(publicationKey)
	if cached, ok := resolver.geometry[key]; ok {
		return cached, nil
	}
	value := geometry.AssemblyGeometry{ID: key, BodyID: reference.InstanceID, Kind: reference.Kind, LengthUnit: "mm"}
	if reference.PublicationRef != nil && reference.PublicationResolution != nil {
		resolution := *reference.PublicationResolution
		var descriptorErr error
		value, descriptorErr = assemblyGeometryFromPublication(value, resolution)
		if descriptorErr != nil {
			return geometry.AssemblyGeometry{}, descriptorErr
		}
		value, descriptorErr = assemblyDerivedGeometry(value, reference.DerivedRole)
		if descriptorErr != nil {
			return geometry.AssemblyGeometry{}, descriptorErr
		}
		// A forwarding Publication on a direct Product is already body-local
		// (its occurrence-relative pose is identity). An explicitly nested leaf
		// Publication must be folded through that leaf path exactly once.
		value = assemblyGeometryInBody(value, localPose)
		resolver.geometry[key] = value
		return value, nil
	}
	switch reference.Kind {
	case "FACE", "EDGE", "VERTEX":
		_, err := resolver.part(instance)
		if err != nil {
			return geometry.AssemblyGeometry{}, err
		}
		if err := validatePersistentAssemblyReference(reference); err != nil {
			return geometry.AssemblyGeometry{}, err
		}
		resolved, err := service.resolvedAssemblyTopologyProperties(ctx, instance.ReferencedDocumentID, ResolvePersistentSelectionRequest{Selection: *reference.PersistentSelection, SourceVersionID: reference.SourceVersionID, TargetVersionID: instance.ReferencedVersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
		if err != nil {
			return geometry.AssemblyGeometry{}, err
		}
		if resolved.Resolution.Status != modelcore.SelectionResolved || resolved.Properties == nil {
			return geometry.AssemblyGeometry{}, fmt.Errorf("%w: supporting element is %s", ErrValidation, resolved.Resolution.Status)
		}
		value, err = assemblyGeometryFromProperties(value, *resolved.Properties)
		if err != nil {
			return geometry.AssemblyGeometry{}, err
		}
	case "PLANE":
		resolved, err := resolver.part(instance)
		if err != nil {
			return geometry.AssemblyGeometry{}, err
		}
		part := resolved.model
		found := false
		for _, plane := range part.DatumPlanes {
			if plane.ID == reference.GeometryID {
				value.Origin, value.Direction, found = plane.Origin, plane.Normal, true
				break
			}
		}
		if !found {
			return geometry.AssemblyGeometry{}, fmt.Errorf("%w: referenced datum plane does not exist", ErrValidation)
		}
	case "AXIS", "POINT", "FRAME":
		resolved, err := resolver.part(instance)
		if err != nil {
			return geometry.AssemblyGeometry{}, err
		}
		part := resolved.model
		found := false
		for _, axis := range part.DatumAxes {
			if axis.ID == reference.GeometryID && reference.Kind == "AXIS" {
				value.Origin, value.Direction, found = axis.Origin, axis.Direction, true
				break
			}
		}
		for _, system := range part.AxisSystems {
			if system.ID != reference.GeometryID {
				continue
			}
			value.Origin, found = system.Origin, true
			if reference.Kind == "FRAME" {
				value, err = assemblyDatumFrame(value, system)
				if err != nil {
					return geometry.AssemblyGeometry{}, err
				}
				break
			}
			if reference.Kind == "AXIS" {
				switch reference.Axis {
				case "X":
					value.Direction = system.XDirection
				case "Y":
					value.Direction = system.YDirection
				case "Z":
					value.Direction = system.ZDirection
				default:
					return geometry.AssemblyGeometry{}, fmt.Errorf("%w: axis-system direction must be X, Y, or Z", ErrValidation)
				}
			}
			break
		}
		if !found {
			return geometry.AssemblyGeometry{}, fmt.Errorf("%w: referenced axis geometry does not exist", ErrValidation)
		}
	default:
		return geometry.AssemblyGeometry{}, fmt.Errorf("%w: assembly references support BODY, POINT, AXIS, PLANE, VERTEX, linear EDGE, and planar/cylindrical FACE", ErrValidation)
	}
	var derivedErr error
	value, derivedErr = assemblyDerivedGeometry(value, reference.DerivedRole)
	if derivedErr != nil {
		return geometry.AssemblyGeometry{}, derivedErr
	}
	value = assemblyGeometryInBody(value, localPose)
	resolver.geometry[key] = value
	return value, nil
}
