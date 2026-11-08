package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/occccad/occccad/internal/geometry"
)

type sketchCurveChain struct {
	ID     string
	Curves []geometry.ProfileCurve
	Closed bool
}

type CurveChainDirection struct {
	EntityID string `json:"entityId"`
	Reversed bool   `json:"reversed,omitempty"`
}

func orientCurveChains(chains []sketchCurveChain, directions []CurveChainDirection) ([]sketchCurveChain, error) {
	out := append([]sketchCurveChain(nil), chains...)
	for i, chain := range out {
		found := false
		reverse := false
		for _, direction := range directions {
			for _, curve := range chain.Curves {
				if curve.EntityID == direction.EntityID {
					candidate := curve.Reversed != direction.Reversed
					if found && candidate != reverse {
						return nil, fmt.Errorf("%w: CURVE_CHAIN_DIRECTION_CONFLICT", ErrValidation)
					}
					found = true
					reverse = candidate
				}
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: CURVE_CHAIN_DIRECTION_MISSING", ErrValidation)
		}
		if reverse {
			out[i] = reverseCurveChain(chain)
		}
	}
	return out, nil
}

// Connectivity is solely the accepted formal endpoint graph. Coordinate checks
// validate those connections; they never weld separate coincident-looking ends.
func sketchCurveChains(feature Feature, ids []string) ([]sketchCurveChain, error) {
	if feature.Sketch == nil || feature.Suppressed || feature.Sketch.Solve.Status == "CONFLICTING" || feature.Sketch.Support.Status == "FAILED_SUPPORT" || feature.Sketch.Support.Status == "BLOCKED" {
		return nil, fmt.Errorf("%w: CURVE_CHAIN_SOURCE_UNAVAILABLE", ErrValidation)
	}
	if len(ids) == 0 || len(ids) > 256 {
		return nil, fmt.Errorf("%w: CURVE_CHAIN_SELECTION_BUDGET", ErrValidation)
	}
	entities, err := evaluatedSketchEntities(*feature.Sketch)
	if err != nil {
		return nil, err
	}
	source := *feature.Sketch
	source.Constraints = append(append([]SketchConstraint(nil), source.Constraints...), sketchDerivedConnectivity(source)...)
	graph := profileConnectivity(Feature{Sketch: &source})
	index := map[string]SketchEntity{}
	for _, e := range entities {
		index[e.ID] = e
	}
	selected := append([]string(nil), ids...)
	sort.Strings(selected)
	edges := []profileEdge{}
	chains := []sketchCurveChain{}
	seen := map[string]bool{}
	for _, id := range selected {
		e, ok := index[id]
		if !ok || e.Suppressed || seen[id] || e.Kind == "POINT" {
			return nil, fmt.Errorf("%w: CURVE_CHAIN_SOURCE_MISSING: %s", ErrValidation, id)
		}
		seen[id] = true
		if e.Kind == "CIRCLE" || e.Kind == "ELLIPSE" || e.Kind == "SPLINE" && e.Closed {
			c := []geometry.ProfileCurve{profileCurve(e, false)}
			chains = append(chains, sketchCurveChain{ID: loopID(c), Curves: c, Closed: true})
			continue
		}
		start, end, ok := entityProfileEndpoints(e)
		if !ok {
			return nil, fmt.Errorf("%w: CURVE_CHAIN_UNSUPPORTED: %s", ErrValidation, id)
		}
		edges = append(edges, profileEdge{entity: e, start: start, end: end, startNode: graph.find(id + "/START"), endNode: graph.find(id + "/END")})
	}
	adjacency := map[string][]int{}
	points := map[string]SketchPoint2{}
	for i, e := range edges {
		for _, v := range []struct {
			id    string
			point SketchPoint2
		}{{e.startNode, e.start}, {e.endNode, e.end}} {
			if p, ok := points[v.id]; ok && math.Hypot(p.X-v.point.X, p.Y-v.point.Y) > profileTolerance {
				return nil, fmt.Errorf("%w: CURVE_CHAIN_CONNECTION_INCONSISTENT", ErrValidation)
			}
			points[v.id] = v.point
			adjacency[v.id] = append(adjacency[v.id], i)
			if len(adjacency[v.id]) > 2 {
				return nil, fmt.Errorf("%w: CURVE_CHAIN_BRANCH", ErrValidation)
			}
		}
	}
	visited := map[int]bool{}
	for seed := range edges {
		if visited[seed] {
			continue
		}
		component := map[int]bool{}
		queue := []int{seed}
		endpoints := []struct{ node, key string }{}
		for len(queue) > 0 {
			i := queue[0]
			queue = queue[1:]
			if component[i] {
				continue
			}
			component[i] = true
			e := edges[i]
			for side, node := range []string{e.startNode, e.endNode} {
				if len(adjacency[node]) == 1 {
					endpoints = append(endpoints, struct{ node, key string }{node, fmt.Sprintf("%s/%d", e.entity.ID, side)})
				}
				for _, next := range adjacency[node] {
					if !component[next] {
						queue = append(queue, next)
					}
				}
			}
		}
		if len(endpoints) != 0 && len(endpoints) != 2 {
			return nil, fmt.Errorf("%w: CURVE_CHAIN_INVALID_ENDS", ErrValidation)
		}
		closed := len(endpoints) == 0
		start := edges[seed].startNode
		if !closed {
			sort.Slice(endpoints, func(i, j int) bool { return endpoints[i].key < endpoints[j].key })
			start = endpoints[0].node
		}
		current := start
		curves := []geometry.ProfileCurve{}
		for {
			next := -1
			for _, i := range adjacency[current] {
				if !visited[i] && (next < 0 || edges[i].entity.ID < edges[next].entity.ID) {
					next = i
				}
			}
			if next < 0 {
				break
			}
			e := edges[next]
			reverse := e.endNode == current
			curves = append(curves, profileCurve(e.entity, reverse))
			visited[next] = true
			if reverse {
				current = e.startNode
			} else {
				current = e.endNode
			}
		}
		if len(curves) != len(component) || closed && current != start {
			return nil, fmt.Errorf("%w: CURVE_CHAIN_INCOMPLETE", ErrValidation)
		}
		identity := loopID(curves)
		if !closed {
			forward := make([]string, len(curves))
			reverse := make([]string, len(curves))
			for i, c := range curves {
				forward[i] = c.EntityID
				reverse[len(curves)-i-1] = c.EntityID
			}
			a, _ := json.Marshal(forward)
			b, _ := json.Marshal(reverse)
			if string(b) < string(a) {
				a = b
			}
			hash := sha256.Sum256(a)
			identity = "profile-chain:" + hex.EncodeToString(hash[:12])
		}
		chains = append(chains, sketchCurveChain{ID: identity, Curves: curves, Closed: closed})
	}
	sort.Slice(chains, func(i, j int) bool { return chains[i].ID < chains[j].ID })
	return chains, nil
}

func reverseCurveChain(chain sketchCurveChain) sketchCurveChain {
	curves := make([]geometry.ProfileCurve, len(chain.Curves))
	for i, c := range chain.Curves {
		c.Reversed = !c.Reversed
		curves[len(curves)-i-1] = c
	}
	chain.Curves = curves
	return chain
}
