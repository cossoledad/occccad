package workspace

import "testing"

func mustStructureNode(t *testing.T, nodes []DocumentStructureNode, kind, id string) DocumentStructureNode {
	t.Helper()
	var find func([]DocumentStructureNode) (DocumentStructureNode, bool)
	find = func(nodes []DocumentStructureNode) (DocumentStructureNode, bool) {
		for _, node := range nodes {
			if node.Kind == kind && (id == "" || node.EntityID == id) {
				return node, true
			}
			if found, ok := find(node.Children); ok {
				return found, true
			}
		}
		return DocumentStructureNode{}, false
	}
	if node, ok := find(nodes); ok {
		return node
	}
	t.Fatalf("missing semantic tree object %s/%s", kind, id)
	return DocumentStructureNode{}
}

func TestReferenceTreeDirectPlanesContainersAndParameterChildren(t *testing.T) {
	model := newPartModel()
	model.GeometricalSets = []GeometricalSet{{ID: "set", Name: "References"}}
	model.DatumPoints = []DatumPoint{{ID: "point", Name: "Point", GeometricalSetID: "set", Construction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{1, 2, 3}}}}
	normalizePartModel(&model)
	if err := validateAndResolvePartParameters(&model); err != nil {
		t.Fatal(err)
	}
	nodes := partStructureChildren(model, "root", "doc", "rev", true)
	for _, node := range nodes {
		if node.Kind == "ORIGIN" {
			t.Fatal("Origin wrapper remains")
		}
	}
	if nodes[0].Kind != "PLANE" {
		t.Fatal("standard planes are not direct", nodes[0])
	}
	set := mustStructureNode(t, nodes, "GEOMETRICAL_SET", "set")
	if len(set.Children) != 1 || set.Children[0].EntityID != "point" {
		t.Fatal("set ownership", set)
	}
	point := mustStructureNode(t, nodes, "REFERENCE_POINT", "point")
	if len(point.Children) != 3 {
		t.Fatal("point parameter children", point)
	}
}
