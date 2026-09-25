package geometry

import (
	"fmt"
	"math"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
)

// ValidateExchangeGraph rejects malformed or unbounded exchange references
// before the coordinator creates any documents. Source identity is opaque.
func ValidateExchangeGraph(graph *ExchangeGraph) error {
	if graph == nil || len(graph.Definitions) == 0 || len(graph.Definitions) > 100000 || len(graph.Roots) == 0 {
		return fmt.Errorf("invalid exchange graph size")
	}
	definitions := map[string]int{}
	for i, def := range graph.Definitions {
		if def == nil || def.Id == "" {
			return fmt.Errorf("missing exchange definition identity")
		}
		if _, ok := definitions[def.Id]; ok {
			return fmt.Errorf("duplicate exchange definition %s", def.Id)
		}
		if def.Kind != "PART" && def.Kind != "PRODUCT" || def.Kind == "PART" && len(def.Children) > 0 {
			return fmt.Errorf("invalid exchange definition kind")
		}
		definitions[def.Id] = i
	}
	state := map[string]int{}
	edges := 0
	var visit func(string, int) error
	visit = func(id string, depth int) error {
		i, ok := definitions[id]
		if !ok {
			return fmt.Errorf("missing exchange definition %s", id)
		}
		if depth > 128 || state[id] == 1 {
			return fmt.Errorf("cyclic/deep exchange graph")
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		seen := map[string]bool{}
		for _, child := range graph.Definitions[i].Children {
			if child == nil || child.Id == "" || seen[child.Id] {
				return fmt.Errorf("duplicate or missing exchange occurrence")
			}
			seen[child.Id] = true
			edges++
			if edges > 1000000 {
				return fmt.Errorf("exchange occurrence limit exceeded")
			}
			if err := validateExchangePose(child.GetTranslation().GetX(), child.GetTranslation().GetY(), child.GetTranslation().GetZ(), child.GetRotation().GetX(), child.GetRotation().GetY(), child.GetRotation().GetZ(), child.GetRotation().GetW()); err != nil {
				return err
			}
			if err := visit(child.DefinitionId, depth+1); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	seen := map[string]bool{}
	for _, root := range graph.Roots {
		if root == nil || root.Id == "" || seen[root.Id] {
			return fmt.Errorf("invalid exchange root")
		}
		seen[root.Id] = true
		if err := validateExchangePose(root.GetTranslation().GetX(), root.GetTranslation().GetY(), root.GetTranslation().GetZ(), root.GetRotation().GetX(), root.GetRotation().GetY(), root.GetRotation().GetZ(), root.GetRotation().GetW()); err != nil {
			return err
		}
		if err := visit(root.DefinitionId, 0); err != nil {
			return err
		}
	}
	if len(state) != len(definitions) {
		return fmt.Errorf("unreachable exchange definitions")
	}
	return nil
}
func validateExchangePose(v ...float64) error {
	for _, n := range v {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Errorf("non-finite exchange placement")
		}
	}
	n := v[3]*v[3] + v[4]*v[4] + v[5]*v[5] + v[6]*v[6]
	if math.Abs(n-1) > 1e-6 {
		return fmt.Errorf("exchange rotation is not a unit quaternion")
	}
	return nil
}

func SinglePartExchangeGraph(name string, ref ArtifactReference) *ExchangeGraph {
	return &ExchangeGraph{Definitions: []*workerv1.ExchangeDefinition{{Id: "part", Name: name, Kind: "PART", Brep: artifactProto(ref)}}, Roots: []*workerv1.ExchangeOccurrence{{Id: "root", DefinitionId: "part", Name: name, Rotation: &workerv1.Quaternion{W: 1}}}}
}
