package workspace

import (
	"fmt"
	"slices"
)

func kinematicsStructure(doc, revision, path string, k KinematicsDefinitions, constraints []AssemblyConstraint, instances ...[]ProductInstance) DocumentStructureNode {
	root := DocumentStructureNode{ID: path + "/applications", Kind: "APPLICATIONS", Name: "Applications", DocumentID: doc, VersionID: revision, Children: []DocumentStructureNode{}}
	node := func(kind, id, name string) DocumentStructureNode {
		return DocumentStructureNode{ID: root.ID + "/" + kind + ":" + id, Kind: kind, EntityID: id, Name: name, DocumentID: doc, OwnerDocumentID: doc, VersionID: revision, Capabilities: []string{"EDIT", "DELETE", "RENAME"}}
	}
	nameFor := func(id string) string {
		for _, list := range instances {
			for _, v := range list {
				if v.ID == id {
					return v.Name
				}
			}
		}
		return id
	}
	for _, m := range k.Mechanisms {
		mn := node("MECHANISM", m.ID, m.Name)
		for _, j := range m.Joints {
			label := j.Name
			if j.Kind == "GROUND" {
				label += " (" + nameFor(j.First.InstanceID) + ")"
			}
			jn := node("MECHANISM_JOINT", j.ID, label)
			jn.EntityType = j.Kind
			for _, relation := range []struct{ role, name, kind string }{
				{"axis", "同心", "COINCIDENT"}, {"axial-location", "偏移", "DISTANCE"}, {"rotation", "方向一致", "ANGLE"},
			} {
				if j.Kind == "GROUND" || relation.role == "axial-location" && j.Kind == "PRISMATIC" || relation.role == "rotation" && j.Kind == "REVOLUTE" {
					continue
				}
				child := node("JOINT_EXPANSION", j.ID+"/"+relation.role, relation.name)
				child.EntityType = relation.kind
				child.Capabilities = []string{"EDIT"}
				child.EvaluationStatus = "NOT_UPDATED"
				child.PresentationRole = "INPUT_REFERENCE"
				ends := []JointEndpoint{j.First}
				if j.Second != nil {
					ends = append(ends, *j.Second)
				}
				for side, end := range ends {
					ref, role := end.Axis, "轴线"
					if relation.role == "axial-location" {
						ref, role = end.Plane, "定位平面"
					}
					if ref == nil {
						continue
					}
					sn := node("JOINT_SUPPORT", fmt.Sprintf("%s/%s/%d", j.ID, relation.role, side), role+" ("+nameFor(end.InstanceID)+")")
					sn.Capabilities = nil
					sn.PresentationRole = "INPUT_REFERENCE"
					child.Children = append(child.Children, sn)
				}
				jn.Children = append(jn.Children, child)
			}
			for _, src := range j.Sources {
				sn := node("JOINT_SOURCE", src.ConstraintID, "来源约束 "+src.ConstraintID)
				sn.ID = jn.ID + "/source:" + src.ConstraintID
				sn.Capabilities = nil
				sn.PresentationRole = "INPUT_REFERENCE"
				i := slices.IndexFunc(constraints, func(c AssemblyConstraint) bool { return c.ID == src.ConstraintID })
				if i < 0 {
					sn.ResolutionStatus = "SOURCE_DELETED"
					sn.Diagnostic = "接合定义保留；可明确解除来源关联"
				} else if semanticConstraint(constraints[i]) != src.Baseline {
					sn.ResolutionStatus = "SOURCE_CHANGED"
					sn.Diagnostic = "来源已修改；接合保持上次确认定义"
				} else {
					sn.ResolutionStatus = "CURRENT"
				}
				if len(jn.Children) > 0 {
					jn.Children[0].Children = append(jn.Children[0].Children, sn)
				} else {
					jn.Children = append(jn.Children, sn)
				}
			}
			// Publishing links describe the existing relation, rather than adding
			// a parallel application-object branch beside the joints.
			for _, a := range k.Associations {
				if a.MechanismID != m.ID || a.JointID != j.ID {
					continue
				}
				an := node("MOTION_ASSOCIATION", j.ID+"/"+a.Role, "装配关联")
				an.ID = jn.ID + "/association:" + a.Role
				an.Capabilities = nil
				an.PresentationRole = "INPUT_REFERENCE"
				name := a.ConstraintID
				for _, c := range constraints {
					if c.ID == a.ConstraintID && c.Name != "" {
						name = c.Name
					}
				}
				ref := node("JOINT_SOURCE", a.ConstraintID, name)
				ref.ID = an.ID + "/constraint:" + a.ConstraintID
				ref.Capabilities = nil
				ref.PresentationRole = "INPUT_REFERENCE"
				an.Children = []DocumentStructureNode{ref}
				role := a.Role
				if role == "orientation" {
					role = "rotation"
				}
				if role == "angle-lock" {
					role = "axis"
				}
				index := slices.IndexFunc(jn.Children, func(n DocumentStructureNode) bool { return n.EntityID == j.ID+"/"+role })
				if index >= 0 {
					jn.Children[index].Children = append(jn.Children[index].Children, an)
				} else if j.Kind == "GROUND" {
					jn.Children = append(jn.Children, an)
				}
			}
			mn.Children = append(mn.Children, jn)
		}
		for _, d := range k.Drivers {
			if d.MechanismID == m.ID {
				mn.Children = append(mn.Children, node("MOTION_DRIVER", d.ID, d.Name))
			}
		}
		for _, s := range k.Studies {
			if s.MechanismID == m.ID {
				mn.Children = append(mn.Children, node("MOTION_STUDY", s.ID, s.Name))
			}
		}

		root.Children = append(root.Children, mn)
	}
	for _, a := range k.Analyses {
		root.Children = append(root.Children, node("INTERFERENCE_ANALYSIS", a.ID, a.Name))
	}
	return root
}
