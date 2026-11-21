package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

func (s *Service) solidAxisFrame(ctx context.Context, requestID string, model PartModel, g GeometricInput) (datumFrame, error) {
	frame, _, err := s.geometryReferenceFrame(ctx, requestID, model, g)
	if err == nil && (frame.point || frame.plane) {
		err = fmt.Errorf("%w: SOLID_AXIS_REQUIRES_LINE", ErrValidation)
	}
	return frame, err
}

func forSolidSpatialReference(f Feature, visit func(string, GeometricInput)) {
	if f.AxisReference != nil {
		visit("AXIS", *f.AxisReference)
	}
	if f.Pattern != nil {
		if f.Pattern.AxisReference != nil {
			visit("AXIS", *f.Pattern.AxisReference)
		}
		if f.Pattern.CenterGeometry != nil {
			visit("CENTER", *f.Pattern.CenterGeometry)
		}
	}
	for _, b := range []*LoftBoundaryDefinition{f.LoftStart, f.LoftEnd} {
		if b != nil && b.Direction != nil {
			visit("LOFT_DIRECTION", *b.Direction)
		}
	}
}
func validSolidPoint(p PatternPointReference) bool {
	g := p.Geometry
	if g == nil || p.SketchID != "" || p.AxisEntityID != "" || p.Reference != (SketchGeometryRef{}) {
		return false
	}
	switch g.Kind {
	case "DATUM":
		return g.Datum != nil && g.Datum.Kind == "POINT"
	case "RESULT":
		return g.FeatureID != "" && g.ResultID != ""
	case "TOPOLOGY":
		return g.Selection != nil && g.Selection.Selection.ExpectedType == modelcore.PersistentTopologyVertex
	}
	return false
}

// Evidence and capture Revision may change on re-selection of the same member.
func solidPointIdentity(p PatternPointReference) string {
	var identity any = p
	if g := p.Geometry; g != nil && g.Selection != nil {
		s := g.Selection.Selection
		identity = []any{g.Kind, g.Selection.SourceFeatureID, s.SourceDocumentID, s.SourceBodyID, s.SourceResult, s.Anchor, s.Selector}
	}
	raw, _ := json.Marshal(identity)
	return "location/" + modelcore.ValueDigest(raw)
}
func solidGeometryReferences(f Feature) []GeometricInput {
	var refs []GeometricInput
	forSolidSpatialReference(f, func(_ string, g GeometricInput) { refs = append(refs, g) })
	if f.HoleDirection != nil {
		refs = append(refs, *f.HoleDirection)
	}
	for _, p := range holePositions(f) {
		if p.Geometry != nil {
			refs = append(refs, *p.Geometry)
		}
	}
	for _, s := range f.Sections {
		if s.Point != nil && s.Point.Geometry != nil {
			refs = append(refs, *s.Point.Geometry)
		}
	}
	return refs
}
func solidReferenceDependency(g GeometricInput) modelcore.DependencyKey {
	var inputs []featureInput
	appendGeometryReferenceInput(&inputs, "", g)
	input := inputs[0]
	prefix := "feature:"
	if input.EntityKind != "FEATURE" {
		prefix = "datum:"
	}
	return modelcore.DependencyKey(prefix + input.FeatureID)
}
