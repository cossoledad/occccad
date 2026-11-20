package workspace

import (
	"context"
	"math"
	"slices"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

// The caller owns cardinality. Finite element picks and functional supports
// share the same authoritative, temporally ordered descendant search.
type namingUse uint8

const (
	finiteElement namingUse = iota
	analyticSupport
	descendantSet
)

func assemblyNamingUse(kind string, ref *AssemblyGeometryRef) namingUse {
	if ref == nil || ref.PersistentSelection == nil || ref.PublicationRef != nil || ref.DerivedRole != "" {
		return finiteElement
	}
	switch kind {
	case "DISTANCE":
		if ref.PersistentSelection.CreationEvidence.GeometryType == "PLANE" || ref.PersistentSelection.CreationEvidence.GeometryType == "LINE" {
			return analyticSupport
		}
	case "COINCIDENT", "CONCENTRIC", "PARALLEL", "PERPENDICULAR", "ANGLE":
		switch ref.PersistentSelection.CreationEvidence.GeometryType {
		case "LINE", "PLANE", "CYLINDER":
			return analyticSupport
		}
	}
	return finiteElement
}

// Only members returned by lineage may participate. Use the target-local
// analytic support; a functional binding remains free to move with the Part.
func commonAnalyticSupport(members []modelcore.ResolvedTopologyElement) *modelcore.AnalyticSupport {
	if len(members) == 0 {
		return nil
	}
	first := members[0].Evidence
	if first.GeometryType != "LINE" && first.GeometryType != "PLANE" && first.GeometryType != "CYLINDER" {
		return nil
	}
	dir, ok := normalize3(first.Direction)
	if !ok || !assemblyFiniteVector(first.Origin) {
		return nil
	}
	if first.GeometryType == "CYLINDER" && (first.RadiusMM == nil || !(*first.RadiusMM > 0) || math.IsInf(*first.RadiusMM, 0) || first.MaterialSide == nil || (*first.MaterialSide != 1 && *first.MaterialSide != -1)) {
		return nil
	}
	const linearMM = modelcore.TopologyNamingLinearToleranceMeters * 1000
	for _, m := range members[1:] {
		e := m.Evidence
		d, ok := normalize3(e.Direction)
		// Opposite orientations can encode different directed constraints.
		if !ok || e.GeometryType != first.GeometryType || !assemblyFiniteVector(e.Origin) || dot3(dir, d) <= 0 {
			return nil
		}
		if first.GeometryType == "CYLINDER" && (e.RadiusMM == nil || !(math.Abs(*e.RadiusMM-*first.RadiusMM) <= linearMM) || e.MaterialSide == nil || *e.MaterialSide != *first.MaterialSide) {
			return nil
		}
		skew := cross3(dir, d)
		if dot3(skew, skew) > modelcore.TopologyNamingAngularToleranceRadians*modelcore.TopologyNamingAngularToleranceRadians {
			return nil
		}
		offset := [3]float64{e.Origin[0] - first.Origin[0], e.Origin[1] - first.Origin[1], e.Origin[2] - first.Origin[2]}
		if first.GeometryType == "PLANE" {
			if math.Abs(dot3(dir, offset)) > linearMM {
				return nil
			}
		} else {
			lateral := cross3(offset, dir)
			if dot3(lateral, lateral) > linearMM*linearMM {
				return nil
			}
		}
	}
	// Unique point on the carrier closest to the Part origin, independent of
	// each trimmed segment's curve parameter zero or each face's UV origin.
	height := dot3(dir, first.Origin)
	var origin [3]float64
	for i := range origin {
		origin[i] = dir[i] * height
		if first.GeometryType != "PLANE" {
			origin[i] = first.Origin[i] - origin[i]
		}
	}
	support := &modelcore.AnalyticSupport{GeometryType: first.GeometryType, Origin: origin, Direction: dir}
	if first.GeometryType == "CYLINDER" {
		support.RadiusMM = *first.RadiusMM
		support.MaterialSide = *first.MaterialSide
	}
	return support
}

func resolveNamingUse(result modelcore.SelectionResolution, use namingUse, selection modelcore.PersistentSelection) modelcore.SelectionResolution {
	if result.Status != modelcore.SelectionAmbiguous {
		return result
	}
	switch use {
	case analyticSupport:
		result.Support = commonAnalyticSupport(result.Candidates)
		if result.Support == nil || result.Support.GeometryType != selection.CreationEvidence.GeometryType {
			result.Support = nil
			return result
		}
	case descendantSet:
		// Freeze the actual membership for dependency snapshots. The target
		// geometry key separately identifies the BREP used by the operation.
		type member struct {
			Ref      modelcore.SemanticTopologyRef
			Evidence string
		}
		members := make([]member, 0, len(result.Candidates))
		for _, candidate := range result.Candidates {
			members = append(members, member{candidate.SemanticRef, candidate.Evidence.EvidenceDigest})
		}
		result.EvidenceDigest = resolvedDigest(members)
	default:
		return result
	}
	if result.Support != nil {
		result.EvidenceDigest = resolvedDigest(*result.Support)
	}
	result.Status = modelcore.SelectionResolved
	result.SupportingElementStatus = modelcore.SupportingElementConnected
	result.DiagnosticCode, result.Diagnostic = "", ""
	return result
}

// A split ordinal only orders the candidates of one evaluation. When the
// creating cut changes, use its actual parent and distinguishing boundaries
// from the verified source tip to recover a finite branch.
func resolveBetweenNamingViews(selection modelcore.PersistentSelection, source, target *topologyManifest, key string, use namingUse) modelcore.SelectionResolution {
	dst := namingSearch(target)
	certifiedSuffix := target.resolutionPrefix != "" && target.resolutionPrefix == source.historyTipDigest
	sameTip := source == target || source.historyTipDigest != "" && source.historyTipDigest == target.historyTipDigest && source.GeometryID == target.GeometryID
	if !isSplitPick(selection) || certifiedSuffix || sameTip {
		return dst.resolveForUse(selection, key, target.GeometryID, nil, use)
	}
	src := namingSearch(source)
	owner := src.owners[namingSelectionOwner(selection)]
	id, ok := src.findSymbol(selection.Anchor)
	if !ok || owner == nil || owner.splitParent[id] == nil {
		return unavailableSelectionResolution("SPLIT_HISTORY_UNAVAILABLE", "creation split has no certified parent")
	}
	family := selection
	family.Anchor = semanticRef(owner.splitParent[id])
	if use == analyticSupport {
		// Nested split ordinals may all be reordered by an upstream edit.
		// A common carrier must hold for the family of the stable ancestor.
		supportFamily := family
		visited := map[int]bool{id: true}
		for isSplitPick(supportFamily) {
			ancestor, exists := src.findSymbol(supportFamily.Anchor)
			if !exists || owner.splitParent[ancestor] == nil || visited[ancestor] {
				return unavailableSelectionResolution("SPLIT_HISTORY_UNAVAILABLE", "split ancestor unavailable")
			}
			visited[ancestor] = true
			supportFamily.Anchor = semanticRef(owner.splitParent[ancestor])
		}
		if support := dst.resolveForUse(supportFamily, key, target.GeometryID, nil, use); support.Status == modelcore.SelectionResolved {
			return support
		}
		// Different supports in the family can still have a unique finite
		// branch identified by the creation boundaries below.
	}
	candidates := dst.resolveForUse(family, key, target.GeometryID, nil, finiteElement)
	if len(candidates.Candidates) == 0 {
		return candidates
	}
	// Removing the cut invalidates a relimited pick. A different later cut
	// on the same parent must not silently enlarge or replace the selection.
	dstOwner := dst.owners[namingSelectionOwner(selection)]
	createdByCut := func(ref modelcore.SemanticTopologyRef) bool {
		if dstOwner == nil || ref.FeatureID != selection.Anchor.FeatureID {
			return false
		}
		symbol, ok := dst.findSymbol(ref)
		return ok && dstOwner.splitParent[symbol] != nil && sameSemanticRef(semanticRef(dstOwner.splitParent[symbol]), family.Anchor)
	}
	if !createdByCut(selection.Anchor) && !slices.ContainsFunc(candidates.Candidates, func(c modelcore.ResolvedTopologyElement) bool { return createdByCut(c.SemanticRef) }) {
		return modelcore.SelectionResolution{Status: modelcore.SelectionMissing, SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "SPLIT_FEATURE_MISSING", Diagnostic: "selected cut branch no longer exists"}
	}
	neighbors := map[string]bool{}
	member := src.semanticOutput(selection)
	if member != nil {
		for _, n := range member.Evidence.GetAdjacent() {
			if n.FeatureId != selection.Anchor.FeatureID {
				neighbors[associationRefKey(n)] = true
			}
		}
	}
	original := src.resolve(family, "", source.GeometryID, nil)
	for _, sibling := range original.Candidates {
		if sameSemanticRef(sibling.SemanticRef, selection.Anchor) {
			continue
		}
		for _, n := range sibling.Evidence.Adjacent {
			delete(neighbors, semanticTupleKey(n.FeatureID, n.OutputSlot, n.SourceIDs))
		}
	}
	var matches []modelcore.ResolvedTopologyElement
	for _, candidate := range candidates.Candidates {
		for _, n := range candidate.Evidence.Adjacent {
			if neighbors[semanticTupleKey(n.FeatureID, n.OutputSlot, n.SourceIDs)] {
				matches = append(matches, candidate)
				break
			}
		}
	}
	if len(matches) == 1 {
		candidates.Candidates = matches
		candidates.Status = modelcore.SelectionResolved
		candidates.SupportingElementStatus = modelcore.SupportingElementConnected
		candidates.EvidenceDigest = matches[0].Evidence.EvidenceDigest
		candidates.Diagnostic, candidates.DiagnosticCode = "", ""
		return candidates
	}
	candidates.Status = modelcore.SelectionAmbiguous
	candidates.SupportingElementStatus = modelcore.SupportingElementNotConnected
	candidates.EvidenceDigest = ""
	candidates.DiagnosticCode = "SPLIT_BRANCH_AMBIGUOUS"
	candidates.Diagnostic = "cut branch has no unique surviving boundary"
	return candidates
}

func isSplitPick(selection modelcore.PersistentSelection) bool {
	return selection.Selector.Kind == modelcore.SelectionLineageDescendant && strings.HasPrefix(selection.Anchor.OutputSlot, "SPLIT_FROM/")
}

func namingCreationContext(ctx context.Context, selection modelcore.PersistentSelection) context.Context {
	if isSplitPick(selection) {
		return context.WithValue(ctx, namingResolverOnlyKey{}, true)
	}
	return context.WithValue(ctx, namingTipOnlyKey{}, true)
}

// Preserve the saved sketch frame in the tangent plane as its carrier moves.
func resolvedPlaneFrame(r modelcore.SelectionResolution, preferred [3]float64) (origin, normal [3]float64, ok bool) {
	if r.Status != modelcore.SelectionResolved {
		return
	}
	if p := r.Support; p != nil && p.GeometryType == "PLANE" {
		distance := dot3(p.Direction, p.Origin) - dot3(p.Direction, preferred)
		for i := range origin {
			origin[i] = preferred[i] + distance*p.Direction[i]
		}
		return origin, p.Direction, true
	}
	if len(r.Candidates) == 1 && r.Candidates[0].Evidence.GeometryType == "PLANE" {
		e := r.Candidates[0].Evidence
		return e.Origin, e.Direction, true
	}
	return
}

// The validated creation tip is the starting state of a finite pick. A later
// append need only interpret events after that immutable Merkle checkpoint.
type namingPrefixKey struct{}

func namingTargetContext(ctx context.Context, selection modelcore.PersistentSelection, source *topologyManifest) context.Context {
	ctx = context.WithValue(ctx, namingResolverOnlyKey{}, true)
	if selection.Selector.Kind == modelcore.SelectionLineageDescendant && source.historyTipDigest != "" {
		return context.WithValue(ctx, namingPrefixKey{}, source.historyTipDigest)
	}
	return ctx
}
func namingPrefix(ctx context.Context) string {
	prefix, _ := ctx.Value(namingPrefixKey{}).(string)
	return prefix
}
