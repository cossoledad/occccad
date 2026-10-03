package workspace

import (
	"context"
	"fmt"
	"math"

	"github.com/occccad/occccad/internal/geometry"
)

// Extension intersects the true support with a selected finite boundary. The
// click selects a branch; it never supplies an authoritative curve endpoint.
func (service *Service) prepareSketchExtension(ctx context.Context, sketch *SketchFeature, op SketchOperation, requestID string) (SketchOperation, error) {
	if len(op.EntityIDs) != 1 || op.OperationID == "" {
		return op, fmt.Errorf("%w: select one local curve", ErrValidation)
	}
	var source SketchEntity
	for _, e := range sketch.Entities {
		if e.ID == op.EntityIDs[0] {
			source = e
		}
	}
	if source.ID == "" {
		return op, fmt.Errorf("%w: external geometry is read only", ErrValidation)
	}
	a, b, err := sketchCurveDomain(source)
	if err != nil {
		return op, err
	}
	interval := SketchCurveInterval{Start: 0, End: 1}
	result := source
	if op.Type == "ARC_COMPLEMENT" {
		if source.Kind != "ARC" && source.Kind != "ELLIPTICAL_ARC" {
			return op, fmt.Errorf("%w: complement requires an arc", ErrValidation)
		}
		result.StartAngle = b
		result.EndAngle = a + math.Copysign(2*math.Pi, b-a)
		interval.Start = 1
		interval.End = (result.EndAngle - a) / (b - a)
	} else if op.Type == "CLOSE_CURVE" {
		switch source.Kind {
		case "ARC":
			result.Kind = "CIRCLE"
		case "ELLIPTICAL_ARC":
			result.Kind = "ELLIPSE"
		default:
			return op, fmt.Errorf("%w: close requires a circular or elliptical arc", ErrValidation)
		}
		result.StartAngle = 0
		result.EndAngle = 0
		interval = SketchCurveInterval{Start: -1, End: 2}
	} else {
		if source.Kind != "LINE" && source.Kind != "ARC" && source.Kind != "ELLIPTICAL_ARC" {
			return op, fmt.Errorf("%w: extend supports lines and analytic arcs; spline extrapolation has no defined branch", ErrValidation)
		}
		if op.FirstReference == nil || op.FirstReference.Target != "ENTITY" || op.FirstReference.EntityID != source.ID || (op.FirstReference.SubElement != "START" && op.FirstReference.SubElement != "END") || len(op.BoundaryIDs) != 1 || op.Point == nil || !finite(op.Point.X) || !finite(op.Point.Y) {
			return op, fmt.Errorf("%w: choose an endpoint, boundary and intersection branch", ErrValidation)
		}
		boundary, ok := sketchEditBoundary(sketch, op.BoundaryIDs[0])
		if !ok || boundary.ID == source.ID {
			return op, fmt.Errorf("%w: invalid extension boundary", ErrValidation)
		}
		support := source
		if source.Kind == "LINE" {
			// A finite exact line window enclosing the boundary's complete bounds is
			// sufficient for every possible intersection, including conic extrema.
			lo, hi := sketchBoundaryProjectionBounds(boundary, *source.Start, *source.End)
			lo = math.Min(lo, -1)
			hi = math.Max(hi, 2)
			support.Start = &SketchPoint2{X: source.Start.X + (source.End.X-source.Start.X)*lo, Y: source.Start.Y + (source.End.Y-source.Start.Y)*lo}
			support.End = &SketchPoint2{X: source.Start.X + (source.End.X-source.Start.X)*hi, Y: source.Start.Y + (source.End.Y-source.Start.Y)*hi}
		} else if source.Kind == "ARC" {
			support.Kind = "CIRCLE"
		} else {
			support.Kind = "ELLIPSE"
		}
		if service.worker == nil {
			return op, fmt.Errorf("%w: exact curve worker unavailable", ErrValidation)
		}
		intersections, e := service.worker.ComputeSketchCurves(ctx, requestID, "INTERSECT", []geometry.ProfileCurve{profileCurve(support, false), profileCurve(boundary, false)}, nil, nil)
		if e != nil {
			return op, fmt.Errorf("%w: %v", ErrValidation, e)
		}
		if len(intersections.Overlaps) > 0 {
			return op, fmt.Errorf("%w: overlapping supports have no unique extension target", ErrValidation)
		}
		best := math.Inf(1)
		target := 0.0
		found := false
		boundaryParameter := 0.0
		boundaryPoint := [2]float64{}
		for _, p := range intersections.Intersections {
			t := 0.0
			if source.Kind == "LINE" {
				dx, dy := source.End.X-source.Start.X, source.End.Y-source.Start.Y
				t = ((p.Point[0]-source.Start.X)*dx + (p.Point[1]-source.Start.Y)*dy) / (dx*dx + dy*dy)
			} else {
				angle := p.FirstParameter
				direction := math.Copysign(1, b-a)
				if op.FirstReference.SubElement == "END" {
					for direction*(angle-b) <= 0 {
						angle += direction * 2 * math.Pi
					}
				} else {
					for direction*(angle-a) >= 0 {
						angle -= direction * 2 * math.Pi
					}
				}
				t = (angle - a) / (b - a)
				if math.Abs((a+(b-a)*t)-a) >= 2*math.Pi || math.Abs(b-(a+(b-a)*t)) >= 2*math.Pi {
					continue
				}
			}
			if (op.FirstReference.SubElement == "START" && t >= 0) || (op.FirstReference.SubElement == "END" && t <= 1) {
				continue
			}
			distance := math.Hypot(p.Point[0]-op.Point.X, p.Point[1]-op.Point.Y)
			if distance < best {
				best = distance
				target = t
				boundaryParameter = p.SecondParameter
				boundaryPoint = p.Point
				found = true
			}
		}
		if !found {
			return op, fmt.Errorf("%w: no intersection in the selected extension direction", ErrValidation)
		}
		reference := SketchGeometryRef{Target: "ENTITY", EntityID: boundary.ID, SubElement: "WHOLE"}
		local := false
		for _, e := range sketch.Entities {
			local = local || e.ID == boundary.ID
		}
		if !local {
			reference.Target = "EXTERNAL"
		}
		kind := "POINT_ON_OBJECT"
		if sub := sketchBoundaryIntersectionEndpoint(boundary, boundaryParameter, boundaryPoint); sub != "" {
			reference.SubElement = sub
			kind = "COINCIDENT"
		}
		op.CutConnections = append(op.CutConnections, SketchCurveCutConnection{Kind: kind, Parameter: target, Reference: reference})
		if op.FirstReference.SubElement == "START" {
			interval.Start = target
		} else {
			interval.End = target
		}
		if source.Kind == "LINE" {
			result.Start = &SketchPoint2{X: source.Start.X + (source.End.X-source.Start.X)*interval.Start, Y: source.Start.Y + (source.End.Y-source.Start.Y)*interval.Start}
			result.End = &SketchPoint2{X: source.Start.X + (source.End.X-source.Start.X)*interval.End, Y: source.Start.Y + (source.End.Y-source.Start.Y)*interval.End}
		} else {
			result.StartAngle = a + (b-a)*interval.Start
			result.EndAngle = a + (b-a)*interval.End
		}
	}
	result.CreatedByOperationID = op.OperationID
	result.SourceEntityID = source.ID
	if interval.Start != 0 {
		result.StartPointID = macroID(op.OperationID, "extended/start")
	}
	if interval.End != 1 {
		result.EndPointID = macroID(op.OperationID, "extended/end")
	}
	if result.Kind == "CIRCLE" || result.Kind == "ELLIPSE" {
		result.StartPointID = ""
		result.EndPointID = ""
	}
	op.Type = "REPLACE_CURVE_INTERVALS"
	op.CurveSourceDigest = sketchCurveDigest(source)
	op.ComputedEntities = []SketchEntity{result}
	op.Intervals = []SketchCurveInterval{interval}
	return op, nil
}

