package workspace

import (
	"fmt"
	"math"
)

type constraintVisual2D struct {
	Kind          string
	Positions     []SketchPoint2
	Label         string
	LabelPosition *SketchPoint2
}

func add2(a, b SketchPoint2) SketchPoint2 { return SketchPoint2{a.X + b.X, a.Y + b.Y} }
func sub2(a, b SketchPoint2) SketchPoint2 { return SketchPoint2{a.X - b.X, a.Y - b.Y} }
func scale2(a SketchPoint2, value float64) SketchPoint2 {
	return SketchPoint2{a.X * value, a.Y * value}
}
func midpoint2(a, b SketchPoint2) SketchPoint2 { return scale2(add2(a, b), 0.5) }
func normalize2(a SketchPoint2) SketchPoint2 {
	length := math.Hypot(a.X, a.Y)
	if length < 1e-9 {
		return SketchPoint2{X: 1}
	}
	return scale2(a, 1/length)
}

func constraintEntityAnchor(entity SketchEntity) (SketchPoint2, bool) {
	switch entity.Kind {
	case "POINT":
		if entity.Point != nil {
			return *entity.Point, true
		}
	case "LINE":
		if entity.Start != nil && entity.End != nil {
			return midpoint2(*entity.Start, *entity.End), true
		}
	case "CIRCLE", "ARC", "SPLINE":
		points := sampleProfileCurve(profileCurve(entity, false))
		if len(points) > 0 {
			return points[(len(points)-1)/2], true
		}
	}
	return SketchPoint2{}, false
}

func constraintReferencePoint(reference SketchGeometryRef, entities map[string]SketchEntity) (SketchPoint2, bool) {
	if reference.Target == "SKETCH_ORIGIN" {
		return SketchPoint2{}, true
	}
	entity, exists := entities[reference.EntityID]
	if !exists {
		return SketchPoint2{}, false
	}
	switch reference.SubElement {
	case "POINT":
		if entity.Point != nil {
			return *entity.Point, true
		}
	case "START", "END":
		first, last, ok := entityProfileEndpoints(entity)
		if ok && reference.SubElement == "START" {
			return first, true
		}
		if ok {
			return last, true
		}
	case "CENTER":
		if entity.Center != nil {
			return *entity.Center, true
		}
	case "WHOLE", "DIRECTION":
		return constraintEntityAnchor(entity)
	}
	return SketchPoint2{}, false
}

func constraintLine(reference SketchGeometryRef, entities map[string]SketchEntity) (SketchPoint2, SketchPoint2, bool) {
	if reference.Target == "SKETCH_X_AXIS" {
		return SketchPoint2{X: -110}, SketchPoint2{X: 110}, true
	}
	if reference.Target == "SKETCH_Y_AXIS" {
		return SketchPoint2{Y: -110}, SketchPoint2{Y: 110}, true
	}
	entity, exists := entities[reference.EntityID]
	if !exists || entity.Kind != "LINE" || entity.Start == nil || entity.End == nil {
		return SketchPoint2{}, SketchPoint2{}, false
	}
	return *entity.Start, *entity.End, true
}

func appendArrow(positions []SketchPoint2, tip, direction SketchPoint2) []SketchPoint2 {
	unit := normalize2(direction)
	normal := SketchPoint2{-unit.Y, unit.X}
	base := add2(tip, scale2(unit, 3))
	return append(positions, tip, add2(base, scale2(normal, 1.35)), tip, add2(base, scale2(normal, -1.35)))
}

func constraintValueLabel(constraint SketchConstraint) string {
	value := "?"
	if constraint.Value != nil {
		value = fmt.Sprintf("%g", math.Round(*constraint.Value*1000)/1000)
	}
	switch constraint.Kind {
	case "RADIUS":
		return "R " + value
	case "DIAMETER":
		return "Ø " + value
	case "ANGLE":
		return value + "°"
	default:
		return value
	}
}

