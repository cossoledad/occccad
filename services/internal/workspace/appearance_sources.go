package workspace

import (
	"context"
	"encoding/json"
)

// Sources belong to a read projection, not the model or an exchange format.
type AppearanceSource struct {
	Kind           string `json:"kind"`
	BodyID         string `json:"bodyId,omitempty"`
	OccurrencePath string `json:"occurrencePath,omitempty"`
}

var graphicFields = []string{"color", "opacity", "roughness", "metalness", "edgeColor", "edgeWidth", "edges", "pickable", "edgeOpacityMode"}

func styleFields(style SurfaceStyle) map[string]json.RawMessage {
	raw, _ := json.Marshal(style)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	return fields
}

func definitionAppearanceSources(out ResolvedAppearance, body PartBody, defaults *SurfaceAppearance) ResolvedAppearance {
	local, inherited := styleFields(appearanceStyle(body.Appearance)), styleFields(appearanceStyle(defaults))
	out.Sources = map[string]AppearanceSource{}
	for _, key := range graphicFields {
		source := AppearanceSource{Kind: "SYSTEM"}
		if inherited[key] != nil {
			source.Kind = "DOCUMENT_DEFAULT"
		}
		if local[key] != nil {
			source = AppearanceSource{Kind: "BODY", BodyID: body.ID}
		}
		out.Sources[key] = source
	}
	out.FaceSources = map[uint64]map[string]AppearanceSource{}
	for id := range out.Faces {
		out.FaceSources[id] = map[string]AppearanceSource{"color": {Kind: "FACE", BodyID: body.ID}}
	}
	for id, properties := range out.FaceProperties {
		if out.FaceSources[id] == nil {
			out.FaceSources[id] = map[string]AppearanceSource{}
		}
		for key := range styleFields(SurfaceStyle{GraphicProperties: properties}) {
			out.FaceSources[id][key] = AppearanceSource{Kind: "FACE", BodyID: body.ID}
		}
	}
	return normalizeAppearanceProjection(out)
}

func normalizeAppearanceProjection(out ResolvedAppearance) ResolvedAppearance {
	if len(out.FaceSources) == 0 {
		out.FaceSources = nil
	}
	if len(out.Faces) == 0 {
		out.Faces = nil
	}
	if len(out.FaceProperties) == 0 {
		out.FaceProperties = nil
	}
	if len(out.FaceInherit) == 0 {
		out.FaceInherit = nil
	}
	return out
}

func (s *Service) bodyAppearanceWithDefaults(ctx context.Context, body PartBody, defaults *SurfaceAppearance) (ResolvedAppearance, error) {
	out, err := s.bodyAppearance(ctx, withDefaultAppearance(body, defaults))
	if err != nil {
		return out, err
	}
	return definitionAppearanceSources(out, body, defaults), nil
}

func copyFaceSources(input map[uint64]map[string]AppearanceSource) map[uint64]map[string]AppearanceSource {
	out := map[uint64]map[string]AppearanceSource{}
	for id, fields := range input {
		out[id] = map[string]AppearanceSource{}
		for key, source := range fields {
			out[id][key] = source
		}
	}
	return out
}

func occurrenceAppearanceSources(base ResolvedAppearance, style SurfaceStyle, source AppearanceSource) ResolvedAppearance {
	sources := map[string]AppearanceSource{}
	for key, value := range base.Sources {
		sources[key] = value
	}
	base.Sources = sources
	base.FaceSources = copyFaceSources(base.FaceSources)
	for key := range styleFields(style) {
		base.Sources[key] = source
		for _, fields := range base.FaceSources {
			delete(fields, key)
		}
	}
	return base
}
