package workspace

// Frozen pre-index evaluator. This deliberately scans transitions in temporal
// order so differential tests can detect changes in deletion/reintroduction,
// generated support, ambiguity and same-stage ordering.
import (
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"sort"
)

func resolveManifestLinearOracle(selection modelcore.PersistentSelection, geometryKey string, manifest *topologyManifest) modelcore.SelectionResolution {
	result := modelcore.SelectionResolution{Status: modelcore.SelectionMissing, SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "PERSISTENT_SELECTION_MISSING", Diagnostic: "semantic anchor has no result in the target body tip"}
	current := map[string]modelcore.SemanticTopologyRef{refKey(selection.Anchor): selection.Anchor}
	seenAnchor := false
	for _, feature := range manifest.GetFeatureResults() {
		if !selectionOwnsFeature(selection, feature) {
			continue
		}
		for _, lineage := range feature.GetTopologyHistory().GetLineage() {
			if sameSemanticRef(semanticRef(lineage.GetResult()), selection.Anchor) {
				seenAnchor = true
			}
			matched := false
			for _, source := range lineage.GetSources() {
				if _, ok := current[refKey(semanticRef(source))]; ok {
					matched = true
					seenAnchor = true
					break
				}
			}
			if matched {
				// Generated records constructive support, not continuation of
				// the source's identity (e.g. shell inner walls, fillet surfaces,
				// or pattern replicas). Explicit generated anchors still start
				// in current; follow their Modified/Split/Merged history normally.
				if lineage.Kind == workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED {
					continue
				}
				ref := semanticRef(lineage.GetResult())
				current[refKey(ref)] = ref
			}
		}
		for _, ambiguous := range feature.GetTopologyHistory().GetAmbiguous() {
			matched := false
			for _, source := range ambiguous.GetSources() {
				if _, ok := current[refKey(semanticRef(source))]; ok {
					matched = true
					seenAnchor = true
				}
			}
			if matched {
				for _, candidate := range ambiguous.GetCandidates() {
					ref := semanticRef(candidate)
					current[refKey(ref)] = ref
				}
			}
		}
		for _, deleted := range feature.GetTopologyHistory().GetDeleted() {
			if sameSemanticRef(semanticRef(deleted.GetSource()), selection.Anchor) {
				seenAnchor = true
			}
			delete(current, refKey(semanticRef(deleted.GetSource())))
		}
	}
	if !seenAnchor {
		for _, feature := range manifest.GetFeatureResults() {
			if !selectionOwnsFeature(selection, feature) {
				continue
			}
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
	tip := manifest.selectionTip(selection)
	if tip == nil {
		return result
	}
	for _, output := range tip.GetSemanticOutputs() {
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
		result.Candidates = append(result.Candidates, modelcore.ResolvedTopologyElement{GeometryID: manifest.GeometryID, GeometryKey: geometryKey, Type: selection.ExpectedType, LocalID: output.GetLocalId(), SemanticRef: ref, Evidence: evidence})
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
