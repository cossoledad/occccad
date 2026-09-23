package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
)

type BindPersistentSelectionRequest struct {
	SourceVersionID string `json:"sourceVersionId"`
	GeometryKey     string `json:"geometryKey"`
	Kind            string `json:"kind"`
	LocalID         uint64 `json:"localId"`
}

type ResolvePersistentSelectionRequest struct {
	Selection       modelcore.PersistentSelection `json:"persistentSelection"`
	SourceVersionID string                        `json:"sourcePartRevisionId"`
	TargetVersionID string                        `json:"targetPartRevisionId"`
	ManifestDigest  string                        `json:"targetPartEvaluationManifestDigest,omitempty"`
	PolicyDigest    string                        `json:"resolverPolicyDigest"`
}

type ResolvedTopologyProperties struct {
	Resolution modelcore.SelectionResolution `json:"resolution"`
	Properties *TopologyElementProperties    `json:"properties,omitempty"`
}

func (service *Service) bindAssemblyPick(ctx context.Context, product *ProductModel, reference *AssemblyGeometryRef) error {
	if reference == nil {
		return nil
	}
	if reference.PublicationRef != nil {
		if _, err := service.resolveAssemblyPublication(ctx, product, reference); err != nil {
			return err
		}
	}
	if reference.Kind != "FACE" && reference.Kind != "EDGE" && reference.Kind != "VERTEX" {
		return nil
	}
	if reference.PersistentSelection != nil {
		reference.GeometryKey, reference.TopologyID = "", 0
		if err := reference.PersistentSelection.Validate(); err != nil {
			return err
		}
		// Publication targets already carry a bound deep link but still need a
		// resolution snapshot for the accepted instance revision.
		if reference.PublicationRef == nil {
			return nil
		}
	}
	instance, _, occurrenceErr := service.assemblyReferenceOccurrence(ctx, *product, *reference)
	if occurrenceErr != nil {
		return occurrenceErr
	}
	selection := reference.PersistentSelection
	if selection == nil {
		bound, err := service.BindPersistentSelection(ctx, instance.ReferencedDocumentID, BindPersistentSelectionRequest{SourceVersionID: instance.ReferencedVersionID, GeometryKey: reference.GeometryKey, Kind: reference.Kind, LocalID: reference.TopologyID})
		if err != nil {
			return err
		}
		selection = &bound
		reference.PersistentSelection, reference.SourceVersionID = selection, instance.ReferencedVersionID
	}
	reference.GeometryKey, reference.TopologyID = "", 0
	_, _, digest, err := service.topologyManifestForVersion(ctx, instance.ReferencedDocumentID, instance.ReferencedVersionID)
	if err != nil {
		return err
	}
	resolution, err := service.ResolvePersistentSelection(ctx, instance.ReferencedDocumentID, ResolvePersistentSelectionRequest{Selection: *selection, SourceVersionID: reference.SourceVersionID, TargetVersionID: instance.ReferencedVersionID, ManifestDigest: digest, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
	if err != nil {
		return err
	}
	reference.Resolution = &ResolutionSnapshot{SourceVersionID: reference.SourceVersionID, TargetVersionID: instance.ReferencedVersionID, ManifestDigest: digest, PolicyDigest: modelcore.TopologyNamingPolicyDigest, Result: resolution}
	return nil
}

func (service *Service) updateProductReferences(ctx context.Context, product *ProductModel) error {
	instances := map[string]*ProductInstance{}
	for index := range product.Instances {
		instance := &product.Instances[index]
		instances[instance.ID] = instance
		if instance.ReferenceMode == "" {
			instance.ReferenceMode = "FOLLOW_HEAD"
		}
		if instance.ReferenceMode == "FOLLOW_HEAD" || instance.ReferenceMode == "FOLLOW_WORKSPACE_WITH_ACCEPT" {
			if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, instance.ReferencedDocumentID).Scan(&instance.ReferencedVersionID); err != nil {
				return err
			}
		}
		instance.ResolvedVersionID, instance.HeadChanged = instance.ReferencedVersionID, false
	}
	for index := range product.ContextBindings {
		binding := &product.ContextBindings[index]
		owningPath, err := service.refreshProductInstancePath(ctx, *product, binding.OwningInstancePath)
		if err != nil {
			return err
		}
		sourcePath, err := service.refreshProductInstancePath(ctx, *product, binding.SourceInstancePath)
		if err != nil {
			return err
		}
		binding.OwningInstancePath, binding.SourceInstancePath = owningPath, sourcePath
		if len(owningPath.Segments) > 0 {
			binding.Accepted.OwningRevisionID = owningPath.Segments[len(owningPath.Segments)-1].ResolvedVersionID
		}
		if binding.ReferenceMode == "PINNED" || len(binding.SourceInstancePath.Segments) == 0 {
			continue
		}
		sourceDocumentID := binding.SourceInstancePath.Segments[len(binding.SourceInstancePath.Segments)-1].ReferencedDocumentID
		var sourceHead string
		if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents
			WHERE id=$1 AND deleted_at IS NULL`, sourceDocumentID).Scan(&sourceHead); err != nil {
			return err
		}
		publication, err := service.publicationAtRevision(ctx, sourceDocumentID, sourceHead, binding.Publication.PublicationID)
		if err != nil {
			return err
		}
		if publication.Type != binding.Publication.ExpectedType ||
			(binding.Publication.CompatibilityVersion != "" && publication.CompatibilityVersion != binding.Publication.CompatibilityVersion) ||
			publication.Resolution.Status != "CONNECTED" {
			return fmt.Errorf("%w: PUBLICATION_CONTRACT_INCOMPATIBLE", ErrValidation)
		}
		binding.SourceInstancePath.Segments[len(binding.SourceInstancePath.Segments)-1].ResolvedVersionID = sourceHead
		binding.Publication.PersistentSelection = publication.Target.PersistentSelection
		binding.Publication.SelectionSourceVersionID = publication.Target.SourceVersionID
		binding.Resolution = publication.Resolution
		binding.Accepted.SourceRevisionID = sourceHead
		binding.Accepted.ContractDigest = resolvedDigest(publication.Contract)
		binding.Accepted.SourceDigest = publication.Resolution.SourceDigest
		binding.Accepted.Status = "CONNECTED"
	}
	if err := service.resolveAssemblySupports(ctx, product); err != nil {
		return err
	}
	variants, err := service.contextVariantsForProductModel(ctx, *product)
	if err != nil {
		return err
	}
	service.resolveProductPublications(ctx, product, variants)
	return nil
}

// Resolve only against accepted instance revisions; activation must not accept new Heads.
func (service *Service) resolveAssemblySupports(ctx context.Context, product *ProductModel, exclusions ...map[string]bool) error {
	instances := map[string]*ProductInstance{}
	for i := range product.Instances {
		instances[product.Instances[i].ID] = &product.Instances[i]
	}
	acceptedParts := map[string]PartModel{}
	resolveEndpoint := func(reference *AssemblyGeometryRef) (modelcore.SelectionResolutionStatus, error) {
		if reference == nil {
			return modelcore.SelectionResolved, nil
		}
		if reference.PublicationRef != nil {
			if _, err := service.resolveAssemblyPublication(ctx, product, reference); err != nil {
				code := "PUBLICATION_RESOLUTION_FAILED"
				if strings.Contains(err.Error(), "PUBLICATION_MISSING") {
					code = "PUBLICATION_MISSING"
				}
				if strings.Contains(err.Error(), "PUBLICATION_CONTRACT_INCOMPATIBLE") {
					code = "PUBLICATION_CONTRACT_INCOMPATIBLE"
				}
				reference.PublicationResolution = &PublicationResolution{Status: "BROKEN_PUBLICATION",
					DiagnosticCode: code, Diagnostic: err.Error()}
				resolution := unavailableSelectionResolution(code, err.Error())
				reference.Resolution = &ResolutionSnapshot{PolicyDigest: modelcore.TopologyNamingPolicyDigest, Result: resolution}
				return modelcore.SelectionSourceUnavailable, nil
			}
		}
		instance, _, occurrenceErr := service.assemblyReferenceOccurrence(ctx, *product, *reference)
		if occurrenceErr != nil {
			if !errors.Is(occurrenceErr, ErrValidation) {
				return modelcore.SelectionSourceUnavailable, occurrenceErr
			}
			reference.Resolution = &ResolutionSnapshot{PolicyDigest: modelcore.TopologyNamingPolicyDigest, Result: unavailableSelectionResolution("INSTANCE_MISSING", occurrenceErr.Error())}
			return modelcore.SelectionSourceUnavailable, nil
		}
		if instances[reference.InstanceID] == nil {
			reference.Resolution = &ResolutionSnapshot{PolicyDigest: modelcore.TopologyNamingPolicyDigest, Result: unavailableSelectionResolution("INSTANCE_MISSING", "support instance no longer exists")}
			return modelcore.SelectionSourceUnavailable, nil
		}
		if reference.Kind != "FACE" && reference.Kind != "EDGE" && reference.Kind != "VERTEX" {
			if reference.Kind == "BODY" || reference.PublicationRef != nil {
				return modelcore.SelectionResolved, nil
			}
			part, ok := acceptedParts[instance.ReferencedVersionID]
			if !ok {
				var raw []byte
				if err := service.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1`, instance.ReferencedVersionID).Scan(&raw); err != nil {
					return modelcore.SelectionSourceUnavailable, err
				}
				if err := json.Unmarshal(raw, &part); err != nil {
					return modelcore.SelectionSourceUnavailable, err
				}
				acceptedParts[instance.ReferencedVersionID] = part
			}
			if !datumAssemblyReferenceExists(part, *reference) {
				reference.Resolution = &ResolutionSnapshot{TargetVersionID: instance.ReferencedVersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest, Result: unavailableSelectionResolution("DATUM_SUPPORT_MISSING", "support datum or axis direction no longer exists in the accepted revision")}
				return modelcore.SelectionSourceUnavailable, nil
			}
			reference.Resolution = nil
			return modelcore.SelectionResolved, nil
		}
		if instance == nil || reference.PersistentSelection == nil {
			resolution := unavailableSelectionResolution("PERSISTENT_SELECTION_UNAVAILABLE", "assembly endpoint has no source instance or persistent selection")
			reference.Resolution = &ResolutionSnapshot{SourceVersionID: reference.SourceVersionID,
				PolicyDigest: modelcore.TopologyNamingPolicyDigest, Result: resolution}
			return modelcore.SelectionSourceUnavailable, nil
		}
		_, _, digest, err := service.topologyManifestForVersion(ctx, instance.ReferencedDocumentID, instance.ReferencedVersionID)
		if err != nil {
			if diagnostic, ok := namingDiagnostic(err); ok {
				resolution := unavailableSelectionResolution(diagnostic.DiagnosticCode, diagnostic.Diagnostic)
				reference.Resolution = &ResolutionSnapshot{SourceVersionID: reference.SourceVersionID, TargetVersionID: instance.ReferencedVersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest, Result: resolution}
				return resolution.Status, nil
			}
			if errors.Is(err, ErrNotFound) {
				resolution := unavailableSelectionResolution("PERSISTENT_SELECTION_UNAVAILABLE", "target revision has no persistent topology manifest")
				reference.Resolution = &ResolutionSnapshot{SourceVersionID: reference.SourceVersionID,
					TargetVersionID: instance.ReferencedVersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest, Result: resolution}
				return modelcore.SelectionSourceUnavailable, nil
			}
			return modelcore.SelectionSourceUnavailable, err
		}
		resolution, err := service.ResolvePersistentSelection(ctx, instance.ReferencedDocumentID, ResolvePersistentSelectionRequest{Selection: *reference.PersistentSelection, SourceVersionID: reference.SourceVersionID, TargetVersionID: instance.ReferencedVersionID, ManifestDigest: digest, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
		if err != nil {
			return modelcore.SelectionSourceUnavailable, err
		}
		reference.Resolution = &ResolutionSnapshot{SourceVersionID: reference.SourceVersionID, TargetVersionID: instance.ReferencedVersionID, ManifestDigest: digest, PolicyDigest: modelcore.TopologyNamingPolicyDigest, Result: resolution}
		return resolution.Status, nil
	}
	for index := range product.Constraints {
		constraint := &product.Constraints[index]
		if len(exclusions) > 0 && exclusions[0][constraint.ID] {
			continue
		}
		first, firstErr := resolveEndpoint(&constraint.First)
		second, secondErr := resolveEndpoint(constraint.Second)
		axis, axisErr := modelcore.SelectionResolved, error(nil)
		if constraint.Kind == "ANGLE" && (constraint.AngleRelation == "DIRECTED" || constraint.AngleRelation == "") {
			axis, axisErr = resolveEndpoint(constraint.AngleAxis)
		}
		if constraint.Suppressed {
			if firstErr != nil || secondErr != nil || axisErr != nil || first != modelcore.SelectionResolved || second != modelcore.SelectionResolved || axis != modelcore.SelectionResolved {
				constraint.EvaluationStatus = modelcore.AssemblyConstraintBroken
				constraint.EvaluationSummary = fmt.Sprintf("inactive support resolution: first=%s second=%s axis=%s", first, second, axis)
			}
			continue
		}
		if firstErr != nil {
			return firstErr
		}
		if secondErr != nil {
			return secondErr
		}
		if axisErr != nil {
			return axisErr
		}
		if first != modelcore.SelectionResolved || second != modelcore.SelectionResolved || axis != modelcore.SelectionResolved {
			constraint.EvaluationStatus = modelcore.AssemblyConstraintBroken
			constraint.EvaluationSummary = fmt.Sprintf("support resolution: first=%s second=%s axis=%s", first, second, axis)
		} else if constraint.EvaluationStatus != modelcore.AssemblyConstraintVerified {
			constraint.EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
			constraint.EvaluationSummary = "references resolved; awaiting authoritative solve"
		}
	}
	return nil
}

