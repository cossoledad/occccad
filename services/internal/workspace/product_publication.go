package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/modelcore"
)

const (
	typeReplaceInstance          = "occccad://product/instance/replace"
	typeCreateProductPublication = "occccad://product/publication/create"
	typeDeleteProductPublication = "occccad://product/publication/delete"
)

type replaceInstancePayload struct {
	InstanceID           string       `json:"instanceId"`
	ReferencedDocumentID string       `json:"referencedDocumentId"`
	ReferencedVersionID  string       `json:"referencedVersionId"`
	Model                ProductModel `json:"model"`
}

type productPublicationPayload struct {
	Publication ProductPublication `json:"publication"`
}

func applyReplaceInstance(_ json.RawMessage, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var payload replaceInstancePayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	next, _ := json.Marshal(payload.Model)
	change, _ := modelcore.NewChange(modelcore.ChangeBind,
		modelcore.PropertyAddress{EntityID: payload.InstanceID, SlotID: "instance.reference"}, nil,
		struct{ DocumentID, VersionID string }{payload.ReferencedDocumentID, payload.ReferencedVersionID})
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change},
		ImpactSeeds: []modelcore.DependencyKey{"reference:" + modelcore.DependencyKey(payload.InstanceID)}}, nil
}

func applyCreateProductPublication(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload productPublicationPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for _, item := range model.Publications {
		if item.ID == payload.Publication.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate Product PublicationId", ErrValidation)
		}
	}
	model.Publications = append(model.Publications, payload.Publication)
	next, _ := json.Marshal(model)
	change, _ := modelcore.NewChange(modelcore.ChangeCreate,
		modelcore.PropertyAddress{EntityID: payload.Publication.ID, SlotID: "product-publication.entity"}, nil, payload.Publication)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change},
		ImpactSeeds: []modelcore.DependencyKey{"product-publication:" + modelcore.DependencyKey(payload.Publication.ID)}}, nil
}

func applyDeleteProductPublication(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload publicationDeletePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for index, item := range model.Publications {
		if item.ID != payload.PublicationID {
			continue
		}
		model.Publications = append(model.Publications[:index], model.Publications[index+1:]...)
		next, _ := json.Marshal(model)
		change, _ := modelcore.NewChange(modelcore.ChangeDelete,
			modelcore.PropertyAddress{EntityID: item.ID, SlotID: "product-publication.entity"}, item, nil)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change},
			ImpactSeeds: []modelcore.DependencyKey{"product-publication:" + modelcore.DependencyKey(item.ID)}}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: Product Publication does not exist", ErrValidation)
}

