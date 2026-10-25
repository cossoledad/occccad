package workspace

// Label coordinates remain layout, never solver input. Capture the exact
// reference-derived default anchor so solver/point edits carry authored labels
// with their supports, without binding to display tessellation or nearest data.
type sketchDimensionPlacement struct{ label, anchor SketchPoint2 }

func sketchDimensionPlacements(sketch *SketchFeature) map[string]sketchDimensionPlacement {
	entities := map[string]SketchEntity{}
	for _, e := range sketch.Entities {
		entities[e.ID] = e
	}
	for _, e := range sketch.ExternalGeometry {
		if e.Status == "CONNECTED" && e.Snapshot != nil {
			entities[e.ID] = e.Snapshot.entity(e.ID)
		}
	}
	result := map[string]sketchDimensionPlacement{}
	for _, c := range sketch.Constraints {
		if c.LabelPosition == nil {
			continue
		}
		label := *c.LabelPosition
		c.LabelPosition = nil
		if visual, ok := constraintVisual(c, entities); ok && visual.LabelPosition != nil {
			result[c.ID] = sketchDimensionPlacement{label, *visual.LabelPosition}
		}
	}
	return result
}
func carrySketchDimensionPlacements(sketch *SketchFeature, before map[string]sketchDimensionPlacement) {
	if len(before) == 0 {
		return
	}
	after := sketchDimensionPlacements(sketch)
	for i := range sketch.Constraints {
		c := &sketch.Constraints[i]
		a, ok := before[c.ID]
		b, valid := after[c.ID]
		if !ok || !valid || c.LabelPosition == nil || *c.LabelPosition != a.label {
			continue
		}
		dx, dy := b.anchor.X-a.anchor.X, b.anchor.Y-a.anchor.Y
		if dx == 0 && dy == 0 {
			continue
		}
		c.LabelPosition = &SketchPoint2{X: a.label.X + dx, Y: a.label.Y + dy}
	}
}
