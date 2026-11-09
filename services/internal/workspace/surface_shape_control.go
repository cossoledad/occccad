package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"math"
	"slices"
)

func surfaceInputKey(input GeometricInput) string {
	input.Reversed = false
	b, _ := json.Marshal(input)
	return string(b)
}
func surfaceInputIndex(inputs []GeometricInput, input GeometricInput) int {
	key := surfaceInputKey(input)
	return slices.IndexFunc(inputs, func(i GeometricInput) bool { return surfaceInputKey(i) == key })
}
func validateSurfaceControls(d *GeometricFeatureDefinition) error {
	s := d.Surface
	bad := func(code string) error { return fmt.Errorf("%w: %s", ErrValidation, code) }
	guided := d.Kind == "SURFACE_LOFT" || d.Kind == "SURFACE_SWEEP"
	if (s.Spine != nil || s.Guide != nil || len(s.Sections) > 0) && !guided {
		return bad("SURFACE_CONTROL_ROLE_INVALID")
	}
	if d.Kind == "SURFACE_SWEEP" && s.Spine != nil {
		return bad("SURFACE_SWEEP_PATH_IS_SPINE")
	}
	if s.Guide != nil && d.Kind == "SURFACE_LOFT" && s.Spine == nil {
		return bad("SURFACE_GUIDE_SPINE_REQUIRED")
	}
	if s.Guide != nil && !slices.Contains([]string{"GUIDE_FRAME", "GUIDE_BORDER"}, s.FrameMode) || s.Guide == nil && slices.Contains([]string{"GUIDE_FRAME", "GUIDE_BORDER"}, s.FrameMode) {
		return bad("SURFACE_GUIDE_MODE_REQUIRED")
	}
	if s.FrameMode == "GUIDE_BORDER" {
		for _, c := range s.Sections {
			if c.ClosingPoint != nil || len(c.Couplings) > 0 {
				return bad("SURFACE_BORDER_GUIDE_SECTION_CONTROL_UNSUPPORTED")
			}
		}
	}
	if s.Ruled && (s.Spine != nil || s.Guide != nil) {
		return bad("SURFACE_GUIDED_SMOOTH_REQUIRED")
	}
	if len(s.Boundaries) > 0 && !slices.Contains([]string{"SURFACE_FILL", "SURFACE_BOUNDARY", "SURFACE_BLEND"}, d.Kind) {
		return bad("SURFACE_CONTROL_ROLE_INVALID")
	}
	if len(s.Sections) > 32 || len(s.Boundaries) > 128 {
		return bad("SURFACE_CONTROL_BUDGET")
	}
	ids, sections, boundaries := map[string]bool{}, map[int]bool{}, map[int]bool{}
	for _, c := range s.Sections {
		i := surfaceInputIndex(d.Inputs, c.Section)
		if uuid.Validate(c.ID) != nil || ids[c.ID] || i < 0 || sections[i] || d.Kind == "SURFACE_SWEEP" && i != 0 {
			return bad("SURFACE_SECTION_CONTROL_INVALID")
		}
		ids[c.ID] = true
		sections[i] = true
		if len(c.Couplings) > 16 {
			return bad("SURFACE_COUPLING_LIMIT")
		}
		couplingIDs := map[string]bool{}
		for _, p := range c.Couplings {
			if uuid.Validate(p.ID) != nil || couplingIDs[p.ID] || p.Point == nil {
				return bad("SURFACE_COUPLING_POINT_REQUIRED")
			}
			couplingIDs[p.ID] = true
		}
		if c.StationPoint != nil && s.Spine == nil {
			return bad("SURFACE_STATION_SPINE_REQUIRED")
		}
	}
	for _, c := range s.Sections {
		if len(c.Couplings) == 0 {
			continue
		}
		if len(s.Sections) != len(d.Inputs) {
			return bad("SURFACE_COUPLING_ALL_SECTIONS_REQUIRED")
		}
		for _, other := range s.Sections {
			if len(other.Couplings) != len(c.Couplings) {
				return bad("SURFACE_COUPLING_ALL_SECTIONS_REQUIRED")
			}
			for i, p := range c.Couplings {
				if other.Couplings[i].ID != p.ID {
					return bad("SURFACE_COUPLING_ORDER_INVALID")
				}
			}
		}
	}
	if s.Spine != nil {
		for i := range d.Inputs {
			if !slices.ContainsFunc(s.Sections, func(c GeometrySectionControl) bool {
				return surfaceInputIndex(d.Inputs, c.Section) == i && c.StationPoint != nil
			}) {
				return bad("SURFACE_SECTION_STATION_REQUIRED")
			}
		}
	}
	for _, c := range s.Boundaries {
		i := surfaceInputIndex(d.Inputs, c.Boundary)
		if uuid.Validate(c.ID) != nil || ids[c.ID] || i < 0 || boundaries[i] || !slices.Contains([]string{"G0", "G1", "G2"}, c.Continuity) || math.IsNaN(c.Tension) || math.IsInf(c.Tension, 0) || c.Tension < .01 || c.Tension > 100 {
			return bad("SURFACE_BOUNDARY_CONTROL_INVALID")
		}
		ids[c.ID] = true
		boundaries[i] = true
		if c.Continuity != "G0" && c.Support == nil {
			return bad("SURFACE_SUPPORT_REQUIRED")
		}
		if c.Continuity == "G0" && c.Support != nil {
			return bad("SURFACE_G0_SUPPORT_UNEXPECTED")
		}
	}
	return nil
}
func (s *Service) compileSurfaceControls(ctx context.Context, requestID string, model PartModel, d *GeometricFeatureDefinition, spec *compiledGeometryFeature, resolve func(GeometricInput) (datumFrame, error)) error {
	if d.Surface == nil {
		return nil
	}
	surface := d.Surface
	source := func(input *GeometricInput) (*workerv1.GeometryFeatureInput, error) {
		if input == nil {
			return nil, nil
		}
		return s.geometrySource(ctx, requestID+"/surface-control", model, *input)
	}
	var err error
	if spec.Surface.Spine, err = source(surface.Spine); err != nil {
		return err
	}
	if spec.Surface.Guide, err = source(surface.Guide); err != nil {
		return err
	}
	for _, c := range surface.Sections {
		i := surfaceInputIndex(d.Inputs, c.Section)
		if i < 0 {
			return fmt.Errorf("%w: SURFACE_SECTION_CONTROL_INVALID", ErrValidation)
		}
		out := &workerv1.GeometrySectionControl{Id: c.ID, SectionIndex: uint32(i)}
		for _, p := range []struct {
			input  *GeometricInput
			output **workerv1.Vec3
		}{{c.ClosingPoint, &out.ClosingPoint}, {c.StationPoint, &out.StationPoint}} {
			if p.input != nil {
				frame, e := resolve(*p.input)
				if e != nil {
					return e
				}
				if !frame.point {
					return fmt.Errorf("%w: SURFACE_CONTROL_POINT_REQUIRED", ErrValidation)
				}
				*p.output = geometryVec(frame.origin)
			}
		}
		for _, p := range c.Couplings {
			if p.Point == nil {
				return fmt.Errorf("%w: SURFACE_COUPLING_POINT_REQUIRED", ErrValidation)
			}
			frame, e := resolve(*p.Point)
			if e != nil {
				return e
			}
			if !frame.point {
				return fmt.Errorf("%w: SURFACE_CONTROL_POINT_REQUIRED", ErrValidation)
			}
			out.Couplings = append(out.Couplings, &workerv1.GeometryCouplingPoint{Id: p.ID, Position: geometryVec(frame.origin)})
		}
		spec.Surface.Sections = append(spec.Surface.Sections, out)
	}
	for _, c := range surface.Boundaries {
		if c.Continuity == "G0" {
			continue
		}
		i := surfaceInputIndex(d.Inputs, c.Boundary)
		if i < 0 {
			return fmt.Errorf("%w: SURFACE_BOUNDARY_CONTROL_INVALID", ErrValidation)
		}
		support, e := source(c.Support)
		if e != nil {
			return e
		}
		spec.Surface.Boundaries = append(spec.Surface.Boundaries, &workerv1.GeometryBoundaryControl{Id: c.ID, BoundaryIndex: uint32(i), Support: support, Continuity: c.Continuity, Tension: c.Tension})
	}
	return nil
}
