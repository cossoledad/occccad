package workspace

import "math"

func positiveSketchAngle(v float64) float64 { return math.Mod(math.Mod(v, 360)+360, 360) }
func sketchSolverAngle(value float64, s *SketchAngleSector) float64 {
	if s.Clockwise {
		value = -value
	}
	if s.FirstReversed {
		value += 180
	}
	if s.SecondReversed {
		value -= 180
	}
	return positiveSketchAngle(value)
}
func sketchMeasuredAngle(a, b SketchPoint2, s *SketchAngleSector) float64 {
	value := math.Atan2(a.X*b.Y-a.Y*b.X, a.X*b.X+a.Y*b.Y) * 180 / math.Pi
	if s.FirstReversed {
		value -= 180
	}
	if s.SecondReversed {
		value += 180
	}
	if s.Clockwise {
		value = -value
	}
	return positiveSketchAngle(value)
}
