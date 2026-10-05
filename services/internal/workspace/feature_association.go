package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	b, _ := json.Marshal(ref)
	return string(b)
}
func uniqueAssociationIDs(ids []string) []string {
	set := map[string]bool{}
	for _, id := range ids {
		if id != "" {
			set[id] = true
		}
	}
	out := []string{}
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// MEMBER slots carry the digest of the exact seed SemanticTopologyRef. Match
// that digest against Naming refs, never against geometry or display positions.
func associationSeedDigest(ref *workerv1.SemanticTopologyRef) string {
	text := ref.FeatureId + "\n" + ref.OutputSlot
	for _, id := range ref.SourceIds {
		text += "\n" + id
	}
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}
func associationMember(ref *workerv1.SemanticTopologyRef) (FeatureAssociationMember, string, bool) {
	parts := strings.Split(ref.OutputSlot, "/")
	if len(parts) != 3 || parts[0] != "MEMBER" {
		return FeatureAssociationMember{}, "", false
	}
	slot, err := strconv.Atoi(parts[1])
	if err != nil || slot < 0 {
		return FeatureAssociationMember{}, "", false
	}
	return FeatureAssociationMember{ref.FeatureId, slot}, parts[2], true
}
func deriveFeatureAssociations(m *topologyManifest) *FeatureAssociationIndex {
	index := &FeatureAssociationIndex{Features: []string{}, Elements: []FeatureTopologyAssociation{}}
	states := map[string]FeatureTopologyAssociation{}
	refs := map[string]*workerv1.SemanticTopologyRef{}
	for _, f := range m.FeatureResults {
		for _, l := range f.GetTopologyHistory().GetLineage() {
			for _, r := range append(append([]*workerv1.SemanticTopologyRef{}, l.Sources...), l.Result) {
				if r != nil {
					refs[associationSeedDigest(r)] = r
				}
			}
		}
	}
	sourceState := func(r *workerv1.SemanticTopologyRef) FeatureTopologyAssociation {
		if v, ok := states[associationRefKey(r)]; ok {
			return v
		}
		v := FeatureTopologyAssociation{Origins: []string{r.FeatureId}, Primary: []string{r.FeatureId}, Status: "MAPPED"}
		if member, digest, ok := associationMember(r); ok {
			v.Members = []FeatureAssociationMember{member}
			if seed := refs[digest]; seed != nil {
				if origin, ok := states[associationRefKey(seed)]; ok {
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
			if source := refs["sha256:"+parts[1]]; source != nil {
				if origin, ok := states[associationRefKey(source)]; ok {
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
	for _, f := range m.FeatureResults {
		index.Features = append(index.Features, f.FeatureId)
		// Every transition reads the previous state, including self-ref unchanged
		// results. Do not let ordering of outputs create spurious ancestry.
		next := map[string]FeatureTopologyAssociation{}
		for k, v := range states {
			next[k] = v
		}
		for _, l := range f.GetTopologyHistory().GetLineage() {
			if l.Result == nil {
				continue
			}
			a := FeatureTopologyAssociation{Status: "MAPPED"}
			var sources []string
			for _, s := range l.Sources {
				prev := sourceState(s)
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
				for _, s := range l.Sources {
					prev := sourceState(s)
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
			next[associationRefKey(l.Result)] = a
		}
		states = next
	}
	index.Features = uniqueAssociationIDs(index.Features)
	for _, tip := range m.Tips {
		for _, o := range tip.SemanticOutputs {
			a, ok := states[associationRefKey(o.SemanticRef)]
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
		for _, id := range e.Origins {
			if e.Kind == "FACE" || e.Kind == "EDGE" {
				out[id] = "CURRENT"
			}
		}
	}
	return out
}
