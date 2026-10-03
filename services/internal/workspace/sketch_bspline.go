package workspace

import (
	"fmt"
	"math"
	"sort"
)

// The Worker exports nonperiodic, clamped canonical rational B-splines. This
// evaluates the persisted exact curve, rather than refitting display samples.
func evaluateCanonicalSpline(e SketchEntity, u float64) (SketchPoint2, error) {
	if e.Periodic {
		return SketchPoint2{}, fmt.Errorf("%w: canonical spline must be nonperiodic", ErrValidation)
	}
	degree := int(e.Degree)
	if degree < 1 || degree > 16 || len(e.Poles) <= degree || len(e.Knots) != len(e.Multiplicities) || len(e.Weights) != len(e.Poles) {
		return SketchPoint2{}, fmt.Errorf("%w: invalid canonical spline basis", ErrValidation)
	}
	knots := []float64{}
	for i, k := range e.Knots {
		if !finite(k) || (i > 0 && k <= e.Knots[i-1]) || e.Multiplicities[i] == 0 || e.Multiplicities[i] > uint32(degree+1) {
			return SketchPoint2{}, fmt.Errorf("%w: invalid canonical spline knot", ErrValidation)
		}
		for n := uint32(0); n < e.Multiplicities[i]; n++ {
			knots = append(knots, k)
		}
	}
	if len(knots) != len(e.Poles)+degree+1 {
		return SketchPoint2{}, fmt.Errorf("%w: invalid canonical spline knot count", ErrValidation)
	}
	first, last := knots[degree], knots[len(e.Poles)]
	if !finite(u) || u < first || u > last {
		return SketchPoint2{}, fmt.Errorf("%w: spline parameter outside its domain", ErrValidation)
	}
	span := sort.Search(len(knots), func(i int) bool { return knots[i] > u }) - 1
	if u == last {
		span = len(e.Poles) - 1
	}
	if span < degree || span >= len(e.Poles) {
		return SketchPoint2{}, fmt.Errorf("%w: invalid spline span", ErrValidation)
	}
	type homogeneous struct{ x, y, w float64 }
	work := make([]homogeneous, degree+1)
	for j := range work {
		i := span - degree + j
		w := e.Weights[i]
		p := e.Poles[i]
		if !positiveFinite(w) || !finite(p.X) || !finite(p.Y) {
			return SketchPoint2{}, fmt.Errorf("%w: invalid spline pole/weight", ErrValidation)
		}
		work[j] = homogeneous{p.X * w, p.Y * w, w}
	}
	for r := 1; r <= degree; r++ {
		for j := degree; j >= r; j-- {
			i := span - degree + j
			denominator := knots[i+degree-r+1] - knots[i]
			alpha := 0.0
			if denominator != 0 {
				alpha = (u - knots[i]) / denominator
			}
			a, b := work[j-1], work[j]
			work[j] = homogeneous{(1-alpha)*a.x + alpha*b.x, (1-alpha)*a.y + alpha*b.y, (1-alpha)*a.w + alpha*b.w}
		}
	}
	h := work[degree]
	if h.w <= 0 || math.IsNaN(h.w) {
		return SketchPoint2{}, fmt.Errorf("%w: invalid rational spline denominator", ErrValidation)
	}
	return SketchPoint2{X: h.x / h.w, Y: h.y / h.w}, nil
}

func validateCanonicalSpline(e SketchEntity) error {
	if e.ParameterStart >= e.ParameterEnd || !finite(e.ParameterStart) || !finite(e.ParameterEnd) {
		return fmt.Errorf("%w: invalid canonical spline domain", ErrValidation)
	}
	if _, err := evaluateCanonicalSpline(e, e.ParameterStart); err != nil {
		return err
	}
	_, err := evaluateCanonicalSpline(e, e.ParameterEnd)
	return err
}