func (service *Service) refreshProductInstancePath(ctx context.Context, root ProductModel, path InstancePath) (InstancePath, error) {
	if len(path.Segments) == 0 {
		return path, fmt.Errorf("%w: ContextBinding path is empty", ErrValidation)
	}
	result := InstancePath{RootDocumentID: path.RootDocumentID}
	current := root
	for index, old := range path.Segments {
		var instance *ProductInstance
		for candidate := range current.Instances {
			if current.Instances[candidate].ID == old.InstanceID {
				instance = &current.Instances[candidate]
				break
			}
		}
		if instance == nil {
			return InstancePath{}, fmt.Errorf("%w: ContextBinding occurrence path disappeared", ErrValidation)
		}
		ownerDocumentID, ownerVersionID := old.OwnerDocumentID, old.OwnerVersionID
		if index > 0 {
			ownerDocumentID = result.Segments[index-1].ReferencedDocumentID
			ownerVersionID = result.Segments[index-1].ResolvedVersionID
		}
		result = appendInstancePath(result, InstancePathSegment{OwnerDocumentID: ownerDocumentID, OwnerVersionID: ownerVersionID,
			InstanceID: instance.ID, InstanceName: instance.Name, ReferencedDocumentID: instance.ReferencedDocumentID,
			ResolvedVersionID: instance.ReferencedVersionID})
		if index == len(path.Segments)-1 {
			break
		}
		var documentType string
		var raw []byte
		if err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v
			JOIN occccad.documents d ON d.id=v.document_id WHERE d.id=$1 AND v.id=$2`, instance.ReferencedDocumentID,
			instance.ReferencedVersionID).Scan(&documentType, &raw); err != nil {
			return InstancePath{}, err
		}
		if documentType != "PRODUCT" {
			return InstancePath{}, fmt.Errorf("%w: ContextBinding path continues through a Part", ErrValidation)
		}
		if err := json.Unmarshal(raw, &current); err != nil {
			return InstancePath{}, err
		}
	}
	return result, nil
}

func (service *Service) GetResolvedTopologyElementProperties(ctx context.Context, documentID string, request ResolvePersistentSelectionRequest) (ResolvedTopologyProperties, error) {
	resolution, err := service.ResolvePersistentSelection(ctx, documentID, request)
	if err != nil {
		return ResolvedTopologyProperties{}, err
	}
	result := ResolvedTopologyProperties{Resolution: resolution}
	if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
		return result, nil
	}
	candidate := resolution.Candidates[0]
	properties, err := service.GetTopologyElementPropertiesAtVersion(ctx, documentID, request.TargetVersionID, candidate.GeometryKey, string(candidate.Type), candidate.LocalID)
	if err != nil {
		return ResolvedTopologyProperties{}, err
	}
	result.Properties = &properties
	return result, nil
}

// The resolver verifies both document-scoped revisions, policy, creation evidence
// and target membership. Do not route its exact candidate through the interactive
// pick API, which authorizes and binds that same topology all over again.
func (service *Service) resolvedAssemblyTopologyProperties(ctx context.Context, documentID string, request ResolvePersistentSelectionRequest) (ResolvedTopologyProperties, error) {
	// Match the inspection API's live-document guard once per solve. User
	// authorization remains at the request boundary and is never cached here.
	if request.Selection.SourceDocumentID != documentID {
		return ResolvedTopologyProperties{}, fmt.Errorf("%w: selection document mismatch", ErrValidation)
	}
	_, err := assemblyRead(ctx, assemblyReadKey{"live-document", documentID, ""}, func() (bool, error) {
		var live bool
		err := service.database.QueryRow(ctx, `SELECT true FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, documentID).Scan(&live)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrNotFound
		}
		return live, err
	})
	if err != nil {
		return ResolvedTopologyProperties{}, err
	}
	resolution, err := service.ResolvePersistentSelection(ctx, documentID, request)
	result := ResolvedTopologyProperties{Resolution: resolution}
	if err != nil || resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
		return result, err
	}
	candidate := resolution.Candidates[0]
	properties, err := service.getTopologyElementPropertiesFromArtifact(ctx, candidate.GeometryKey, string(candidate.Type), candidate.LocalID)
	if err != nil {
		return result, err
	}
	result.Properties = &properties
	return result, nil
}

