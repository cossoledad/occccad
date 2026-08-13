package modelcore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

type DependencyKey string
type EdgeKind string

const (
	ReadValue         EdgeKind = "READ_VALUE"
	ReadGeometry      EdgeKind = "READ_GEOMETRY"
	ReadTopology      EdgeKind = "READ_TOPOLOGY"
	ReadStructure     EdgeKind = "READ_STRUCTURE"
	ReadConfiguration EdgeKind = "READ_CONFIGURATION"
)

var ErrDependencyCycle = errors.New("DEPENDENCY_CYCLE")

type DependencyNode struct {
	Key            DependencyKey   `json:"key"`
	Phase          uint16          `json:"phase"`
	Type           string          `json:"type"`
	CanonicalInput json.RawMessage `json:"canonicalInput"`
}
type DependencyEdge struct {
	Source DependencyKey `json:"source"`
	Target DependencyKey `json:"target"`
	Kind   EdgeKind      `json:"kind"`
}
type DependencyGraph struct {
	Nodes map[DependencyKey]DependencyNode `json:"nodes"`
	Edges []DependencyEdge                 `json:"edges"`
}

func NewDependencyGraph(nodes []DependencyNode, edges []DependencyEdge) (*DependencyGraph, error) {
	graph := &DependencyGraph{Nodes: map[DependencyKey]DependencyNode{}, Edges: append([]DependencyEdge(nil), edges...)}
	for _, node := range nodes {
		if node.Key == "" {
			return nil, fmt.Errorf("dependency key is required")
		}
		if _, exists := graph.Nodes[node.Key]; exists {
			return nil, fmt.Errorf("duplicate dependency node %s", node.Key)
		}
		graph.Nodes[node.Key] = node
	}
	for _, edge := range edges {
		source, sourceOK := graph.Nodes[edge.Source]
		target, targetOK := graph.Nodes[edge.Target]
		if !sourceOK || !targetOK {
			return nil, fmt.Errorf("edge %s -> %s references an unknown node", edge.Source, edge.Target)
		}
		if source.Phase > target.Phase {
			return nil, fmt.Errorf("dependency %s in phase %d cannot drive earlier phase %d", edge.Source, source.Phase, target.Phase)
		}
	}
	if cycle := graph.Cycle(); len(cycle) > 0 {
		return nil, fmt.Errorf("%w: %v", ErrDependencyCycle, cycle)
	}
	return graph, nil
}

func (graph *DependencyGraph) Cycle() []DependencyKey {
	state := map[DependencyKey]uint8{}
	stack := []DependencyKey{}
	positions := map[DependencyKey]int{}
	adj := graph.adjacency()
	var visit func(DependencyKey) []DependencyKey
	visit = func(key DependencyKey) []DependencyKey {
		state[key] = 1
		positions[key] = len(stack)
		stack = append(stack, key)
		for _, next := range adj[key] {
			if state[next] == 0 {
				if cycle := visit(next); len(cycle) > 0 {
					return cycle
				}
			} else if state[next] == 1 {
				cycle := append([]DependencyKey(nil), stack[positions[next]:]...)
				return append(cycle, next)
			}
		}
		stack = stack[:len(stack)-1]
		delete(positions, key)
		state[key] = 2
		return nil
	}
	for _, key := range graph.sortedKeys() {
		if state[key] == 0 {
			if cycle := visit(key); len(cycle) > 0 {
				return cycle
			}
		}
	}
	return nil
}

func (graph *DependencyGraph) DirtyClosure(seeds []DependencyKey) []DependencyKey {
	seen := map[DependencyKey]struct{}{}
	queue := append([]DependencyKey(nil), seeds...)
	adj := graph.adjacency()
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if _, exists := seen[key]; exists {
			continue
		}
		if _, exists := graph.Nodes[key]; !exists {
			continue
		}
		seen[key] = struct{}{}
		queue = append(queue, adj[key]...)
	}
	result := make([]DependencyKey, 0, len(seen))
	for key := range seen {
		result = append(result, key)
	}
	sortDependencyKeys(result)
	return result
}

func (graph *DependencyGraph) TopologicalOrder() []DependencyKey {
	indegree := map[DependencyKey]int{}
	adj := graph.adjacency()
	for key := range graph.Nodes {
		indegree[key] = 0
	}
	for _, edge := range graph.Edges {
		indegree[edge.Target]++
	}
	ready := []DependencyKey{}
	for key, value := range indegree {
		if value == 0 {
			ready = append(ready, key)
		}
	}
	sortDependencyKeys(ready)
	result := []DependencyKey{}
	for len(ready) > 0 {
		key := ready[0]
		ready = ready[1:]
		result = append(result, key)
		for _, target := range adj[key] {
			indegree[target]--
			if indegree[target] == 0 {
				ready = append(ready, target)
				sortDependencyKeys(ready)
			}
		}
	}
	return result
}

