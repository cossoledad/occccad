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
		if scopedNameKey(item.Name) == scopedNameKey(payload.Publication.Name) {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: Product Publication name must be unique", ErrValidation)
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

func (service *Service) variantPublicationForAssembly(ctx context.Context, product ProductModel, instanceID, publicationID string,
	variants map[string]ContextVariantSnapshot) (Publication, bool, error) {
	var rootInstance *ProductInstance
	for index := range product.Instances {
		if product.Instances[index].ID == instanceID {
			rootInstance = &product.Instances[index]
			break
		}
	}
	if rootInstance == nil {
		return Publication{}, false, fmt.Errorf("%w: assembly Publication references an unknown instance", ErrValidation)
	}
	if variant, ok := variants[instanceID]; ok {
		for _, publication := range variant.Publications {
			if publication.ID == publicationID {
				return publication, true, nil
			}
		}
	}
	var documentType string
	var raw []byte
	if err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v
		JOIN occccad.documents d ON d.id=v.document_id WHERE d.id=$1 AND v.id=$2`, rootInstance.ReferencedDocumentID,
		rootInstance.ReferencedVersionID).Scan(&documentType, &raw); err != nil {
		return Publication{}, false, err
	}
	if documentType != "PRODUCT" {
		return Publication{}, false, nil
	}
	var child ProductModel
	if err := json.Unmarshal(raw, &child); err != nil {
		return Publication{}, false, err
	}
	for _, forwarding := range child.Publications {
		if forwarding.ID != publicationID {
			continue
		}
		candidate, ok, err := service.variantPublicationAtPath(ctx, child, forwarding.Target.InstancePath,
			forwarding.Target.PublicationID, variants, instanceID, InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, 0)
		if err != nil || !ok {
			return Publication{}, ok, err
		}
		candidate.ID, candidate.Name, candidate.Type = forwarding.ID, forwarding.Name, forwarding.Type
		candidate.SemanticPurpose, candidate.CompatibilityVersion, candidate.Contract = forwarding.SemanticPurpose,
			forwarding.CompatibilityVersion, forwarding.Contract
		return candidate, true, nil
	}
	return Publication{}, false, nil
}

func (service *Service) variantPublicationAtPath(ctx context.Context, product ProductModel, path InstancePath, publicationID string,
	variants map[string]ContextVariantSnapshot, prefix string, pose InstancePose, depth int) (Publication, bool, error) {
	if depth > instancePathMaxDepth || len(path.Segments) == 0 {
		return Publication{}, false, fmt.Errorf("%w: invalid Product Publication variant path", ErrValidation)
	}
	current := product
	var finalType string
	var finalRaw []byte
	for segmentIndex, segment := range path.Segments {
		var selected *ProductInstance
		for index := range current.Instances {
			if current.Instances[index].ID == segment.InstanceID {
				selected = &current.Instances[index]
				break
			}
		}
		if selected == nil || selected.ReferencedDocumentID != segment.ReferencedDocumentID ||
			selected.ReferencedVersionID != segment.ResolvedVersionID {
			return Publication{}, false, fmt.Errorf("%w: Product Publication path disappeared", ErrValidation)
		}
		prefix += "/" + selected.ID
		pose = composeInstancePose(pose, InstancePose{Translation: selected.Translation,
			Rotation: normalizedInstanceRotation(selected.Rotation)})
		if err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v
			JOIN occccad.documents d ON d.id=v.document_id WHERE d.id=$1 AND v.id=$2`, selected.ReferencedDocumentID,
			selected.ReferencedVersionID).Scan(&finalType, &finalRaw); err != nil {
			return Publication{}, false, err
		}
		if segmentIndex < len(path.Segments)-1 {
			if finalType != "PRODUCT" {
				return Publication{}, false, fmt.Errorf("%w: Product Publication path continues through a Part", ErrValidation)
			}
			if err := json.Unmarshal(finalRaw, &current); err != nil {
				return Publication{}, false, err
			}
		}
	}
	canonical := strings.TrimPrefix(prefix, "/")
	if finalType == "PART" {
		variant, ok := variants[canonical]
		if !ok {
			return Publication{}, false, nil
		}
		for _, publication := range variant.Publications {
			if publication.ID == publicationID {
				return publicationThroughRigidPose(publication, pose), true, nil
			}
		}
		return Publication{}, false, nil
	}
	var nested ProductModel
	if err := json.Unmarshal(finalRaw, &nested); err != nil {
		return Publication{}, false, err
	}
	for _, forwarding := range nested.Publications {
		if forwarding.ID != publicationID {
			continue
		}
		candidate, ok, err := service.variantPublicationAtPath(ctx, nested, forwarding.Target.InstancePath,
			forwarding.Target.PublicationID, variants, canonical, pose, depth+1)
		if err != nil || !ok {
			return Publication{}, ok, err
		}
		candidate.ID, candidate.Name, candidate.Type = forwarding.ID, forwarding.Name, forwarding.Type
		candidate.SemanticPurpose, candidate.CompatibilityVersion, candidate.Contract = forwarding.SemanticPurpose,
			forwarding.CompatibilityVersion, forwarding.Contract
		return candidate, true, nil
	}
	return Publication{}, false, nil
}