func semanticRef(source *workerv1.SemanticTopologyRef) modelcore.SemanticTopologyRef {
	return modelcore.SemanticTopologyRef{FeatureID: source.GetFeatureId(), OutputSlot: source.GetOutputSlot(), SourceIDs: append([]string(nil), source.GetSourceIds()...)}
}

func selectionEvidence(source *workerv1.SelectionEvidence) modelcore.TopologySelectionEvidence {
	result := modelcore.TopologySelectionEvidence{GeometryType: source.GetGeometryType(), MeasureDimension: source.GetMeasureDimension(), EvidenceDigest: source.GetEvidenceDigest()}
	if source.MeasureSi != nil {
		value := source.GetMeasureSi()
		result.MeasureSI = &value
	}
	if source.ParameterStart != nil {
		value := source.GetParameterStart()
		result.ParameterStart = &value
	}
	if source.ParameterEnd != nil {
		value := source.GetParameterEnd()
		result.ParameterEnd = &value
	}
	result.EndpointRole = source.GetEndpointRole()
	if v := source.GetCentroid(); v != nil {
		result.Centroid = [3]float64{v.GetX(), v.GetY(), v.GetZ()}
	}
	if v := source.GetOrigin(); v != nil {
		result.Origin = [3]float64{v.GetX(), v.GetY(), v.GetZ()}
	}
	if v := source.GetDirection(); v != nil {
		result.Direction = [3]float64{v.GetX(), v.GetY(), v.GetZ()}
	}
	for _, adjacent := range source.GetAdjacent() {
		result.Adjacent = append(result.Adjacent, semanticRef(adjacent))
	}
	return result
}

