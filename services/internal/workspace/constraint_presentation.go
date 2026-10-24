package workspace

import (
	"fmt"
	"strconv"
	"strings"
)

func constraintTypeLabel(c AssemblyConstraint) string {
	switch c.Family {
	case "Coincidence":
		return "重合"
	case "Offset":
		return "偏移"
	case "Fix":
		return "固定"
	case "FixTogether":
		return "固联组"
	case "Contact":
		return "接触"
	case "Angle":
		if c.Subtype == "PARALLEL" {
			return "平行"
		}
		if c.Subtype == "PERPENDICULAR" {
			return "垂直"
		}
		return "角度"
	}
	switch c.Kind {
	case "COINCIDENT", "CONCENTRIC":
		return "重合"
	case "DISTANCE", "OFFSET":
		return "偏移"
	case "FIX":
		return "固定"
	case "ANGLE":
		return "角度"
	}
	return c.Kind
}

func assignConstraintName(c *AssemblyConstraint, counters *map[string]int, existing []AssemblyConstraint) {
	if c.Name != "" {
		return
	}
	label := automaticTypePrefix(c.Kind)
	if *counters == nil {
		*counters = map[string]int{}
	}
	names := []string{fmt.Sprintf("%s.%d", label, (*counters)[label])}
	for _, v := range existing {
		names = append(names, v.Name)
	}
	c.Name = nextAutomaticName(label, names)
	n, _ := strconv.Atoi(strings.TrimPrefix(c.Name, label+"."))
	(*counters)[label] = n
}

func constraintDisplayName(c AssemblyConstraint, nameFor func(string) string, groups ...AssemblyConstraint) string {
	name := c.Name
	if name == "" {
		name = constraintTypeLabel(c)
	}
	if isAssemblyGroup(c) {
		members := []string{}
		for _, member := range c.GroupMembers {
			label := nameFor(member.InstanceID)
			if member.GroupID != "" {
				label = member.GroupID
				for _, group := range groups {
					if group.ID == member.GroupID {
						label = group.Name
						break
					}
				}
			}
			members = append(members, label)
		}
		if len(members) == 0 {
			return name
		}
		return name + " (" + strings.Join(members, ",") + ")"
	}
	label := name + " (" + nameFor(c.First.InstanceID)
	if c.Second != nil {
		label += "," + nameFor(c.Second.InstanceID)
	}
	return label + ")"
}

func mechanismConstraintRole(c AssemblyConstraint) string {
	if c.RelationRole != "" {
		return c.RelationRole
	}
	switch c.Kind {
	case "FIX":
		return "ground"
	case "DISTANCE", "OFFSET":
		return "axial-location"
	case "ANGLE":
		return "angle-lock"
	}
	return "axis"
}
