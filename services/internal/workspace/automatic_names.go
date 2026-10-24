package workspace

import (
	"strconv"
	"strings"
)

// Display names belong to definitions, never to a tree projection or a mesh.
func automaticTypePrefix(kind string) string {
	switch strings.ToUpper(kind) {
	case "PAD", "LINEAR_EXTRUDE":
		return "Extrude"
	case "IMPORT_BODY":
		return "Import"
	case "SOLID_PATTERN":
		return "Pattern"
	}
	var out strings.Builder
	for _, word := range strings.Split(strings.ToLower(kind), "_") {
		if word != "" {
			out.WriteString(strings.ToUpper(word[:1]) + word[1:])
		}
	}
	return out.String()
}

func nextAutomaticName(prefix string, names []string) string {
	used := map[string]bool{}
	max := 0
	for _, name := range names {
		used[scopedNameKey(name)] = true
		if suffix, ok := strings.CutPrefix(strings.ToLower(name), strings.ToLower(prefix)+"."); ok {
			if n, err := strconv.Atoi(suffix); err == nil && n > max {
				max = n
			}
		}
	}
	for n := max + 1; ; n++ {
		candidate := prefix + "." + strconv.Itoa(n)
		if !used[scopedNameKey(candidate)] {
			return candidate
		}
	}
}

func assignSketchNames(sketch *SketchFeature, before SketchFeature) {
	// Existing IDs retain their accepted names, including names copied by tools.
	old := map[string]string{}
	used := []string{}
	for _, e := range before.Entities {
		old[e.ID] = e.Name
		if e.Name != "" {
			used = append(used, e.Name)
		}
	}
	for _, c := range before.Constraints {
		old[c.ID] = c.Name
		if c.Name != "" {
			used = append(used, c.Name)
		}
	}
	for i := range sketch.Entities {
		e := &sketch.Entities[i]
		if name, exists := old[e.ID]; exists && name != "" {
			e.Name = name
			continue
		}
		if e.Name == "" || containsDisplayName(used, e.Name) {
			e.Name = nextAutomaticName(automaticTypePrefix(e.Kind), used)
		}
		used = append(used, e.Name)
	}
	for i := range sketch.Constraints {
		c := &sketch.Constraints[i]
		if name, exists := old[c.ID]; exists && name != "" {
			c.Name = name
			continue
		}
		if c.Name == "" || containsDisplayName(used, c.Name) {
			c.Name = nextAutomaticName(automaticTypePrefix(c.Kind), used)
		}
		used = append(used, c.Name)
	}
	externalNames := []string{}
	for _, e := range sketch.ExternalGeometry {
		if e.Name != "" {
			externalNames = append(externalNames, e.Name)
		}
	}
	for i := range sketch.ExternalGeometry {
		e := &sketch.ExternalGeometry[i]
		if e.Name == "" {
			prefix := "External"
			if e.Snapshot != nil {
				prefix = "External" + automaticTypePrefix(e.Snapshot.Kind)
			}
			e.Name = nextAutomaticName(prefix, externalNames)
			externalNames = append(externalNames, e.Name)
		}
	}
	patternNames := []string{}
	for _, p := range sketch.Patterns {
		if p.Name != "" {
			patternNames = append(patternNames, p.Name)
		}
	}
	for i := range sketch.Patterns {
		p := &sketch.Patterns[i]
		if p.Name == "" {
			p.Name = nextAutomaticName(automaticTypePrefix(p.Kind)+"Pattern", patternNames)
			patternNames = append(patternNames, p.Name)
		}
	}
}
func containsDisplayName(names []string, name string) bool {
	for _, n := range names {
		if scopedNameKey(n) == scopedNameKey(name) {
			return true
		}
	}
	return false
}
