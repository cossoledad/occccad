package workspace

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// A picked center is a model reference, not a sampled viewport coordinate.
type PatternPointReference struct {
	SketchID     string            `json:"sketchId,omitempty"`
	Reference    SketchGeometryRef `json:"reference"`
	AxisEntityID string            `json:"axisEntityId,omitempty"`
}

func patternReferencePoint(model PartModel, reference PatternPointReference) ([3]float64, error) {
	if reference.AxisEntityID != "" {
		axis, err := resolvePatternAxisDefinition(model, PatternDefinition{AxisEntityID: reference.AxisEntityID})
		return axis.Origin, err
	}
	for _, f := range model.Features {
		if f.ID != reference.SketchID || f.Sketch == nil || f.Suppressed {
			continue
		}
		entities := patternSketchReferenceEntities(*f.Sketch)
		r := reference.Reference
		if r.Target != "SKETCH_ORIGIN" && ((r.Target != "ENTITY" && r.Target != "EXTERNAL") || !strings.Contains("|POINT|START|END|CENTER|", "|"+r.SubElement+"|")) {
			break
		}
		if r.Target == "ENTITY" && r.PointID != "" {
			entity := entities[r.EntityID]
			if (r.SubElement == "START" && entity.StartPointID != r.PointID) || (r.SubElement == "END" && entity.EndPointID != r.PointID) {
				break
			}
		}
		point, ok := constraintReferencePoint(r, entities)
		if !ok {
			break
		}
		o, u, n, ok := supportFrame(model, f.Sketch.Support)
		if !ok {
			break
		}
		v := patternCross(n, u)
		for i := range o {
			o[i] += u[i]*point.X + v[i]*point.Y
		}
		return o, nil
	}
	return [3]float64{}, fmt.Errorf("%w: PATTERN_CENTER_UNAVAILABLE", ErrValidation)
}

func resolvedPatternDefinition(model PartModel, p PatternDefinition) (PatternDefinition, error) {
	resolved, err := resolvePatternAxisDefinition(model, p)
	if err != nil {
		return p, err
	}
	if p.CenterReference != nil {
		resolved.Origin, err = patternReferencePoint(model, *p.CenterReference)
	}
	return resolved, err
}

func resolveSketchPatternReferences(model *PartModel, indices ...int) error {
	for i := range model.Features {
		if len(indices) > 0 && i != indices[0] {
			continue
		}
		f := &model.Features[i]
		if f.Sketch == nil || f.Suppressed {
			continue
		}
		for j := range f.Sketch.Patterns {
			p := &f.Sketch.Patterns[j]
			if p.Suppressed {
				continue
			}
			if p.CenterReference != nil && (p.CenterReference.SketchID != f.ID || p.CenterReference.AxisEntityID != "") {
				return fmt.Errorf("%w: PATTERN_REFERENCE_OUTSIDE_SKETCH", ErrValidation)
			}
			if p.DirectionReference != nil {
				a, b, ok := constraintLine(*p.DirectionReference, patternSketchReferenceEntities(*f.Sketch))
				if !ok {
					return fmt.Errorf("%w: PATTERN_DIRECTION_UNAVAILABLE", ErrValidation)
				}
				p.Direction = [3]float64{b.X - a.X, b.Y - a.Y, 0}
			} else if p.AxisEntityID == "SKETCH_AXIS:U" {
				p.Direction = [3]float64{1, 0, 0}
			} else if p.AxisEntityID == "SKETCH_AXIS:V" {
				p.Direction = [3]float64{0, 1, 0}
			} else if p.AxisEntityID != "" {
				parts := strings.Split(p.AxisEntityID, ":")
				if len(parts) != 3 || parts[0] != "SKETCH_LINE" || parts[1] != f.ID {
					return fmt.Errorf("%w: PATTERN_REFERENCE_OUTSIDE_SKETCH", ErrValidation)
				}
				a, b, ok := constraintLine(SketchGeometryRef{Target: "ENTITY", EntityID: parts[2], SubElement: "DIRECTION"}, patternSketchReferenceEntities(*f.Sketch))
				if !ok {
					return fmt.Errorf("%w: PATTERN_DIRECTION_UNAVAILABLE", ErrValidation)
				}
				p.Direction = [3]float64{b.X - a.X, b.Y - a.Y, 0}
			}
			if p.CenterReference != nil {
				center, err := patternReferencePoint(*model, *p.CenterReference)
				if err != nil {
					return err
				}
				o, u, n, ok := supportFrame(*model, f.Sketch.Support)
				if !ok {
					return fmt.Errorf("%w: pattern sketch frame unavailable", ErrValidation)
				}
				for k := range center {
					center[k] -= o[k]
				}
				if math.Abs(patternDot(center, n)) > 1e-7 {
					return fmt.Errorf("%w: PATTERN_CENTER_OUT_OF_SKETCH_PLANE", ErrValidation)
				}
				p.Origin = [3]float64{patternDot(center, u), patternDot(center, patternCross(n, u)), 0}
			}
			if _, err := patternPlacements(p.PatternDefinition); err != nil {
				return err
			}
		}
	}
	return nil
}

func patternReferenceFeatures(p PatternDefinition) []string {
	var ids []string
	if p.MirrorPlane != nil {
		ids = append(ids, p.MirrorPlane.Selection.Anchor.FeatureID)
		if p.MirrorPlane.SourceFeatureID != "" {
			ids = append(ids, p.MirrorPlane.SourceFeatureID)
		}
	}
	for _, axis := range []string{p.AxisEntityID, func() string {
		if p.CenterReference != nil {
			return p.CenterReference.AxisEntityID
		}
		return ""
	}()} {
		parts := strings.Split(axis, ":")
		if len(parts) == 3 && parts[0] == "SKETCH_LINE" {
			ids = append(ids, parts[1])
		}
	}
	if p.CenterReference != nil && p.CenterReference.SketchID != "" {
		ids = append(ids, p.CenterReference.SketchID)
	}
	return ids
}

func patternDot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func patternCross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func mustPatternDeletePayload(sketchID, patternID string) json.RawMessage {
	data, _ := json.Marshal(editSketchPayload{SketchID: sketchID, Operations: []SketchOperation{{Type: "DELETE_PATTERN", PatternID: patternID}}})
	return data
}

func patternSketchReferenceEntities(sketch SketchFeature) map[string]SketchEntity {
	entities := map[string]SketchEntity{}
	for _, e := range sketch.Entities {
		if !e.Suppressed {
			entities[e.ID] = e
		}
	}
	for _, external := range sketch.ExternalGeometry {
		if external.Status == "CONNECTED" && external.Snapshot != nil {
			s := external.Snapshot
			entities[external.ID] = SketchEntity{ID: external.ID, Kind: s.Kind, Point: s.Point, Start: s.Start, End: s.End, Center: s.Center}
		}
	}
	return entities
}
