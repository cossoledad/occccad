package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/occccad/occccad/internal/modelcore"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	instancePathMaxDepth     = 32
	productContextMaxMembers = 10000
	nameNormalizationProfile = "nfkc-casefold-v1"

	typeRenameInstance         = "occccad://product/instance/rename"
	typeEditProductPublication = "occccad://product/publication/edit"
	typeCreateContextInput     = "occccad://part/context-input/create"
	typeEditContextInput       = "occccad://part/context-input/edit"
	typeDeleteContextInput     = "occccad://part/context-input/delete"
	typeCreateContextBinding   = "occccad://product/context-binding/create"
	typeDeleteContextBinding   = "occccad://product/context-binding/delete"
)

func scopedNameKey(value string) string {
	return cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
}

func validateScopedName(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > 120 {
		return fmt.Errorf("%w: name is required and limited to 120 characters", ErrValidation)
	}
	if strings.ContainsAny(value, "/\\") {
		return fmt.Errorf("%w: name cannot contain path separators", ErrValidation)
	}
	for _, value := range value {
		if unicode.IsControl(value) {
			return fmt.Errorf("%w: name cannot contain control characters", ErrValidation)
		}
	}
	return nil
}

func ensureUniqueScopedName(value string, existing []string, self string) error {
	if err := validateScopedName(value); err != nil {
		return err
	}
	wanted := scopedNameKey(value)
	for _, candidate := range existing {
		if candidate != self && scopedNameKey(candidate) == wanted {
			return fmt.Errorf("%w: name must be unique in its owning reference", ErrValidation)
		}
	}
	return nil
}

func nextScopedName(base string, existing []string) string {
	used := make(map[string]bool, len(existing))
	for _, item := range existing {
		used[scopedNameKey(item)] = true
	}
	for ordinal := 1; ; ordinal++ {
		candidate := fmt.Sprintf("%s%d", base, ordinal)
		if !used[scopedNameKey(candidate)] {
			return candidate
		}
	}
}

func defaultPublicationBase(targetKind, publicationType string) string {
	switch strings.ToUpper(strings.TrimSpace(targetKind)) {
	case "FACE":
		return "Face"
	case "EDGE":
		return "Edge"
	case "VERTEX", "POINT":
		return "Point"
	case "PLANE":
		return "Plane"
	case "AXIS", "DATUM_AXIS":
		return "Axis"
	case "AXIS_SYSTEM", "FRAME":
		return "Frame"
	case "BODY":
		return "Body"
	case "PARAMETER":
		return "Parameter"
	}
	if publicationType != "" {
		return strings.ToUpper(publicationType[:1]) + strings.ToLower(publicationType[1:])
	}
	return "Publication"
}

type renameInstancePayload struct {
	InstanceID string `json:"instanceId"`
	Name       string `json:"name"`
}

type productPublicationEditPayload struct {
	PublicationID   string `json:"publicationId"`
	Name            string `json:"name"`
	SemanticPurpose string `json:"semanticPurpose,omitempty"`
}

type contextInputPayload struct {
	Input ContextInput `json:"input"`
}

type contextInputEditPayload struct {
	ContextInputID string `json:"contextInputId"`
	Name           string `json:"name"`
	Required       bool   `json:"required"`
}

type contextInputDeletePayload struct {
	ContextInputID string `json:"contextInputId"`
}

type contextBindingPayload struct {
	Binding ContextBinding `json:"binding"`
}

type contextBindingDeletePayload struct {
	ContextBindingID string `json:"contextBindingId"`
}

func applyRenameInstance(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload renameInstancePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	existing := make([]string, 0, len(model.Instances))
	old := ""
	for _, item := range model.Instances {
		existing = append(existing, item.Name)
		if item.ID == payload.InstanceID {
			old = item.Name
		}
	}
	if old == "" {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: selected instance does not exist", ErrValidation)
	}
	if err := ensureUniqueScopedName(payload.Name, existing, old); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for index := range model.Instances {
		if model.Instances[index].ID == payload.InstanceID {
			model.Instances[index].Name = strings.TrimSpace(payload.Name)
		}
	}
	next, _ := json.Marshal(model)
	change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: payload.InstanceID, SlotID: "instance.name"}, old, strings.TrimSpace(payload.Name))
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"instance:" + modelcore.DependencyKey(payload.InstanceID)}}, nil
}

