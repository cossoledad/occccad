package geometry

import (
	"fmt"
	"math"
	"regexp"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
)

var exchangeOpaqueColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

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
		if err := validateExchangeAppearance(def.Appearance); err != nil {
			return err
		}
		for _, body := range def.Bodies {
			if body == nil || body.Brep == nil {
				return fmt.Errorf("missing exchange Body BREP")
			}
			if err := validateExchangeAppearance(body.Appearance); err != nil {
				return err
			}
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
			if err := validateExchangeOpacity(child.Opacity); err != nil {
				return err
			}
			if child.Color != "" && !exchangeOpaqueColor.MatchString(child.Color) {
				return fmt.Errorf("invalid exchange occurrence color")
			}
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
		if err := validateExchangeOpacity(root.Opacity); err != nil {
			return err
		}
		if root == nil || root.Id == "" || seen[root.Id] {
			return fmt.Errorf("invalid exchange root")
		}
		seen[root.Id] = true
		if root.Color != "" && !exchangeOpaqueColor.MatchString(root.Color) {
			return fmt.Errorf("invalid exchange root color")
		}
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
func validateExchangeAppearance(appearance *workerv1.ExchangeAppearance) error {
	if appearance == nil {
		return nil
	}
	if appearance.Color != "" && !exchangeOpaqueColor.MatchString(appearance.Color) {
		return fmt.Errorf("invalid exchange color")
	}
	if err := validateExchangeOpacity(appearance.Opacity); err != nil {
		return err
	}
	if len(appearance.Faces) > 1_000_000 {
		return fmt.Errorf("exchange face color limit exceeded")
	}
	seen := map[uint64]bool{}
	for _, face := range appearance.Faces {
		if face == nil || face.LocalId == 0 || seen[face.LocalId] || (face.Color != "" && !exchangeOpaqueColor.MatchString(face.Color)) || (face.Color == "" && face.Opacity == nil) {
			return fmt.Errorf("invalid exchange face color")
		}
		if err := validateExchangeOpacity(face.Opacity); err != nil {
			return err
		}
		seen[face.LocalId] = true
	}
	return nil
}
func validateExchangeOpacity(value *float64) error {
	if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 1) {
		return fmt.Errorf("exchange opacity must be between 0 and 1")
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