func linearConstraintVisual(a, b SketchPoint2, label string, placement *SketchPoint2) constraintVisual2D {
	direction := normalize2(sub2(b, a))
	normal := SketchPoint2{-direction.Y, direction.X}
	offset := math.Max(8, math.Min(18, math.Hypot(b.X-a.X, b.Y-a.Y)*0.25))
	if placement != nil {
		delta := sub2(*placement, midpoint2(a, b))
		offset = delta.X*normal.X + delta.Y*normal.Y
		if math.Abs(offset) < 4 {
			if offset < 0 {
				offset = -4
			} else {
				offset = 4
			}
		}
	}
	qa, qb := add2(a, scale2(normal, offset)), add2(b, scale2(normal, offset))
	positions := []SketchPoint2{a, qa, b, qb, qa, qb}
	positions = appendArrow(positions, qa, direction)
	positions = appendArrow(positions, qb, scale2(direction, -1))
	labelPosition := add2(midpoint2(qa, qb), scale2(normal, 2.5))
	if offset < 0 {
		labelPosition = add2(midpoint2(qa, qb), scale2(normal, -2.5))
	}
	if placement != nil {
		labelPosition = *placement
	}
	return constraintVisual2D{Kind: "LINE_SEGMENTS", Positions: positions, Label: label, LabelPosition: &labelPosition}
}

func constraintLineIntersection(a1, a2, b1, b2 SketchPoint2) SketchPoint2 {
	a, b := sub2(a2, a1), sub2(b2, b1)
	denominator := a.X*b.Y - a.Y*b.X
	if math.Abs(denominator) < 1e-9 {
		return midpoint2(midpoint2(a1, a2), midpoint2(b1, b2))
	}
	delta := sub2(b1, a1)
	return add2(a1, scale2(a, (delta.X*b.Y-delta.Y*b.X)/denominator))
}

