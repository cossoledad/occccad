package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

// References remain design inputs. Only their frozen frames cross the Worker
// boundary; changing a support never turns it into an independent vector slot.
func geometryReferences(d *GeometricFeatureDefinition) []GeometricInput {
	if d == nil {
		return nil
	}
	var out []GeometricInput
	for _, ref := range []*GeometricInput{d.Direction, d.Axis, d.MirrorPlane, d.KeepPoint} {
		if ref != nil {
			out = append(out, *ref)
		}
	}
	if d.Curve != nil && d.Curve.Support != nil {
		out = append(out, *d.Curve.Support)
	}
	return out
}

func validateGeometryReference(input GeometricInput, earlier map[string]Feature, model PartModel) error {
	bad := func() error { return fmt.Errorf("%w: GEOMETRY_REFERENCE_INVALID", ErrValidation) }
	if input.Kind == "DATUM" || input.Kind == "POINT" {
		if input.FeatureID != "" || input.ResultID != "" || input.PlaneID != "" || input.Selection != nil || len(input.EntityIDs) != 0 {
			return bad()
		}
		if input.Kind == "POINT" {
			if input.Point == nil || input.Datum != nil || input.Point.SketchID == "" || input.Point.AxisEntityID != "" || input.Point.Reference.ControlPointIndex != nil {
				return bad()
			}
			sketch, ok := earlier[input.Point.SketchID]
			if !ok || sketch.Sketch == nil || sketch.Suppressed {
				return bad()
			}
			if input.Point.Reference.Target != "SKETCH_ORIGIN" {
				entity, exists := patternSketchReferenceEntities(*sketch.Sketch)[input.Point.Reference.EntityID]
				if !exists || entity.Suppressed {
					return bad()
				}
			}
			_, err := patternReferencePoint(model, *input.Point)
			return err
		}
		if input.Datum == nil || input.Point != nil {
			return bad()
		}
		ref := *input.Datum
		if err := validateDatumReference(ref); err != nil {
			return err
		}
		switch ref.Kind {
		case "AXIS":
			if !slices.ContainsFunc(model.DatumAxes, func(d DatumAxis) bool { return d.ID == ref.EntityID }) {
				return bad()
			}
		case "POINT":
			if !slices.ContainsFunc(model.DatumPoints, func(d DatumPoint) bool { return d.ID == ref.EntityID }) && !slices.ContainsFunc(model.AxisSystems, func(d AxisSystem) bool { return d.ID == ref.EntityID }) {
				return bad()
			}
		case "AXIS_SYSTEM":
			if !slices.ContainsFunc(model.AxisSystems, func(d AxisSystem) bool { return d.ID == ref.EntityID }) {
				return bad()
			}
		case "SKETCH_LINE":
			f, ok := earlier[ref.FeatureID]
			if !ok || f.Sketch == nil {
				return bad()
			}
			entity, exists := patternSketchReferenceEntities(*f.Sketch)[ref.EntityID]
			if !exists || entity.Kind != "LINE" || entity.Suppressed {
				return bad()
			}
		default:
			return bad()
		}
		return nil
	}
	if input.Datum != nil || input.Point != nil {
		return bad()
	}
	if input.Kind != "RESULT" && input.Kind != "TOPOLOGY" && input.Kind != "PLANE" {
		return bad()
	}
	f := Feature{Type: "GEOMETRY", Geometry: &GeometricFeatureDefinition{Kind: "EXTRACT", Inputs: []GeometricInput{input}}}
	return validateGeometricFeature(f, earlier, model)
}

type compiledGeometryFeature struct {
	*workerv1.GeometryFeatureSpec
	References []any
}

type geometryReferenceSnapshot struct {
	Reference            GeometricInput
	Origin, Direction, U [3]float64
	Point, Plane         bool
	Source               *workerv1.GeometryFeatureInput
}

