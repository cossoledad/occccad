package workspace

import "sort"

// Project the accepted Coincident equivalence classes onto surviving points.
// Deleting a curve removes its endpoints, not the proven equality of the other
// endpoints at that junction. Coordinates and curve membership are not evidence.
func sketchCoincidencesAfterDeletion(sketch *SketchFeature, removed, released map[string]bool) []SketchConstraint {
	before, after := newDisjointSet(), newDisjointSet()
	refs := map[string]SketchGeometryRef{}
	active := []SketchConstraint{}
	key := func(r SketchGeometryRef) string { return sketchPointReferenceKey(sketch, r) }
	for _, c := range sketch.Constraints {
		if c.Kind != "COINCIDENT" || c.Suppressed || released[c.ID] || len(c.References) != 2 {
			continue
		}
		a, b := c.References[0], c.References[1]
		before.union(key(a), key(b))
		active = append(active, c)
		if !removed[a.EntityID] && !removed[b.EntityID] {
			after.union(key(a), key(b))
		}
		for _, r := range c.References {
			if removed[r.EntityID] {
				continue
			}
			k := key(r)
			prior, exists := refs[k]
			if !exists || r.EntityID+"/"+r.SubElement < prior.EntityID+"/"+prior.SubElement {
				refs[k] = r
			}
		}
	}
	proof := map[string]string{}
	internal := map[string]bool{}
	for _, c := range active {
		root := before.find(key(c.References[0]))
		if proof[root] == "" || c.ID < proof[root] {
			proof[root] = c.ID
		}
		if _, ok := internal[root]; !ok {
			internal[root] = true
		}
		internal[root] = internal[root] && c.Internal
	}
	groups := map[string][]string{}
	for k := range refs {
		root := before.find(k)
		groups[root] = append(groups[root], k)
	}
	roots := make([]string, 0, len(groups))
	for root := range groups {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	result := []SketchConstraint{}
	for _, root := range roots {
		points := groups[root]
		sort.Strings(points)
		for _, other := range points[1:] {
			first := points[0]
			if after.find(first) == after.find(other) {
				continue
			}
			result = append(result, SketchConstraint{ID: macroID(proof[root], "surviving-coincident/"+first+"/"+other), Kind: "COINCIDENT", Internal: internal[root], References: []SketchGeometryRef{refs[first], refs[other]}})
			after.union(first, other)
		}
	}
	return result
}
