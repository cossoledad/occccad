package workspace

import (
	"fmt"
	"strings"
)

// Host-scoped names are display aliases. They never rewrite a shared Product's
// instance definition or the typed identity of an InstancePath.
func projectOccurrenceNames(root *DocumentStructureNode, model ProductModel, base string) {
	names := map[string]string{}
	for _, override := range model.AppearanceOverrides {
		if override.Name != nil {
			path := override.InstancePath.Canonical
			if base != "" {
				path = base + "/" + path
			}
			names[path] = *override.Name
		}
	}
	if len(names) == 0 {
		return
	}
	updatePath := func(path *InstancePath) {
		if path == nil {
			return
		}
		path.Segments = append([]InstancePathSegment(nil), path.Segments...)
		ids := []string{}
		display := []string{}
		for i := range path.Segments {
			ids = append(ids, path.Segments[i].InstanceID)
			if name, ok := names[strings.Join(ids, "/")]; ok {
				path.Segments[i].InstanceName = name
			}
			display = append(display, path.Segments[i].InstanceName)
		}
		path.Display = strings.Join(display, "/")
	}
	var visit func(*DocumentStructureNode)
	visit = func(node *DocumentStructureNode) {
		if node.Kind == "INSTANCE" && node.InstancePath != nil {
			if name, ok := names[node.InstancePath.Canonical]; ok {
				node.Name = fmt.Sprintf("%s(%s)", node.ReferenceName, name)
				node.InstanceName = name
			}
		}
		updatePath(node.InstancePath)
		if node.Occurrence != nil {
			updatePath(&node.Occurrence.InstancePath)
		}
		for i := range node.Children {
			visit(&node.Children[i])
		}
	}
	visit(root)
}

func projectResolvedOccurrenceNames(items []ResolvedInstance, rules map[string][]occurrenceAppearanceRule) {
	for i := range items {
		item := &items[i]
		names := map[string]string{}
		depths := map[string]int{}
		for _, r := range rules[item.OccurrencePath] {
			if r.Name == nil {
				continue
			}
			if d, ok := depths[r.Path]; !ok || r.OwnerDepth < d {
				names[r.Path] = *r.Name
				depths[r.Path] = r.OwnerDepth
			}
		}
		item.InstancePath.Segments = append([]InstancePathSegment(nil), item.InstancePath.Segments...)
		ids, display := []string{}, []string{}
		for j := range item.InstancePath.Segments {
			segment := &item.InstancePath.Segments[j]
			ids = append(ids, segment.InstanceID)
			if name, ok := names[strings.Join(ids, "/")]; ok {
				segment.InstanceName = name
			}
			display = append(display, segment.InstanceName)
		}
		item.InstancePath.Display = strings.Join(display, "/")
		if name, ok := names[item.OccurrencePath]; ok {
			item.Name = name
		}
	}
}
