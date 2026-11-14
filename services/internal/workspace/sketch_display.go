package workspace

// Sketch display is a projection of the accepted Revision, independently of
// the immutable Body GLB. This also serves thumbnails and pinned occurrences.
func sketchDisplayProjection(model PartModel) VisualizationManifest {
	v := visualizationManifest(model)
	features := map[string]Feature{}
	for _, f := range model.Features {
		features[f.ID] = f
	}
	bodies := map[string]PartBody{}
	for _, b := range model.Bodies {
		bodies[b.ID] = b
	}
	visible := v.Primitives[:0]
	for _, p := range v.Primitives {
		f, ok := features[p.FeatureID]
		if !ok && p.PatternID != "" {
			f, ok = features[p.PatternID]
		}
		if !ok {
			continue
		}
		if f.Suppressed || !visibleOrDefault(f.Visible) || !GeometryFeatureVisible(model, f) {
			continue
		}
		if b, ok := bodies[f.BodyID]; ok && (!b.Visible || b.Consumed) {
			continue
		}
		visible = append(visible, p)
	}
	v.Primitives = visible
	return v
}