func sketchEditBoundary(sketch *SketchFeature, id string) (SketchEntity, bool) {
	for _, e := range sketch.Entities {
		if e.ID == id {
			return e, true
		}
	}
	for _, e := range sketch.ExternalGeometry {
		if e.ID == id && e.Status == "CONNECTED" && e.Snapshot != nil {
			s := e.Snapshot
			return SketchEntity{ID: id, Kind: s.Kind, Start: s.Start, End: s.End, Center: s.Center, Radius: s.Radius}, true
		}
	}
	return SketchEntity{}, false
}

func sketchBoundaryProjectionBounds(e SketchEntity, a, b SketchPoint2) (float64, float64) {
	dx, dy := b.X-a.X, b.Y-a.Y
	length := dx*dx + dy*dy
	lo, hi := math.Inf(1), math.Inf(-1)
	add := func(p SketchPoint2) {
		t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / length
		lo = math.Min(lo, t)
		hi = math.Max(hi, t)
	}
	if e.Start != nil {
		add(*e.Start)
	}
	if e.End != nil {
		add(*e.End)
	}
	if e.Center != nil {
		r := math.Max(e.Radius, e.MajorRadius)
		c := ((e.Center.X-a.X)*dx + (e.Center.Y-a.Y)*dy) / length
		lo = c - r/math.Sqrt(length)
		hi = c + r/math.Sqrt(length)
	}
	for _, p := range e.Poles {
		add(p)
	}
	return lo, hi
}

// An intersection can identify a boundary endpoint only when its own curve
// parameter and geometric position agree with that endpoint. This tolerance
// validates a selected exact intersection; it does not weld nearby geometry.
func sketchBoundaryIntersectionEndpoint(e SketchEntity, parameter float64, p [2]float64) string {
	a, b, ok := entityProfileEndpoints(e)
	if !ok {
		return ""
	}
	start, end, err := sketchCurveDomain(e)
	if err != nil {
		return ""
	}
	parameter = unwrapSketchCurveParameter(e, parameter)
	scale := 1.0
	switch e.Kind {
	case "ARC":
		scale = e.Radius
	case "ELLIPTICAL_ARC":
		scale = e.MajorRadius
	case "SPLINE":
		scale = math.Max(1, math.Hypot(b.X-a.X, b.Y-a.Y)/math.Abs(end-start))
	}
	first := math.Abs(parameter-start)*scale <= profileTolerance && math.Hypot(p[0]-a.X, p[1]-a.Y) <= profileTolerance
	last := math.Abs(parameter-end)*scale <= profileTolerance && math.Hypot(p[0]-b.X, p[1]-b.Y) <= profileTolerance
	if first == last {
		return ""
	}
	if first {
		return "START"
	}
	return "END"
}