func constraintVisual(constraint SketchConstraint, entities map[string]SketchEntity) (constraintVisual2D, bool) {
	points := make([]SketchPoint2, 0, len(constraint.References))
	for _, reference := range constraint.References {
		if point, exists := constraintReferencePoint(reference, entities); exists {
			points = append(points, point)
		}
	}
	switch constraint.Kind {
	case "DISTANCE":
		if len(points) >= 2 {
			return linearConstraintVisual(points[0], points[1], constraintValueLabel(constraint), constraint.LabelPosition), true
		}
	case "LENGTH":
		if len(constraint.References) > 0 {
			if first, second, exists := constraintLine(constraint.References[0], entities); exists {
				return linearConstraintVisual(first, second, constraintValueLabel(constraint), constraint.LabelPosition), true
			}
		}
	case "RADIUS", "DIAMETER":
		if len(constraint.References) == 0 {
			break
		}
		entity, exists := entities[constraint.References[0].EntityID]
		if !exists || entity.Center == nil || (entity.Kind != "CIRCLE" && entity.Kind != "ARC") {
			break
		}
		angle := math.Pi / 4
		if entity.Kind == "ARC" {
			angle = (entity.StartAngle + entity.EndAngle) / 2
		}
		direction := SketchPoint2{math.Cos(angle), math.Sin(angle)}
		if constraint.LabelPosition != nil {
			direction = normalize2(sub2(*constraint.LabelPosition, *entity.Center))
		}
		first := add2(*entity.Center, scale2(direction, entity.Radius))
		second := *entity.Center
		if constraint.Kind == "DIAMETER" {
			second = add2(*entity.Center, scale2(direction, -entity.Radius))
		}
		positions := appendArrow([]SketchPoint2{second, first}, first, sub2(second, first))
		if constraint.Kind == "DIAMETER" {
			positions = appendArrow(positions, second, sub2(first, second))
		}
		labelPosition := add2(midpoint2(first, second), scale2(SketchPoint2{-direction.Y, direction.X}, 3))
		if constraint.LabelPosition != nil {
			positions = append(positions, first, *constraint.LabelPosition)
			labelPosition = *constraint.LabelPosition
		}
		return constraintVisual2D{Kind: "LINE_SEGMENTS", Positions: positions,
			Label: constraintValueLabel(constraint), LabelPosition: &labelPosition}, true
	case "ANGLE":
		if len(constraint.References) < 2 {
			break
		}
		a1, a2, aok := constraintLine(constraint.References[0], entities)
		b1, b2, bok := constraintLine(constraint.References[1], entities)
		if !aok || !bok {
			break
		}
		origin := constraintLineIntersection(a1, a2, b1, b2)
		a, b := normalize2(sub2(a2, a1)), normalize2(sub2(b2, b1))
		if constraint.LabelPosition != nil {
			towardLabel := normalize2(sub2(*constraint.LabelPosition, origin))
			if a.X*towardLabel.X+a.Y*towardLabel.Y < 0 {
				a = scale2(a, -1)
			}
			if b.X*towardLabel.X+b.Y*towardLabel.Y < 0 {
				b = scale2(b, -1)
			}
		}
		firstAngle, secondAngle := math.Atan2(a.Y, a.X), math.Atan2(b.Y, b.X)
		for secondAngle < firstAngle {
			secondAngle += 2 * math.Pi
		}
		if secondAngle-firstAngle > math.Pi {
			firstAngle, secondAngle = secondAngle, firstAngle+2*math.Pi
		}
		arc := make([]SketchPoint2, 13)
		radius := 14.0
		if constraint.LabelPosition != nil {
			radius = math.Max(6, math.Hypot(constraint.LabelPosition.X-origin.X, constraint.LabelPosition.Y-origin.Y)-4)
		}
		for index := range arc {
			angle := firstAngle + (secondAngle-firstAngle)*float64(index)/12
			arc[index] = add2(origin, SketchPoint2{radius * math.Cos(angle), radius * math.Sin(angle)})
		}
		positions := []SketchPoint2{origin, arc[0], origin, arc[len(arc)-1]}
		for index := 1; index < len(arc); index++ {
			positions = append(positions, arc[index-1], arc[index])
		}
		middleAngle := (firstAngle + secondAngle) / 2
		labelPosition := add2(origin, SketchPoint2{(radius + 4) * math.Cos(middleAngle), (radius + 4) * math.Sin(middleAngle)})
		if constraint.LabelPosition != nil {
			labelPosition = *constraint.LabelPosition
		}
		return constraintVisual2D{Kind: "LINE_SEGMENTS", Positions: positions,
			Label: constraintValueLabel(constraint), LabelPosition: &labelPosition}, true
	default:
		anchors := []SketchPoint2{}
		if constraint.Kind == "FIXED_POINT" && constraint.FixedPoint != nil {
			anchors = append(anchors, *constraint.FixedPoint)
		} else if constraint.Kind == "CONCENTRIC" && len(constraint.References) > 0 {
			if entity, exists := entities[constraint.References[0].EntityID]; exists && entity.Center != nil {
				anchors = append(anchors, *entity.Center)
			}
		} else if (constraint.Kind == "PARALLEL" || constraint.Kind == "EQUAL") && len(points) > 0 {
			for _, point := range points {
				anchors = append(anchors, add2(point, SketchPoint2{4, 4}))
			}
		} else if (constraint.Kind == "TANGENT" || constraint.Kind == "PERPENDICULAR") && len(points) >= 2 {
			anchors = append(anchors, add2(midpoint2(points[0], points[1]), SketchPoint2{4, 4}))
		} else if len(points) > 0 {
			point := points[0]
			if constraint.Kind != "COINCIDENT" {
				point = add2(point, SketchPoint2{4, 4})
			}
			anchors = append(anchors, point)
		}
		if len(anchors) > 0 {
			return constraintVisual2D{Kind: "POINTS", Positions: anchors}, true
		}
	}
	return constraintVisual2D{}, false
}