func (service *Service) publicationAtRevision(ctx context.Context, documentID, revisionID, publicationID string) (Publication, error) {
	var documentType string
	var raw []byte
	if err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v
		JOIN occccad.documents d ON d.id=v.document_id WHERE d.id=$1 AND v.id=$2 AND d.deleted_at IS NULL`,
		documentID, revisionID).Scan(&documentType, &raw); errors.Is(err, pgx.ErrNoRows) {
		return Publication{}, fmt.Errorf("%w: PUBLICATION_SOURCE_REVISION_MISSING", ErrValidation)
	} else if err != nil {
		return Publication{}, err
	}
	if documentType == "PART" {
		var model PartModel
		if err := json.Unmarshal(raw, &model); err != nil {
			return Publication{}, err
		}
		for _, publication := range model.Publications {
			if publication.ID == publicationID {
				return publication, nil
			}
		}
	} else if documentType == "PRODUCT" {
		var model ProductModel
		if err := json.Unmarshal(raw, &model); err != nil {
			return Publication{}, err
		}
		for _, forwarding := range model.Publications {
			if forwarding.ID != publicationID {
				continue
			}
			return Publication{ID: forwarding.ID, Name: forwarding.Name, Type: forwarding.Type,
				SemanticPurpose: forwarding.SemanticPurpose, CompatibilityVersion: forwarding.CompatibilityVersion,
				Contract: forwarding.Contract, Resolution: forwarding.Resolution}, nil
		}
	}
	return Publication{}, fmt.Errorf("%w: PUBLICATION_MISSING", ErrValidation)
}

func publicationReferenceCompatible(reference PublicationRef, publication Publication) bool {
	return publication.ID == reference.PublicationID && publication.Type == reference.ExpectedType &&
		(reference.CompatibilityVersion == "" || publication.CompatibilityVersion == reference.CompatibilityVersion)
}

func applyPublicationDescriptor(reference *AssemblyGeometryRef, publication Publication) error {
	if publication.Resolution.Status != "CONNECTED" {
		return fmt.Errorf("%w: %s: %s", ErrValidation, publication.Resolution.DiagnosticCode, publication.Resolution.Diagnostic)
	}
	reference.GeometryID, reference.Axis = "", ""
	switch publication.Type {
	case "POINT", "FRAME":
		reference.Kind = "POINT"
	case "AXIS":
		reference.Kind = "AXIS"
	case "PLANE":
		reference.Kind = "PLANE"
	case "SURFACE":
		reference.Kind = "FACE"
	case "CURVE":
		reference.Kind = "EDGE"
	case "BODY":
		reference.Kind = "BODY"
	default:
		return fmt.Errorf("%w: Publication type %s is not an assembly endpoint", ErrValidation, publication.Type)
	}
	if publication.Target.Kind == "DATUM" {
		reference.GeometryID, reference.Axis = publication.Target.DatumID, publication.Target.Axis
	}
	if publication.Target.Kind == "TOPOLOGY" {
		reference.PersistentSelection = publication.Target.PersistentSelection
		reference.SourceVersionID = publication.Target.SourceVersionID
	}
	return nil
}

func (service *Service) resolveAssemblyPublication(ctx context.Context, product *ProductModel, reference *AssemblyGeometryRef) (Publication, error) {
	if reference == nil || reference.PublicationRef == nil {
		return Publication{}, nil
	}
	var instance *ProductInstance
	for index := range product.Instances {
		if product.Instances[index].ID == reference.InstanceID {
			instance = &product.Instances[index]
			break
		}
	}
	if instance == nil {
		return Publication{}, fmt.Errorf("%w: assembly Publication references an unknown instance", ErrValidation)
	}
	publication, err := service.publicationAtRevision(ctx, instance.ReferencedDocumentID, instance.ReferencedVersionID,
		reference.PublicationRef.PublicationID)
	if err != nil {
		return Publication{}, err
	}
	if !publicationReferenceCompatible(*reference.PublicationRef, publication) {
		return Publication{}, fmt.Errorf("%w: PUBLICATION_CONTRACT_INCOMPATIBLE", ErrValidation)
	}
	if err := applyPublicationDescriptor(reference, publication); err != nil {
		return Publication{}, err
	}
	resolution := publication.Resolution
	reference.PublicationResolution = &resolution
	if publication.Target.PersistentSelection != nil {
		copy := *publication.Target.PersistentSelection
		reference.PublicationRef.PersistentSelection = &copy
	}
	return publication, nil
}

func (service *Service) resolveProductPublications(ctx context.Context, product *ProductModel) {
	instances := make(map[string]ProductInstance, len(product.Instances))
	for _, instance := range product.Instances {
		instances[instance.ID] = instance
	}
	for index := range product.Publications {
		forwarding := &product.Publications[index]
		broken := func(code, diagnostic string) {
			forwarding.Resolution = PublicationResolution{Status: "BROKEN_PUBLICATION", DiagnosticCode: code, Diagnostic: diagnostic}
		}
		if len(forwarding.Target.InstancePath.Segments) != 1 {
			broken("PRODUCT_PUBLICATION_PATH_UNSUPPORTED", "P9 forwarding requires one relative occurrence segment")
			continue
		}
		segment := forwarding.Target.InstancePath.Segments[0]
		instance, ok := instances[segment.InstanceID]
		if !ok {
			broken("PRODUCT_PUBLICATION_OCCURRENCE_MISSING", "forwarded occurrence no longer exists")
			continue
		}
		forwarding.Target.InstancePath.Segments[0].ReferencedDocumentID = instance.ReferencedDocumentID
		forwarding.Target.InstancePath.Segments[0].ResolvedVersionID = instance.ReferencedVersionID
		publication, err := service.publicationAtRevision(ctx, instance.ReferencedDocumentID, instance.ReferencedVersionID, forwarding.Target.PublicationID)
		if err != nil {
			code := "PRODUCT_PUBLICATION_TARGET_MISSING"
			if strings.Contains(err.Error(), "PUBLICATION_MISSING") {
				code = "PUBLICATION_MISSING"
			}
			broken(code, err.Error())
			continue
		}
		contractProbe := Publication{Type: forwarding.Type, Contract: forwarding.Contract}
		if forwarding.CompatibilityVersion != publication.CompatibilityVersion || !publicationContractsCompatible(contractProbe, publication) {
			broken("PUBLICATION_CONTRACT_INCOMPATIBLE", "forwarded child Publication contract changed")
			continue
		}
		forwarding.Resolution = publication.Resolution
	}
}

func productPublicationFromChild(instance ProductInstance, child Publication, id, name, purpose string) ProductPublication {
	path := appendInstancePath(InstancePath{}, InstancePathSegment{InstanceID: instance.ID, InstanceName: instance.Name,
		ReferencedDocumentID: instance.ReferencedDocumentID, ResolvedVersionID: instance.ReferencedVersionID})
	if strings.TrimSpace(name) == "" {
		name = child.Name
	}
	return ProductPublication{ID: id, Name: name, Type: child.Type, SemanticPurpose: purpose,
		CompatibilityVersion: child.CompatibilityVersion, Contract: child.Contract,
		Target: ProductPublicationTarget{InstancePath: path, PublicationID: child.ID}, Resolution: child.Resolution}
}