func applyEditProductPublication(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload productPublicationEditPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	names := make([]string, 0, len(model.Publications))
	oldIndex := -1
	for index, item := range model.Publications {
		names = append(names, item.Name)
		if item.ID == payload.PublicationID {
			oldIndex = index
		}
	}
	if oldIndex < 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: Product Publication does not exist", ErrValidation)
	}
	old := model.Publications[oldIndex]
	if err := ensureUniqueScopedName(payload.Name, names, old.Name); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	model.Publications[oldIndex].Name = strings.TrimSpace(payload.Name)
	model.Publications[oldIndex].SemanticPurpose = strings.TrimSpace(payload.SemanticPurpose)
	next, _ := json.Marshal(model)
	change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: old.ID, SlotID: "product-publication.entity"}, old, model.Publications[oldIndex])
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"product-publication:" + modelcore.DependencyKey(old.ID)}}, nil
}

func applyCreateContextInput(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload contextInputPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	names := make([]string, 0, len(model.ContextInputs))
	for _, item := range model.ContextInputs {
		names = append(names, item.Name)
		if item.ID == payload.Input.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate ContextInputId", ErrValidation)
		}
	}
	if err := ensureUniqueScopedName(payload.Input.Name, names, ""); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	model.ContextInputs = append(model.ContextInputs, payload.Input)
	next, _ := json.Marshal(model)
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: payload.Input.ID, SlotID: "context-input.entity"}, nil, payload.Input)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"context-input:" + modelcore.DependencyKey(payload.Input.ID)}}, nil
}

func applyEditContextInput(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload contextInputEditPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	names := make([]string, 0, len(model.ContextInputs))
	index := -1
	for candidate, item := range model.ContextInputs {
		names = append(names, item.Name)
		if item.ID == payload.ContextInputID {
			index = candidate
		}
	}
	if index < 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: ContextInput does not exist", ErrValidation)
	}
	before := model.ContextInputs[index]
	if err := ensureUniqueScopedName(payload.Name, names, before.Name); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	model.ContextInputs[index].Name, model.ContextInputs[index].Required = strings.TrimSpace(payload.Name), payload.Required
	next, _ := json.Marshal(model)
	change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: before.ID, SlotID: "context-input.entity"}, before, model.ContextInputs[index])
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"context-input:" + modelcore.DependencyKey(before.ID)}}, nil
}

func applyDeleteContextInput(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload contextInputDeletePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for index, item := range model.ContextInputs {
		if item.ID != payload.ContextInputID {
			continue
		}
		model.ContextInputs = append(model.ContextInputs[:index], model.ContextInputs[index+1:]...)
		next, _ := json.Marshal(model)
		change, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: item.ID, SlotID: "context-input.entity"}, item, nil)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"context-input:" + modelcore.DependencyKey(item.ID)}}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: ContextInput does not exist", ErrValidation)
}

func applyCreateContextBinding(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload contextBindingPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	names := make([]string, 0, len(model.ContextBindings))
	for _, item := range model.ContextBindings {
		names = append(names, item.Name)
		if item.ID == payload.Binding.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate ContextBindingId", ErrValidation)
		}
		if item.OwningInstancePath.Canonical == payload.Binding.OwningInstancePath.Canonical && item.ContextInputID == payload.Binding.ContextInputID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: ContextInput is already bound in this occurrence", ErrValidation)
		}
	}
	if err := ensureUniqueScopedName(payload.Binding.Name, names, ""); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	model.ContextBindings = append(model.ContextBindings, payload.Binding)
	next, _ := json.Marshal(model)
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: payload.Binding.ID, SlotID: "context-binding.entity"}, nil, payload.Binding)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"context-binding:" + modelcore.DependencyKey(payload.Binding.ID)}}, nil
}

