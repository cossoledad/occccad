package workspace

import (
	"context"
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
)

type CurveChainReference struct {
	SketchID  string               `json:"sketchId"`
	EntityIDs []string             `json:"entityIds"`
	Direction *CurveChainDirection `json:"direction,omitempty"`
	Reversed  bool                 `json:"reversed,omitempty"`
}
type SweepDefinition struct {
	Path       CurveChainReference `json:"path"`
	FrameMode  string              `json:"frameMode"`
	CornerMode string              `json:"cornerMode"`
}

func sweepChain(model PartModel, f Feature) (Feature, sketchCurveChain, error) {
	if f.Sweep == nil {
		return Feature{}, sketchCurveChain{}, fmt.Errorf("%w: SWEEP_PATH_REQUIRED", ErrValidation)
	}
	var source Feature
	for _, s := range model.Features {
		if s.ID == f.Sweep.Path.SketchID {
			source = s
			break
		}
	}
	chains, err := sketchCurveChains(source, f.Sweep.Path.EntityIDs)
	if err != nil {
		return source, sketchCurveChain{}, err
	}
	if len(chains) != 1 {
		return source, sketchCurveChain{}, fmt.Errorf("%w: SWEEP_REQUIRES_SINGLE_CHAIN", ErrValidation)
	}
	if f.Sweep.Path.Direction != nil {
		chains, err = orientCurveChains(chains, []CurveChainDirection{*f.Sweep.Path.Direction})
		if err != nil {
			return source, sketchCurveChain{}, err
		}
	}
	c := chains[0]
	if f.Sweep.Path.Reversed {
		c = reverseCurveChain(c)
	}
	return source, c, nil
}
func resolveSweepDirection(model PartModel, f *Feature) error {
	if f.Type != "SWEEP" {
		return nil
	}
	_, chain, err := sweepChain(model, *f)
	if err != nil {
		return err
	}
	if f.Sweep.Path.Direction == nil {
		copy := *f.Sweep
		copy.Path.EntityIDs = append([]string(nil), copy.Path.EntityIDs...)
		c := chain.Curves[0]
		copy.Path.Direction = &CurveChainDirection{EntityID: c.EntityID, Reversed: c.Reversed != copy.Path.Reversed}
		f.Sweep = &copy
	}
	return nil
}
func validateSweep(f Feature, earlier map[string]Feature) error {
	if f.Type != "SWEEP" {
		if f.Sweep != nil {
			return fmt.Errorf("%w: sweep definition requires SWEEP", ErrValidation)
		}
		return nil
	}
	if f.Sweep == nil || (f.Sweep.FrameMode != "CORRECTED_FRENET" && f.Sweep.FrameMode != "FRENET") || (f.Sweep.CornerMode != "MITER" && f.Sweep.CornerMode != "G1_ONLY") {
		return fmt.Errorf("%w: SWEEP_DEFINITION_INVALID", ErrValidation)
	}
	if source, ok := earlier[f.Sweep.Path.SketchID]; !ok || source.Sketch == nil {
		return fmt.Errorf("%w: SWEEP_PATH_REQUIRES_EARLIER_SKETCH", ErrValidation)
	}
	if f.Extent != "" || f.Length != 0 || f.Length2 != 0 || f.Angle != 0 || f.Reversed || f.AxisEntityID != "" {
		return fmt.Errorf("%w: SWEEP_HAS_NO_LINEAR_EXTENT", ErrValidation)
	}
	return nil
}
func (service *Service) sweepPath(ctx context.Context, model PartModel, f Feature) (*geometry.SweepPath, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source, chain, err := sweepChain(model, f)
	if err != nil {
		return nil, err
	}
	origin, u, normal, ok := supportFrame(model, source.Sketch.Support)
	if !ok {
		return nil, fmt.Errorf("%w: SWEEP_PATH_SUPPORT_UNAVAILABLE", ErrValidation)
	}
	return &geometry.SweepPath{SketchID: source.ID, ChainID: chain.ID, Curves: chain.Curves, PlaneOrigin: origin, PlaneUDirection: u, PlaneNormal: normal, Closed: chain.Closed, FrameMode: f.Sweep.FrameMode, CornerMode: f.Sweep.CornerMode}, nil
}
