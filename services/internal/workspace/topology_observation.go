package workspace

import (
	"math"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
)

// TopologyGeometry is measured at the requested Naming Body tip, in PART_LOCAL
// coordinates. Units are encoded in property names; directions are unit vectors.
// Absent fields carry no value. Unavailable lists applicable fields whose
// measurement is missing/invalid; inapplicable fields (e.g. plane radius) are omitted.
type TopologyGeometry struct {
	CentroidMM       *[3]float64         `json:"centroidMM,omitempty"`
	OriginMM         *[3]float64         `json:"originMM,omitempty"`
	Direction        *[3]float64         `json:"direction,omitempty"`
	XDirection       *[3]float64         `json:"xDirection,omitempty"`
	RadiusMM         *float64            `json:"radiusMM,omitempty"`
	MinorRadiusMM    *float64            `json:"minorRadiusMM,omitempty"`
	LengthMM         *float64            `json:"lengthMM,omitempty"`
	AreaMM2          *float64            `json:"areaMM2,omitempty"`
	HalfAngleRadians *float64            `json:"halfAngleRadians,omitempty"`
	MaterialSide     *int32              `json:"materialSide,omitempty"`
	Range            *TopologyCurveRange `json:"range,omitempty"`
	Unavailable      []string            `json:"unavailable,omitempty"`
}

type TopologyCurveRange struct {
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Convention string  `json:"convention"` // LINE_MM | ANGLE_RADIANS | NATIVE_CURVE_PARAMETER
}

// Geometry families emitted by the current native producer. Numeric/unknown
// OCCT families remain UNKNOWN, never inferred from semantic naming strings.
func observedGeometryType(value string) string {
	switch value {
	case "POINT", "LINE", "CIRCLE", "ELLIPSE", "BSPLINE", "BEZIER", "HYPERBOLA", "PARABOLA", "OFFSET", "OTHER_CURVE", "PLANE", "CYLINDER", "SPHERE", "CONE", "TORUS":
		return value
	default:
		return "UNKNOWN"
	}
}

func observeTopologyGeometry(e *workerv1.SelectionEvidence) TopologyGeometry {
	if e == nil {
		e = &workerv1.SelectionEvidence{}
	}
	g := TopologyGeometry{}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	vector := func(v *workerv1.Vec3) *[3]float64 {
		if v == nil || !finite(v.X) || !finite(v.Y) || !finite(v.Z) {
			return nil
		}
		return &[3]float64{v.X, v.Y, v.Z}
	}
	positive := func(v *float64) *float64 {
		if v == nil || !finite(*v) || *v <= 0 {
			return nil
		}
		return v
	}
	kind := observedGeometryType(e.GetGeometryType())
	if kind == "UNKNOWN" {
		g.Unavailable = append(g.Unavailable, "geometryType")
	}
	g.CentroidMM = vector(e.GetCentroid())
	if g.CentroidMM == nil {
		g.Unavailable = append(g.Unavailable, "centroidMM")
	}
	frame := kind == "LINE" || kind == "CIRCLE" || kind == "ELLIPSE" || kind == "PLANE" || kind == "CYLINDER" || kind == "CONE" || kind == "SPHERE" || kind == "POINT"
	if frame {
		g.OriginMM = vector(e.GetOrigin())
		if g.OriginMM == nil {
			g.Unavailable = append(g.Unavailable, "originMM")
		}
		if kind != "POINT" && kind != "SPHERE" {
			g.Direction = vector(e.GetDirection())
			if g.Direction != nil {
				d := g.Direction
				if math.Abs(d[0]*d[0]+d[1]*d[1]+d[2]*d[2]-1) > 1e-9 {
					g.Direction = nil
				}
			}
			if g.Direction == nil {
				g.Unavailable = append(g.Unavailable, "direction")
			}
		}
	} else {
		g.Unavailable = append(g.Unavailable, "analyticFrame")
	}
	if kind == "CIRCLE" || kind == "ELLIPSE" || kind == "CYLINDER" || kind == "SPHERE" || kind == "TORUS" {
		g.RadiusMM = positive(e.RadiusMm)
		if g.RadiusMM == nil {
			g.Unavailable = append(g.Unavailable, "radiusMM")
		}
	}
	if kind == "ELLIPSE" || kind == "TORUS" {
		g.MinorRadiusMM = positive(e.MinorRadiusMm)
		if g.MinorRadiusMM == nil {
			g.Unavailable = append(g.Unavailable, "minorRadiusMM")
		}
	}
	if kind == "CIRCLE" || kind == "ELLIPSE" {
		g.XDirection = vector(e.GetXDirection())
		if g.XDirection != nil {
			x := g.XDirection
			if math.Abs(x[0]*x[0]+x[1]*x[1]+x[2]*x[2]-1) > 1e-9 || g.Direction == nil {
				g.XDirection = nil
			} else {
				d := g.Direction
				if math.Abs(x[0]*d[0]+x[1]*d[1]+x[2]*d[2]) > 1e-9 {
					g.XDirection = nil
				}
			}
		}
		if g.XDirection == nil {
			g.Unavailable = append(g.Unavailable, "xDirection")
		}
	}
	measureDimension := ""
	switch kind {
	case "LINE", "CIRCLE", "ELLIPSE", "BSPLINE", "BEZIER", "HYPERBOLA", "PARABOLA", "OFFSET", "OTHER_CURVE":
		measureDimension = "LENGTH"
	case "PLANE", "CYLINDER", "CONE", "SPHERE", "TORUS":
		measureDimension = "AREA"
	}
	if measureDimension != "" {
		name, scale := "lengthMM", 1e3
		if measureDimension == "AREA" {
			name, scale = "areaMM2", 1e6
		}
		if e.GetMeasureDimension() == measureDimension && e.MeasureSi != nil && finite(*e.MeasureSi) && *e.MeasureSi >= 0 && finite(*e.MeasureSi*scale) {
			v := *e.MeasureSi * scale
			if name == "lengthMM" {
				g.LengthMM = &v
			} else {
				g.AreaMM2 = &v
			}
		} else {
			g.Unavailable = append(g.Unavailable, name)
		}
	}
	if kind == "CONE" {
		g.HalfAngleRadians = positive(e.HalfAngleRadians)
		if g.HalfAngleRadians == nil {
			g.Unavailable = append(g.Unavailable, "halfAngleRadians")
		}
	}
	if kind == "PLANE" || kind == "CYLINDER" || kind == "SPHERE" || kind == "CONE" {
		if e.MaterialSide != nil && (*e.MaterialSide == 1 || *e.MaterialSide == -1) {
			g.MaterialSide = e.MaterialSide
		} else {
			g.Unavailable = append(g.Unavailable, "materialSide")
		}
	}
	switch kind {
	case "LINE", "CIRCLE", "ELLIPSE", "BSPLINE", "BEZIER", "HYPERBOLA", "PARABOLA", "OFFSET", "OTHER_CURVE":
		if e.ParameterStart != nil && e.ParameterEnd != nil && finite(*e.ParameterStart) && finite(*e.ParameterEnd) {
			convention := "NATIVE_CURVE_PARAMETER"
			if kind == "LINE" {
				convention = "LINE_MM"
			} else if kind == "CIRCLE" || kind == "ELLIPSE" {
				convention = "ANGLE_RADIANS"
			}
			g.Range = &TopologyCurveRange{Start: *e.ParameterStart, End: *e.ParameterEnd, Convention: convention}
		} else {
			g.Unavailable = append(g.Unavailable, "range")
		}
	}
	return g
}