func topologyType(source workerv1.PersistentTopologyType) modelcore.PersistentTopologyType {
	switch source {
	case workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE:
		return modelcore.PersistentTopologyFace
	case workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_EDGE:
		return modelcore.PersistentTopologyEdge
	case workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_VERTEX:
		return modelcore.PersistentTopologyVertex
	}
	return ""
}

func sameSemanticRef(left, right modelcore.SemanticTopologyRef) bool {
	if left.FeatureID != right.FeatureID || left.OutputSlot != right.OutputSlot || len(left.SourceIDs) != len(right.SourceIDs) {
		return false
	}
	for index := range left.SourceIDs {
		if left.SourceIDs[index] != right.SourceIDs[index] {
			return false
		}
	}
	return true
}

func refKey(ref modelcore.SemanticTopologyRef) string {
	data, _ := json.Marshal(ref)
	return string(data)
}

func containsRef(values []modelcore.SemanticTopologyRef, expected modelcore.SemanticTopologyRef) bool {
	for _, value := range values {
		if sameSemanticRef(value, expected) {
			return true
		}
	}
	return false
}

func candidateMatchesRecipe(selection modelcore.PersistentSelection, ref modelcore.SemanticTopologyRef, evidence modelcore.TopologySelectionEvidence) bool {
	if selection.CreationEvidence.GeometryType != "" && evidence.GeometryType != selection.CreationEvidence.GeometryType {
		return false
	}
	if selection.CreationEvidence.MeasureDimension != "" && evidence.MeasureDimension != selection.CreationEvidence.MeasureDimension {
		return false
	}
	switch selection.Selector.Kind {
	case modelcore.SelectionDirectSemanticOutput:
		return sameSemanticRef(ref, selection.Anchor)
	case modelcore.SelectionLineageDescendant:
		return true
	case modelcore.SelectionAdjacentTo, modelcore.SelectionIntersectionOf:
		for _, operand := range selection.Selector.Operands {
			if !containsRef(evidence.Adjacent, operand) {
				return false
			}
		}
		return len(selection.Selector.Operands) > 0
	case modelcore.SelectionOwnedByBoundary:
		for _, operand := range selection.Selector.Operands {
			for _, sourceID := range operand.SourceIDs {
				if containsString(ref.SourceIDs, sourceID) {
					return true
				}
			}
		}
	}
	return false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func manifestOutput(manifest *workerv1.PartTopologyManifest, kind modelcore.PersistentTopologyType, localID uint64) (*workerv1.SemanticTopologyOutput, string) {
	features := manifest.GetFeatureResults()
	if len(features) == 0 {
		return nil, ""
	}
	feature := features[len(features)-1]
	for _, output := range feature.GetSemanticOutputs() {
		if topologyType(output.GetTopologyType()) == kind && output.GetLocalId() == localID {
			return output, feature.GetBodyId()
		}
	}
	return nil, ""
}

func manifestSemanticOutput(manifest *workerv1.PartTopologyManifest, selection modelcore.PersistentSelection) *workerv1.SemanticTopologyOutput {
	features := manifest.GetFeatureResults()
	if len(features) == 0 || features[len(features)-1].GetBodyId() != selection.SourceBodyID {
		return nil
	}
	for _, output := range features[len(features)-1].GetSemanticOutputs() {
		if topologyType(output.GetTopologyType()) == selection.ExpectedType && sameSemanticRef(semanticRef(output.GetSemanticRef()), selection.Anchor) {
			return output
		}
	}
	return nil
}

func topologyHistoryComplete(manifest *workerv1.PartTopologyManifest) bool {
	features := manifest.GetFeatureResults()
	if len(features) == 0 {
		return false
	}
	for _, feature := range features {
		if !feature.GetTopologyHistoryComplete() {
			return false
		}
	}
	return true
}

func unavailableSelectionResolution(code, diagnostic string) modelcore.SelectionResolution {
	return modelcore.SelectionResolution{Status: modelcore.SelectionSourceUnavailable,
		SupportingElementStatus: modelcore.SupportingElementNotConnected,
		DiagnosticCode:          code, Diagnostic: diagnostic}
}

// resolveManifest follows semantic lineage only. Geometry evidence confirms and explains
// candidates; it is never used as a nearest-shape identity fallback.
func resolveManifest(selection modelcore.PersistentSelection, geometryKey string, manifest *workerv1.PartTopologyManifest) modelcore.SelectionResolution {
	result := modelcore.SelectionResolution{Status: modelcore.SelectionMissing, SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "PERSISTENT_SELECTION_MISSING", Diagnostic: "semantic anchor has no result in the target body tip"}
	current := map[string]modelcore.SemanticTopologyRef{refKey(selection.Anchor): selection.Anchor}
	seenAnchor := false
	for _, feature := range manifest.GetFeatureResults() {
		if feature.GetBodyId() != selection.SourceBodyID {
			continue
		}
		for _, lineage := range feature.GetTopologyHistory().GetLineage() {
			matched := false
			for _, source := range lineage.GetSources() {
				if _, ok := current[refKey(semanticRef(source))]; ok {
					matched = true
					seenAnchor = true
					break
				}
			}
			if matched {
				ref := semanticRef(lineage.GetResult())
				current[refKey(ref)] = ref
			}
		}
		for _, deleted := range feature.GetTopologyHistory().GetDeleted() {
			delete(current, refKey(semanticRef(deleted.GetSource())))
		}
	}
	if !seenAnchor {
		for _, feature := range manifest.GetFeatureResults() {
			for _, output := range feature.GetSemanticOutputs() {
				if sameSemanticRef(semanticRef(output.GetSemanticRef()), selection.Anchor) {
					seenAnchor = true
				}
			}
		}
	}
	if !seenAnchor {
		result.Status = modelcore.SelectionOutsideCurrentTip
		result.DiagnosticCode = "PERSISTENT_SELECTION_OUTSIDE_CURRENT_TIP"
		result.Diagnostic = "anchor feature is not part of the target body tip"
		return result
	}
	last := manifest.GetFeatureResults()
	if len(last) == 0 {
		return result
	}
	for _, output := range last[len(last)-1].GetSemanticOutputs() {
		ref := semanticRef(output.GetSemanticRef())
		if _, ok := current[refKey(ref)]; !ok {
			continue
		}
		if topologyType(output.GetTopologyType()) != selection.ExpectedType {
			result.Status = modelcore.SelectionTypeMismatch
			result.DiagnosticCode = "PERSISTENT_SELECTION_TYPE_MISMATCH"
			continue
		}
		evidence := selectionEvidence(output.GetEvidence())
		if !candidateMatchesRecipe(selection, ref, evidence) {
			continue
		}
		result.Candidates = append(result.Candidates, modelcore.ResolvedTopologyElement{GeometryID: last[len(last)-1].GetResultGeometryId(), GeometryKey: geometryKey, Type: selection.ExpectedType, LocalID: output.GetLocalId(), SemanticRef: ref, Evidence: evidence})
	}
	sort.Slice(result.Candidates, func(i, j int) bool {
		return refKey(result.Candidates[i].SemanticRef) < refKey(result.Candidates[j].SemanticRef)
	})
	switch len(result.Candidates) {
	case 1:
		result.Status = modelcore.SelectionResolved
		result.SupportingElementStatus = modelcore.SupportingElementConnected
		result.DiagnosticCode = ""
		result.Diagnostic = ""
		result.EvidenceDigest = result.Candidates[0].Evidence.EvidenceDigest
	case 0:
		if result.Status != modelcore.SelectionTypeMismatch {
			result.Status = modelcore.SelectionMissing
		}
	default:
		result.Status = modelcore.SelectionAmbiguous
		result.DiagnosticCode = "PERSISTENT_SELECTION_AMBIGUOUS"
		result.Diagnostic = "lineage produced multiple valid candidates; reconnect or use a more specific recipe"
	}
	return result
}

func (service *Service) topologyManifestForVersion(ctx context.Context, documentID, versionID string) (*workerv1.PartTopologyManifest, string, string, error) {
	value, err := assemblyRead(ctx, assemblyReadKey{"manifest", documentID, versionID}, func() (assemblyManifestRead, error) {
		manifest, geometry, digest, err := service.loadTopologyManifestForVersion(ctx, documentID, versionID)
		return assemblyManifestRead{manifest, geometry, digest}, err
	})
	return value.manifest, value.geometry, value.digest, err
}

func (service *Service) loadTopologyManifestForVersion(ctx context.Context, documentID, versionID string) (*workerv1.PartTopologyManifest, string, string, error) {
	defer perf.Start(ctx, "topology-manifest-read")()

	var geometryKey string
	var digest, objectID *string
	var inline []byte
	err := service.database.QueryRow(ctx, `SELECT v.geometry_key,COALESCE(a.topology_manifest_data,''::bytea),a.topology_manifest_object_id::text,a.topology_manifest_digest FROM occccad.document_versions v JOIN occccad.geometry_artifacts a ON a.geometry_key=v.geometry_key WHERE v.id=$2 AND v.document_id=$1`, documentID, versionID).Scan(&geometryKey, &inline, &objectID, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", "", ErrNotFound
	}
	if err != nil {
		return nil, "", "", err
	}
	manifest, resolvedDigest, err := service.readTopologyManifest(ctx, inline, objectID, digest)
	return manifest, geometryKey, resolvedDigest, err
}

func (service *Service) BindPersistentSelection(ctx context.Context, documentID string, request BindPersistentSelectionRequest) (modelcore.PersistentSelection, error) {
	kind := modelcore.PersistentTopologyType(strings.ToUpper(request.Kind))
	if request.SourceVersionID == "" || request.GeometryKey == "" || request.LocalID == 0 {
		return modelcore.PersistentSelection{}, fmt.Errorf("%w: sourceVersionId, geometryKey and localId are required", ErrValidation)
	}
	manifest, geometryKey, _, err := service.topologyManifestForVersion(ctx, documentID, request.SourceVersionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return modelcore.PersistentSelection{}, fmt.Errorf("%w: PERSISTENT_SELECTION_UNAVAILABLE", ErrValidation)
		}
		return modelcore.PersistentSelection{}, err
	}
	if !topologyHistoryComplete(manifest) {
		return modelcore.PersistentSelection{}, fmt.Errorf("%w: TOPOLOGY_HISTORY_INCOMPLETE", ErrValidation)
	}
	if geometryKey != request.GeometryKey {
		return modelcore.PersistentSelection{}, fmt.Errorf("%w: geometryKey does not belong to source revision", ErrValidation)
	}
	output, bodyID := manifestOutput(manifest, kind, request.LocalID)
	if output == nil {
		return modelcore.PersistentSelection{}, ErrNotFound
	}
	selection := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: documentID, SourceBodyID: bodyID, Anchor: semanticRef(output.GetSemanticRef()), ExpectedType: kind, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: selectionEvidence(output.GetEvidence())}
	return selection, selection.Validate()
}

