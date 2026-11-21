package workspace

import (
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
	"math"
)

// ISO 273 medium-series clearance diameters in millimetres.
var metricClearanceDiameters = map[string]float64{
	"ISO273_MEDIUM_M3": 3.4, "ISO273_MEDIUM_M4": 4.5,
	"ISO273_MEDIUM_M5": 5.5, "ISO273_MEDIUM_M6": 6.6,
	"ISO273_MEDIUM_M8": 9, "ISO273_MEDIUM_M10": 11,
}

type HoleThreadDefinition struct {
	Mode          string  `json:"mode"` // ANNOTATION: engineering intent; never a helical BREP.
	Designation   string  `json:"designation"`
	Hand          string  `json:"hand"` // RIGHT | LEFT
	MajorDiameter float64 `json:"majorDiameter"`
	Pitch         float64 `json:"pitch"`
	Depth         float64 `json:"depth"` // axial annotation extent, from the hole entrance
}

func holePositions(f Feature) []PatternPointReference {
	if f.HolePoint != nil {
		return []PatternPointReference{*f.HolePoint}
	}
	return f.HolePoints
}
func validateHolePositionSet(f Feature) error {
	if f.HolePoint != nil && len(f.HolePoints) > 0 || len(holePositions(f)) == 0 || len(holePositions(f)) > 128 {
		return fmt.Errorf("%w: HOLE_POSITION_SET_INVALID", ErrValidation)
	}
	seen := map[[4]string]bool{}
	for _, p := range holePositions(f) {
		key := [4]string{p.SketchID, p.Reference.EntityID, p.Reference.SubElement, p.Reference.PointID}
		if seen[key] {
			return fmt.Errorf("%w: HOLE_POSITION_DUPLICATE", ErrValidation)
		}
		seen[key] = true
	}
	return nil
}
func validateHoleEngineering(f Feature) error {
	if f.DrillTipAngle != 0 {
		if f.Extent != "BLIND" || !positiveFinite(f.DrillTipAngle) || f.DrillTipAngle >= 180 {
			return fmt.Errorf("%w: HOLE_DRILL_TIP_INVALID", ErrValidation)
		}
		if f.DepthConvention != "CYLINDER" && f.DepthConvention != "TIP" {
			return fmt.Errorf("%w: HOLE_DEPTH_CONVENTION_REQUIRED", ErrValidation)
		}
	} else if f.DepthConvention != "" {
		return fmt.Errorf("%w: HOLE_DEPTH_CONVENTION_REQUIRES_TIP", ErrValidation)
	}
	if t := f.Thread; t != nil {
		if t.Mode != "ANNOTATION" {
			return fmt.Errorf("%w: HOLE_MODELED_THREAD_UNSUPPORTED", ErrValidation)
		}
		if len(t.Designation) == 0 || len(t.Designation) > 128 || (t.Hand != "RIGHT" && t.Hand != "LEFT") {
			return fmt.Errorf("%w: HOLE_THREAD_ANNOTATION_INVALID", ErrValidation)
		}
	}
	return nil
}
func holeCylinderDepth(f Feature) float64 {
	if f.DrillTipAngle != 0 && f.DepthConvention == "TIP" {
		return f.Depth - f.Diameter/(2*math.Tan(f.DrillTipAngle*math.Pi/360))
	}
	return f.Depth
}
func holeGroupInput(model PartModel, f Feature) (geometry.ProfilePad, error) {
	var result geometry.ProfilePad
	for i, p := range holePositions(f) {
		single := f
		single.HolePoint = &p
		single.HolePoints = nil
		spec, err := singleHoleInput(model, single)
		if err != nil {
			return result, err
		}
		if i == 0 {
			result = spec
		}
		if len(holePositions(f)) > 1 {
			result.HolePositions = append(result.HolePositions, geometry.HolePosition{ProfileFeatureID: spec.ProfileFeatureID, Region: spec.Regions[0], Origin: spec.PlaneOrigin, Normal: spec.PlaneNormal, UDirection: spec.PlaneUDirection})
		}
	}
	result.DrillTipAngle = f.DrillTipAngle * math.Pi / 180
	result.DepthConvention = f.DepthConvention
	return result, nil
}
