package workspace

import (
	"fmt"
	"strings"
)

// Frozen server structure carries semantic addresses and persisted SHOW/HIDE
// projections. Display node IDs and editing-time reveal are deliberately unused.
type exportVisibility struct {
	entries   map[exportAddress]exportDisplayEntry
	instances map[string]exportAddress
	cache     map[exportAddress]bool
}
type exportAddress struct{ document, path, kind, entity string }
type exportDisplayEntry struct {
	local    bool
	mode     string
	eligible bool
	owner    string
}

// A failure in an excluded object must not block valid selected content. A
// selected failure must never silently fall back to an old display artifact.
func selectExchangePartContents(part PartModel, bodies []PartBody, doc, path string, visibility *exportVisibility, visibleOnly bool) ([]PartBody, []Feature, error) {
	failed := func(feature Feature) bool {
		return !feature.Suppressed && (feature.EvaluationStatus == "FAILED" || feature.EvaluationStatus == "BLOCKED")
	}
	failedBodies := map[string]bool{}
	for _, feature := range part.Features {
		if feature.BodyID != "" && failed(feature) {
			failedBodies[feature.BodyID] = true
		}
	}
	selected := make([]PartBody, 0, len(bodies))
	for _, body := range bodies {
		if body.Consumed || visibleOnly && !visibility.visible(exportAddress{doc, path, "BODY", body.ID}) {
			continue
		}
		if failedBodies[body.ID] {
			return nil, nil, fmt.Errorf("%w: selected Body has no authoritative export result", ErrValidation)
		}
		if body.GeometryKey != "" {
			selected = append(selected, body)
		}
	}
	var results []Feature
	for _, feature := range part.Features {
		if feature.Type != "GEOMETRY" || feature.Suppressed || visibleOnly && !visibility.visible(exportAddress{doc, path, "FEATURE", feature.ID}) {
			continue
		}
		if failed(feature) {
			return nil, nil, fmt.Errorf("%w: selected geometry has no authoritative export result", ErrValidation)
		}
		if feature.Result != nil && feature.Result.GeometryKey != "" {
			results = append(results, feature)
		}
	}
	return selected, results, nil
}

func newExportVisibility(root *DocumentStructureNode) *exportVisibility {
	v := &exportVisibility{map[exportAddress]exportDisplayEntry{}, map[string]exportAddress{}, map[exportAddress]bool{}}
	var visit func(*DocumentStructureNode)
	visit = func(node *DocumentStructureNode) {
		if node == nil {
			return
		}
		kind := node.Kind
		if kind == "BODY" || kind == "PART" || kind == "INSTANCE" || kind == "GEOMETRICAL_SET" || kind == "FEATURE" && node.BodyID == "" {
			a := exportAddress{document: node.DocumentID, kind: kind, entity: node.EntityID}
			if node.Subject != nil {
				a.document, a.entity = node.Subject.DocumentID, node.Subject.EntityID
			}
			if node.InstancePath != nil {
				a.path = node.InstancePath.Canonical
			}
			if a.entity == "" && kind == "PART" {
				a.entity = a.document
			}
			v.entries[a] = exportDisplayEntry{visibleOrDefault(node.LocalVisible), node.VisibilityMode, !node.Suppressed && !node.Consumed, node.OwnerEntityID}
			if kind == "INSTANCE" {
				v.instances[a.path] = a
			}
		}
		for i := range node.Children {
			visit(&node.Children[i])
		}
	}
	visit(root)
	return v
}

func (v *exportVisibility) visible(a exportAddress) bool {
	if result, ok := v.cache[a]; ok {
		return result
	}
	return v.resolve(a, map[exportAddress]bool{})
}
func (v *exportVisibility) resolve(a exportAddress, seen map[exportAddress]bool) bool {
	if result, ok := v.cache[a]; ok {
		return result
	}
	if seen[a] {
		return false
	}
	seen[a] = true
	entry, exists := v.entries[a]
	local := !exists || entry.local
	if entry.mode == "SHOW" {
		local = true
	} else if entry.mode == "HIDE" {
		local = false
	}
	if exists && !entry.eligible {
		local = false
	}
	var parent exportAddress
	switch a.kind {
	case "BODY":
		parent = exportAddress{a.document, a.path, "PART", a.document}
	case "FEATURE", "GEOMETRICAL_SET":
		if entry.owner != "" {
			parent = exportAddress{a.document, a.path, "GEOMETRICAL_SET", entry.owner}
		} else {
			parent = exportAddress{a.document, a.path, "PART", a.document}
		}
	case "PART":
		parent = v.instances[a.path]
	case "INSTANCE":
		if i := strings.LastIndex(a.path, "/"); i >= 0 {
			parent = v.instances[a.path[:i]]
		}
	}
	if parent.entity != "" {
		local = local && v.resolve(parent, seen)
	}
	v.cache[a] = local
	return local
}

func (v *exportVisibility) instanceVisible(path string) bool {
	if a, ok := v.instances[path]; ok {
		return v.visible(a)
	}
	return true
}

func (v *exportVisibility) localVisible(a exportAddress) bool {
	entry, ok := v.entries[a]
	if !ok {
		return true
	}
	return entry.eligible && (entry.mode == "SHOW" || entry.mode != "HIDE" && entry.local)
}

// Exported definitions/occurrences retain their own state. A hidden parent
// must not permanently mark every descendant hidden in an ALL export.
func (v *exportVisibility) resultLocalVisible(a exportAddress) bool {
	return v.resultLocalVisibleSeen(a, map[exportAddress]bool{})
}
func (v *exportVisibility) resultLocalVisibleSeen(a exportAddress, seen map[exportAddress]bool) bool {
	if seen[a] {
		return false
	}
	seen[a] = true
	if !v.localVisible(a) {
		return false
	}
	if a.kind == "FEATURE" || a.kind == "GEOMETRICAL_SET" {
		if owner := v.entries[a].owner; owner != "" {
			return v.resultLocalVisibleSeen(exportAddress{a.document, a.path, "GEOMETRICAL_SET", owner}, seen)
		}
	}
	return true
}
