package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/modelcore"
)

const (
	maxProductUpdateBindings  = 4096
	maxProductContextVariants = 1024
)

func (service *Service) GetProductUpdatePlan(ctx context.Context, rootDocumentID string) (ProductUpdatePlan, error) {
	revisionID, items, err := service.productContextRoot(ctx, rootDocumentID)
	if err != nil {
		return ProductUpdatePlan{}, err
	}
	plan := ProductUpdatePlan{RootProductDocumentID: rootDocumentID, RootProductRevisionID: revisionID, CanAccept: true}
	var rootRaw []byte
	if err := service.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1`, revisionID).Scan(&rootRaw); err != nil {
		return ProductUpdatePlan{}, err
	}
	var rootModel ProductModel
	if err := json.Unmarshal(rootRaw, &rootModel); err != nil {
		return ProductUpdatePlan{}, err
	}
	variantBindings := map[string][]ContextBinding{}
	candidateOccurrenceRevisions := map[string]string{}
	for _, occurrence := range items {
		if len(occurrence.Path.Segments) == 0 || occurrence.ReferenceMode == "PINNED" {
			continue
		}
		entry := ProductUpdatePlanEntry{Kind: "OCCURRENCE_REFERENCE", BindingID: "occurrence:" + occurrence.Path.Canonical,
			Name: occurrence.Name, SourceDisplayPath: occurrence.Path.Display, OwningDisplayPath: occurrence.Path.Display,
			AcceptedRevisionID: occurrence.RevisionID, Connection: "CONNECTED", Currency: "CURRENT", Evaluation: "READY"}
		var head string
		if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents
			WHERE id=$1 AND deleted_at IS NULL`, occurrence.DocumentID).Scan(&head); err != nil {
			entry.Connection, entry.Currency, entry.Evaluation = "BROKEN", "UPDATE_BLOCKED", "BLOCKED_BY_UPSTREAM"
			entry.DiagnosticCode, entry.Diagnostic = "REFERENCE_SOURCE_MISSING", "referenced occurrence Head is unavailable"
			plan.CanAccept = false
		} else {
			entry.CandidateRevisionID = head
			if head != occurrence.RevisionID {
				entry.Currency, plan.HasUpdates = "UPDATE_AVAILABLE", true
				if len(occurrence.Path.Segments) > 1 {
					entry.Currency, entry.Evaluation = "UPDATE_BLOCKED", "BLOCKED_BY_UPSTREAM"
					entry.DiagnosticCode, entry.Diagnostic = "NESTED_REFERENCE_UPDATE_REQUIRED", "accept the nested Product definition update before accepting the root occurrence"
					plan.CanAccept = false
				} else {
					candidateOccurrenceRevisions[occurrence.Path.Canonical] = head
				}
			}
		}
		plan.Entries = append(plan.Entries, entry)
	}
	for index := range rootModel.Instances {
		if candidate := candidateOccurrenceRevisions[rootModel.Instances[index].ID]; candidate != "" {
			rootModel.Instances[index].ReferencedVersionID = candidate
			rootModel.Instances[index].ResolvedVersionID = candidate
		}
	}
	bindings := []ContextBinding{}
	for _, item := range items {
		bindings = append(bindings, item.ContextBindings...)
	}
	if len(bindings) > maxProductUpdateBindings {
		return ProductUpdatePlan{}, fmt.Errorf("%w: PRODUCT_UPDATE_BINDING_LIMIT_EXCEEDED", ErrValidation)
	}
	for _, binding := range bindings {
		projectedBinding := binding
		if path, refreshErr := service.refreshProductInstancePath(ctx, rootModel, projectedBinding.OwningInstancePath); refreshErr == nil {
			projectedBinding.OwningInstancePath = path
		} else {
			return ProductUpdatePlan{}, refreshErr
		}
		if path, refreshErr := service.refreshProductInstancePath(ctx, rootModel, projectedBinding.SourceInstancePath); refreshErr == nil {
			projectedBinding.SourceInstancePath = path
		} else {
			return ProductUpdatePlan{}, refreshErr
		}
		entry := ProductUpdatePlanEntry{Kind: "CONTEXT_BINDING", BindingID: binding.ID, Name: binding.Name,
			SourceDisplayPath: binding.SourceInstancePath.Display, OwningDisplayPath: binding.OwningInstancePath.Display,
			AcceptedRevisionID: binding.Accepted.SourceRevisionID, Connection: "CONNECTED", Currency: "CURRENT", Evaluation: "READY"}
		source, sourceOK := occurrenceByCanonical(items, binding.SourceInstancePath.Canonical)
		owner, ownerOK := occurrenceByCanonical(items, binding.OwningInstancePath.Canonical)
		if !sourceOK || !ownerOK {
			entry.Connection, entry.Currency, entry.Evaluation = "BROKEN", "UPDATE_BLOCKED", "BLOCKED_BY_UPSTREAM"
			entry.DiagnosticCode, entry.Diagnostic = "INSTANCE_PATH_NOT_RESOLVED", "binding endpoint is outside the accepted root Product snapshot"
		} else {
			var head string
			if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents
				WHERE id=$1 AND deleted_at IS NULL`, source.DocumentID).Scan(&head); err != nil {
				entry.Connection, entry.Currency, entry.Evaluation = "BROKEN", "UPDATE_BLOCKED", "BLOCKED_BY_UPSTREAM"
				entry.DiagnosticCode, entry.Diagnostic = "REFERENCE_SOURCE_MISSING", "source document Head is unavailable"
			} else {
				entry.CandidateRevisionID = head
				if head != binding.Accepted.SourceRevisionID {
					entry.Currency, plan.HasUpdates = "UPDATE_AVAILABLE", true
				}
				publication, publicationErr := service.publicationAtRevision(ctx, source.DocumentID, head, binding.Publication.PublicationID)
				if publicationErr != nil || publication.Type != binding.Publication.ExpectedType ||
					(binding.Publication.CompatibilityVersion != "" && publication.CompatibilityVersion != binding.Publication.CompatibilityVersion) {
					entry.Connection, entry.Currency, entry.Evaluation = "INCOMPATIBLE", "UPDATE_BLOCKED", "BLOCKED_BY_UPSTREAM"
					entry.DiagnosticCode, entry.Diagnostic = "PUBLICATION_CONTRACT_INCOMPATIBLE", "candidate Publication is missing, broken, or contract-incompatible"
				} else if publication.Resolution.Status != "CONNECTED" {
					entry.Connection, entry.Currency, entry.Evaluation = "BROKEN", "UPDATE_BLOCKED", "BLOCKED_BY_UPSTREAM"
					entry.DiagnosticCode, entry.Diagnostic = publication.Resolution.DiagnosticCode, publication.Resolution.Diagnostic
				} else {
					projectedBinding.SourceInstancePath.Segments[len(projectedBinding.SourceInstancePath.Segments)-1].ResolvedVersionID = head
					projectedBinding.Publication.PersistentSelection = publication.Target.PersistentSelection
					projectedBinding.Publication.SelectionSourceVersionID = publication.Target.SourceVersionID
					projectedBinding.Resolution = publication.Resolution
					projectedBinding.Accepted.SourceRevisionID = head
					projectedBinding.Accepted.ContractDigest = resolvedDigest(publication.Contract)
					projectedBinding.Accepted.SourceDigest = publication.Resolution.SourceDigest
					projectedBinding.Accepted.Status = "CONNECTED"
				}
				if len(binding.SourceInstancePath.Segments) > 1 && head != source.RevisionID {
					entry.Currency, entry.Evaluation = "UPDATE_BLOCKED", "BLOCKED_BY_UPSTREAM"
					entry.DiagnosticCode, entry.Diagnostic = "NESTED_REFERENCE_UPDATE_REQUIRED", "accept nested source Product references from leaf to root before accepting this binding"
				}
			}
			variantBindings[owner.Path.Canonical] = append(variantBindings[owner.Path.Canonical], projectedBinding)
		}
		if entry.Connection != "CONNECTED" || entry.Currency == "UPDATE_BLOCKED" || entry.Evaluation != "READY" {
			plan.CanAccept = false
		}
		plan.Entries = append(plan.Entries, entry)
	}
	variantPaths := make([]string, 0, len(variantBindings))
	for path := range variantBindings {
		variantPaths = append(variantPaths, path)
	}
	sort.Strings(variantPaths)
	for _, path := range variantPaths {
		bindings := variantBindings[path]
		if len(plan.ContextVariants) >= maxProductContextVariants {
			return ProductUpdatePlan{}, fmt.Errorf("%w: PRODUCT_CONTEXT_VARIANT_LIMIT_EXCEEDED", ErrValidation)
		}
		owner, _ := occurrenceByCanonical(items, path)
		if len(bindings) > 0 && len(bindings[0].OwningInstancePath.Segments) > 0 {
			candidateRevision := bindings[0].OwningInstancePath.Segments[len(bindings[0].OwningInstancePath.Segments)-1].ResolvedVersionID
			owner.RevisionID, owner.Path = candidateRevision, bindings[0].OwningInstancePath
			for index := range bindings {
				bindings[index].Accepted.OwningRevisionID = candidateRevision
			}
		}
		variant, variantErr := service.contextVariantSnapshot(ctx, owner, bindings)
		if variantErr != nil {
			variant = ContextVariantSnapshot{OwningInstancePath: owner.Path, BaseDocumentID: owner.DocumentID,
				BaseRevisionID: owner.RevisionID, Status: "FAILED", DiagnosticCode: "CONTEXT_VARIANT_BUILD_FAILED", Diagnostic: variantErr.Error()}
		}
		if variant.Status != "READY" {
			plan.CanAccept = false
			failed := map[string]bool{}
			for _, id := range variant.BindingIDs {
				failed[id] = true
			}
			for index := range plan.Entries {
				if failed[plan.Entries[index].BindingID] {
					plan.Entries[index].Evaluation = "FAILED"
					plan.Entries[index].DiagnosticCode = variant.DiagnosticCode
					plan.Entries[index].Diagnostic = variant.Diagnostic
				}
			}
		}
		plan.ContextVariants = append(plan.ContextVariants, variant)
	}
	for _, constraint := range rootModel.Constraints {
		if constraint.Suppressed {
			continue
		}
		if plan.HasUpdates || constraint.EvaluationStatus != modelcore.AssemblyConstraintVerified {
			plan.AffectedConstraintIDs = append(plan.AffectedConstraintIDs, constraint.ID)
		}
		if constraint.EvaluationStatus == modelcore.AssemblyConstraintVerified {
			continue
		}
		entry := ProductUpdatePlanEntry{Kind: "ASSEMBLY_SOLVE", BindingID: "constraint:" + constraint.ID,
			Name: constraint.Kind + " " + constraint.ID, SourceDisplayPath: "Product assembly", OwningDisplayPath: "Product assembly",
			AcceptedRevisionID: revisionID, CandidateRevisionID: revisionID, Connection: "CONNECTED",
			Currency: "UPDATE_AVAILABLE", Evaluation: "READY"}
		plan.HasUpdates = true
		if constraint.EvaluationStatus == modelcore.AssemblyConstraintBroken {
			entry.Connection, entry.Currency, entry.Evaluation = "BROKEN", "UPDATE_BLOCKED", "BLOCKED_BY_UPSTREAM"
			entry.DiagnosticCode, entry.Diagnostic = "ASSEMBLY_SUPPORT_NOT_CONNECTED", constraint.EvaluationSummary
			plan.CanAccept = false
		}
		plan.Entries = append(plan.Entries, entry)
	}
	sort.Slice(plan.Entries, func(i, j int) bool { return plan.Entries[i].BindingID < plan.Entries[j].BindingID })
	sort.Slice(plan.ContextVariants, func(i, j int) bool {
		return plan.ContextVariants[i].OwningInstancePath.Canonical < plan.ContextVariants[j].OwningInstancePath.Canonical
	})
	plan.Digest = resolvedDigest(struct {
		Root, Revision string
		Entries        []ProductUpdatePlanEntry
		Variants       []ContextVariantSnapshot
	}{rootDocumentID, revisionID, plan.Entries, plan.ContextVariants})
	return plan, nil
}

func (service *Service) contextVariantSnapshot(ctx context.Context, owner expandedOccurrence, bindings []ContextBinding) (ContextVariantSnapshot, error) {
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].ID < bindings[j].ID })
	ids := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		ids = append(ids, binding.ID)
	}
	var modelJSON []byte
	var state string
	if err := service.database.QueryRow(ctx, `SELECT model_json,state FROM occccad.document_versions
		WHERE id=$1 AND document_id=$2`, owner.RevisionID, owner.DocumentID).Scan(&modelJSON, &state); err != nil {
		return ContextVariantSnapshot{}, err
	}
	bindingDigest := contextVariantBindingDigest(bindings)
	variant := ContextVariantSnapshot{OwningInstancePath: owner.Path, BaseDocumentID: owner.DocumentID,
		BaseRevisionID: owner.RevisionID, BindingIDs: ids, BindingDigest: bindingDigest, Status: "READY"}
	variant.VariantKey = contextVariantKey(owner.RevisionID, bindingDigest)
	var cachedPublications []byte
	if err := service.database.QueryRow(ctx, `SELECT evaluation_manifest_digest,geometry_key,publications
		FROM occccad.product_context_variants WHERE variant_key=$1 AND base_document_id=$2 AND base_revision_id=$3
		AND binding_digest=$4 AND evaluator_version=$5`, variant.VariantKey, owner.DocumentID, owner.RevisionID,
		bindingDigest, evaluatorVersion).Scan(&variant.EvaluationManifestDigest, &variant.GeometryKey, &cachedPublications); err == nil {
		if err := json.Unmarshal(cachedPublications, &variant.Publications); err != nil {
			return ContextVariantSnapshot{}, err
		}
		return variant, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ContextVariantSnapshot{}, err
	}
	if state != "READY" {
		variant.Status, variant.DiagnosticCode, variant.Diagnostic = "FAILED", "BASE_EVALUATION_NOT_READY", "base Part Revision evaluation is not ready"
		return variant, nil
	}
	var model PartModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return ContextVariantSnapshot{}, err
	}
	for _, binding := range bindings {
		if err := materializeContextBindingVariant(&model, binding); err != nil {
			variant.Status, variant.DiagnosticCode, variant.Diagnostic = "FAILED", "CONTEXT_INPUT_MATERIALIZATION_FAILED", err.Error()
			return variant, nil
		}
	}
	if err := validateAndResolvePartParameters(&model); err != nil {
		variant.Status, variant.DiagnosticCode, variant.Diagnostic = "FAILED", "CONTEXT_VARIANT_PARAMETER_EVALUATION_FAILED", err.Error()
		return variant, nil
	}
	if err := service.resolveAndSolveSketches(ctx, owner.DocumentID, "context-variant/"+variant.VariantKey, &model); err != nil {
		variant.Status, variant.DiagnosticCode, variant.Diagnostic = "FAILED", "CONTEXT_VARIANT_SKETCH_EVALUATION_FAILED", err.Error()
		return variant, nil
	}
	if err := service.resolvePartPublications(ctx, owner.DocumentID, "context-variant/"+variant.VariantKey,
		owner.RevisionID+"@"+variant.VariantKey, &model); err != nil {
		variant.Status, variant.DiagnosticCode, variant.Diagnostic = "FAILED", "CONTEXT_VARIANT_PUBLICATION_EVALUATION_FAILED", err.Error()
		return variant, nil
	}
	for _, publication := range model.Publications {
		if publication.Resolution.Status != "CONNECTED" {
			variant.Status, variant.DiagnosticCode, variant.Diagnostic = "FAILED", "CONTEXT_VARIANT_PUBLICATION_BROKEN", publication.Resolution.Diagnostic
			return variant, nil
		}
	}
	geometryKey, err := service.evaluatePart(ctx, "context-variant/"+variant.VariantKey, model)
	if err != nil {
		variant.Status, variant.DiagnosticCode, variant.Diagnostic = "FAILED", "CONTEXT_VARIANT_GEOMETRY_EVALUATION_FAILED", err.Error()
		return variant, nil
	}
	derivedJSON, _ := json.Marshal(model)
	modelHash := canonicalModelHash(derivedJSON)
	_, manifest, err := buildPartEvaluation(model, owner.RevisionID+"@"+variant.VariantKey, modelHash, nil, nil)
	if err != nil {
		variant.Status, variant.DiagnosticCode, variant.Diagnostic = "FAILED", "CONTEXT_VARIANT_MANIFEST_FAILED", err.Error()
		return variant, nil
	}
	manifestJSON, _ := json.Marshal(manifest)
	variant.GeometryKey = geometryKey
	variant.EvaluationManifestDigest = modelcore.ValueDigest(manifestJSON)
	variant.Publications = model.Publications
	publicationsJSON, _ := json.Marshal(variant.Publications)
	if _, err := service.database.Exec(ctx, `INSERT INTO occccad.product_context_variants(
		variant_key,base_document_id,base_revision_id,binding_digest,evaluator_version,
		evaluation_manifest_digest,geometry_key,publications) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (variant_key) DO NOTHING`, variant.VariantKey, owner.DocumentID, owner.RevisionID,
		variant.BindingDigest, evaluatorVersion, variant.EvaluationManifestDigest, variant.GeometryKey, publicationsJSON); err != nil {
		return ContextVariantSnapshot{}, err
	}
	var storedManifest, storedGeometry string
	var storedPublications []byte
	if err := service.database.QueryRow(ctx, `SELECT evaluation_manifest_digest,geometry_key,publications
		FROM occccad.product_context_variants WHERE variant_key=$1`, variant.VariantKey).
		Scan(&storedManifest, &storedGeometry, &storedPublications); err != nil {
		return ContextVariantSnapshot{}, err
	}
	var stored []Publication
	if err := json.Unmarshal(storedPublications, &stored); err != nil {
		return ContextVariantSnapshot{}, err
	}
	if storedManifest != variant.EvaluationManifestDigest || storedGeometry != variant.GeometryKey ||
		resolvedDigest(stored) != resolvedDigest(variant.Publications) {
		return ContextVariantSnapshot{}, fmt.Errorf("%w: CONTEXT_VARIANT_NONDETERMINISTIC", ErrValidation)
	}
	return variant, nil
}

type contextVariantInput struct {
	ContextInputID   string                   `json:"contextInputId"`
	Publication      PublicationRef           `json:"publication"`
	SourceRevisionID string                   `json:"sourceRevisionId"`
	OwningRevisionID string                   `json:"owningRevisionId"`
	ContractDigest   string                   `json:"contractDigest"`
	SourceDigest     string                   `json:"sourceDigest"`
	Status           string                   `json:"status"`
	Transform        InstancePose             `json:"transform"`
	Resolution       contextVariantResolution `json:"resolution"`
}

type contextVariantResolution struct {
	Status, GeometryKey, GeometryID, TopologyKind, GeometryKind, Symmetry string
	LocalID                                                               uint64
	Radius                                                                float64
	Origin, XDirection, YDirection, ZDirection                            [3]float64
	SourceDigest, ValueDigest, ManifestDigest, NamingPolicyDigest         string
	Value                                                                 *modelcore.Quantity
	EvaluatorVersion, OCCTVersion                                         string
}

func contextVariantBindingDigest(bindings []ContextBinding) string {
	inputs := make([]contextVariantInput, 0, len(bindings))
	for _, binding := range bindings {
		resolution := binding.Resolution
		transform := binding.Transform
		if binding.Publication.ExpectedType == "PARAMETER" {
			transform = InstancePose{Rotation: [4]float64{0, 0, 0, 1}}
		}
		inputs = append(inputs, contextVariantInput{ContextInputID: binding.ContextInputID, Publication: binding.Publication,
			SourceRevisionID: binding.Accepted.SourceRevisionID, OwningRevisionID: binding.Accepted.OwningRevisionID,
			ContractDigest: binding.Accepted.ContractDigest, SourceDigest: binding.Accepted.SourceDigest,
			Status: binding.Accepted.Status, Transform: transform,
			Resolution: contextVariantResolution{Status: resolution.Status, GeometryKey: resolution.GeometryKey,
				GeometryID: resolution.GeometryID, TopologyKind: resolution.TopologyKind, GeometryKind: resolution.GeometryKind,
				Symmetry: resolution.Symmetry, LocalID: resolution.LocalID, Radius: resolution.Radius, Origin: resolution.Origin,
				XDirection: resolution.XDirection, YDirection: resolution.YDirection, ZDirection: resolution.ZDirection,
				SourceDigest: resolution.SourceDigest, Value: resolution.Value, ValueDigest: resolution.ValueDigest,
				ManifestDigest: resolution.ManifestDigest, NamingPolicyDigest: resolution.NamingPolicyDigest,
				EvaluatorVersion: resolution.EvaluatorVersion, OCCTVersion: resolution.OCCTVersion}})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].ContextInputID < inputs[j].ContextInputID })
	return resolvedDigest(inputs)
}

func contextVariantKey(baseRevisionID, bindingDigest string) string {
	return resolvedDigest(struct{ BaseRevisionID, BindingDigest, Evaluator string }{baseRevisionID, bindingDigest, evaluatorVersion})
}

func (service *Service) acceptedContextVariants(ctx context.Context, items []expandedOccurrence) ([]ContextVariantSnapshot, error) {
	grouped := map[string][]ContextBinding{}
	for _, item := range items {
		for _, binding := range item.ContextBindings {
			grouped[binding.OwningInstancePath.Canonical] = append(grouped[binding.OwningInstancePath.Canonical], binding)
		}
	}
	paths := make([]string, 0, len(grouped))
	for path := range grouped {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]ContextVariantSnapshot, 0, len(paths))
	for _, path := range paths {
		owner, ok := occurrenceByCanonical(items, path)
		if !ok {
			return nil, fmt.Errorf("%w: ContextBinding owner is outside the Product snapshot", ErrValidation)
		}
		variant, err := service.contextVariantSnapshot(ctx, owner, grouped[path])
		if err != nil {
			return nil, err
		}
		result = append(result, variant)
	}
	return result, nil
}

func (service *Service) contextVariantsForProductModel(ctx context.Context, model ProductModel) (map[string]ContextVariantSnapshot, error) {
	grouped := map[string][]ContextBinding{}
	paths := map[string]InstancePath{}
	for _, binding := range model.ContextBindings {
		if len(binding.OwningInstancePath.Segments) == 0 {
			continue
		}
		canonical := binding.OwningInstancePath.Canonical
		grouped[canonical] = append(grouped[canonical], binding)
		paths[canonical] = binding.OwningInstancePath
	}
	result := map[string]ContextVariantSnapshot{}
	for canonical, bindings := range grouped {
		path := paths[canonical]
		leaf := path.Segments[len(path.Segments)-1]
		owner := expandedOccurrence{Path: path, DocumentID: leaf.ReferencedDocumentID,
			RevisionID: leaf.ResolvedVersionID, DocumentType: "PART", Name: leaf.InstanceName}
		variant, err := service.contextVariantSnapshot(ctx, owner, bindings)
		if err != nil {
			return nil, err
		}
		if variant.Status != "READY" {
			return nil, fmt.Errorf("%w: %s: %s", ErrValidation, variant.DiagnosticCode, variant.Diagnostic)
		}
		result[canonical] = variant
	}
	return result, nil
}

func materializeContextBindingVariant(model *PartModel, binding ContextBinding) error {
	var input *ContextInput
	for index := range model.ContextInputs {
		if model.ContextInputs[index].ID == binding.ContextInputID {
			input = &model.ContextInputs[index]
			break
		}
	}
	if input == nil {
		return fmt.Errorf("%w: ContextInput %s is missing", ErrValidation, binding.ContextInputID)
	}
	if binding.Accepted.Status != "CONNECTED" || binding.Resolution.Status != "CONNECTED" {
		return fmt.Errorf("%w: ContextBinding %s is not connected", ErrValidation, binding.ID)
	}
	if input.Target.Kind == "PARAMETER" {
		if binding.Resolution.Value == nil {
			return fmt.Errorf("%w: parameter Publication has no frozen value", ErrValidation)
		}
		for index := range model.Parameters {
			if model.Parameters[index].ParameterID == input.Target.TargetID {
				value := *binding.Resolution.Value
				model.Parameters[index].Source = modelcore.ValueSource{Literal: &value}
				return nil
			}
		}
		return fmt.Errorf("%w: ContextInput target parameter is missing", ErrValidation)
	}
	resolution := binding.Resolution
	resolution.Origin = pointByPose(binding.Transform, resolution.Origin)
	resolution.XDirection = rotateByPose(binding.Transform, resolution.XDirection)
	resolution.YDirection = rotateByPose(binding.Transform, resolution.YDirection)
	resolution.ZDirection = rotateByPose(binding.Transform, resolution.ZDirection)
	sourceDocumentID := ""
	if count := len(binding.SourceInstancePath.Segments); count > 0 {
		sourceDocumentID = binding.SourceInstancePath.Segments[count-1].ReferencedDocumentID
	}
	reference := ContextReference{ID: "context-variant-" + binding.ID, Name: binding.Name, OwningWorkspace: "context-variant",
		SourceDocumentID: sourceDocumentID, ReferenceMode: "PINNED", SourceInstancePath: &binding.SourceInstancePath,
		Transform: binding.Transform, ResolvedRevisionID: binding.Accepted.SourceRevisionID, LocalTargetID: input.Target.TargetID,
		Publication: binding.Publication, Resolution: resolution}
	return materializeContextReference(model, &reference, input.Target.TargetID)
}

func (service *Service) CreateProductRelease(ctx context.Context, rootDocumentID string, request CreateProductReleaseRequest) (ProductRelease, error) {
	request.RequestID, request.ActorID = requestID(request.RequestID), actorID(request.ActorID)
	name, _, err := validateNameAndDescription(request.Name, "", "release")
	if err != nil {
		return ProductRelease{}, err
	}
	rawRequest, _ := json.Marshal(request)
	requestDigest := modelcore.ValueDigest(rawRequest)
	var existingRaw []byte
	var existingID, existingName, existingCreated, storedDigest string
	if err := service.database.QueryRow(ctx, `SELECT id::text,name,created_at::text,request_digest,manifest
		FROM occccad.product_releases WHERE root_product_document_id=$1 AND request_id=$2`, rootDocumentID, request.RequestID).
		Scan(&existingID, &existingName, &existingCreated, &storedDigest, &existingRaw); err == nil {
		if storedDigest != requestDigest {
			return ProductRelease{}, fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
		}
		var manifest ProductReleaseManifest
		_ = json.Unmarshal(existingRaw, &manifest)
		return ProductRelease{ID: existingID, Name: existingName, CreatedAt: existingCreated, Manifest: manifest}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ProductRelease{}, err
	}
	revisionID, items, err := service.productContextRoot(ctx, rootDocumentID)
	if err != nil {
		return ProductRelease{}, err
	}
	plan, err := service.GetProductUpdatePlan(ctx, rootDocumentID)
	if err != nil {
		return ProductRelease{}, err
	}
	manifest := ProductReleaseManifest{SchemaVersion: 1, RootProductDocumentID: rootDocumentID,
		RootProductRevisionID: revisionID, RootSnapshotDigest: resolvedDigest(items), ContextVariants: plan.ContextVariants,
		EvaluatorVersion: evaluatorVersion, NamingPolicyDigest: modelcore.TopologyNamingPolicyDigest}
	gates := []ProductReleaseGate{{Code: "ROOT_SNAPSHOT_RESOLVED", Status: "PASSED"}}
	variantsByPath := map[string]ContextVariantSnapshot{}
	for _, variant := range plan.ContextVariants {
		variantsByPath[variant.OwningInstancePath.Canonical] = variant
	}
	for _, item := range items {
		var geometryKey *string
		var evaluationJSON []byte
		var state string
		if err := service.database.QueryRow(ctx, `SELECT geometry_key,evaluation_manifest,state FROM occccad.document_versions
			WHERE id=$1 AND document_id=$2`, item.RevisionID, item.DocumentID).Scan(&geometryKey, &evaluationJSON, &state); err != nil {
			return ProductRelease{}, err
		}
		occurrence := ProductReleaseOccurrence{InstancePath: item.Path, DocumentID: item.DocumentID, DocumentType: item.DocumentType,
			RevisionID: item.RevisionID, Pose: item.Pose, EvaluationManifestDigest: modelcore.ValueDigest(evaluationJSON)}
		if geometryKey != nil {
			occurrence.GeometryKey = *geometryKey
		}
		if variant, ok := variantsByPath[item.Path.Canonical]; ok {
			occurrence.GeometryKey = variant.GeometryKey
			occurrence.EvaluationManifestDigest = variant.EvaluationManifestDigest
		}
		manifest.Occurrences = append(manifest.Occurrences, occurrence)
		if state != "READY" {
			gates = append(gates, ProductReleaseGate{Code: "OCCURRENCE_EVALUATION_READY", Status: "FAILED", Diagnostic: item.Path.Display + " is not READY"})
		}
		manifest.ContextBindings = append(manifest.ContextBindings, item.ContextBindings...)
	}
	if !plan.CanAccept || plan.HasUpdates {
		gates = append(gates, ProductReleaseGate{Code: "PRODUCT_REFERENCES_CURRENT", Status: "FAILED", Diagnostic: "Product Update Plan must be current and unblocked"})
	} else {
		gates = append(gates, ProductReleaseGate{Code: "PRODUCT_REFERENCES_CURRENT", Status: "PASSED"})
	}
	var rootRaw []byte
	if err := service.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1`, revisionID).Scan(&rootRaw); err != nil {
		return ProductRelease{}, err
	}
	var root ProductModel
	_ = json.Unmarshal(rootRaw, &root)
	manifest.ProductPublications = root.Publications
	constraintsReady := true
	var inactiveConstraintIDs []string
	for _, constraint := range root.Constraints {
		if constraint.Suppressed {
			inactiveConstraintIDs = append(inactiveConstraintIDs, constraint.ID)
			continue
		}
		if constraint.EvaluationStatus != modelcore.AssemblyConstraintVerified {
			constraintsReady = false
		}
	}
	if constraintsReady {
		gates = append(gates, ProductReleaseGate{Code: "ASSEMBLY_CONSTRAINTS_VERIFIED", Status: "PASSED"})
	} else {
		gates = append(gates, ProductReleaseGate{Code: "ASSEMBLY_CONSTRAINTS_VERIFIED", Status: "FAILED", Diagnostic: "one or more assembly constraints are not Verified"})
	}
	if len(inactiveConstraintIDs) > 0 {
		sort.Strings(inactiveConstraintIDs)
		gates = append(gates, ProductReleaseGate{Code: "ASSEMBLY_INACTIVE_DEFINITIONS", Status: "PASSED", Diagnostic: fmt.Sprintf("excluded from active verification because suppressed; definitions retained in SolveManifest: %v", inactiveConstraintIDs)})
	}
	if len(root.Constraints) > 0 {
		if err := service.database.QueryRow(ctx, `SELECT digest FROM occccad.product_solve_manifests m WHERE
			m.root_product_document_id=$1 AND m.root_product_revision_id=$2 AND m.manifest->>'purpose'='COMMIT' AND EXISTS (
				SELECT 1 FROM occccad.product_solve_results r WHERE r.manifest_digest=m.digest AND r.status='CONVERGED')
			ORDER BY m.created_at DESC LIMIT 1`, rootDocumentID, revisionID).Scan(&manifest.AssemblySolveManifest); errors.Is(err, pgx.ErrNoRows) {
			gates = append(gates, ProductReleaseGate{Code: "SOLVE_MANIFEST_REPLAYABLE", Status: "FAILED", Diagnostic: "no successful SolveManifest exists for the root model"})
		} else if err != nil {
			return ProductRelease{}, err
		} else {
			gates = append(gates, ProductReleaseGate{Code: "SOLVE_MANIFEST_REPLAYABLE", Status: "PASSED"})
		}
	}
	manifest.Gates = gates
	for _, gate := range gates {
		if gate.Status == "FAILED" {
			return ProductRelease{}, fmt.Errorf("%w: RELEASE_GATE_FAILED: %s: %s", ErrValidation, gate.Code, gate.Diagnostic)
		}
	}
	manifest.Digest = resolvedDigest(func() ProductReleaseManifest { value := manifest; value.Digest = ""; return value }())
	manifestRaw, _ := json.Marshal(manifest)
	var id, created string
	if err := service.database.QueryRow(ctx, `INSERT INTO occccad.product_releases(root_product_document_id,
		root_product_revision_id,name,request_id,request_digest,manifest_digest,manifest,gate_status,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,'PASSED',$8) RETURNING id::text,created_at::text`, rootDocumentID, revisionID,
		name, request.RequestID, requestDigest, manifest.Digest, manifestRaw, request.ActorID).Scan(&id, &created); err != nil {
		return ProductRelease{}, err
	}
	return ProductRelease{ID: id, Name: name, CreatedAt: created, Manifest: manifest}, nil
}