func (service *Service) resolveProductPublications(ctx context.Context, product *ProductModel, variants ...map[string]ContextVariantSnapshot) {
	variantByInstance := map[string]ContextVariantSnapshot{}
	if len(variants) > 0 {
		variantByInstance = variants[0]
	}
	for index := range product.Publications {
		forwarding := &product.Publications[index]
		broken := func(code, diagnostic string) {
			forwarding.Resolution = PublicationResolution{Status: "BROKEN_PUBLICATION", DiagnosticCode: code, Diagnostic: diagnostic}
		}
		publication, path, err := service.publicationAtRelativePath(ctx, *product, forwarding.Target.InstancePath, forwarding.Target.PublicationID)
		if err != nil {
			code := "PRODUCT_PUBLICATION_TARGET_MISSING"
			if strings.Contains(err.Error(), "PUBLICATION_MISSING") {
				code = "PUBLICATION_MISSING"
			}
			broken(code, err.Error())
			continue
		}
		if len(path.Segments) > 0 {
			candidate, found, variantErr := service.variantPublicationAtPath(ctx, *product, path,
				forwarding.Target.PublicationID, variantByInstance, "", InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, 0)
			if variantErr != nil {
				broken("PRODUCT_PUBLICATION_VARIANT_FAILED", variantErr.Error())
				continue
			}
			if found {
				publication = candidate
			}
		}
		forwarding.Target.InstancePath = path
		contractProbe := Publication{Type: forwarding.Type, Contract: forwarding.Contract}
		if forwarding.CompatibilityVersion != publication.CompatibilityVersion || !publicationContractsCompatible(contractProbe, publication) {
			broken("PUBLICATION_CONTRACT_INCOMPATIBLE", "forwarded child Publication contract changed")
			continue
		}
		forwarding.Resolution = publication.Resolution
	}
}