func applyDeleteContextBinding(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload contextBindingDeletePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for index, item := range model.ContextBindings {
		if item.ID != payload.ContextBindingID {
			continue
		}
		model.ContextBindings = append(model.ContextBindings[:index], model.ContextBindings[index+1:]...)
		next, _ := json.Marshal(model)
		change, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: item.ID, SlotID: "context-binding.entity"}, item, nil)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"context-binding:" + modelcore.DependencyKey(item.ID)}}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: ContextBinding does not exist", ErrValidation)
}

type expandedOccurrence struct {
	Path            InstancePath
	DocumentID      string
	RevisionID      string
	DocumentType    string
	Name            string
	Pose            InstancePose
	Publications    []Publication
	ContextInputs   []ContextInput
	ContextBindings []ContextBinding
}

func rebaseInstancePath(rootDocumentID string, prefix, relative InstancePath) InstancePath {
	result := InstancePath{RootDocumentID: rootDocumentID}
	for _, segment := range prefix.Segments {
		result = appendInstancePath(result, segment)
	}
	for _, segment := range relative.Segments {
		result = appendInstancePath(result, segment)
	}
	return result
}

func (service *Service) expandProductContext(ctx context.Context, rootDocumentID, rootRevisionID string) ([]expandedOccurrence, error) {
	result := []expandedOccurrence{}
	visiting := map[string]bool{}
	var expand func(string, string, InstancePath, InstancePose, int) error
	expand = func(documentID, revisionID string, path InstancePath, pose InstancePose, depth int) error {
		if depth > instancePathMaxDepth {
			return fmt.Errorf("%w: INSTANCE_PATH_DEPTH_EXCEEDED", ErrValidation)
		}
		if len(result) >= productContextMaxMembers {
			return fmt.Errorf("%w: PRODUCT_CONTEXT_SIZE_EXCEEDED", ErrValidation)
		}
		key := documentID + "@" + revisionID
		if visiting[key] {
			return fmt.Errorf("%w: PRODUCT_REFERENCE_CYCLE", ErrValidation)
		}
		var documentType, name string
		var raw []byte
		if err := service.database.QueryRow(ctx, `SELECT d.document_type,d.name,v.model_json FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id WHERE d.id=$1 AND v.id=$2 AND d.deleted_at IS NULL`, documentID, revisionID).Scan(&documentType, &name, &raw); err != nil {
			return err
		}
		item := expandedOccurrence{Path: path, DocumentID: documentID, RevisionID: revisionID, DocumentType: documentType, Name: name, Pose: pose}
		if documentType == "PART" {
			var model PartModel
			if err := json.Unmarshal(raw, &model); err != nil {
				return err
			}
			item.Publications, item.ContextInputs = model.Publications, model.ContextInputs
			result = append(result, item)
			return nil
		}
		var model ProductModel
		if err := json.Unmarshal(raw, &model); err != nil {
			return err
		}
		for _, publication := range model.Publications {
			item.Publications = append(item.Publications, Publication{ID: publication.ID, Name: publication.Name, Type: publication.Type,
				SemanticPurpose: publication.SemanticPurpose, CompatibilityVersion: publication.CompatibilityVersion,
				Contract: publication.Contract, Resolution: publication.Resolution})
		}
		for _, binding := range model.ContextBindings {
			binding.OwningInstancePath = rebaseInstancePath(rootDocumentID, path, binding.OwningInstancePath)
			binding.SourceInstancePath = rebaseInstancePath(rootDocumentID, path, binding.SourceInstancePath)
			item.ContextBindings = append(item.ContextBindings, binding)
		}
		result = append(result, item)
		visiting[key] = true
		defer delete(visiting, key)
		for _, instance := range model.Instances {
			childPath := appendInstancePath(path, InstancePathSegment{OwnerDocumentID: documentID, OwnerVersionID: revisionID,
				InstanceID: instance.ID, InstanceName: instance.Name, ReferencedDocumentID: instance.ReferencedDocumentID,
				ResolvedVersionID: instance.ReferencedVersionID})
			childPose := composeInstancePose(pose, InstancePose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)})
			if err := expand(instance.ReferencedDocumentID, instance.ReferencedVersionID, childPath, childPose, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := expand(rootDocumentID, rootRevisionID, InstancePath{RootDocumentID: rootDocumentID}, InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, 0); err != nil {
		return nil, err
	}
	return result, nil
}

func occurrenceByCanonical(items []expandedOccurrence, canonical string) (expandedOccurrence, bool) {
	for _, item := range items {
		if item.Path.Canonical == canonical {
			return item, true
		}
	}
	return expandedOccurrence{}, false
}

func validateNonRootInstancePath(path InstancePath, rootDocumentID string) error {
	if path.RootDocumentID != rootDocumentID || len(path.Segments) == 0 || len(path.Segments) > instancePathMaxDepth {
		return fmt.Errorf("%w: InstancePath is outside the root Product", ErrValidation)
	}
	ids := make([]string, 0, len(path.Segments))
	for _, segment := range path.Segments {
		if segment.InstanceID == "" {
			return fmt.Errorf("%w: InstancePath contains an empty stable member identity", ErrValidation)
		}
		ids = append(ids, segment.InstanceID)
	}
	if strings.Join(ids, "/") != path.Canonical {
		return fmt.Errorf("%w: InstancePath canonical identity does not match its typed segments", ErrValidation)
	}
	return nil
}

func validateResolvedInstancePath(requested, resolved InstancePath) error {
	if requested.RootDocumentID != resolved.RootDocumentID || requested.Canonical != resolved.Canonical || len(requested.Segments) != len(resolved.Segments) {
		return fmt.Errorf("%w: InstancePath does not resolve in the root snapshot", ErrValidation)
	}
	for index := range requested.Segments {
		left, right := requested.Segments[index], resolved.Segments[index]
		if left.OwnerDocumentID != right.OwnerDocumentID || left.OwnerVersionID != right.OwnerVersionID ||
			left.InstanceID != right.InstanceID || left.ReferencedDocumentID != right.ReferencedDocumentID ||
			left.ResolvedVersionID != right.ResolvedVersionID {
			return fmt.Errorf("%w: InstancePath snapshot fields do not match the resolved occurrence", ErrValidation)
		}
	}
	return nil
}

func (service *Service) productContextRoot(ctx context.Context, rootDocumentID string) (string, []expandedOccurrence, error) {
	var revisionID, documentType string
	if err := service.database.QueryRow(ctx, `SELECT head_version_id::text,document_type FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, rootDocumentID).Scan(&revisionID, &documentType); err != nil {
		return "", nil, err
	}
	if documentType != "PRODUCT" {
		return "", nil, fmt.Errorf("%w: design context root must be a Product", ErrValidation)
	}
	items, err := service.expandProductContext(ctx, rootDocumentID, revisionID)
	return revisionID, items, err
}

func (service *Service) GetProductDesignSession(ctx context.Context, rootDocumentID, activeCanonical string) (ProductDesignSession, error) {
	revisionID, items, err := service.productContextRoot(ctx, rootDocumentID)
	if err != nil {
		return ProductDesignSession{}, err
	}
	active, ok := occurrenceByCanonical(items, activeCanonical)
	if !ok {
		return ProductDesignSession{}, fmt.Errorf("%w: active occurrence is outside the Product context", ErrValidation)
	}
	catalog, err := service.GetContextCatalog(ctx, rootDocumentID, activeCanonical, "")
	if err != nil {
		return ProductDesignSession{}, err
	}
	path := active.Path
	return ProductDesignSession{RootProductDocumentID: rootDocumentID, RootProductRevisionID: revisionID,
		RootSnapshotDigest: resolvedDigest(items), ActiveInstancePath: &path, ActiveDocumentID: active.DocumentID,
		ActiveRevisionID: active.RevisionID, ContextCatalogDigest: catalog.Digest}, nil
}

func (service *Service) GetContextCatalog(ctx context.Context, rootDocumentID, activeCanonical, expectedType string) (ContextCatalog, error) {
	revisionID, items, err := service.productContextRoot(ctx, rootDocumentID)
	if err != nil {
		return ContextCatalog{}, err
	}
	active, ok := occurrenceByCanonical(items, activeCanonical)
	if !ok {
		return ContextCatalog{}, fmt.Errorf("%w: active occurrence is outside the Product context", ErrValidation)
	}
	expectedType = strings.ToUpper(strings.TrimSpace(expectedType))
	result := ContextCatalog{RootProductDocumentID: rootDocumentID, RootProductRevisionID: revisionID, ExpectedType: expectedType}
	path := active.Path
	result.ActiveInstancePath = &path
	dependencyEdges := map[string][]string{}
	for _, occurrence := range items {
		for _, binding := range occurrence.ContextBindings {
			dependencyEdges[binding.SourceInstancePath.Canonical] = append(dependencyEdges[binding.SourceInstancePath.Canonical], binding.OwningInstancePath.Canonical)
		}
	}
	for _, occurrence := range items {
		if len(occurrence.Path.Segments) == 0 || occurrence.Path.Canonical == activeCanonical {
			continue
		}
		for _, publication := range occurrence.Publications {
			if expectedType != "" && publication.Type != expectedType {
				continue
			}
			selectable, diagnostic := true, ""
			if contextDependencyReachable(dependencyEdges, activeCanonical, occurrence.Path.Canonical) {
				selectable, diagnostic = false, "CONTEXT_REFERENCE_CYCLE"
			}
			if publication.Resolution.Status != "CONNECTED" {
				selectable, diagnostic = false, "BROKEN_PUBLICATION"
			}
			display := publication.Name
			if occurrence.Path.Display != "" {
				display = occurrence.Path.Display + " / " + publication.Name
			}
			result.Publications = append(result.Publications, ContextCatalogPublication{InstancePath: occurrence.Path,
				DocumentID: occurrence.DocumentID, RevisionID: occurrence.RevisionID, Publication: publication,
				DisplayPath: display, Selectable: selectable, Diagnostic: diagnostic})
		}
	}
	sort.Slice(result.Publications, func(i, j int) bool { return result.Publications[i].DisplayPath < result.Publications[j].DisplayPath })
	result.Digest = resolvedDigest(struct {
		Root, Revision, Active, Expected string
		Publications                     []ContextCatalogPublication
	}{rootDocumentID, revisionID, activeCanonical, expectedType, result.Publications})
	return result, nil
}

func contextDependencyReachable(edges map[string][]string, start, target string) bool {
	if start == target {
		return true
	}
	visited, pending := map[string]bool{}, []string{start}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if visited[current] {
			continue
		}
		visited[current] = true
		for _, next := range edges[current] {
			if next == target {
				return true
			}
			pending = append(pending, next)
		}
	}
	return false
}

func contractsCompatible(expected PublicationContract, expectedType string, actual Publication) bool {
	probe := Publication{Type: expectedType, Contract: expected}
	return publicationContractsCompatible(probe, actual)
}

func contextInputFromRequest(model PartModel, request CommandRequest) (ContextInput, error) {
	kind := strings.ToUpper(strings.TrimSpace(request.TargetKind))
	targetID := strings.TrimSpace(request.TargetID)
	inputType := strings.ToUpper(strings.TrimSpace(request.PublicationType))
	contract := PublicationContract{}
	switch kind {
	case "PARAMETER":
		inputType = "PARAMETER"
		found := false
		for _, parameter := range model.Parameters {
			if parameter.ParameterID != targetID {
				continue
			}
			dimension := parameter.Dimension
			contract = PublicationContract{ValueType: parameter.ValueType, Dimension: &dimension, UnitPolicy: "SI_CANONICAL"}
			found = true
			break
		}
		if !found {
			return ContextInput{}, fmt.Errorf("%w: ContextInput target parameter does not exist", ErrValidation)
		}
	case "SKETCH_EXTERNAL_GEOMETRY":
		if inputType == "" {
			inputType = "CURVE"
		}
		if inputType != "CURVE" {
			return ContextInput{}, fmt.Errorf("%w: sketch external input requires CURVE", ErrValidation)
		}
		found := false
		for _, feature := range model.Features {
			if feature.ID == targetID && feature.Sketch != nil {
				found = true
				break
			}
		}
		if !found {
			return ContextInput{}, fmt.Errorf("%w: ContextInput target sketch does not exist", ErrValidation)
		}
		contract = PublicationContract{GeometryKind: "CURVE", Symmetry: "PARAMETER_DIRECTION"}
	case "DATUM":
		if inputType != "PLANE" && inputType != "AXIS" && inputType != "POINT" && inputType != "FRAME" {
			return ContextInput{}, fmt.Errorf("%w: datum ContextInput type is unsupported", ErrValidation)
		}
		found := false
		if inputType == "PLANE" {
			for _, datum := range model.DatumPlanes {
				found = found || datum.ID == targetID
			}
		} else if inputType == "AXIS" {
			for _, datum := range model.DatumAxes {
				found = found || datum.ID == targetID
			}
		} else {
			for _, datum := range model.AxisSystems {
				found = found || datum.ID == targetID
			}
		}
		if !found {
			return ContextInput{}, fmt.Errorf("%w: ContextInput target datum does not exist", ErrValidation)
		}
		contract = PublicationContract{GeometryKind: inputType, Symmetry: "NONE"}
		if inputType == "PLANE" {
			contract.Symmetry = "NORMAL_UNORIENTED"
		}
		if inputType == "AXIS" {
			contract.Symmetry = "DIRECTION_UNORIENTED"
		}
		if inputType == "FRAME" {
			contract.Symmetry = "RIGHT_HANDED_FRAME"
		}
	case "FEATURE_INPUT":
		if inputType != "CURVE" && inputType != "SURFACE" && inputType != "BODY" {
			return ContextInput{}, fmt.Errorf("%w: feature ContextInput type is unsupported", ErrValidation)
		}
		found := false
		for _, feature := range model.Features {
			if feature.ID == targetID {
				found = true
				break
			}
		}
		if !found {
			return ContextInput{}, fmt.Errorf("%w: ContextInput target feature does not exist", ErrValidation)
		}
		contract = PublicationContract{GeometryKind: inputType, Symmetry: "NONE"}
	default:
		return ContextInput{}, fmt.Errorf("%w: unsupported ContextInput target kind", ErrValidation)
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		existing := make([]string, 0, len(model.ContextInputs))
		for _, item := range model.ContextInputs {
			existing = append(existing, item.Name)
		}
		name = nextScopedName("Input", existing)
	}
	return ContextInput{ID: commandEntityID("context-input", request.RequestID), Name: name, Type: inputType,
		Required: request.Required, Target: ContextInputTarget{Kind: kind, TargetID: targetID}, Contract: contract}, nil
}

func (service *Service) contextBindingFromRequest(ctx context.Context, rootProductDocumentID, rootRevisionID string, request CommandRequest) (ContextBinding, error) {
	if request.OwningInstancePath == nil || request.SourceInstancePath == nil {
		return ContextBinding{}, fmt.Errorf("%w: source and owning InstancePath are required", ErrValidation)
	}
	if err := validateNonRootInstancePath(*request.OwningInstancePath, rootProductDocumentID); err != nil {
		return ContextBinding{}, err
	}
	if err := validateNonRootInstancePath(*request.SourceInstancePath, rootProductDocumentID); err != nil {
		return ContextBinding{}, err
	}
	items, err := service.expandProductContext(ctx, rootProductDocumentID, rootRevisionID)
	if err != nil {
		return ContextBinding{}, err
	}
	owner, ownerOK := occurrenceByCanonical(items, request.OwningInstancePath.Canonical)
	source, sourceOK := occurrenceByCanonical(items, request.SourceInstancePath.Canonical)
	if !ownerOK || !sourceOK || owner.DocumentType != "PART" {
		return ContextBinding{}, fmt.Errorf("%w: context binding occurrence is outside the root Product", ErrValidation)
	}
	if err := validateResolvedInstancePath(*request.OwningInstancePath, owner.Path); err != nil {
		return ContextBinding{}, err
	}
	if err := validateResolvedInstancePath(*request.SourceInstancePath, source.Path); err != nil {
		return ContextBinding{}, err
	}
	if owner.Path.Canonical == source.Path.Canonical {
		return ContextBinding{}, fmt.Errorf("%w: CONTEXT_REFERENCE_CYCLE", ErrValidation)
	}
	edges := map[string][]string{}
	for _, occurrence := range items {
		for _, existing := range occurrence.ContextBindings {
			edges[existing.SourceInstancePath.Canonical] = append(edges[existing.SourceInstancePath.Canonical], existing.OwningInstancePath.Canonical)
		}
	}
	if contextDependencyReachable(edges, owner.Path.Canonical, source.Path.Canonical) {
		return ContextBinding{}, fmt.Errorf("%w: CONTEXT_REFERENCE_CYCLE", ErrValidation)
	}
	var input *ContextInput
	for index := range owner.ContextInputs {
		if owner.ContextInputs[index].ID == request.ContextInputID {
			input = &owner.ContextInputs[index]
			break
		}
	}
	if input == nil {
		return ContextBinding{}, fmt.Errorf("%w: ContextInput does not exist in owning Part", ErrValidation)
	}
	var publication *Publication
	for index := range source.Publications {
		if source.Publications[index].ID == request.PublicationID {
			publication = &source.Publications[index]
			break
		}
	}
	if publication == nil {
		return ContextBinding{}, fmt.Errorf("%w: source Publication does not exist", ErrValidation)
	}
	if !contractsCompatible(input.Contract, input.Type, *publication) {
		return ContextBinding{}, fmt.Errorf("%w: PUBLICATION_CONTRACT_INCOMPATIBLE", ErrValidation)
	}
	if publication.Resolution.Status != "CONNECTED" {
		return ContextBinding{}, fmt.Errorf("%w: BROKEN_PUBLICATION", ErrValidation)
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = input.Name + " <- " + publication.Name
	}
	mode := strings.ToUpper(strings.TrimSpace(request.ReferenceMode))
	if mode == "" {
		mode = "FOLLOW_HEAD"
	}
	if mode != "FOLLOW_HEAD" && mode != "FOLLOW_WORKSPACE_WITH_ACCEPT" && mode != "PINNED" {
		return ContextBinding{}, fmt.Errorf("%w: unsupported context binding mode", ErrValidation)
	}
	return ContextBinding{ID: commandEntityID("context-binding", request.RequestID), Name: name,
		OwningInstancePath: owner.Path, ContextInputID: input.ID, SourceInstancePath: source.Path,
		Publication:   PublicationRef{PublicationID: publication.ID, ExpectedType: publication.Type, CompatibilityVersion: publication.CompatibilityVersion},
		ReferenceMode: mode, Transform: inverseRelativePose(source.Pose, owner.Pose), Resolution: publication.Resolution,
		Accepted: ContextBindingResolutionSnapshot{RootProductRevisionID: rootRevisionID, SourceRevisionID: source.RevisionID,
			OwningRevisionID: owner.RevisionID, ContractDigest: resolvedDigest(publication.Contract),
			SourceDigest: publication.Resolution.SourceDigest, Status: "CONNECTED"}}, nil
}
