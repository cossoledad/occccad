package automation

import (
	"math"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/workspace"
)

type TopologyRadiusFilter struct {
	EqualMM     *float64 `json:"equalMM,omitempty"`
	MinMM       *float64 `json:"minMM,omitempty"`
	MaxMM       *float64 `json:"maxMM,omitempty"`
	ToleranceMM float64  `json:"toleranceMM"`
}
type TopologyFilter struct {
	GeometryType   string                `json:"geometryType,omitempty"`
	Radius         *TopologyRadiusFilter `json:"radius,omitempty"`
	AdjacentFaceID string                `json:"adjacentFaceId,omitempty"`
}

func (f *TopologyFilter) validate() error {
	if f == nil {
		return nil
	}
	if f.GeometryType != "" {
		switch f.GeometryType {
		case "POINT", "LINE", "CIRCLE", "ELLIPSE", "BSPLINE", "BEZIER", "HYPERBOLA", "PARABOLA", "OFFSET", "OTHER_CURVE", "PLANE", "CYLINDER", "SPHERE", "CONE", "TORUS":
		default:
			return fault("INVALID_QUERY", "unsupported geometryType filter")
		}
	}
	if len(f.AdjacentFaceID) > 160 {
		return fault("INVALID_QUERY", "adjacentFaceId must be a queried stable face ID")
	}
	if r := f.Radius; r != nil {
		finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
		if !finite(r.ToleranceMM) || r.ToleranceMM <= 0 || r.EqualMM == nil && r.MinMM == nil && r.MaxMM == nil || r.EqualMM != nil && (r.MinMM != nil || r.MaxMM != nil) {
			return fault("INVALID_QUERY", "radius requires equalMM or minMM/maxMM and an explicit positive finite toleranceMM")
		}
		for _, value := range []*float64{r.EqualMM, r.MinMM, r.MaxMM} {
			if value != nil && (!finite(*value) || *value < 0) {
				return fault("INVALID_QUERY", "radius values must be finite and nonnegative mm")
			}
		}
		if r.MinMM != nil && r.MaxMM != nil && *r.MinMM > *r.MaxMM {
			return fault("INVALID_QUERY", "radius minMM exceeds maxMM")
		}
	}
	return nil
}

// Three-valued conjunction: a known false predicate excludes an item even if
// another property is unknown. Otherwise unknown membership is reported, never
// silently treated as a complete matching collection.
func (f *TopologyFilter) match(item workspace.BodyTopologyItem) (match, known bool) {
	if f == nil {
		return true, true
	}
	unknown := false
	if f.GeometryType != "" {
		if item.GeometryType == "" || item.GeometryType == "UNKNOWN" {
			unknown = true
		} else if item.GeometryType != f.GeometryType {
			return false, true
		}
	}
	if r := f.Radius; r != nil {
		if v := item.Geometry.RadiusMM; v != nil && !math.IsNaN(*v) && !math.IsInf(*v, 0) && *v > 0 {
			if r.EqualMM != nil && math.Abs(*v-*r.EqualMM) > r.ToleranceMM || r.MinMM != nil && *v < *r.MinMM-r.ToleranceMM || r.MaxMM != nil && *v > *r.MaxMM+r.ToleranceMM {
				return false, true
			}
		} else {
			switch item.GeometryType {
			case "LINE", "POINT", "PLANE", "CONE":
				return false, true
			default:
				unknown = true
			}
		}
	}
	if f.AdjacentFaceID != "" {
		found := false
		for _, id := range item.FaceIDs {
			if id == f.AdjacentFaceID {
				found = true
				break
			}
		}
		if !found {
			if item.RelationsComplete {
				return false, true
			}
			unknown = true
		}
	}
	if unknown {
		return false, false
	}
	return true, true
}

type topologyMatches struct {
	items               []workspace.BodyTopologyItem
	candidates, unknown int
}

// Used by query, selection-set resolution and verification. IDs restrict the
// fixed scope before predicates; pagination never enters this function.
func matchTopology(data workspace.BodyTopology, ids []string, filter *TopologyFilter) (topologyMatches, error) {
	out := topologyMatches{items: []workspace.BodyTopologyItem{}}
	if err := filter.validate(); err != nil {
		return out, err
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || wanted[id] {
			return out, fault("INVALID_QUERY", "unique stable topology IDs required")
		}
		wanted[id] = true
	}
	for _, item := range data.Items {
		if len(wanted) > 0 && !wanted[item.ID] {
			continue
		}
		out.candidates++
		matches, known := filter.match(item)
		if !known {
			out.unknown++
		} else if matches {
			out.items = append(out.items, item)
		}
	}
	if len(wanted) > 0 && out.candidates != len(wanted) {
		return out, fault("TOPOLOGY_NOT_FOUND", "some topology IDs do not belong to this snapshot")
	}
	sort.Slice(out.items, func(i, j int) bool { return out.items[i].ID < out.items[j].ID })
	return out, nil
}