func (service *Service) publicationAtRelativePath(ctx context.Context, root ProductModel, path InstancePath, publicationID string) (Publication, InstancePath, error) {
	if strings.TrimSpace(path.RootDocumentID) == "" || len(path.Segments) == 0 || len(path.Segments) > instancePathMaxDepth {
		return Publication{}, InstancePath{}, fmt.Errorf("%w: invalid Product Publication InstancePath", ErrValidation)
	}
	model := root
	canonical := InstancePath{RootDocumentID: path.RootDocumentID}
	ownerDocumentID := path.RootDocumentID
	ownerVersionID := path.Segments[0].OwnerVersionID
	pose := InstancePose{Rotation: [4]float64{0, 0, 0, 1}}
	var selected ProductInstance
	for depth, requested := range path.Segments {
		if requested.OwnerDocumentID != ownerDocumentID || requested.OwnerVersionID == "" || requested.OwnerVersionID != ownerVersionID {
			return Publication{}, InstancePath{}, fmt.Errorf("%w: InstancePath owner does not match the referenced Product", ErrValidation)
		}
		found := false
		for _, instance := range model.Instances {
			if instance.ID == requested.InstanceID {
				selected, found = instance, true
				break
			}
		}
		if !found {
			return Publication{}, InstancePath{}, fmt.Errorf("%w: PRODUCT_PUBLICATION_OCCURRENCE_MISSING", ErrValidation)
		}
		if requested.ReferencedDocumentID != selected.ReferencedDocumentID || requested.ResolvedVersionID != selected.ReferencedVersionID {
			return Publication{}, InstancePath{}, fmt.Errorf("%w: InstancePath reference does not match the selected member", ErrValidation)
		}
		canonical = appendInstancePath(canonical, InstancePathSegment{OwnerDocumentID: ownerDocumentID,
			OwnerVersionID: ownerVersionID, InstanceID: selected.ID, InstanceName: selected.Name,
			ReferencedDocumentID: selected.ReferencedDocumentID, ResolvedVersionID: selected.ReferencedVersionID})
		pose = composeInstancePose(pose, InstancePose{Translation: selected.Translation, Rotation: normalizedInstanceRotation(selected.Rotation)})
		if depth == len(path.Segments)-1 {
			break
		}
		var documentType string
		var raw []byte
		if err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id WHERE d.id=$1 AND v.id=$2`, selected.ReferencedDocumentID, selected.ReferencedVersionID).Scan(&documentType, &raw); err != nil {
			return Publication{}, InstancePath{}, err
		}
		if documentType != "PRODUCT" {
			return Publication{}, InstancePath{}, fmt.Errorf("%w: InstancePath continues through a Part", ErrValidation)
		}
		if err := json.Unmarshal(raw, &model); err != nil {
			return Publication{}, InstancePath{}, err
		}
		ownerDocumentID, ownerVersionID = selected.ReferencedDocumentID, selected.ReferencedVersionID
	}
	if path.Canonical != "" && path.Canonical != canonical.Canonical {
		return Publication{}, InstancePath{}, fmt.Errorf("%w: InstancePath canonical identity does not match its typed segments", ErrValidation)
	}
	publication, err := service.publicationAtRevision(ctx, selected.ReferencedDocumentID, selected.ReferencedVersionID, publicationID)
	if err == nil {
		publication = publicationThroughRigidPose(publication, pose)
	}
	return publication, canonical, err
}

func publicationThroughRigidPose(publication Publication, pose InstancePose) Publication {
	if publication.Resolution.Status != "CONNECTED" {
		return publication
	}
	switch publication.Type {
	case "POINT", "AXIS", "PLANE", "FRAME", "CURVE", "SURFACE":
		publication.Resolution.Origin = pointByPose(pose, publication.Resolution.Origin)
		publication.Resolution.XDirection = rotateByPose(pose, publication.Resolution.XDirection)
		publication.Resolution.YDirection = rotateByPose(pose, publication.Resolution.YDirection)
		publication.Resolution.ZDirection = rotateByPose(pose, publication.Resolution.ZDirection)
		publication.Resolution.SourceDigest = resolvedDigest(struct {
			Source string
			Pose   InstancePose
		}{publication.Resolution.SourceDigest, pose})
	}
	return publication
}

func productPublicationFromChild(instance ProductInstance, child Publication, id, name, purpose string) ProductPublication {
	path := appendInstancePath(InstancePath{}, InstancePathSegment{InstanceID: instance.ID, InstanceName: instance.Name,
		ReferencedDocumentID: instance.ReferencedDocumentID, ResolvedVersionID: instance.ReferencedVersionID})
	return productPublicationFromPath(path, child, id, name, purpose)
}

func productPublicationFromPath(path InstancePath, child Publication, id, name, purpose string) ProductPublication {
	if strings.TrimSpace(name) == "" {
		name = child.Name
	}
	return ProductPublication{ID: id, Name: name, Type: child.Type, SemanticPurpose: purpose,
		CompatibilityVersion: child.CompatibilityVersion, Contract: child.Contract,
		Target: ProductPublicationTarget{InstancePath: path, PublicationID: child.ID}, Resolution: child.Resolution}
}
