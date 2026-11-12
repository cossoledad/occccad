package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/occccad/occccad/internal/modelcore"
)

type partPropertyObject struct {
	name        *string
	appearance  **SurfaceAppearance
	prefix      string
	containerID string
}

func partPropertyObjectFor(m *PartModel, kind, id string) (partPropertyObject, bool) {
	switch kind {
	case "GEOMETRICAL_SET":
		for i := range m.GeometricalSets {
			v := &m.GeometricalSets[i]
			if v.ID == id {
				return partPropertyObject{&v.Name, &v.Appearance, "geometrical-set", v.ParentID}, true
			}
		}
	case "REFERENCE_POINT":
		for i := range m.DatumPoints {
			v := &m.DatumPoints[i]
			if v.ID == id {
				return partPropertyObject{&v.Name, &v.Appearance, "reference-point", v.GeometricalSetID}, true
			}
		}
	case "PLANE", "DATUM_PLANE":
		for i := range m.DatumPlanes {
			v := &m.DatumPlanes[i]
			if v.ID == id {
				return partPropertyObject{&v.Name, &v.Appearance, "datum-plane", v.GeometricalSetID}, true
			}
		}
	case "DATUM_AXIS":
		for i := range m.DatumAxes {
			v := &m.DatumAxes[i]
			if v.ID == id {
				return partPropertyObject{&v.Name, &v.Appearance, "datum-axis", v.GeometricalSetID}, true
			}
		}
	case "FEATURE", "SKETCH":
		for i := range m.Features {
			v := &m.Features[i]
			if v.ID == id {
				var appearance **SurfaceAppearance
				if v.Type == "GEOMETRY" {
					appearance = &v.Appearance
				}
				return partPropertyObject{&v.Name, appearance, "feature", v.GeometricalSetID}, true
			}
		}
	}
	return partPropertyObject{}, false
}
func applyPartObjectProperties(m *PartModel, t PropertyTarget, input PropertyUpdateRequest, set *modelcore.ChangeSet) (bool, error) {
	kind, id := t.Kind, t.ID
	if kind == "FACE" && t.SourceResult != nil {
		kind, id = "FEATURE", t.SourceResult.FeatureID
	}
	if kind != "FEATURE" && kind != "SKETCH" && kind != "PLANE" && kind != "DATUM_PLANE" && kind != "DATUM_AXIS" && kind != "REFERENCE_POINT" && kind != "GEOMETRICAL_SET" {
		return false, nil
	}
	object, ok := partPropertyObjectFor(m, kind, id)
	if !ok {
		return true, fmt.Errorf("%w: property target does not exist", ErrValidation)
	}
	if t.Kind == "FACE" && (t.SourceResult.ResultID != "result" || t.Anchor == nil) {
		return true, fmt.Errorf("%w: independent face owner is incomplete", ErrValidation)
	}
	if input.Name != nil {
		if (kind == "PLANE" || kind == "DATUM_PLANE") && isStandardDatumPlane(id) {
			return true, fmt.Errorf("%w: standard plane names are protected", ErrValidation)
		}
		before := *object.name
		*object.name = *input.Name
		addPropertyChange(set, id, object.prefix+".name", before, *object.name)
	}
	if len(input.Appearance) > 0 || input.ClearFaceOverrides {
		if object.appearance == nil {
			return true, fmt.Errorf("%w: select the owning Body to edit solid appearance", ErrValidation)
		}
		before := *object.appearance
		after, err := patchTargetAppearance(before, t, input)
		if err != nil {
			return true, err
		}
		if input.ClearFaceOverrides && kind == "FEATURE" {
			for _, f := range m.Features {
				if f.ID != id || f.ImportedAppearance == nil {
					continue
				}
				if after == nil {
					after = &SurfaceAppearance{}
				}
				for _, face := range f.ImportedAppearance.Faces {
					after.Faces = append(after.Faces, FaceAppearance{Anchor: face.Anchor, InheritedFields: []string{"color", "opacity", "roughness", "metalness", "pickable"}})
				}
			}
		}
		*object.appearance = after
		addPropertyChange(set, id, object.prefix+".appearance", before, after)
	}
	return true, nil
}
func geometricalSetAppearance(m PartModel, id string) *SurfaceAppearance {
	chain := []*SurfaceAppearance{}
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		found := false
		for _, v := range m.GeometricalSets {
			if v.ID == id {
				chain = append(chain, v.Appearance)
				id = v.ParentID
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	style := appearanceStyle(m.GeometryDefaultAppearance)
	for i := len(chain) - 1; i >= 0; i-- {
		style = overlayStyle(style, appearanceStyle(chain[i]))
	}
	if reflect.DeepEqual(style, SurfaceStyle{}) {
		return nil
	}
	return &SurfaceAppearance{GraphicProperties: style.GraphicProperties, Color: style.Color}
}
func (s *Service) partResultAppearance(ctx context.Context, m PartModel, f Feature) (ResolvedAppearance, error) {
	inherited := geometricalSetAppearance(m, f.GeometricalSetID)
	imported := f.ImportedAppearance
	if f.Appearance != nil {
		style := overlayStyle(appearanceStyle(imported), appearanceStyle(f.Appearance))
		combined := SurfaceAppearance{GraphicProperties: style.GraphicProperties, Color: style.Color}
		if imported != nil {
			combined.Faces = append(combined.Faces, imported.Faces...)
		}
		for _, face := range f.Appearance.Faces {
			replaced := false
			for i, prior := range combined.Faces {
				a, _ := json.Marshal(face.Anchor)
				b, _ := json.Marshal(prior.Anchor)
				if string(a) == string(b) {
					combined.Faces[i] = face
					replaced = true
					break
				}
			}
			if !replaced {
				combined.Faces = append(combined.Faces, face)
			}
		}
		f.ImportedAppearance = &combined
	}
	result, err := s.resultAppearance(ctx, f, inherited)
	if err != nil {
		return result, err
	}
	if result.Sources == nil {
		result.Sources = map[string]AppearanceSource{}
	}
	for _, key := range graphicStyleFields {
		source := AppearanceSource{Kind: "SYSTEM"}
		if styleFieldPresent(appearanceStyle(m.GeometryDefaultAppearance), key) {
			source.Kind = "DOCUMENT_DEFAULT"
		}
		chain := []GeometricalSet{}
		id := f.GeometricalSetID
		for id != "" {
			found := false
			for _, set := range m.GeometricalSets {
				if set.ID == id {
					chain = append(chain, set)
					id = set.ParentID
					found = true
					break
				}
			}
			if !found {
				break
			}
		}
		for i := len(chain) - 1; i >= 0; i-- {
			if styleFieldPresent(appearanceStyle(chain[i].Appearance), key) {
				source.Kind = "GEOMETRICAL_SET"
			}
		}
		if styleFieldPresent(appearanceStyle(imported), key) {
			source.Kind = "IMPORTED_RESULT"
		}
		if styleFieldPresent(appearanceStyle(f.Appearance), key) {
			source.Kind = "GEOMETRY_RESULT"
		}
		result.Sources[key] = source
	}
	return result, nil
}

var graphicStyleFields = []string{"color", "opacity", "roughness", "metalness", "edgeColor", "edgeWidth", "edges", "pickable", "edgeOpacityMode", "lineStyle", "pointSize", "pointSymbol"}

func styleFieldPresent(style SurfaceStyle, key string) bool {
	switch key {
	case "color":
		return style.Color != ""
	case "opacity":
		return style.Opacity != nil
	case "roughness":
		return style.Roughness != nil
	case "metalness":
		return style.Metalness != nil
	case "edgeColor":
		return style.EdgeColor != ""
	case "edgeWidth":
		return style.EdgeWidth != nil
	case "edges":
		return style.Edges != nil
	case "pickable":
		return style.Pickable != nil
	case "edgeOpacityMode":
		return style.EdgeOpacityMode != ""
	case "lineStyle":
		return style.LineStyle != ""
	case "pointSize":
		return style.PointSize != nil
	case "pointSymbol":
		return style.PointSymbol != ""
	}
	return false
}
func inheritedObjectStyleSource(m PartModel, container, field string) AppearanceSource {
	seen := map[string]bool{}
	for container != "" && !seen[container] {
		seen[container] = true
		next := ""
		for _, set := range m.GeometricalSets {
			if set.ID == container {
				if styleFieldPresent(appearanceStyle(set.Appearance), field) {
					return AppearanceSource{Kind: "GEOMETRICAL_SET"}
				}
				next = set.ParentID
				break
			}
		}
		container = next
	}
	if styleFieldPresent(appearanceStyle(m.GeometryDefaultAppearance), field) {
		return AppearanceSource{Kind: "DOCUMENT_DEFAULT"}
	}
	return AppearanceSource{Kind: "SYSTEM"}
}
func validatePartObjectStyles(m PartModel) error {
	check := func(a *SurfaceAppearance) error {
		if err := validateSurfaceStyle(appearanceStyle(a)); err != nil {
			return err
		}
		if a != nil {
			for _, f := range a.Faces {
				if err := validateSurfaceStyle(SurfaceStyle{Color: f.Color, GraphicProperties: f.GraphicProperties}); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := check(m.DefaultAppearance); err != nil {
		return err
	}
	if err := check(m.GeometryDefaultAppearance); err != nil {
		return err
	}
	// Validate active values directly; constructing a lifecycle index here
	// would copy every definition and turn validation into quadratic lookups.
	for _, f := range m.Features {
		if err := check(f.Appearance); err != nil {
			return err
		}
	}
	for _, g := range m.GeometricalSets {
		if err := check(g.Appearance); err != nil {
			return err
		}
	}
	for _, p := range m.DatumPoints {
		if err := check(p.Appearance); err != nil {
			return err
		}
	}
	for _, p := range m.DatumPlanes {
		if err := check(p.Appearance); err != nil {
			return err
		}
	}
	for _, a := range m.DatumAxes {
		if err := check(a.Appearance); err != nil {
			return err
		}
	}

	for _, b := range m.Bodies {
		if err := check(b.Appearance); err != nil {
			return err
		}
	}
	return nil
}

func partObjectAppearances(m PartModel) map[string]ResolvedAppearance {
	out := map[string]ResolvedAppearance{}
	project := func(kind, id, containerID string, appearance *SurfaceAppearance) {
		base := geometricalSetAppearance(m, containerID)
		style := overlayStyle(appearanceStyle(base), appearanceStyle(appearance))
		sources := map[string]AppearanceSource{}
		for _, key := range graphicStyleFields {
			source := inheritedObjectStyleSource(m, containerID, key)
			if styleFieldPresent(appearanceStyle(appearance), key) {
				source.Kind = kind
			}
			sources[key] = source
		}
		if kind == "PLANE" {
			kind = "DATUM_PLANE"
		}
		out[kind+":"+id] = ResolvedAppearance{GraphicProperties: style.GraphicProperties, Color: style.Color, Sources: sources}
	}
	for _, v := range m.GeometricalSets {
		project("GEOMETRICAL_SET", v.ID, v.ParentID, v.Appearance)
	}
	for _, v := range m.DatumPoints {
		project("REFERENCE_POINT", v.ID, v.GeometricalSetID, v.Appearance)
	}
	for _, v := range m.DatumPlanes {
		project("PLANE", v.ID, v.GeometricalSetID, v.Appearance)
	}
	for _, v := range m.DatumAxes {
		project("DATUM_AXIS", v.ID, v.GeometricalSetID, v.Appearance)
	}
	return out
}
