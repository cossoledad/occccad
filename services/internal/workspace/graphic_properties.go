package workspace

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// GraphicProperties are optional, independently inherited display values.
// Nil means inherit; zero opacity, black, and false are explicit values.
type GraphicProperties struct {
	Opacity         *float64 `json:"opacity,omitempty"`
	Roughness       *float64 `json:"roughness,omitempty"`
	Metalness       *float64 `json:"metalness,omitempty"`
	EdgeColor       string   `json:"edgeColor,omitempty"`
	EdgeWidth       *float64 `json:"edgeWidth,omitempty"`
	Edges           *bool    `json:"edges,omitempty"`
	Pickable        *bool    `json:"pickable,omitempty"`
	EdgeOpacityMode string   `json:"edgeOpacityMode,omitempty"`
}

type SurfaceStyle struct {
	GraphicProperties
	Color string `json:"color,omitempty"`
}

func appearanceStyle(a *SurfaceAppearance) SurfaceStyle {
	if a == nil {
		return SurfaceStyle{}
	}
	return SurfaceStyle{GraphicProperties: a.GraphicProperties, Color: a.Color}
}

// overlayStyle replaces only explicitly set fields. It never mutates a shared
// definition or multiplies opacities along an occurrence path.
func overlayStyle(base, over SurfaceStyle) SurfaceStyle {
	if over.Color != "" {
		base.Color = over.Color
	}
	b, o := reflect.ValueOf(&base.GraphicProperties).Elem(), reflect.ValueOf(over.GraphicProperties)
	for i := 0; i < o.NumField(); i++ {
		if !o.Field(i).IsZero() {
			b.Field(i).Set(o.Field(i))
		}
	}
	return base
}

func mergeLineageStyle(a, b SurfaceStyle) (SurfaceStyle, bool) {
	conflict := a.Color != "" && b.Color != "" && a.Color != b.Color
	av, bv := reflect.ValueOf(a.GraphicProperties), reflect.ValueOf(b.GraphicProperties)
	for i := 0; i < av.NumField(); i++ {
		if !av.Field(i).IsZero() && !bv.Field(i).IsZero() && !reflect.DeepEqual(av.Field(i).Interface(), bv.Field(i).Interface()) {
			conflict = true
		}
	}
	return overlayStyle(a, b), conflict
}

func patchAppearance(before *SurfaceAppearance, patch map[string]json.RawMessage, clearFaces bool) (*SurfaceAppearance, error) {
	values := map[string]json.RawMessage{}
	if before != nil {
		raw, _ := json.Marshal(before)
		_ = json.Unmarshal(raw, &values)
	}
	for key, value := range patch {
		switch key {
		case "color", "opacity", "roughness", "metalness", "edgeColor", "edgeWidth", "edges", "pickable", "edgeOpacityMode":
		default:
			return nil, fmt.Errorf("%w: unsupported graphic property %s", ErrValidation, key)
		}
		if string(value) == "null" {
			delete(values, key)
		} else {
			values[key] = value
		}
	}
	if clearFaces {
		delete(values, "faces")
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	var result SurfaceAppearance
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid graphic property: %v", ErrValidation, err)
	}
	result.Color = strings.ToLower(result.Color)
	result.EdgeColor = strings.ToLower(result.EdgeColor)
	if err = validateSurfaceStyle(appearanceStyle(&result)); err != nil {
		return nil, err
	}
	if reflect.DeepEqual(result, SurfaceAppearance{}) {
		return nil, nil
	}
	return &result, nil
}

func validateSurfaceStyle(s SurfaceStyle) error {
	for _, color := range []string{s.Color, s.EdgeColor} {
		if color != "" && !opaqueColor.MatchString(color) {
			return fmt.Errorf("%w: color must be sRGB #rrggbb", ErrValidation)
		}
	}
	for _, value := range []*float64{s.Opacity, s.Roughness, s.Metalness} {
		if value != nil && (!finite(*value) || *value < 0 || *value > 1) {
			return fmt.Errorf("%w: appearance coefficient must be between 0 and 1", ErrValidation)
		}
	}
	if s.EdgeWidth != nil && (!finite(*s.EdgeWidth) || *s.EdgeWidth < 0.5 || *s.EdgeWidth > 8) {
		return fmt.Errorf("%w: edge width must be between 0.5 and 8 pixels", ErrValidation)
	}
	if s.EdgeOpacityMode != "" && s.EdgeOpacityMode != "FOLLOW" && s.EdgeOpacityMode != "CLEAR" {
		return fmt.Errorf("%w: invalid edge opacity mode", ErrValidation)
	}
	return nil
}

func withDefaultAppearance(body PartBody, defaults *SurfaceAppearance) PartBody {
	if defaults == nil {
		return body
	}
	style := overlayStyle(appearanceStyle(defaults), appearanceStyle(body.Appearance))
	a := &SurfaceAppearance{GraphicProperties: style.GraphicProperties, Color: style.Color}
	if body.Appearance != nil {
		a.Faces = body.Appearance.Faces
	}
	body.Appearance = a
	return body
}

// A whole-occurrence override takes precedence over definition face values for
// that field only. Unrelated face values remain intact.
func overlayResolvedAppearance(base ResolvedAppearance, over SurfaceStyle) ResolvedAppearance {
	s := overlayStyle(SurfaceStyle{GraphicProperties: base.GraphicProperties, Color: base.Color}, over)
	base.GraphicProperties, base.Color = s.GraphicProperties, s.Color
	if over.Color != "" {
		base.Faces = nil
	}
	if len(base.FaceProperties) > 0 {
		faces := make(map[uint64]GraphicProperties, len(base.FaceProperties))
		for id, p := range base.FaceProperties {
			faces[id] = overlayStyle(SurfaceStyle{GraphicProperties: p}, over).GraphicProperties
		}
		base.FaceProperties = faces
	}
	return base
}

func hasExchangeStyle(a *SurfaceAppearance) bool {
	if a == nil {
		return false
	}
	if a.Color != "" || a.Opacity != nil {
		return true
	}
	for _, face := range a.Faces {
		if face.Color != "" || face.Opacity != nil || slices.Contains(face.InheritedFields, "color") || slices.Contains(face.InheritedFields, "opacity") {
			return true
		}
	}
	return false
}

func withoutGraphicFields(p GraphicProperties, fields []string) GraphicProperties {
	for _, field := range fields {
		switch field {
		case "opacity":
			p.Opacity = nil
		case "roughness":
			p.Roughness = nil
		case "metalness":
			p.Metalness = nil
		case "pickable":
			p.Pickable = nil
		}
	}
	return p
}

// Creation defaults are an explicit snapshot, never a read-time user preference.
func creationAppearance(style *SurfaceStyle) (*SurfaceAppearance, error) {
	if style == nil {
		return nil, nil
	}
	if err := validateSurfaceStyle(*style); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(style)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return patchAppearance(nil, fields, false)
}
func seedCreationAppearance(raw []byte, kind string, style *SurfaceStyle) ([]byte, error) {
	if style == nil {
		return raw, nil
	}
	if kind != "PART" {
		return nil, fmt.Errorf("%w: creation surface defaults belong to a Part", ErrValidation)
	}
	appearance, err := creationAppearance(style)
	if err != nil {
		return nil, err
	}
	var model PartModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, err
	}
	model.DefaultAppearance = appearance
	return json.Marshal(model)
}