func (service *Service) ResolvePersistentSelection(ctx context.Context, documentID string, request ResolvePersistentSelectionRequest) (modelcore.SelectionResolution, error) {
	if err := request.Selection.Validate(); err != nil {
		return modelcore.SelectionResolution{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if request.Selection.SourceDocumentID != documentID || request.SourceVersionID == "" || request.TargetVersionID == "" {
		return modelcore.SelectionResolution{}, fmt.Errorf("%w: selection document and source/target revisions are required", ErrValidation)
	}
	if request.PolicyDigest != modelcore.TopologyNamingPolicyDigest {
		return modelcore.SelectionResolution{Status: modelcore.SelectionContractMismatch, SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "RESOLVER_POLICY_MISMATCH"}, nil
	}
	sourceManifest, _, _, err := service.topologyManifestForVersion(ctx, documentID, request.SourceVersionID)
	if err != nil {
		if diagnostic, ok := namingDiagnostic(err); ok {
			return unavailableSelectionResolution(diagnostic.DiagnosticCode, diagnostic.Diagnostic), nil
		}
		if errors.Is(err, ErrNotFound) {
			return unavailableSelectionResolution("PERSISTENT_SELECTION_UNAVAILABLE", "source revision has no persistent topology manifest"), nil
		}
		return modelcore.SelectionResolution{}, err
	}
	if !topologyHistoryComplete(sourceManifest) {
		return unavailableSelectionResolution("TOPOLOGY_HISTORY_INCOMPLETE", "source revision does not declare complete topology history"), nil
	}
	sourceOutput := manifestSemanticOutput(sourceManifest, request.Selection)
	if sourceOutput == nil {
		return unavailableSelectionResolution("PERSISTENT_SELECTION_UNAVAILABLE", "source semantic output is unavailable"), nil
	}
	if request.Selection.CreationEvidence.EvidenceDigest != "" && request.Selection.CreationEvidence.EvidenceDigest != sourceOutput.GetEvidence().GetEvidenceDigest() {
		return modelcore.SelectionResolution{Status: modelcore.SelectionContractMismatch, SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "CREATION_EVIDENCE_MISMATCH"}, nil
	}
	manifest, geometryKey, digest, err := service.topologyManifestForVersion(ctx, documentID, request.TargetVersionID)
	if err != nil {
		if diagnostic, ok := namingDiagnostic(err); ok {
			return unavailableSelectionResolution(diagnostic.DiagnosticCode, diagnostic.Diagnostic), nil
		}
		if errors.Is(err, ErrNotFound) {
			return unavailableSelectionResolution("PERSISTENT_SELECTION_UNAVAILABLE", "target revision has no persistent topology manifest"), nil
		}
		return modelcore.SelectionResolution{}, err
	}
	if !topologyHistoryComplete(manifest) {
		return unavailableSelectionResolution("TOPOLOGY_HISTORY_INCOMPLETE", "target revision does not declare complete topology history"), nil
	}
	if request.ManifestDigest != "" && !strings.EqualFold(request.ManifestDigest, digest) {
		return modelcore.SelectionResolution{Status: modelcore.SelectionContractMismatch, SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "TARGET_MANIFEST_MISMATCH"}, nil
	}
	selectionJSON, _ := json.Marshal(request.Selection)
	cacheHash := sha256.Sum256(append(selectionJSON, []byte("|"+request.TargetVersionID+"|"+digest+"|"+request.PolicyDigest)...))
	cacheKey := hex.EncodeToString(cacheHash[:])
	if cached, ok := service.selectionResolutions.Load(cacheKey); ok {
		return cached.(modelcore.SelectionResolution), nil
	}
	resolution := resolveManifest(request.Selection, geometryKey, manifest)
	service.selectionResolutions.Store(cacheKey, resolution)
	return resolution, nil
}

func datumAssemblyReferenceExists(part PartModel, reference AssemblyGeometryRef) bool {
	if reference.Kind == "PLANE" {
		for _, plane := range part.DatumPlanes {
			if plane.ID == reference.GeometryID {
				return true
			}
		}
	}
	if reference.Kind == "AXIS" {
		for _, axis := range part.DatumAxes {
			if axis.ID == reference.GeometryID {
				return true
			}
		}
	}
	if reference.Kind == "AXIS" || reference.Kind == "POINT" {
		for _, frame := range part.AxisSystems {
			if frame.ID == reference.GeometryID {
				return reference.Kind == "POINT" || reference.Axis == "X" || reference.Axis == "Y" || reference.Axis == "Z"
			}
		}
	}
	return false
}