func (graph *DependencyGraph) Digest() (string, error) {
	nodes := make([]DependencyNode, 0, len(graph.Nodes))
	for _, key := range graph.sortedKeys() {
		nodes = append(nodes, graph.Nodes[key])
	}
	edges := append([]DependencyEdge(nil), graph.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		if edges[i].Target != edges[j].Target {
			return edges[i].Target < edges[j].Target
		}
		return edges[i].Kind < edges[j].Kind
	})
	data, err := json.Marshal(struct {
		Nodes []DependencyNode `json:"nodes"`
		Edges []DependencyEdge `json:"edges"`
	}{nodes, edges})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func (graph *DependencyGraph) adjacency() map[DependencyKey][]DependencyKey {
	result := map[DependencyKey][]DependencyKey{}
	for _, edge := range graph.Edges {
		result[edge.Source] = append(result[edge.Source], edge.Target)
	}
	for key := range result {
		sortDependencyKeys(result[key])
	}
	return result
}
func (graph *DependencyGraph) sortedKeys() []DependencyKey {
	keys := make([]DependencyKey, 0, len(graph.Nodes))
	for key := range graph.Nodes {
		keys = append(keys, key)
	}
	sortDependencyKeys(keys)
	return keys
}
func sortDependencyKeys(keys []DependencyKey) {
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
}

type NodeResult struct {
	InputDigest  string `json:"inputDigest"`
	OutputDigest string `json:"outputDigest"`
	Status       string `json:"status"`
}
type EvaluationManifest struct {
	RevisionID               string                       `json:"revisionId"`
	ModelHash                string                       `json:"modelHash"`
	DependencySnapshotDigest string                       `json:"dependencySnapshotDigest"`
	EvaluatorDigest          string                       `json:"evaluatorDigest"`
	UnitProfileDigest        string                       `json:"unitProfileDigest"`
	NodeResults              map[DependencyKey]NodeResult `json:"nodeResults"`
	DirtyNodes               []DependencyKey              `json:"dirtyNodes"`
}
type NodeEvaluator func(node DependencyNode, dependencyResults map[DependencyKey]NodeResult) (string, error)

func (graph *DependencyGraph) Evaluate(revisionID, modelHash, evaluatorDigest, unitDigest string, seeds []DependencyKey, prior *EvaluationManifest, evaluator NodeEvaluator) (EvaluationManifest, error) {
	graphDigest, err := graph.Digest()
	if err != nil {
		return EvaluationManifest{}, err
	}
	dirty := graph.DirtyClosure(seeds)
	if prior == nil {
		dirty = graph.sortedKeys()
	}
	dirtySet := map[DependencyKey]struct{}{}
	for _, key := range dirty {
		dirtySet[key] = struct{}{}
	}
	manifest := EvaluationManifest{RevisionID: revisionID, ModelHash: modelHash, DependencySnapshotDigest: graphDigest, EvaluatorDigest: evaluatorDigest, UnitProfileDigest: unitDigest, NodeResults: map[DependencyKey]NodeResult{}, DirtyNodes: dirty}
	incoming := map[DependencyKey][]DependencyKey{}
	for _, edge := range graph.Edges {
		incoming[edge.Target] = append(incoming[edge.Target], edge.Source)
	}
	for _, key := range graph.TopologicalOrder() {
		node := graph.Nodes[key]
		deps := map[DependencyKey]NodeResult{}
		for _, source := range incoming[key] {
			deps[source] = manifest.NodeResults[source]
		}
		inputData, _ := json.Marshal(struct {
			Node             DependencyNode
			Deps             map[DependencyKey]NodeResult
			Evaluator, Units string
		}{node, deps, evaluatorDigest, unitDigest})
		inputDigest := ValueDigest(inputData)
		if _, isDirty := dirtySet[key]; !isDirty && prior != nil {
			if previous, exists := prior.NodeResults[key]; exists && previous.InputDigest == inputDigest {
				manifest.NodeResults[key] = previous
				continue
			}
		}
		output, evalErr := evaluator(node, deps)
		status := "SUCCEEDED"
		if evalErr != nil {
			status = "FAILED"
		}
		manifest.NodeResults[key] = NodeResult{InputDigest: inputDigest, OutputDigest: output, Status: status}
		if evalErr != nil {
			return manifest, fmt.Errorf("evaluate %s: %w", key, evalErr)
		}
	}
	return manifest, nil
}
