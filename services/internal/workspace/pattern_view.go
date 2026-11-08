package workspace

import "fmt"

type sketchPatternMember struct {
	ID, PatternID string
	Slot          int
}

func spatialSketchMembers(feature Feature) []sketchPatternMember {
	if feature.Type != "SKETCH_PATTERN" || feature.Pattern == nil || feature.Suppressed {
		return nil
	}
	placements, err := patternPlacements(feature.Pattern.PatternDefinition)
	if err != nil {
		return nil
	}
	var members []sketchPatternMember
	for _, p := range placements {
		members = append(members, sketchPatternMember{patternMemberID(feature.ID, p.Slot, feature.Pattern.Source.FeatureID), feature.ID, p.Slot})
	}
	return members
}

func sketchEntityDefinitionStatus(sketch SketchFeature, entity SketchEntity) string {
	source := entity.ID
	if entity.SourceEntityID != "" {
		source = entity.SourceEntityID
	}
	for _, source := range []string{entity.ID, source} {
		for _, component := range sketch.Solve.Components {
			for _, id := range component.EntityIDs {
				if id == source {
					if component.DefinitionStatus != "" {
						return component.DefinitionStatus
					}
					return component.Status
				}
			}
		}
	}
	if sketch.Solve.DefinitionStatus != "" {
		return sketch.Solve.DefinitionStatus
	}
	return sketch.Solve.Status
}

func sketchPatternNodes(sketch SketchFeature, path, sketchID, documentID, versionID string, editable bool) []DocumentStructureNode {
	entities, _ := evaluatedSketchEntities(sketch)
	var nodes []DocumentStructureNode
	for _, p := range sketch.Patterns {
		node := DocumentStructureNode{ID: path + "/patterns/" + p.ID, Kind: "SKETCH_PATTERN_DEFINITION", Name: func() string {
			if p.Name != "" {
				return p.Name
			}
			return automaticTypePrefix(p.Kind) + "Pattern"
		}(), EntityID: p.ID, OwnerEntityID: sketchID, DocumentID: documentID, VersionID: versionID, Suppressed: p.Suppressed}
		if editable {
			node.Capabilities = []string{"EDIT", "DELETE", "SUPPRESS"}
		}
		labels := map[string]string{}
		placements, _ := patternPlacements(p.PatternDefinition)
		for _, placement := range placements {
			for _, id := range p.EntityIDs {
				label := "几何"
				for _, source := range sketch.Entities {
					if source.ID == id {
						label = source.Name
						if label == "" {
							label = automaticTypePrefix(source.Kind)
						}
						break
					}
				}
				labels[patternMemberID(p.ID, placement.Slot, id)] = fmt.Sprintf("成员 %d · %s", placement.Slot+1, label)
			}
		}
		for _, e := range entities {
			if e.CreatedByOperationID != p.ID {
				continue
			}
			node.Children = append(node.Children, DocumentStructureNode{ID: node.ID + "/entity:" + e.ID, Kind: "SKETCH_PATTERN_ENTITY", Name: labels[e.ID], EntityID: e.ID, OwnerEntityID: sketchID, PatternID: p.ID, LocalVisible: e.Visible, EntityType: e.Kind, Role: e.Role, DocumentID: documentID, VersionID: versionID, Diagnostic: sketchEntityDefinitionStatus(sketch, e)})
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// Definitions, generated display children and diagnostics share one tree route.
func sketchDerivedNodes(sketch SketchFeature, path, sketchID, documentID, versionID string, editable bool) []DocumentStructureNode {
	nodes := sketchPatternNodes(sketch, path, sketchID, documentID, versionID, editable)
	for _, offset := range sketch.Offsets {
		name := offset.Name
		if name == "" {
			name = "关联偏移"
		}
		node := DocumentStructureNode{ID: path + "/offsets/" + offset.ID, Kind: "SKETCH_OFFSET_DEFINITION", Name: name, EntityID: offset.ID, OwnerEntityID: sketchID, DocumentID: documentID, VersionID: versionID, Suppressed: offset.Suppressed}
		if editable {
			node.Capabilities = []string{"EDIT", "DELETE", "SUPPRESS", "DETACH"}
		}
		entities, _, err := evaluateSketchOffset(sketch, offset)
		if err != nil {
			node.Diagnostic = err.Error()
		}
		for _, entity := range entities {
			node.Children = append(node.Children, DocumentStructureNode{ID: node.ID + "/entity:" + entity.ID, Kind: "SKETCH_OFFSET_ENTITY", Name: automaticTypePrefix(entity.Kind), EntityID: entity.ID, OwnerEntityID: sketchID, DocumentID: documentID, VersionID: versionID, EntityType: entity.Kind, Role: entity.Role})
		}
		nodes = append(nodes, node)
	}
	return nodes
}
