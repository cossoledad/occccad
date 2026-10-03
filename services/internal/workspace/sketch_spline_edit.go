package workspace

import (
	"fmt"
	"math"
	"sort"
)

func applySketchSplineEdit(sketch *SketchFeature, op SketchOperation) error {
	if op.OperationID == "" {
		return fmt.Errorf("%w: spline edit requires operation identity", ErrValidation)
	}
	index := -1
	for i, e := range sketch.Entities {
		if e.ID == op.EntityID {
			index = i
			break
		}
	}
	if index < 0 || sketch.Entities[index].Kind != "SPLINE" || sketch.Entities[index].Suppressed {
		return fmt.Errorf("%w: select editable local spline", ErrValidation)
	}
	prior := sketch.Entities[index]
	next := prior
	affected := map[string]bool{}
	endpointsChanged := false
	if op.Type == "CONVERT_SPLINE_TO_CONTROL" {
		if prior.Mode == "CONTROL" {
			return nil
		}
		if err := validateCanonicalSpline(prior); err != nil {
			return fmt.Errorf("%w: solve FIT spline before converting to exact CONTROL mode", ErrValidation)
		}
		next.Mode = "CONTROL"
		for _, id := range prior.ControlPointIDs {
			affected[id] = true
		}
	} else if op.Type == "SET_SPLINE_CLOSED" {
		if op.Closed == nil {
			return fmt.Errorf("%w: specify open or closed", ErrValidation)
		}
		if next.Closed == *op.Closed {
			return nil
		}
		endpointsChanged = true
		next.Closed = *op.Closed
		if next.Closed && next.Mode != "CONTROL" && len(next.ControlPoints) < 3 {
			return fmt.Errorf("%w: closed FIT spline requires at least three fit points", ErrValidation)
		}
		if next.Mode == "CONTROL" {
			if next.Closed {
				if len(next.Poles) < 2 {
					return fmt.Errorf("%w: closed spline requires poles", ErrValidation)
				}
				if next.Poles[0] != next.Poles[len(next.Poles)-1] {
					next.Poles = append(append([]SketchPoint2(nil), next.Poles...), next.Poles[0])
					if len(next.Weights) > 0 {
						next.Weights = append(append([]float64(nil), next.Weights...), next.Weights[0])
					}
					next.PoleIDs = append(append([]string(nil), next.PoleIDs...), macroID(op.OperationID, "closure/pole"))
				}
			} else if len(next.Poles) > int(next.Degree)+1 && next.Poles[0] == next.Poles[len(next.Poles)-1] {
				affected[next.PoleIDs[len(next.PoleIDs)-1]] = true
				next.Poles = append([]SketchPoint2(nil), next.Poles[:len(next.Poles)-1]...)
				if len(next.Weights) > 0 {
					next.Weights = append([]float64(nil), next.Weights[:len(next.Weights)-1]...)
				}
				next.PoleIDs = append([]string(nil), next.PoleIDs[:len(next.PoleIDs)-1]...)
			}
			uniformSplineBasis(&next)
		} else {
			clearSplineCanonical(&next)
		}
		next.StartPointID = macroID(op.OperationID, "endpoint/start")
		next.EndPointID = macroID(op.OperationID, "endpoint/end")
	} else if next.Mode == "CONTROL" {
		if op.KnotParameter == nil {
			return fmt.Errorf("%w: CONTROL editing inserts or removes a knot; arbitrary pole deletion would alter the basis", ErrValidation)
		}
		var err error
		next, affected, err = editCanonicalSplineKnot(prior, op.OperationID, *op.KnotParameter, op.PointAction)
		if err != nil {
			return err
		}
	} else {
		points := append([]SketchPoint2(nil), next.ControlPoints...)
		ids := append([]string(nil), next.ControlPointIDs...)
		at := -1
		if op.PointIndex != nil {
			at = *op.PointIndex
		}
		if op.ControlPointID != "" {
			at = -1
			for i, id := range ids {
				if id == op.ControlPointID {
					at = i
					break
				}
			}
		}
		switch op.PointAction {
		case "INSERT":
			if at < 0 || at > len(points) || op.Point == nil || !finite(op.Point.X) || !finite(op.Point.Y) || len(points) >= 512 {
				return fmt.Errorf("%w: fit insertion requires an index and finite fit point", ErrValidation)
			}
			points = append(points, SketchPoint2{})
			copy(points[at+1:], points[at:])
			points[at] = *op.Point
			ids = append(ids, "")
			copy(ids[at+1:], ids[at:])
			ids[at] = macroID(op.OperationID, "fit/insert")
			if at == 0 || at == len(points)-1 {
				endpointsChanged = true
			}
		case "DELETE":
			if at < 0 || at >= len(points) || len(points) <= 3 {
				return fmt.Errorf("%w: deletion must retain sufficient fit points", ErrValidation)
			}
			affected[ids[at]] = true
			endpointsChanged = at == 0 || at == len(points)-1
			points = append(points[:at], points[at+1:]...)
			ids = append(ids[:at], ids[at+1:]...)
		default:
			return fmt.Errorf("%w: spline point action must be INSERT or DELETE", ErrValidation)
		}
		next.ControlPoints = points
		next.ControlPointIDs = ids
		clearSplineCanonical(&next)
		if endpointsChanged {
			if points[0] != prior.ControlPoints[0] {
				next.StartPointID = macroID(op.OperationID, "endpoint/start")
			}
			if points[len(points)-1] != prior.ControlPoints[len(prior.ControlPoints)-1] {
				next.EndPointID = macroID(op.OperationID, "endpoint/end")
			}
		}
	}
	detached := map[string]bool{}
	for _, id := range op.DetachConstraintIDs {
		if id == "" || detached[id] {
			return fmt.Errorf("%w: duplicate constraint release", ErrValidation)
		}
		detached[id] = true
	}
	constraints := []SketchConstraint{}
	for _, c := range sketch.Constraints {
		invalid := false
		for _, r := range c.References {
			if r.EntityID != prior.ID {
				continue
			}
			if r.SubElement == "CONTROL" && affected[r.ControlPointID] {
				invalid = true
			}
			if endpointsChanged && (r.SubElement == "START" || r.SubElement == "END") {
				endpointID := next.StartPointID
				if r.SubElement == "END" {
					endpointID = next.EndPointID
				}
				if next.Closed || r.PointID != endpointID {
					invalid = true
				}
			}
			if c.Kind == "MIRROR" || c.Kind == "SAME_SUPPORT" {
				invalid = true
			}
		}
		if detached[c.ID] {
			if !invalid {
				return fmt.Errorf("%w: release %s is unrelated to spline edit", ErrValidation, c.ID)
			}
			delete(detached, c.ID)
			continue
		}
		if invalid {
			return fmt.Errorf("%w: spline edit invalidates constraint %s; explicitly release it", ErrValidation, c.ID)
		}
		constraints = append(constraints, c)
	}
	if len(detached) > 0 {
		return fmt.Errorf("%w: requested release does not exist", ErrValidation)
	}
	sketch.Entities[index] = next
	sketch.Constraints = constraints
	return nil
}
func clearSplineCanonical(e *SketchEntity) {
	e.Poles = nil
	e.PoleIDs = nil
	e.Knots = nil
	e.Multiplicities = nil
	e.Weights = nil
	e.ParameterStart = 0
	e.ParameterEnd = 0
	e.Periodic = false
}
func uniformSplineBasis(e *SketchEntity) {
	p := int(e.Degree)
	if p >= len(e.Poles) {
		p = len(e.Poles) - 1
		e.Degree = uint32(p)
	}
	e.Knots = []float64{0}
	e.Multiplicities = []uint32{uint32(p + 1)}
	for j := 1; j < len(e.Poles)-p; j++ {
		e.Knots = append(e.Knots, float64(j)/float64(len(e.Poles)-p))
		e.Multiplicities = append(e.Multiplicities, 1)
	}
	e.Knots = append(e.Knots, 1)
	e.Multiplicities = append(e.Multiplicities, uint32(p+1))
	if len(e.Weights) != len(e.Poles) {
		e.Weights = make([]float64, len(e.Poles))
		for i := range e.Weights {
			e.Weights[i] = 1
		}
	}
	e.ParameterStart = 0
	e.ParameterEnd = 1
	e.Periodic = false
}

