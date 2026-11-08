package workspace

import (
	"slices"
	"testing"
)

func TestSketchTreeConstraintEditingRespectsOperationOwnership(t *testing.T) {
	sketch := SketchFeature{Entities: []SketchEntity{}, Constraints: []SketchConstraint{{ID: "dimension", Kind: "LENGTH"}, {ID: "logical", Kind: "PARALLEL"}, {ID: "internal", Kind: "LENGTH", Internal: true}, {ID: "mirror", Kind: "MIRROR"}, {ID: "support", Kind: "SAME_SUPPORT"}}}
	for _, editable := range []bool{true, false} {
		nodes := sketchStructureChildren(sketch, "path", "sketch", "doc", "revision", editable)
		var walk func([]DocumentStructureNode)
		walk = func(nodes []DocumentStructureNode) {
			for _, n := range nodes {
				if n.Kind == "SKETCH_CONSTRAINT" {
					want := editable && (n.EntityID == "dimension" || n.EntityID == "logical")
					if slices.Contains(n.Capabilities, "EDIT") != want {
						t.Fatal(n.EntityID, editable, n.Capabilities)
					}
				}
				walk(n.Children)
			}
		}
		walk(nodes)
	}
}
