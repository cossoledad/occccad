package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
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
	properties, err := service.GetTopologyElementProperties(ctx, documentID, candidate.GeometryKey, string(candidate.Type), candidate.LocalID)
	if err != nil {
		return ResolvedTopologyProperties{}, err
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
	var geometryKey, digest string
	var inline []byte
	var objectID *string
	err := service.database.QueryRow(ctx, `SELECT v.geometry_key,COALESCE(a.topology_manifest_data,''::bytea),a.topology_manifest_object_id::text,a.topology_manifest_digest FROM occccad.document_versions v JOIN occccad.geometry_artifacts a ON a.geometry_key=v.geometry_key WHERE v.id=$2 AND v.document_id=$1`, documentID, versionID).Scan(&geometryKey, &inline, &objectID, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", "", ErrNotFound
	}
	if err != nil {
		return nil, "", "", err
	}
	data := inline
	if len(data) == 0 && objectID != nil && service.artifacts != nil {
		_, reader, openErr := service.artifacts.Open(ctx, *objectID)
		if openErr != nil {
			return nil, "", "", openErr
		}
		data, err = io.ReadAll(reader)
		closeErr := reader.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, "", "", err
		}
	}
	if len(data) == 0 {
		return nil, "", "", fmt.Errorf("%w: topology manifest is unavailable", ErrNotFound)
	}
	hash := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(hash[:]), strings.TrimSpace(digest)) {
		return nil, "", "", fmt.Errorf("topology manifest digest mismatch")
	}
	manifest := &workerv1.PartTopologyManifest{}
	if err := proto.Unmarshal(data, manifest); err != nil {
		return nil, "", "", err
	}
	if manifest.GetSchemaVersion() != modelcore.TopologyNamingSchemaVersion || manifest.GetPolicyDigest() != modelcore.TopologyNamingPolicyDigest {
		return nil, "", "", fmt.Errorf("topology manifest naming contract mismatch")
	}
	return manifest, geometryKey, strings.TrimSpace(digest), nil
}

func (service *Service) BindPersistentSelection(ctx context.Context, documentID string, request BindPersistentSelectionRequest) (modelcore.PersistentSelection, error) {
	kind := modelcore.PersistentTopologyType(strings.ToUpper(request.Kind))
	if request.SourceVersionID == "" || request.GeometryKey == "" || request.LocalID == 0 {
		return modelcore.PersistentSelection{}, fmt.Errorf("%w: sourceVersionId, geometryKey and localId are required", ErrValidation)
	}
	manifest, geometryKey, _, err := service.topologyManifestForVersion(ctx, documentID, request.SourceVersionID)
	if err != nil {
		return modelcore.PersistentSelection{}, err
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
		if errors.Is(err, ErrNotFound) {
			return modelcore.SelectionResolution{Status: modelcore.SelectionSourceUnavailable, SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "PERSISTENT_SELECTION_SOURCE_UNAVAILABLE"}, nil
		}
		return modelcore.SelectionResolution{}, err
	}
	sourceOutput := manifestSemanticOutput(sourceManifest, request.Selection)
	if sourceOutput == nil {
		return modelcore.SelectionResolution{Status: modelcore.SelectionSourceUnavailable, SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "PERSISTENT_SELECTION_SOURCE_UNAVAILABLE"}, nil
	}
	if request.Selection.CreationEvidence.EvidenceDigest != "" && request.Selection.CreationEvidence.EvidenceDigest != sourceOutput.GetEvidence().GetEvidenceDigest() {
		return modelcore.SelectionResolution{Status: modelcore.SelectionContractMismatch, SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "CREATION_EVIDENCE_MISMATCH"}, nil
	}
	manifest, geometryKey, digest, err := service.topologyManifestForVersion(ctx, documentID, request.TargetVersionID)
	if err != nil {
		return modelcore.SelectionResolution{}, err
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
