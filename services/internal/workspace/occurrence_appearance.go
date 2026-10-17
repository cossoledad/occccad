package workspace

import (
	"context"
	"slices"
	"strings"
)

// These are read-time rules, never persisted identities or geometry inputs.
type occurrenceAppearanceRule struct {
	Path       string
	BodyID     string
	DocumentID string
	OwnerDepth int
	Imported   bool
	Name       *string
	Appearance *SurfaceAppearance
}

func occurrenceAppearanceRules(model ProductModel, base string, inherited []occurrenceAppearanceRule) []occurrenceAppearanceRule {
	rules := append([]occurrenceAppearanceRule(nil), inherited...)
	depth := pathDepth(base)
	if model.DefaultAppearance != nil {
		rules = append(rules, occurrenceAppearanceRule{Path: base, OwnerDepth: depth, Appearance: model.DefaultAppearance})
	}
	for _, override := range model.AppearanceOverrides {
		path := override.InstancePath.Canonical
		if base != "" {
			path = base + "/" + path
		}
		var documentID string
		if n := len(override.InstancePath.Segments); n > 0 {
			documentID = override.InstancePath.Segments[n-1].ReferencedDocumentID
		}
		rules = append(rules, occurrenceAppearanceRule{Path: path, BodyID: override.BodyID, DocumentID: documentID, OwnerDepth: depth, Name: override.Name, Appearance: override.Appearance})
	}
	return rules
}

func pathDepth(path string) int {
	if path == "" {
		return 0
	}
	return strings.Count(path, "/") + 1
}
func pathContains(parent, child string) bool {
	return parent == "" || parent == child || strings.HasPrefix(child, parent+"/")
}

func (s *Service) applyOccurrenceAppearance(ctx context.Context, documentID string, body PartBody, base ResolvedAppearance, path string, rules []occurrenceAppearanceRule) (ResolvedAppearance, error) {
	rules = append([]occurrenceAppearanceRule(nil), rules...)
	slices.SortStableFunc(rules, func(a, b occurrenceAppearanceRule) int {
		if d := pathDepth(a.Path) - pathDepth(b.Path); d != 0 {
			return d
		}
		if d := b.OwnerDepth - a.OwnerDepth; d != 0 {
			return d
		}
		if a.BodyID == "" && b.BodyID != "" {
			return -1
		}
		if a.BodyID != "" && b.BodyID == "" {
			return 1
		}
		return 0
	})
	for _, r := range rules {
		if r.Appearance == nil || !pathContains(r.Path, path) || (r.BodyID != "" && r.BodyID != body.ID) {
			continue
		}
		if r.BodyID != "" && r.DocumentID != "" && r.DocumentID != documentID {
			base.Issues = append(base.Issues, AppearanceIssue{Code: "APPEARANCE_SOURCE_REPLACED", OccurrencePath: r.Path, BodyID: r.BodyID})
			continue
		}
		base = overlayResolvedAppearance(base, appearanceStyle(r.Appearance))
		source := AppearanceSource{Kind: "INSTANCE", BodyID: r.BodyID, OccurrencePath: r.Path}
		if r.Path == "" {
			source.Kind = "ASSEMBLY_DEFAULT"
		}
		if r.Imported {
			source.Kind = "IMPORTED_INSTANCE"
		}
		base = occurrenceAppearanceSources(base, appearanceStyle(r.Appearance), source)

		if len(r.Appearance.Faces) > 0 {
			localBody := body
			localBody.Appearance = &SurfaceAppearance{Faces: r.Appearance.Faces}
			local, err := s.bodyAppearance(ctx, localBody)
			if err != nil {
				return ResolvedAppearance{}, err
			}
			// Clone projections because cached definition projections are shared.
			colors := map[uint64]string{}
			for id, c := range base.Faces {
				colors[id] = c
			}
			for id, fields := range local.FaceInherit {
				if slices.Contains(fields, "color") {
					delete(colors, id)
				}
			}
			for id, c := range local.Faces {
				colors[id] = c
			}
			base.Faces = colors
			props := map[uint64]GraphicProperties{}
			for id, p := range base.FaceProperties {
				props[id] = p
			}
			for id, fields := range local.FaceInherit {
				props[id] = withoutGraphicFields(props[id], fields)
			}
			for id, p := range local.FaceProperties {
				props[id] = overlayStyle(SurfaceStyle{GraphicProperties: props[id]}, SurfaceStyle{GraphicProperties: p}).GraphicProperties
			}
			base.FaceProperties = props
			for id, fields := range local.FaceInherit {
				for _, key := range fields {
					delete(base.FaceSources[id], key)
				}
			}
			faceSource := source
			faceSource.Kind = "INSTANCE_FACE"
			for id := range local.Faces {
				if base.FaceSources[id] == nil {
					base.FaceSources[id] = map[string]AppearanceSource{}
				}
				base.FaceSources[id]["color"] = faceSource
			}
			for id, properties := range local.FaceProperties {
				if base.FaceSources[id] == nil {
					base.FaceSources[id] = map[string]AppearanceSource{}
				}
				for key := range styleFields(SurfaceStyle{GraphicProperties: properties}) {
					base.FaceSources[id][key] = faceSource
				}
			}

			base.UnresolvedFaces += local.UnresolvedFaces
		}
	}
	return normalizeAppearanceProjection(base), nil
}

// Captured rules belong to a single read, never a DocumentView or motion snapshot.
type occurrenceRulesKey struct{}

func withOccurrenceRules(ctx context.Context) (context.Context, map[string][]occurrenceAppearanceRule) {
	if rules, ok := ctx.Value(occurrenceRulesKey{}).(map[string][]occurrenceAppearanceRule); ok {
		return ctx, rules
	}
	rules := map[string][]occurrenceAppearanceRule{}
	return context.WithValue(ctx, occurrenceRulesKey{}, rules), rules
}
