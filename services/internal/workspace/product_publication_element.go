package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

// Load the exact accepted occurrence, including its owning Product's inputs.
// This never reads a Part Head or creates a hidden publication in that Part.
func (s *Service) productElementPart(ctx context.Context, root ProductModel, path InstancePath, variants map[string]ContextVariantSnapshot) (PartModel, InstancePose, error) {
	selected, _, pose, err := s.productOccurrenceAtPath(ctx, root, path)
	if err != nil {
		return PartModel{}, pose, err
	}
	if v, ok := variants[path.Canonical]; ok {
		if v.Status != "READY" || v.Part == nil {
			return PartModel{}, pose, fmt.Errorf("%w: PRODUCT_PUBLICATION_VARIANT_UNAVAILABLE", ErrValidation)
		}
		return clonePartModel(*v.Part), pose, nil
	}
	current := root
	var bindings []ContextBinding
	var raw []byte
	for depth, segment := range path.Segments {
		suffix := make([]string, 0, len(path.Segments)-depth)
		for _, p := range path.Segments[depth:] {
			suffix = append(suffix, p.InstanceID)
		}
		for _, b := range current.ContextBindings {
			if b.OwningInstancePath.Canonical == strings.Join(suffix, "/") {
				bindings = append(bindings, b)
			}
		}
		var kind string
		if err = s.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.documents d JOIN occccad.document_versions v ON v.document_id=d.id WHERE d.id=$1 AND v.id=$2 AND d.deleted_at IS NULL`, segment.ReferencedDocumentID, segment.ResolvedVersionID).Scan(&kind, &raw); err != nil {
			return PartModel{}, pose, err
		}
		if depth < len(path.Segments)-1 {
			if kind != "PRODUCT" {
				return PartModel{}, pose, ErrValidation
			}
			if err = json.Unmarshal(raw, &current); err != nil {
				return PartModel{}, pose, err
			}
		} else if kind != "PART" {
			return PartModel{}, pose, fmt.Errorf("%w: PRODUCT_PUBLICATION_PART_ELEMENT_REQUIRED", ErrValidation)
		}
	}
	var part PartModel
	if err = json.Unmarshal(raw, &part); err != nil {
		return part, pose, err
	}
	if len(bindings) > 0 {
		v, e := s.contextVariantSnapshot(ctx, expandedOccurrence{Path: path, DocumentID: selected.ReferencedDocumentID, RevisionID: selected.ReferencedVersionID, DocumentType: "PART"}, bindings)
		if e != nil {
			return part, pose, e
		}
		if v.Status != "READY" || v.Part == nil {
			return part, pose, fmt.Errorf("%w: PRODUCT_PUBLICATION_VARIANT_UNAVAILABLE", ErrValidation)
		}
		part = clonePartModel(*v.Part)
	}
	return part, pose, nil
}

func (s *Service) productElementPublicationFromRequest(ctx context.Context, documentID string, product ProductModel, request CommandRequest, id string) (ProductPublication, error) {
	if request.InstancePath == nil || request.InstancePath.RootDocumentID != documentID || len(request.InstancePath.Segments) == 0 {
		return ProductPublication{}, fmt.Errorf("%w: Product element occurrence required", ErrValidation)
	}
	if request.VersionID != "" && request.InstancePath.Segments[0].OwnerVersionID != request.VersionID {
		return ProductPublication{}, fmt.Errorf("%w: PRODUCT_PUBLICATION_SCOPE_CHANGED", ErrValidation)
	}
	_, path, _, err := s.productOccurrenceAtPath(ctx, product, *request.InstancePath)
	if err != nil {
		return ProductPublication{}, err
	}
	part, pose, err := s.productElementPart(ctx, product, path, nil)
	if err != nil {
		return ProductPublication{}, err
	}
	leaf := path.Segments[len(path.Segments)-1]
	if request.SourceDocumentID != "" && request.SourceDocumentID != leaf.ReferencedDocumentID || request.SourceVersionID != "" && request.SourceVersionID != leaf.ResolvedVersionID {
		return ProductPublication{}, fmt.Errorf("%w: PRODUCT_PUBLICATION_SCOPE_CHANGED", ErrValidation)
	}
	local := request
	local.VersionID = leaf.ResolvedVersionID
	local.InstancePath = nil
	// Automatic names belong to the publishing Product, not to the source Part.
	if local.Name == "" {
		names := []string{}
		for _, p := range product.Publications {
			names = append(names, p.Name)
		}
		local.Name = nextScopedName(defaultPublicationBase(local.TargetKind, local.PublicationType), names)
	}
	p, err := s.publicationFromSnapshot(ctx, leaf.ReferencedDocumentID, part, local, id)
	if err != nil {
		return ProductPublication{}, err
	}
	// Resolve the existing ports and the picked element in the same accepted
	// Part/variant. A raw viewport hit must not bypass a reusable public port.
	part.Publications = append(part.Publications, p)
	if err = s.resolvePartPublications(ctx, leaf.ReferencedDocumentID, request.RequestID+"/element", leaf.ResolvedVersionID, &part, true); err != nil {
		return ProductPublication{}, err
	}
	p = part.Publications[len(part.Publications)-1]
	if p.Resolution.Status != "CONNECTED" {
		return ProductPublication{}, fmt.Errorf("%w: %s", ErrValidation, p.Resolution.DiagnosticCode)
	}
	var source *Publication
	for i := range part.Publications[:len(part.Publications)-1] {
		candidate := &part.Publications[i]
		if samePublicationElement(*candidate, p) {
			if source != nil {
				return ProductPublication{}, fmt.Errorf("%w: PUBLICATION_SOURCE_AMBIGUOUS", ErrValidation)
			}
			source = candidate
		}
	}
	if source != nil {
		name := request.Name
		if name == "" {
			name = forwardedPublicationName(product, source.Name)
		}
		return productPublicationFromPath(path, publicationThroughRigidPose(*source, pose), id, name, request.SemanticPurpose), nil
	}
	p = publicationThroughRigidPose(p, pose)
	return ProductPublication{ID: id, Name: p.Name, Type: p.Type, SemanticPurpose: p.SemanticPurpose, CompatibilityVersion: p.CompatibilityVersion, Contract: p.Contract, Target: ProductPublicationTarget{InstancePath: path, Element: &p.Target}, Resolution: p.Resolution}, nil
}

// Current exact resolution is selection evidence, never a persisted identity.
// Once identified, the parent stores the child Publication ID instead.
func samePublicationElement(a, b Publication) bool {
	if a.Resolution.Status != "CONNECTED" || b.Resolution.Status != "CONNECTED" || a.Target.Kind != b.Target.Kind || a.Type != b.Type {
		return false
	}
	x, y := a.Target, b.Target
	switch x.Kind {
	case "TOPOLOGY":
		return a.Resolution.GeometryKey != "" && a.Resolution.GeometryKey == b.Resolution.GeometryKey && a.Resolution.LocalID != 0 && a.Resolution.LocalID == b.Resolution.LocalID && a.Resolution.TopologyKind == b.Resolution.TopologyKind
	case "DATUM":
		return x.DatumID == y.DatumID && x.Axis == y.Axis
	case "BODY_RESULT":
		return x.BodyID == y.BodyID
	case "PARAMETER":
		return x.ParameterID == y.ParameterID
	case "SKETCH_ENTITY":
		return x.SketchID == y.SketchID && x.EntityID == y.EntityID
	}
	return false
}

func (s *Service) resolveProductElementPublication(ctx context.Context, product ProductModel, path InstancePath, p ProductPublication, variants map[string]ContextVariantSnapshot) (Publication, error) {
	part, pose, err := s.productElementPart(ctx, product, path, variants)
	if err != nil {
		return Publication{}, err
	}
	leaf := path.Segments[len(path.Segments)-1]
	v := Publication{ID: p.ID, Name: p.Name, Type: p.Type, Contract: p.Contract, CompatibilityVersion: p.CompatibilityVersion, Target: *p.Target.Element}
	part.Publications = []Publication{v}
	if err = s.resolvePartPublications(ctx, leaf.ReferencedDocumentID, "product-element/"+p.ID, leaf.ResolvedVersionID, &part, true); err != nil {
		return Publication{}, err
	}
	return publicationThroughRigidPose(part.Publications[0], pose), nil
}

// A local topology number is creation evidence only. The accepted Part/variant
// owns the artifact; persistent links use its semantic anchor and Body identity.
func (s *Service) bindPublicationGeometry(ctx context.Context, documentID string, model PartModel, request CommandRequest) (modelcore.PersistentSelection, error) {
	bodyID := ""
	for _, b := range model.Bodies {
		if b.GeometryKey == request.GeometryKey {
			if bodyID != "" {
				return modelcore.PersistentSelection{}, fmt.Errorf("%w: ambiguous publication Body", ErrValidation)
			}
			bodyID = b.ID
		}
	}
	if bodyID == "" {
		return modelcore.PersistentSelection{}, fmt.Errorf("%w: publication geometry is outside accepted occurrence", ErrValidation)
	}
	manifest, _, err := s.topologyManifestForGeometryKey(ctx, request.GeometryKey)
	if err != nil {
		return modelcore.PersistentSelection{}, err
	}
	if !topologyHistoryComplete(manifest) {
		return modelcore.PersistentSelection{}, fmt.Errorf("%w: TOPOLOGY_HISTORY_INCOMPLETE", ErrValidation)
	}
	kind := modelcore.PersistentTopologyType(strings.ToUpper(request.TargetKind))
	output, owner := manifestOutput(manifest, kind, request.TopologyID)
	if output == nil || owner != bodyID {
		return modelcore.PersistentSelection{}, ErrNotFound
	}
	selection := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: documentID, SourceBodyID: bodyID, Anchor: semanticRef(output.GetSemanticRef()), ExpectedType: kind, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: selectionEvidence(output.GetEvidence())}
	return selection, selection.Validate()
}

func (s *Service) resolvePublicationSelection(ctx context.Context, documentID string, target PublicationTarget, geometryKey string) (modelcore.SelectionResolution, string, error) {
	if target.SourceGeometryKey == "" {
		return s.resolveSelectionAgainstGeometry(ctx, documentID, target.SourceVersionID, geometryKey, *target.PersistentSelection)
	}
	selection := *target.PersistentSelection
	if selection.SourceDocumentID != documentID {
		return unavailableSelectionResolution("PUBLICATION_SOURCE_DOCUMENT_CHANGED", "published occurrence was replaced"), "", nil
	}
	source, _, err := s.topologyManifestForGeometryKey(ctx, target.SourceGeometryKey)
	if err != nil {
		return modelcore.SelectionResolution{}, "", err
	}
	output := manifestSemanticOutput(source, selection)
	if !topologyHistoryComplete(source) || output == nil {
		return unavailableSelectionResolution("TOPOLOGY_HISTORY_INCOMPLETE", "publication creation history is unavailable"), "", nil
	}
	if selection.CreationEvidence.EvidenceDigest != "" && selection.CreationEvidence.EvidenceDigest != output.GetEvidence().GetEvidenceDigest() {
		return unavailableSelectionResolution("CREATION_EVIDENCE_MISMATCH", "publication creation evidence changed"), "", nil
	}
	manifest, digest, err := s.topologyManifestForGeometryKey(ctx, geometryKey)
	if err != nil {
		return modelcore.SelectionResolution{}, "", err
	}
	if !topologyHistoryComplete(manifest) {
		return unavailableSelectionResolution("TOPOLOGY_HISTORY_INCOMPLETE", "target topology history is unavailable"), digest, nil
	}
	return resolveManifest(selection, geometryKey, manifest), digest, nil
}

func relativePublicationVariants(variants map[string]ContextVariantSnapshot, prefix string) map[string]ContextVariantSnapshot {
	result := map[string]ContextVariantSnapshot{}
	for key, value := range variants {
		if strings.HasPrefix(key, prefix+"/") {
			result[strings.TrimPrefix(key, prefix+"/")] = value
		}
	}
	return result
}
