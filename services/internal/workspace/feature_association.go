package workspace

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"sort"
	"strconv"
	"strings"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
)

// Display-only index rebuilt from Naming. Local IDs have meaning only in the
// paired GLB/Naming snapshot; this is not a persistent selection scheme.
type FeatureAssociationIndex struct {
	Features []string                     `json:"features"`
	Elements []FeatureTopologyAssociation `json:"elements"`
}
type FeatureTopologyAssociation struct {
	Kind      string                     `json:"kind"`
	LocalID   uint64                     `json:"localId"`
	Origins   []string                   `json:"origins"`
	Primary   []string                   `json:"primary"`
	Modifiers []string                   `json:"modifiers,omitempty"`
	Supports  []string                   `json:"supports,omitempty"`
	Members   []FeatureAssociationMember `json:"members,omitempty"`
	Status    string                     `json:"status"`
}
type FeatureAssociationMember struct {
	PatternID string `json:"patternId"`
	Slot      int    `json:"slot"`
}

func associationRefKey(ref *workerv1.SemanticTopologyRef) string {
	if ref == nil {
		return ""
	}
	return semanticTupleKey(ref.FeatureId, ref.OutputSlot, ref.SourceIds)
}

func semanticTupleKey(feature, slot string, sources []string) string {
	var key strings.Builder
	put := func(value string) {
		var length [binary.MaxVarintLen64]byte
		n := binary.PutUvarint(length[:], uint64(len(value)))
		key.Write(length[:n])
		key.WriteString(value)
	}
	put(feature)
	put(slot)
	for _, id := range sources {
		put(id)
	}
	return key.String()
}
func uniqueAssociationIDs(ids []string) []string {
	// Callers pass their own newly assembled slice, never a frozen ancestor.
	if len(ids) == 0 {
		return []string{}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if ids[0] == "" {
		ids = ids[1:]
	}
	return ids
}

// MEMBER slots carry the digest of the exact seed SemanticTopologyRef. Match
// that digest against Naming refs, never against geometry or display positions.
func associationSeedDigest(ref *workerv1.SemanticTopologyRef) string {
	var text strings.Builder
	text.WriteString(ref.FeatureId)
	text.WriteByte('\n')
	text.WriteString(ref.OutputSlot)
	for _, id := range ref.SourceIds {
		text.WriteByte('\n')
		text.WriteString(id)
	}
	sum := sha256.Sum256([]byte(text.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}
func associationMember(ref *workerv1.SemanticTopologyRef) (FeatureAssociationMember, string, bool) {
	patternID, slot, digest, ok := semanticPatternMember(ref)
	return FeatureAssociationMember{patternID, slot}, digest, ok
}
func deriveFeatureAssociations(m *topologyManifest) *FeatureAssociationIndex {
	return deriveFeatureAssociationsFrom(m, map[string]FeatureTopologyAssociation{}, nil)
}

func deriveFeatureAssociationsFrom(m *topologyManifest, states map[string]FeatureTopologyAssociation, inherited []string) *FeatureAssociationIndex {
	index := &FeatureAssociationIndex{Features: append([]string{}, inherited...), Elements: []FeatureTopologyAssociation{}}
	// Naming v2 interns these immutable refs. Pointers only memoize their full
	// canonical value within this call; equal refs still share a value key.
	keys := map[*workerv1.SemanticTopologyRef]string{}
	refKey := func(r *workerv1.SemanticTopologyRef) string {
		if key, ok := keys[r]; ok {
			return key
		}
		key := associationRefKey(r)
		keys[r] = key
		return key
	}
	var refs map[string]*workerv1.SemanticTopologyRef
	digestRef := func(digest string) *workerv1.SemanticTopologyRef {
		if refs == nil {
			// Only MEMBER and MODIFIER_GENERATED aliases need this reverse
			// index. Ordinary transitions do not hash every historical ref.
			refs = map[string]*workerv1.SemanticTopologyRef{}
			digests := map[string]string{}
			visit := func(r *workerv1.SemanticTopologyRef) {
				if r == nil {
					return
				}
				key := refKey(r)
				value, ok := digests[key]
				if !ok {
					value = associationSeedDigest(r)
					digests[key] = value
				}
				refs[value] = r
			}
			for _, f := range m.FeatureResults {
				for _, l := range f.GetTopologyHistory().GetLineage() {
					for _, r := range l.Sources {
						visit(r)
					}
					visit(l.Result)
				}
			}
		}
		return refs[digest]
	}
	sourceState := func(r *workerv1.SemanticTopologyRef) FeatureTopologyAssociation {
		if v, ok := states[refKey(r)]; ok {
			return v
		}
		v := FeatureTopologyAssociation{Origins: []string{r.FeatureId}, Primary: []string{r.FeatureId}, Status: "MAPPED"}
		if member, digest, ok := associationMember(r); ok {
			v.Members = []FeatureAssociationMember{member}
			if seed := digestRef(digest); seed != nil {
				if origin, ok := states[refKey(seed)]; ok {
					v.Origins = append(v.Origins, origin.Origins...)
				} else {
					v.Origins = append(v.Origins, seed.FeatureId)
				}
			} else {
				v.Status = "MAPPING_MISSING"
			}
		}
		// MODIFIER_GENERATED aliases can merge/split before becoming an
		// accepted output. Their exact source digest is emitted by OCCT Naming;
		// resolve it to preserve support and member context without geometry guesses.
		parts := strings.Split(r.OutputSlot, "/")
		if len(parts) == 3 && parts[0] == "MODIFIER_GENERATED" {
			if source := digestRef("sha256:" + parts[1]); source != nil {
				if origin, ok := states[refKey(source)]; ok {
					v.Supports = append(v.Supports, origin.Origins...)
					v.Supports = append(v.Supports, origin.Supports...)
					v.Members = append(v.Members, origin.Members...)
					v.Status = origin.Status
				} else {
					v.Status = "MAPPING_MISSING"
				}
			} else {
				v.Status = "MAPPING_MISSING"
			}
		}
		return v
	}
	next := map[string]FeatureTopologyAssociation{}
	for _, f := range m.FeatureResults {
		index.Features = append(index.Features, f.FeatureId)
		// Every transition reads the previous state, including self-ref unchanged
		// results. Do not let ordering of outputs create spurious ancestry.
		clear(next)
		for _, l := range f.GetTopologyHistory().GetLineage() {
			if l.Result == nil {
				continue
			}
			resultKey := refKey(l.Result)
			if l.Kind == workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_UNCHANGED && len(l.Sources) == 1 && refKey(l.Sources[0]) == resultKey {
				if previous, ok := states[resultKey]; ok && len(previous.Origins) > 0 && len(previous.Primary) > 0 {
					// An identity transition adds no ownership, support or modifier.
					// Share its immutable sorted sets instead of rebuilding them.
					next[resultKey] = previous
					continue
				}
			}
			a := FeatureTopologyAssociation{Status: "MAPPED"}
			var sources []string
			var previous []FeatureTopologyAssociation
			for _, s := range l.Sources {
				prev := sourceState(s)
				// Loft can merge/split several generated sides into one output.
				// Its flattened PROFILE sources are section inputs, not existing
				// solid contributors. Preserve them as supports of the generating
				// Feature, while keeping real solid-source merges multi-owner.
				_, known := states[refKey(s)]
				role, id, profile := strings.Cut(s.OutputSlot, "/")
				if !known && profile && id != "" && isProfileNamingRole(role) &&
					l.Result.FeatureId == f.FeatureId && (l.Kind == workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_MERGED || l.Kind == workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_SPLIT) {
					prev = FeatureTopologyAssociation{Origins: []string{f.FeatureId}, Primary: []string{f.FeatureId}, Supports: []string{s.FeatureId}, Status: "MAPPED"}
				}
				previous = append(previous, prev)
				sources = append(sources, prev.Origins...)
				a.Modifiers = append(a.Modifiers, prev.Modifiers...)
				a.Supports = append(a.Supports, prev.Supports...)
				a.Members = append(a.Members, prev.Members...)
				if prev.Status == "MAPPING_MISSING" {
					a.Status = "MAPPING_MISSING"
				}
			}
			if l.Kind == workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED {
				a.Origins = []string{l.Result.FeatureId}
				a.Primary = []string{l.Result.FeatureId}
				a.Supports = append(a.Supports, sources...)
				// MEMBER is a semantic Naming output slot emitted by the actual rigid
				// transform history, never a display name or mesh index.
				parts := strings.Split(l.Result.OutputSlot, "/")
				if len(parts) == 3 && parts[0] == "MEMBER" {
					if slot, err := strconv.Atoi(parts[1]); err == nil {
						a.Origins = append(a.Origins, sources...)
						a.Members = append(a.Members, FeatureAssociationMember{l.Result.FeatureId, slot})
					}
				}
			} else {
				a.Origins = append(a.Origins, sources...)
				for _, prev := range previous {
					a.Primary = append(a.Primary, prev.Primary...)
				}
				if len(a.Primary) == 0 {
					a.Primary = append(a.Primary, a.Origins...)
				}
				if l.Kind != workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_UNCHANGED {
					a.Modifiers = append(a.Modifiers, f.FeatureId)
				}
				if len(a.Origins) == 0 {
					a.Status = "MAPPING_MISSING"
				}
			}
			// Draft's explicit angle-face output changes engineering ownership,
			// while Modified remains the real continuation used by selection
			// resolution. Other trimmed/modified faces keep their prior owner.
			if l.Kind == workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_MODIFIED && l.Result.FeatureId == f.FeatureId && strings.HasPrefix(l.Result.OutputSlot, "DRAFTED_FROM/") {
				a.Primary = []string{f.FeatureId}
				a.Origins = append(a.Origins, f.FeatureId)
			}
			a.Origins = uniqueAssociationIDs(a.Origins)
			a.Primary = uniqueAssociationIDs(a.Primary)
			a.Modifiers = uniqueAssociationIDs(a.Modifiers)
			a.Supports = uniqueAssociationIDs(a.Supports)
			members := map[FeatureAssociationMember]bool{}
			for _, v := range a.Members {
				members[v] = true
			}
			a.Members = nil
			for v := range members {
				a.Members = append(a.Members, v)
			}
			sort.Slice(a.Members, func(i, j int) bool {
				if a.Members[i].PatternID != a.Members[j].PatternID {
					return a.Members[i].PatternID < a.Members[j].PatternID
				}
				return a.Members[i].Slot < a.Members[j].Slot
			})
			next[resultKey] = a
		}
		// Commit after all reads, without copying the accumulated history.
		for key, value := range next {
			states[key] = value
		}
	}
	index.Features = uniqueAssociationIDs(index.Features)
	tips := make([]*workerv1.FeatureResult, 0, len(m.Tips)+len(m.Results))
	for _, tip := range m.Tips {
		tips = append(tips, tip)
	}
	for _, tip := range m.Results {
		tips = append(tips, tip)
	}
	for _, tip := range tips {
		for _, o := range tip.SemanticOutputs {
			a, ok := states[refKey(o.SemanticRef)]
			if !ok {
				a.Status = "MAPPING_MISSING"
			}
			a.LocalID = o.LocalId
			a.Kind = strings.TrimPrefix(o.TopologyType.String(), "PERSISTENT_TOPOLOGY_TYPE_")
			index.Elements = append(index.Elements, a)
		}
	}
	sort.Slice(index.Elements, func(i, j int) bool {
		if index.Elements[i].Kind != index.Elements[j].Kind {
			return index.Elements[i].Kind < index.Elements[j].Kind
		}
		return index.Elements[i].LocalID < index.Elements[j].LocalID
	})
	return index
}

func featureContributionStatuses(index *FeatureAssociationIndex) map[string]string {
	out := map[string]string{}
	missing := false
	for _, e := range index.Elements {
		if e.Status == "MAPPING_MISSING" {
			missing = true
		}
	}
	for _, id := range index.Features {
		out[id] = "NO_CURRENT_CONTRIBUTION"
		if missing {
			out[id] = "MAPPING_MISSING"
		}
	}
	for _, e := range index.Elements {
		for _, id := range e.Primary {
			if e.Kind == "FACE" || e.Kind == "EDGE" {
				out[id] = "CURRENT"
			}
		}
	}
	return out
}

// Profile roles identify sketch inputs emitted by the geometry kernel.
func isProfileNamingRole(role string) bool {
	switch role {
	case "PROFILE_EDGE", "PROFILE_REGION", "PROFILE_POINT", "PROFILE_VERTEX", "PATH_EDGE":
		return true
	}
	return false
}