func (service *Service) ListProductReleases(ctx context.Context, rootDocumentID string) ([]ProductRelease, error) {
	rows, err := service.database.Query(ctx, `SELECT id::text,name,created_at::text,manifest FROM occccad.product_releases
		WHERE root_product_document_id=$1 ORDER BY created_at DESC`, rootDocumentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ProductRelease{}
	for rows.Next() {
		var item ProductRelease
		var raw []byte
		if err := rows.Scan(&item.ID, &item.Name, &item.CreatedAt, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &item.Manifest); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) GetProductRelease(ctx context.Context, rootDocumentID, releaseID string) (ProductRelease, error) {
	var result ProductRelease
	var raw []byte
	if err := service.database.QueryRow(ctx, `SELECT id::text,name,created_at::text,manifest FROM occccad.product_releases
		WHERE id=$1 AND root_product_document_id=$2`, releaseID, rootDocumentID).
		Scan(&result.ID, &result.Name, &result.CreatedAt, &raw); errors.Is(err, pgx.ErrNoRows) {
		return ProductRelease{}, ErrNotFound
	} else if err != nil {
		return ProductRelease{}, err
	}
	if err := json.Unmarshal(raw, &result.Manifest); err != nil {
		return ProductRelease{}, err
	}
	return result, nil
}

func (service *Service) ReplayProductRelease(ctx context.Context, rootDocumentID, releaseID, requestID string) (ProductReleaseReplay, error) {
	release, err := service.GetProductRelease(ctx, rootDocumentID, releaseID)
	if err != nil {
		return ProductReleaseReplay{}, err
	}
	result := ProductReleaseReplay{ReleaseID: release.ID, ManifestDigest: release.Manifest.Digest,
		Status: "REPLAYABLE_NO_ASSEMBLY_CONSTRAINTS"}
	if release.Manifest.AssemblySolveManifest == "" {
		return result, nil
	}
	solve, err := service.ReplayAssemblySolveManifest(ctx, rootDocumentID, release.Manifest.AssemblySolveManifest,
		requestID+"/release/"+release.ID)
	if err != nil {
		return ProductReleaseReplay{}, err
	}
	result.Status, result.Assembly = solve.Status, &solve
	return result, nil
}
