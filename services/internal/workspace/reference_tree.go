package workspace

import "fmt"

func referenceStructure(model PartModel, legacy []DocumentStructureNode, path, doc, revision string, editable bool) []DocumentStructureNode {
	containers := map[string]string{}
	for _, p := range model.DatumPlanes {
		containers[p.ID] = p.GeometricalSetID
	}
	for _, p := range model.DatumAxes {
		containers[p.ID] = p.GeometricalSetID
	}
	for _, p := range model.DatumPoints {
		containers[p.ID] = p.GeometricalSetID
		caps := []string{}
		if editable {
			caps = []string{"EDIT", "DELETE", "MOVE"}
		}
		legacy = append(legacy, DocumentStructureNode{ID: path + "/point:" + p.ID, Kind: "REFERENCE_POINT", EntityID: p.ID, Name: p.Name, DocumentID: doc, VersionID: revision, LocalVisible: boolPointer(visibleOrDefault(p.Visible)), Capabilities: caps})
	}
	dependents := map[string]bool{}
	for _, f := range model.Features {
		for _, input := range featureInputIDs(f) {
			dependents[input] = true
		}
	}
	for _, f := range model.Features {
		if f.GeometricalSetID == "" {
			continue
		}
		containers[f.ID] = f.GeometricalSetID
		digest, _ := featureDefinitionDigest(model, f.ID)
		node := featureStructureNode(f, path+"/geometrical-set:"+f.GeometricalSetID, doc, revision, digest, editable, editable)
		if f.Result != nil {
			node.GeometryKey = f.Result.GeometryKey
		}
		legacy = append(legacy, node)
	}
	children := map[string][]DocumentStructureNode{}
	for _, node := range legacy {
		for _, v := range model.DatumPoints {
			if v.ID == node.EntityID {
				node.EvaluationStatus = v.EvaluationStatus
				node.Diagnostic = v.Diagnostic
			}
		}
		for _, v := range model.DatumPlanes {
			if v.ID == node.EntityID {
				node.EvaluationStatus = v.EvaluationStatus
				node.Diagnostic = v.Diagnostic
			}
		}
		for _, v := range model.DatumAxes {
			if v.ID == node.EntityID {
				node.EvaluationStatus = v.EvaluationStatus
				node.Diagnostic = v.Diagnostic
			}
		}
		container := containers[node.EntityID]
		node.OwnerEntityID = container
		if editable && node.Kind != "ORIGIN" && node.Kind != "AXIS_SYSTEM" && (container != "" || node.Kind == "REFERENCE_POINT" || node.Kind == "DATUM_AXIS" || node.Kind == "PLANE" && !isStandardDatumPlane(node.EntityID)) {
			node.Capabilities = append(node.Capabilities, "MOVE")
		}
		for _, p := range model.Parameters {
			if p.OwnerFeatureID != node.EntityID {
				continue
			}
			name := p.Label
			if p.EvaluatedValue != nil {
				value, err := quantityInDisplayUnit(*p.EvaluatedValue, p.DisplayUnit)
				if err == nil {
					name += fmt.Sprintf(" = %.7g %s", value, p.DisplayUnit)
				}
			}
			caps := []string{}
			if editable {
				caps = []string{"EDIT"}
			}
			node.Children = append(node.Children, DocumentStructureNode{ID: node.ID + "/parameter:" + p.ParameterID, Kind: "PARAMETER", EntityID: p.ParameterID, Name: name, ParameterAlias: p.Key, OwnerEntityID: node.EntityID, DocumentID: doc, VersionID: revision, Capabilities: caps, CurrencyStatus: p.CurrencyStatus})
		}
		children[container] = append(children[container], node)
	}
	var build func(string, map[string]bool) []DocumentStructureNode
	build = func(parent string, seen map[string]bool) []DocumentStructureNode {
		out := append([]DocumentStructureNode(nil), children[parent]...)
		for _, set := range model.GeometricalSets {
			if set.ParentID != parent || seen[set.ID] {
				continue
			}
			seen[set.ID] = true
			caps := []string{}
			if editable {
				caps = []string{"EDIT", "DELETE", "MOVE"}
			}
			node := DocumentStructureNode{ID: path + "/geometrical-set:" + set.ID, Kind: "GEOMETRICAL_SET", EntityID: set.ID, Name: set.Name, OwnerEntityID: set.ParentID, DocumentID: doc, VersionID: revision, LocalVisible: boolPointer(visibleOrDefault(set.Visible)), Capabilities: caps}
			node.Children = build(set.ID, seen)
			out = append(out, node)
		}
		return out
	}
	return build("", map[string]bool{})
}
