package workspace

import (
	"fmt"
	"math"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func validateHole(f Feature, earlier map[string]Feature) error {
	if k := holeKind(f); k != "SIMPLE" && k != "COUNTERBORE" && k != "COUNTERSINK" {
		return fmt.Errorf("%w: HOLE_KIND_INVALID", ErrValidation)
	}
	if err := validateTerminationPlane(f, earlier); err != nil {
		return err
	}
	p := f.HolePoint
	if f.Sketch != nil || f.ProfileMemberSlot != nil || f.KeepTools || f.Operation != "REMOVE" || f.Profile != "" || f.ProfileSelection != nil || f.NeutralPlane != nil || f.NeutralPlaneID != "" || f.AxisEntityID != "" || len(f.Tools) > 0 || len(f.Selections) > 0 || len(f.Sections) > 0 || f.Pattern != nil || (f.Extent != "BLIND" && f.Extent != "THROUGH_ALL" && f.Extent != "TO_PLANE") {
		return fmt.Errorf("%w: HOLE_DEFINITION_INVALID", ErrValidation)
	}
	if p == nil || p.AxisEntityID != "" || p.Reference.Target != "ENTITY" || p.Reference.SubElement != "POINT" || p.Reference.EntityID == "" || p.Reference.PointID != "" || p.Reference.ControlPointIndex != nil || p.Reference.ControlPointID != "" {
		return fmt.Errorf("%w: HOLE_EXPLICIT_SKETCH_POINT_REQUIRED", ErrValidation)
	}
	source, ok := earlier[p.SketchID]
	if !ok || source.Type != "SKETCH" || source.Sketch == nil {
		return fmt.Errorf("%w: HOLE_POINT_MUST_BE_UPSTREAM_SKETCH", ErrValidation)
	}
	target := false
	for _, prior := range earlier {
		if prior.BodyID == f.BodyID && isBodyFeature(prior.Type) {
			target = true
		}
	}
	if !target {
		return fmt.Errorf("%w: HOLE_REQUIRES_TARGET_BODY", ErrValidation)
	}
	return nil
}

// Structural identity remains valid after point deletion. Resolution is separate
// so a legal sketch edit can leave an inspectable, explicitly repairable Hole.
func holePointState(model PartModel, f Feature) FeatureInputResolution {
	missing := FeatureInputResolution{Status: modelcore.SelectionMissing, DiagnosticCode: "HOLE_POINT_MISSING", Diagnostic: "定位草图点已失效，请明确重新选择"}
	if f.HolePoint == nil {
		return missing
	}
	for _, sketch := range model.Features {
		if sketch.ID != f.HolePoint.SketchID || sketch.Sketch == nil || sketch.Suppressed {
			continue
		}
		if sketch.Sketch.Support.Status == "FAILED_SUPPORT" || sketch.Sketch.Support.Status == "BLOCKED" {
			return FeatureInputResolution{Status: modelcore.SelectionSourceUnavailable, DiagnosticCode: "HOLE_SUPPORT_UNAVAILABLE", Diagnostic: "请先修复定位草图支撑"}
		}
		for _, e := range sketch.Sketch.Entities {
			if e.ID == f.HolePoint.Reference.EntityID {
				if e.Suppressed {
					return missing
				}
				if e.Kind != "POINT" || e.Point == nil || e.SourceEntityID != "" {
					return FeatureInputResolution{Status: modelcore.SelectionTypeMismatch, DiagnosticCode: "HOLE_POINT_TYPE_MISMATCH", Diagnostic: "孔位必须引用普通草图的显式点"}
				}
				return FeatureInputResolution{Status: modelcore.SelectionResolved}
			}
		}
	}
	return missing
}

func holeInput(model PartModel, f Feature) (geometry.ProfilePad, error) {
	state := holePointState(model, f)
	if state.Status != modelcore.SelectionResolved {
		return geometry.ProfilePad{}, fmt.Errorf("%w: %s: %s", ErrValidation, state.DiagnosticCode, state.Diagnostic)
	}
	origin, err := patternReferencePoint(model, *f.HolePoint)
	if err != nil {
		return geometry.ProfilePad{}, err
	}
	var source Feature
	for _, s := range model.Features {
		if s.ID == f.HolePoint.SketchID {
			source = s
			break
		}
	}
	_, u, n, ok := supportFrame(model, source.Sketch.Support)
	if !ok {
		return geometry.ProfilePad{}, fmt.Errorf("%w: HOLE_SUPPORT_UNAVAILABLE", ErrValidation)
	}
	id := f.HolePoint.Reference.EntityID
	return geometry.ProfilePad{FeatureID: f.ID, BodyID: f.BodyID, Generator: "HOLE", HoleKind: holeKind(f), RecessDiameter: f.RecessDiameter, RecessDepth: f.RecessDepth, CountersinkAngle: f.CountersinkAngle * math.Pi / 180, BodyOperation: "REMOVE", ProfileFeatureID: source.ID, Extent: f.Extent, Length: f.Depth, Reversed: f.Reversed, PlaneOrigin: origin, PlaneNormal: n, PlaneUDirection: u, Regions: []geometry.ProfileRegion{{ID: "hole/" + source.ID + "/" + id, Outer: geometry.ProfileLoop{ID: "hole-loop/" + id, Curves: []geometry.ProfileCurve{{EntityID: id, Kind: "CIRCLE", Radius: f.Diameter / 2}}}}}}, nil
}

func holeKind(f Feature) string {
	if f.HoleKind == "" {
		return "SIMPLE"
	}
	return f.HoleKind
}

type holeParameter struct {
	slot, label, unit string
	value             float64
	dimension         modelcore.Dimension
}

func holeParameters(f Feature) []holeParameter {
	result := []holeParameter{{"diameter", "Diameter", "mm", f.Diameter, modelcore.LengthDimension}}
	if f.Extent == "BLIND" {
		result = append(result, holeParameter{"depth", "Depth", "mm", f.Depth, modelcore.LengthDimension})
	}
	if holeKind(f) != "SIMPLE" {
		result = append(result, holeParameter{"recessDiameter", "Recess diameter", "mm", f.RecessDiameter, modelcore.LengthDimension})
	}
	if holeKind(f) == "COUNTERBORE" {
		result = append(result, holeParameter{"recessDepth", "Recess depth", "mm", f.RecessDepth, modelcore.LengthDimension})
	}
	if holeKind(f) == "COUNTERSINK" {
		result = append(result, holeParameter{"countersinkAngle", "Countersink angle", "deg", f.CountersinkAngle, modelcore.AngleDimension})
	}
	return result
}
func validateHoleDimensions(f Feature) error {
	if !positiveFinite(f.Diameter) {
		return fmt.Errorf("%w: HOLE_DIAMETER_INVALID", ErrValidation)
	}
	if f.Extent == "BLIND" && !positiveFinite(f.Depth) {
		return fmt.Errorf("%w: HOLE_DEPTH_INVALID", ErrValidation)
	}
	if holeKind(f) == "SIMPLE" {
		return nil
	}
	if !positiveFinite(f.RecessDiameter) || f.RecessDiameter <= f.Diameter {
		return fmt.Errorf("%w: HOLE_RECESS_DIAMETER_INVALID", ErrValidation)
	}
	depth := f.RecessDepth
	if holeKind(f) == "COUNTERBORE" {
		if !positiveFinite(depth) {
			return fmt.Errorf("%w: HOLE_RECESS_DEPTH_INVALID", ErrValidation)
		}
	} else {
		if !positiveFinite(f.CountersinkAngle) || f.CountersinkAngle >= 180 {
			return fmt.Errorf("%w: HOLE_COUNTERSINK_ANGLE_INVALID", ErrValidation)
		}
		depth = (f.RecessDiameter - f.Diameter) / (2 * math.Tan(f.CountersinkAngle*math.Pi/360))
	}
	if !positiveFinite(depth) || f.Extent == "BLIND" && depth >= f.Depth {
		return fmt.Errorf("%w: HOLE_RECESS_EXCEEDS_DEPTH", ErrValidation)
	}
	return nil
}
