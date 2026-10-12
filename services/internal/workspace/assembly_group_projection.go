package workspace

// Reference nodes never own the referenced Instance/group and expose no Delete.
func assemblyGroupReferenceNodes(parent, document, mechanism string, c AssemblyConstraint, nameFor func(string) string, groups []AssemblyConstraint) []DocumentStructureNode {
	children := []DocumentStructureNode{}
	for _, member := range c.GroupMembers {
		id, kind, name := member.InstanceID, "INSTANCE", nameFor(member.InstanceID)
		if member.GroupID != "" {
			id, kind = member.GroupID, "FIX_TOGETHER"
			for _, group := range groups {
				if group.ID == id {
					name = group.Name
				}
			}
		}
		if name == "" {
			name = id
		}
		children = append(children, DocumentStructureNode{ID: parent + "/member:" + kind + ":" + id, Kind: "ASSEMBLY_GROUP_MEMBER_REFERENCE", EntityID: id, EntityType: kind, OwnerEntityID: c.ID, MechanismID: mechanism, DocumentID: document, Name: name + "（成员引用）", PresentationRole: "REFERENCE", Capabilities: []string{"EDIT"}})
	}
	return children
}
