package workspace

import "fmt"

// A simulation projects the same mechanism with a distinct tree path and explicit owners.
func kinematicsStructure(doc, revision, path string, k KinematicsDefinitions, constraints []AssemblyConstraint, instances ...[]ProductInstance) DocumentStructureNode {
	root := DocumentStructureNode{ID: path + "/applications", Kind: "APPLICATIONS", Name: "Applications", DocumentID: doc, VersionID: revision}
	node := func(parent, kind, id, name string) DocumentStructureNode {
		return DocumentStructureNode{ID: parent + "/" + kind + ":" + id, Kind: kind, EntityID: id, Name: name, DocumentID: doc, OwnerDocumentID: doc, VersionID: revision, Capabilities: []string{"EDIT", "DELETE", "RENAME"}}
	}
	group := func(parent, role, name string) DocumentStructureNode {
		n := node(parent, "KINEMATICS_GROUP", role, name)
		n.EntityID = ""
		n.InputRole = role
		n.Capabilities = nil
		return n
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
	project := func(parent string, m Mechanism, reference bool) DocumentStructureNode {
		kind := "MECHANISM"
		if reference {
			kind = "MECHANISM_REFERENCE"
		}
		label := m.Name
		if m.Evaluation != nil {
			label += fmt.Sprintf("，自由度=%d", m.Evaluation.RelativeDOF)
		} else {
			label += "，自由度=待计算"
		}
		mn := node(parent, kind, m.ID, label)
		mn.MechanismID = m.ID
		if reference {
			mn.Capabilities = []string{"ACTIVATE"}
			mn.PresentationRole = "INPUT_REFERENCE"
		}
		joints := group(mn.ID, "joints", "接合")
		drivers := group(mn.ID, "commands", "命令")
		ground := group(mn.ID, "ground", "固定零件")
		for _, j := range m.Joints {
			label := j.Name + " (" + nameFor(j.First.InstanceID)
			if j.Second != nil {
				label += "," + nameFor(j.Second.InstanceID)
			}
			label += ")"
			parent := joints.ID
			if j.Kind == "GROUND" {
				parent = ground.ID
			}
			jn := node(parent, "MECHANISM_JOINT", j.ID, label)
			jn.EntityType = j.Kind
			jn.MechanismID = m.ID
			for _, c := range j.Constraints {
				cn := node(jn.ID, "JOINT_EXPANSION", c.ID, constraintDisplayName(c, nameFor, j.Constraints...))
				cn.EntityType = c.Kind
				cn.OwnerEntityID = j.ID
				cn.MechanismID = m.ID
				cn.RelationRole = mechanismConstraintRole(c)
				cn.Capabilities = []string{"EDIT"}
				cn.PresentationRole = "DEFINITION"
				cn.EvaluationStatus = string(c.EvaluationStatus)
				if isAssemblyGroup(c) {
					cn.Children = assemblyGroupReferenceNodes(cn.ID, doc, m.ID, c, nameFor, j.Constraints)
				}
				jn.Children = append(jn.Children, cn)
			}
			if j.Kind == "GROUND" {
				ground.Children = append(ground.Children, jn)
			} else {
				joints.Children = append(joints.Children, jn)
			}
		}
		for _, d := range k.Drivers {
			if d.MechanismID == m.ID {
				label := d.Name
				for _, j := range m.Joints {
					if j.ID == d.JointID {
						parameter := "长度"
						for _, coordinate := range j.Coordinates {
							if coordinate.ID == d.CoordinateID && coordinate.angular() {
								parameter = "角度"
							}
						}
						label += " (" + j.Name + "," + parameter + ")"
					}
				}
				n := node(drivers.ID, "MOTION_DRIVER", d.ID, label)
				n.MechanismID = m.ID
				n.OwnerEntityID = d.JointID
				drivers.Children = append(drivers.Children, n)
			}
		}

		mn.Children = []DocumentStructureNode{joints, drivers, ground}
		return mn
	}
	mechanisms := group(root.ID, "mechanisms", "机械装置")
	simulations := group(root.ID, "simulations", "模拟")
	for _, m := range k.Mechanisms {
		mechanisms.Children = append(mechanisms.Children, project(mechanisms.ID, m, false))
	}
	for _, s := range k.Studies {
		sn := node(simulations.ID, "MOTION_STUDY", s.ID, s.Name)
		sn.MechanismID = s.MechanismID
		sn.OwnerEntityID = s.MechanismID
		for _, m := range k.Mechanisms {
			if m.ID == s.MechanismID {
				sn.Children = append(sn.Children, project(sn.ID, m, true))
			}
		}
		simulations.Children = append(simulations.Children, sn)
	}
	root.Children = []DocumentStructureNode{mechanisms, simulations}
	return root
}