func (s *Service) geometryReferenceFrame(ctx context.Context, requestID string, model PartModel, input GeometricInput) (datumFrame, geometryReferenceSnapshot, error) {
	var frame datumFrame
	snapshot := geometryReferenceSnapshot{Reference: input}
	bad := func() (datumFrame, geometryReferenceSnapshot, error) {
		return frame, snapshot, fmt.Errorf("%w: GEOMETRY_REFERENCE_UNAVAILABLE", ErrValidation)
	}
	if input.Kind == "PLANE" {
		for _, p := range model.DatumPlanes {
			if p.ID == input.PlaneID {
				frame = datumFrame{origin: p.Origin, direction: p.Normal, u: p.UDirection, plane: true}
				break
			}
		}
	} else if input.Kind == "POINT" && input.Point != nil {
		position, err := patternReferencePoint(model, *input.Point)
		if err != nil {
			return frame, snapshot, err
		}
		frame = datumFrame{origin: position, point: true}
	} else if input.Kind == "DATUM" && input.Datum != nil {
		ref := *input.Datum
		switch ref.Kind {
		case "POINT":
			for _, p := range model.DatumPoints {
				if p.ID == ref.EntityID {
					frame = datumFrame{origin: p.Position, point: true}
				}
			}
			for _, p := range model.AxisSystems {
				if p.ID == ref.EntityID {
					frame = datumFrame{origin: p.Origin, point: true}
				}
			}
		case "AXIS":
			for _, p := range model.DatumAxes {
				if p.ID == ref.EntityID {
					frame = datumFrame{origin: p.Origin, direction: p.Direction}
				}
			}
		case "AXIS_SYSTEM":
			for _, p := range model.AxisSystems {
				if p.ID == ref.EntityID {
					direction := p.ZDirection
					if ref.Axis == "X" {
						direction = p.XDirection
					}
					if ref.Axis == "Y" {
						direction = p.YDirection
					}
					frame = datumFrame{origin: p.Origin, direction: direction}
				}
			}
		case "SKETCH_LINE":
			for _, f := range model.Features {
				if f.ID != ref.FeatureID || f.Sketch == nil || f.Suppressed {
					continue
				}
				for _, e := range patternSketchReferenceEntities(*f.Sketch) {
					if e.ID != ref.EntityID || e.Kind != "LINE" || e.Suppressed || e.Start == nil || e.End == nil {
						continue
					}
					o, u, n, ok := supportFrame(model, f.Sketch.Support)
					if !ok {
						return bad()
					}
					v := geometryCross(n, u)
					frame.origin = o
					for j := range 3 {
						frame.origin[j] += u[j]*e.Start.X + v[j]*e.Start.Y
						frame.direction[j] = u[j]*(e.End.X-e.Start.X) + v[j]*(e.End.Y-e.Start.Y)
					}
				}
			}
		default:
			return bad()
		}
	} else if input.Kind == "RESULT" || input.Kind == "TOPOLOGY" {
		source, err := s.geometrySource(ctx, requestID+"/reference", model, input)
		if err != nil {
			return frame, snapshot, err
		}
		snapshot.Source = source
		body := ""
		if input.Selection != nil {
			body = input.Selection.Selection.SourceBodyID
		}
		key, err := s.featureStageInModel(ctx, model, "", geometryInputFeatureID(input), body)
		if err != nil {
			return frame, snapshot, err
		}
		manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
		if err != nil {
			return frame, snapshot, err
		}
		artifact, err := s.loadArtifact(ctx, key)
		if err != nil {
			return frame, snapshot, err
		}
		wanted := modelcore.PersistentTopologyEdge
		if artifact.GeometryDomain == "SHEET_SET" {
			wanted = modelcore.PersistentTopologyFace
		}
		if artifact.GeometryDomain == "POINT_SET" {
			wanted = modelcore.PersistentTopologyVertex
		}
		var selected *workerv1.SemanticTopologyOutput
		for _, entry := range manifest.Locators {
			o := entry.Output
			if len(source.Members) > 0 && !sameSemanticRef(semanticRef(o.SemanticRef), semanticRef(source.Members[0])) {
				continue
			}
			if len(source.Members) == 0 && topologyType(o.TopologyType) != wanted {
				continue
			}
			if selected != nil {
				return frame, snapshot, fmt.Errorf("%w: GEOMETRY_REFERENCE_AMBIGUOUS", ErrValidation)
			}
			selected = o
		}
		if selected == nil {
			return bad()
		}
		e := selectionEvidence(selected.Evidence)
		frame.origin = e.Origin
		switch {
		case topologyType(selected.TopologyType) == modelcore.PersistentTopologyVertex:
			frame.point = true
		case e.GeometryType == "LINE" && topologyType(selected.TopologyType) == modelcore.PersistentTopologyEdge:
			frame.direction = e.Direction
		case e.GeometryType == "PLANE" && topologyType(selected.TopologyType) == modelcore.PersistentTopologyFace:
			frame.direction = e.Direction
			frame.u = e.XDirection
			frame.plane = true
		default:
			return bad()
		}
		if frame.plane {
			props, err := s.getTopologyElementPropertiesFromArtifact(ctx, key, "FACE", selected.LocalId)
			if err != nil {
				return frame, snapshot, err
			}
			if normal, ok := props.Properties["normal"].([3]float64); ok {
				frame.direction = normal
			}
		}
	} else {
		return bad()
	}
	if !frame.point {
		var ok bool
		frame.direction, ok = normalize3(frame.direction)
		if !ok {
			return bad()
		}
		if input.Reversed {
			for i := range frame.direction {
				frame.direction[i] *= -1
			}
		}
		if frame.plane {
			frame.u, ok = stableSupportX(frame.direction, frame.u)
			if !ok {
				return bad()
			}
		}
	}
	snapshot.Origin, snapshot.Direction, snapshot.U = frame.origin, frame.direction, frame.u
	snapshot.Point, snapshot.Plane = frame.point, frame.plane
	return frame, snapshot, nil
}

func geometryReferenceSource(ref GeometricInput) string {
	encoded, _ := json.Marshal(ref)
	return modelcore.ValueDigest(encoded)
}

func appendGeometryReferenceInput(out *[]featureInput, role string, input GeometricInput) {
	if input.Kind == "PLANE" {
		*out = append(*out, featureInput{Role: role, EntityKind: "PLANE", FeatureID: input.PlaneID})
		return
	}
	if input.Datum != nil {
		ref := input.Datum
		if ref.Kind == "SKETCH_LINE" {
			*out = append(*out, featureInput{Role: role, EntityKind: "FEATURE", FeatureID: ref.FeatureID, EntityID: ref.EntityID})
		} else {
			*out = append(*out, featureInput{Role: role, EntityKind: ref.Kind, FeatureID: ref.EntityID})
		}
		return
	}
	*out = append(*out, featureInput{Role: role, EntityKind: "FEATURE", FeatureID: geometryInputFeatureID(input)})
}