type splineHomogeneous struct{ x, y, w float64 }

func splineHomogeneousPoles(e SketchEntity) []splineHomogeneous {
	r := make([]splineHomogeneous, len(e.Poles))
	for i, p := range e.Poles {
		r[i] = splineHomogeneous{p.X * e.Weights[i], p.Y * e.Weights[i], e.Weights[i]}
	}
	return r
}
func expandedSplineKnots(e SketchEntity) []float64 {
	var u []float64
	for i, k := range e.Knots {
		for j := uint32(0); j < e.Multiplicities[i]; j++ {
			u = append(u, k)
		}
	}
	return u
}
func insertHomogeneousKnot(p []splineHomogeneous, u []float64, degree int, knot float64) ([]splineHomogeneous, []float64, int, int, error) {
	k := sort.Search(len(u), func(i int) bool { return u[i] > knot }) - 1
	s := 0
	for _, v := range u {
		if v == knot {
			s++
		}
	}
	if k < degree || k >= len(p) || s >= degree {
		return nil, nil, 0, 0, fmt.Errorf("%w: interior knot multiplicity exceeds degree", ErrValidation)
	}
	q := make([]splineHomogeneous, len(p)+1)
	copy(q[:k-degree+1], p[:k-degree+1])
	copy(q[k-s+1:], p[k-s:])
	for i := k - degree + 1; i <= k-s; i++ {
		denom := u[i+degree] - u[i]
		if denom <= 0 {
			return nil, nil, 0, 0, fmt.Errorf("%w: degenerate knot span", ErrValidation)
		}
		a := (knot - u[i]) / denom
		left, right := p[i-1], p[i]
		q[i] = splineHomogeneous{(1-a)*left.x + a*right.x, (1-a)*left.y + a*right.y, (1-a)*left.w + a*right.w}
	}
	knots := append([]float64(nil), u[:k+1]...)
	knots = append(knots, knot)
	knots = append(knots, u[k+1:]...)
	return q, knots, k, s, nil
}
func editCanonicalSplineKnot(e SketchEntity, operationID string, knot float64, action string) (SketchEntity, map[string]bool, error) {
	affected := map[string]bool{}
	if err := validateCanonicalSpline(e); err != nil {
		return e, affected, err
	}
	if !finite(knot) || knot <= e.ParameterStart || knot >= e.ParameterEnd {
		return e, affected, fmt.Errorf("%w: choose an interior knot in the active spline domain", ErrValidation)
	}
	p := splineHomogeneousPoles(e)
	u := expandedSplineKnots(e)
	degree := int(e.Degree)
	var result []splineHomogeneous
	var knots []float64
	var ids []string
	if action == "INSERT" {
		if len(p) >= 512 {
			return e, affected, fmt.Errorf("%w: spline pole limit exceeded", ErrValidation)
		}
		q, v, k, s, err := insertHomogeneousKnot(p, u, degree, knot)
		if err != nil {
			return e, affected, err
		}
		result, knots = q, v
		ids = make([]string, len(q))
		copy(ids[:k-degree+1], e.PoleIDs[:k-degree+1])
		copy(ids[k-s+1:], e.PoleIDs[k-s:])
		for i := k - degree + 1; i <= k-s; i++ {
			ids[i] = macroID(operationID, fmt.Sprintf("pole/insert/%d", i))
		}
		for i := k - degree + 1; i < k-s; i++ {
			affected[e.PoleIDs[i]] = true
		}
	} else if action == "DELETE" {
		remove := -1
		for i, v := range u {
			if v == knot {
				remove = i
				break
			}
		}
		if remove < 0 || len(p) <= degree+1 {
			return e, affected, fmt.Errorf("%w: removal requires an existing removable interior knot", ErrValidation)
		}
		knots = append(append([]float64(nil), u[:remove]...), u[remove+1:]...)
		k := sort.Search(len(knots), func(i int) bool { return knots[i] > knot }) - 1
		s := 0
		for _, v := range knots {
			if v == knot {
				s++
			}
		}
		result = make([]splineHomogeneous, len(p)-1)
		ids = make([]string, len(result))
		copy(result[:k-degree+1], p[:k-degree+1])
		copy(ids[:k-degree+1], e.PoleIDs[:k-degree+1])
		copy(result[k-s:], p[k-s+1:])
		copy(ids[k-s:], e.PoleIDs[k-s+1:])
		for i := k - degree + 1; i <= k-s; i++ {
			denom := knots[i+degree] - knots[i]
			a := (knot - knots[i]) / denom
			if denom <= 0 || a <= 0 {
				return e, affected, fmt.Errorf("%w: degenerate knot removal", ErrValidation)
			}
			q, prev := p[i], result[i-1]
			candidate := splineHomogeneous{(q.x - (1-a)*prev.x) / a, (q.y - (1-a)*prev.y) / a, (q.w - (1-a)*prev.w) / a}
			if i < k-s {
				result[i] = candidate
				ids[i] = macroID(operationID, fmt.Sprintf("pole/remove/%d", i))
			}
		}
		reproduced, _, _, _, err := insertHomogeneousKnot(result, knots, degree, knot)
		if err != nil {
			return e, affected, err
		}
		for i, q := range reproduced {
			want := p[i]
			scale := math.Max(1, math.Max(math.Abs(want.x), math.Max(math.Abs(want.y), math.Abs(want.w))))
			if math.Abs(q.x-want.x) > 1e-11*scale || math.Abs(q.y-want.y) > 1e-11*scale || math.Abs(q.w-want.w) > 1e-11*scale {
				return e, affected, fmt.Errorf("%w: knot cannot be removed without changing the exact curve", ErrValidation)
			}
		}
		for i := k - degree + 1; i <= k-s; i++ {
			affected[e.PoleIDs[i]] = true
		}
	} else {
		return e, affected, fmt.Errorf("%w: knot action must be INSERT or DELETE", ErrValidation)
	}
	next := e
	next.Poles = make([]SketchPoint2, len(result))
	next.Weights = make([]float64, len(result))
	next.PoleIDs = ids
	for i, h := range result {
		if !positiveFinite(h.w) || !finite(h.x) || !finite(h.y) {
			return e, affected, fmt.Errorf("%w: knot operation produced invalid rational weight", ErrValidation)
		}
		next.Poles[i] = SketchPoint2{X: h.x / h.w, Y: h.y / h.w}
		next.Weights[i] = h.w
	}
	next.Knots = nil
	next.Multiplicities = nil
	for _, v := range knots {
		if len(next.Knots) == 0 || v != next.Knots[len(next.Knots)-1] {
			next.Knots = append(next.Knots, v)
			next.Multiplicities = append(next.Multiplicities, 1)
		} else {
			next.Multiplicities[len(next.Multiplicities)-1]++
		}
	}
	return next, affected, validateCanonicalSpline(next)
}
