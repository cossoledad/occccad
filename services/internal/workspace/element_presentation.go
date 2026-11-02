package workspace

import (
	"context"
	"strings"
)

// ElementPresentation is a readable projection of stable model identities.
// Paths, translated labels and aliases are never used to bind or repair links.
type ElementPresentation struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	BodyID    string `json:"bodyId,omitempty"`
	FeatureID string `json:"featureId,omitempty"`
}

func partElementPresentation(model PartModel, target PublicationTarget) ElementPresentation {
	p := ElementPresentation{BodyID: target.BodyID, FeatureID: target.FeatureID}
	if s := target.PersistentSelection; s != nil {
		p.BodyID = s.SourceBodyID
		p.FeatureID = s.Anchor.FeatureID
		p.Name = map[string]string{"FACE": "面", "EDGE": "边线", "VERTEX": "顶点"}[string(s.ExpectedType)]
	}
	for _, f := range model.Features {
		if f.ID == p.FeatureID {
			if p.BodyID == "" {
				p.BodyID = f.BodyID
			}
			if p.Name == "" {
				p.Name = f.Name
			}
			break
		}
	}
	if target.Kind == "SKETCH_ENTITY" {
		p.FeatureID = target.SketchID
		p.Name = map[string]string{"POINT": "点", "LINE": "直线", "CIRCLE": "圆", "ARC": "圆弧"}[target.EntityKind]
	}
	if target.Kind == "DATUM" {
		for _, v := range model.DatumPlanes {
			if v.ID == target.DatumID {
				p.Name = v.Name
			}
		}
		for _, v := range model.DatumAxes {
			if v.ID == target.DatumID {
				p.Name = v.Name
			}
		}
		for _, v := range model.DatumPoints {
			if v.ID == target.DatumID {
				p.Name = v.Name
			}
		}
		for _, v := range model.AxisSystems {
			if v.ID == target.DatumID {
				p.Name = v.Name
				if target.Axis != "" {
					p.Name += " / " + target.Axis
				}
			}
		}
	}
	if target.Kind == "PARAMETER" {
		for _, v := range model.Parameters {
			if v.ParameterID == target.ParameterID {
				p.Name = v.Key
			}
		}
	}
	parts := []string{}
	for _, b := range model.Bodies {
		if b.ID == p.BodyID {
			parts = append(parts, b.Name)
			if p.Name == "" {
				p.Name = b.Name
			}
		}
	}
	for _, f := range model.Features {
		if f.ID == p.FeatureID && f.Name != "" {
			parts = append(parts, f.Name)
		}
	}
	if p.Name == "" {
		p.Name = "来源元素"
	}
	if len(parts) == 0 || parts[len(parts)-1] != p.Name {
		parts = append(parts, p.Name)
	}
	p.Path = strings.Join(parts, " / ")
	return p
}

// Rebuild display locators from the accepted immutable Body artifact. Existing
// exact-only analysis results must not be mistaken for the displayed topology.
func (s *Service) projectFrozenPublications(ctx context.Context, documentID, revisionID string, model *PartModel) error {
	needs := false
	for _, p := range model.Publications {
		if p.Target.Kind == "TOPOLOGY" || p.Target.Kind == "BODY_RESULT" {
			bodyID := p.Target.BodyID
			if p.Target.PersistentSelection != nil {
				bodyID = p.Target.PersistentSelection.SourceBodyID
			}
			for _, b := range model.Bodies {
				if b.ID == bodyID && b.GeometryKey != "" && b.GeometryKey != p.Resolution.GeometryKey {
					needs = true
				}
			}
		}
	}
	if !needs {
		return nil
	}
	projected, err := assemblyRead(ctx, assemblyReadKey{"publication-display", documentID, revisionID}, func() ([]Publication, error) {
		copy := clonePartModel(*model)
		copy.Publications = nil
		for _, p := range model.Publications {
			if p.Target.Kind == "TOPOLOGY" || p.Target.Kind == "BODY_RESULT" {
				copy.Publications = append(copy.Publications, p)
			}
		}
		err := s.resolvePartPublications(ctx, documentID, "display/"+revisionID, revisionID, &copy, true)
		return copy.Publications, err
	})
	if err != nil {
		return err
	}
	for i := range model.Publications {
		for _, p := range projected {
			if model.Publications[i].ID == p.ID {
				model.Publications[i].Resolution = p.Resolution
				break
			}
		}
	}
	return nil
}
