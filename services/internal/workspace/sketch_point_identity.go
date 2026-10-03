package workspace

import "fmt"

// Fit points and control poles are distinct persistent subelement collections.
// Indices are only solver addresses; insertion does not change point identity.
func normalizeSketchPointIdentities(sketch *SketchFeature) error {
	byID := map[string]*SketchEntity{}
	for i := range sketch.Entities {
		e := &sketch.Entities[i]
		byID[e.ID] = e
		if e.Kind == "LINE" || e.Kind == "ARC" || e.Kind == "ELLIPTICAL_ARC" || (e.Kind == "SPLINE" && !e.Closed) {
			if e.StartPointID == "" {
				e.StartPointID = macroID(e.ID, "endpoint/start")
			}
			if e.EndPointID == "" {
				e.EndPointID = macroID(e.ID, "endpoint/end")
			}
			if e.StartPointID == e.EndPointID {
				return fmt.Errorf("%w: curve endpoints require distinct identities", ErrValidation)
			}
		}
		if e.Kind != "SPLINE" {
			continue
		}
		collections := []struct {
			prefix string
			points []SketchPoint2
			ids    *[]string
		}{{"fit", e.ControlPoints, &e.ControlPointIDs}, {"pole", e.Poles, &e.PoleIDs}}
		for _, collection := range collections {
			if len(*collection.ids) > 0 && len(*collection.ids) != len(collection.points) {
				return fmt.Errorf("%w: spline point identity count mismatch", ErrValidation)
			}
			if len(*collection.ids) == 0 {
				for i := range collection.points {
					*collection.ids = append(*collection.ids, macroID(e.ID, fmt.Sprintf("%s/%d", collection.prefix, i)))
				}
			}
			seen := map[string]bool{}
			for _, id := range *collection.ids {
				if id == "" || seen[id] {
					return fmt.Errorf("%w: duplicate spline point identity", ErrValidation)
				}
				seen[id] = true
			}
		}
	}
	for ci := range sketch.Constraints {
		for ri := range sketch.Constraints[ci].References {
			r := &sketch.Constraints[ci].References[ri]
			if r.Target == "ENTITY" && (r.SubElement == "START" || r.SubElement == "END") {
				e := byID[r.EntityID]
				if e == nil {
					return fmt.Errorf("%w: endpoint target missing", ErrValidation)
				}
				id := e.StartPointID
				if r.SubElement == "END" {
					id = e.EndPointID
				}
				if id == "" || (r.PointID != "" && r.PointID != id) {
					return fmt.Errorf("%w: topology edit invalidates endpoint reference %s", ErrValidation, r.PointID)
				}
				r.PointID = id
			}
			if r.Target != "ENTITY" || r.SubElement != "CONTROL" {
				continue
			}
			e := byID[r.EntityID]
			if e == nil || e.Kind != "SPLINE" {
				return fmt.Errorf("%w: spline point reference target missing", ErrValidation)
			}
			ids := e.ControlPointIDs
			if e.Mode == "CONTROL" {
				ids = e.PoleIDs
			}
			index := -1
			if r.ControlPointIndex != nil {
				index = *r.ControlPointIndex
			}
			if r.ControlPointID != "" {
				index = -1
				for i, id := range ids {
					if id == r.ControlPointID {
						index = i
						break
					}
				}
			}
			if index < 0 || index >= len(ids) {
				return fmt.Errorf("%w: spline point reference %s is missing", ErrValidation, r.ControlPointID)
			}
			r.ControlPointID = ids[index]
			r.ControlPointIndex = &index
		}
	}
	return nil
}
