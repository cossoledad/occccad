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
			label := j.Name + " (" + nameFor(j.First.InstanceID)
			if j.Second != nil {
				label += " ↔ " + nameFor(j.Second.InstanceID)
			}
			label += ")"
			jn := node("MECHANISM_JOINT", j.ID, label)
			jn.EntityType = j.Kind
			roles := []string{}
			if j.Kind != "GROUND" {
				roles = append(roles, "轴线相合")
			}
			if j.Kind != "GROUND" && j.Kind != "PRISMATIC" {
				roles = append(roles, "轴向定位")
			}
			if j.Kind == "RIGID" || j.Kind == "PRISMATIC" {
				roles = append(roles, "方向一致")
			}
			for _, role := range roles {
				if j.Kind != "GROUND" {
					child := node("JOINT_EXPANSION", j.ID+"/"+role, role)
					child.Capabilities = nil
					child.PresentationRole = "INPUT_REFERENCE"
					jn.Children = append(jn.Children, child)
				}
			}
			for side, end := range []JointEndpoint{j.First, func() JointEndpoint {
				if j.Second != nil {
					return *j.Second
				}
				return JointEndpoint{}
			}()} {
				if end.InstanceID == "" {
					continue
				}
				for _, support := range []struct {
					role string
					ref  *AssemblyGeometryRef
				}{{"axis", end.Axis}, {"plane", end.Plane}} {
					if support.ref == nil {
						continue
					}
					label := "轴线"
					if support.role == "plane" {
						label = "定位平面"
					}
					sn := node("JOINT_SUPPORT", fmt.Sprintf("%s/%d/%s", j.ID, side, support.role), label+" ("+nameFor(end.InstanceID)+")")
					sn.Capabilities = nil
					sn.PresentationRole = "INPUT_REFERENCE"
					jn.Children = append(jn.Children, sn)
				}
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
				jn.Children = append(jn.Children, sn)
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
		for _, a := range k.Associations {
			if a.MechanismID == m.ID {
				an := node("MOTION_ASSOCIATION", a.JointID+"/"+a.Role, "装配关联 / "+a.Role)
				an.Capabilities = nil
				an.PresentationRole = "INPUT_REFERENCE"
				an.Children = []DocumentStructureNode{node("JOINT_SOURCE", a.ConstraintID, a.ConstraintID)}
				an.Children[0].ID = an.ID + "/constraint:" + a.ConstraintID
				an.Children[0].Capabilities = nil
				an.Children[0].PresentationRole = "INPUT_REFERENCE"
				mn.Children = append(mn.Children, an)
			}
		}
		root.Children = append(root.Children, mn)
	}
	for _, a := range k.Analyses {
		root.Children = append(root.Children, node("INTERFERENCE_ANALYSIS", a.ID, a.Name))
	}
	return root
}
