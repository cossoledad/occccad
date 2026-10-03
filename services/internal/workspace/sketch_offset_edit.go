package workspace

import (
	"fmt"
	"math"
	"sort"
)

// OFFSET_ENTITIES creates independent exact curves. Positive distance follows
// the directed chain's left normal; a stand-alone circle is counterclockwise.
type offsetEdge struct {
	source   SketchEntity
	reversed bool
}

func offsetReverse(e SketchEntity) SketchEntity {
	if e.Kind == "LINE" {
		e.Start, e.End = e.End, e.Start
	} else if e.Kind == "ARC" {
		e.StartAngle, e.EndAngle = e.EndAngle, e.StartAngle
	}
	return e
}
func offsetTangent(e SketchEntity, end bool) SketchPoint2 {
	if e.Kind == "LINE" {
		d := sub2(*e.End, *e.Start)
		return scale2(d, 1/math.Hypot(d.X, d.Y))
	}
	a := e.StartAngle
	if end {
		a = e.EndAngle
	}
	s := 1.0
	if e.EndAngle < e.StartAngle {
		s = -1
	}
	return SketchPoint2{-s * math.Sin(a), s * math.Cos(a)}
}
func offsetMoveEndpoint(e *SketchEntity, sub string, p SketchPoint2) bool {
	if e.Kind == "LINE" {
		direction := sub2(*e.End, *e.Start)
		if sub == "START" {
			if cornerDot(sub2(*e.End, p), direction) <= 0 {
				return false
			}
			e.Start = &p
		} else {
			if cornerDot(sub2(p, *e.Start), direction) <= 0 {
				return false
			}
			e.End = &p
		}
		return cornerDistance(*e.Start, *e.End) > profileTolerance
	}
	oldStart, oldEnd := e.StartAngle, e.EndAngle
	u := math.Atan2(p.Y-e.Center.Y, p.X-e.Center.X)
	target := oldStart
	if sub == "END" {
		target = oldEnd
	}
	u += math.Round((target-u)/(2*math.Pi)) * 2 * math.Pi
	if sub == "START" {
		e.StartAngle = u
	} else {
		e.EndAngle = u
	}
	sweep := e.EndAngle - e.StartAngle
	return sweep*(oldEnd-oldStart) > 0 && math.Abs(sweep) < 2*math.Pi && e.Radius*math.Abs(sweep) > profileTolerance
}
func offsetSupportHits(a, b SketchEntity) []SketchPoint2 {
	if a.Kind == "LINE" && b.Kind == "LINE" {
		if p, ok := cornerLineIntersection(*a.Start, sub2(*a.End, *a.Start), *b.Start, sub2(*b.End, *b.Start)); ok {
			return []SketchPoint2{p}
		}
		return nil
	}
	if a.Kind == "LINE" {
		return cornerLineCircle(*a.Start, sub2(*a.End, *a.Start), *b.Center, b.Radius)
	}
	if b.Kind == "LINE" {
		return cornerLineCircle(*b.Start, sub2(*b.End, *b.Start), *a.Center, a.Radius)
	}
	return cornerCircleCircle(*a.Center, a.Radius, *b.Center, b.Radius)
}
func offsetBoundedContains(e SketchEntity, p SketchPoint2) bool {
	if e.Kind == "CIRCLE" {
		return math.Abs(cornerDistance(p, *e.Center)-e.Radius) <= profileTolerance
	}
	return cornerContains(e, p)
}

// Interval arithmetic, not sampled chords, distinguishes coincident-support
// overlap from the isolated endpoints allowed at a formal chain joint.
func offsetCircularOverlap(a, b SketchEntity) bool {
	if a.Kind == "CIRCLE" || b.Kind == "CIRCLE" {
		return true
	}
	alo, ahi := math.Min(a.StartAngle, a.EndAngle), math.Max(a.StartAngle, a.EndAngle)
	blo, bhi := math.Min(b.StartAngle, b.EndAngle), math.Max(b.StartAngle, b.EndAngle)
	shift := math.Round(((alo+ahi)-(blo+bhi))/(4*math.Pi)) * 2 * math.Pi
	for _, delta := range []float64{shift - 2*math.Pi, shift, shift + 2*math.Pi} {
		if (math.Min(ahi, bhi+delta)-math.Max(alo, blo+delta))*a.Radius > profileTolerance {
			return true
		}
	}
	return false
}
func offsetFiniteHits(a, b SketchEntity) ([]SketchPoint2, bool) {
	if a.Kind == "LINE" && b.Kind == "LINE" {
		ad, bd := sub2(*a.End, *a.Start), sub2(*b.End, *b.Start)
		if cornerCross(ad, bd) == 0 && math.Abs(cornerCross(sub2(*b.Start, *a.Start), ad))/math.Hypot(ad.X, ad.Y) <= profileTolerance {
			l := math.Hypot(ad.X, ad.Y)
			u := scale2(ad, 1/l)
			t1, t2 := cornerDot(sub2(*b.Start, *a.Start), u), cornerDot(sub2(*b.End, *a.Start), u)
			if math.Min(l, math.Max(t1, t2))-math.Max(0, math.Min(t1, t2)) > profileTolerance {
				return nil, true
			}
		}
	}
	if a.Kind != "LINE" && b.Kind != "LINE" && cornerDistance(*a.Center, *b.Center) <= profileTolerance && math.Abs(a.Radius-b.Radius) <= profileTolerance {
		if offsetCircularOverlap(a, b) {
			return nil, true
		}
		points := []SketchPoint2{}
		if a.Kind == "ARC" {
			for _, sub := range []string{"START", "END"} {
				p := cornerEndpoint(a, sub)
				if offsetBoundedContains(b, p) {
					points = append(points, p)
				}
			}
		}
		return points, false
	}
	result := []SketchPoint2{}
	for _, p := range offsetSupportHits(a, b) {
		if offsetBoundedContains(a, p) && offsetBoundedContains(b, p) {
			result = append(result, p)
		}
	}
	return result, false
}
func applySketchOffsetEdit(sketch *SketchFeature, op SketchOperation) error {
	if op.OperationID == "" || op.Value == nil || !finite(*op.Value) || *op.Value == 0 {
		return fmt.Errorf("%w: offset requires an operation identity and a finite nonzero signed distance", ErrValidation)
	}
	mode := op.Mode
	if mode == "" {
		mode = "MITER"
	}
	if mode != "MITER" && mode != "ROUND" {
		return fmt.Errorf("%w: offset join must be MITER or ROUND", ErrValidation)
	}
	if len(op.EntityIDs) == 0 && op.EntityID != "" {
		op.EntityIDs = []string{op.EntityID}
	}
	if len(op.EntityIDs) == 0 {
		return fmt.Errorf("%w: select local Line, Circle or Arc geometry", ErrValidation)
	}
	selected := map[string]SketchEntity{}
	existing := map[string]bool{}
	for _, e := range sketch.Entities {
		existing[e.ID] = true
		for _, id := range op.EntityIDs {
			if e.ID == id {
				selected[id] = e
			}
		}
	}
	seen := map[string]bool{}
	for _, id := range op.EntityIDs {
		e, ok := selected[id]
		if !ok || seen[id] || e.Suppressed {
			return fmt.Errorf("%w: offset requires unique editable local geometry; external geometry must be detached", ErrValidation)
		}
		seen[id] = true
		if e.Kind != "LINE" && e.Kind != "CIRCLE" && e.Kind != "ARC" {
			return fmt.Errorf("%w: exact offset supports Line, Circle and Arc; ellipse/spline offsets are not same-type curves", ErrValidation)
		}
	}
	dsu := profileConnectivity(Feature{Sketch: sketch})
	nodes := map[string][]string{}
	node := func(e SketchEntity, sub string) string { return dsu.find(e.ID + "/" + sub) }
	for _, id := range op.EntityIDs {
		e := selected[id]
		if e.Kind == "CIRCLE" {
			continue
		}
		for _, sub := range []string{"START", "END"} {
			n := node(e, sub)
			nodes[n] = append(nodes[n], id)
			if len(nodes[n]) > 2 {
				return fmt.Errorf("%w: offset selection branches at a formal connection", ErrValidation)
			}
		}
	}
	processed := map[string]bool{}
	output := []SketchEntity{}
	constraints := []SketchConstraint{}
	newEntity := func(e SketchEntity, slot string) (SketchEntity, error) {
		source := e.ID
		e.ID = macroID(op.OperationID, "offset/"+slot)
		if existing[e.ID] {
			return SketchEntity{}, fmt.Errorf("%w: offset operation identity already creates geometry", ErrValidation)
		}
		existing[e.ID] = true
		e.CreatedByOperationID = op.OperationID
		e.SourceEntityID = source
		e.StartPointID = ""
		e.EndPointID = ""
		return e, nil
	}
	join := func(a, b SketchEntity) {
		constraints = append(constraints, SketchConstraint{ID: macroID(op.OperationID, "offset/join/"+a.ID+"/"+b.ID), Kind: "COINCIDENT", Internal: true, References: []SketchGeometryRef{cornerRef(a.ID, "END"), cornerRef(b.ID, "START")}})
	}
	for _, seed := range op.EntityIDs {
		if processed[seed] {
			continue
		}
		source := selected[seed]
		if source.Kind == "CIRCLE" {
			e, err := newEntity(source, source.ID)
			if err != nil {
				return err
			}
			e.Radius -= *op.Value
			if e.Radius <= 0 {
				return fmt.Errorf("%w: offset circle radius becomes nonpositive", ErrValidation)
			}
			output = append(output, e)
			processed[seed] = true
			continue
		}
		chain := []offsetEdge{{source: source}}
		processed[seed] = true
		// Preserve the first selected entity's direction, extending both ends via
		// formal Coincident classes. Coordinates only validate those connections.
		for _, front := range []bool{false, true} {
			for {
				at := chain[len(chain)-1]
				sub := "END"
				if front {
					at = chain[0]
					sub = "START"
				}
				oriented := at.source
				if at.reversed {
					oriented = offsetReverse(oriented)
				}
				originalSub := sub
				if at.reversed {
					if sub == "START" {
						originalSub = "END"
					} else {
						originalSub = "START"
					}
				}
				n := node(at.source, originalSub)
				next := ""
				for _, id := range nodes[n] {
					if !processed[id] {
						next = id
						break
					}
				}
				if next == "" {
					break
				}
				e := selected[next]
				rev := node(e, "START") != n
				if front {
					rev = node(e, "END") != n
				}
				edge := offsetEdge{source: e, reversed: rev}
				candidate := e
				if rev {
					candidate = offsetReverse(candidate)
				}
				p := cornerEndpoint(oriented, sub)
				q := cornerEndpoint(candidate, "START")
				if front {
					q = cornerEndpoint(candidate, "END")
				}
				if cornerDistance(p, q) > profileTolerance {
					return fmt.Errorf("%w: formal offset connection is geometrically inconsistent", ErrValidation)
				}
				processed[next] = true
				if front {
					chain = append([]offsetEdge{edge}, chain...)
				} else {
					chain = append(chain, edge)
				}
			}
		}
		first, last := chain[0], chain[len(chain)-1]
		firstSub, lastSub := "START", "END"
		if first.reversed {
			firstSub = "END"
		}
		if last.reversed {
			lastSub = "START"
		}
		closed := node(first.source, firstSub) == node(last.source, lastSub)
		curves := make([]SketchEntity, len(chain))
		original := make([]SketchEntity, len(chain))
		for i, edge := range chain {
			e := edge.source
			if edge.reversed {
				e = offsetReverse(e)
			}
			original[i] = e
			e, err := newEntity(e, e.ID)
			if err != nil {
				return err
			}
			if e.Kind == "LINE" {
				u := offsetTangent(e, false)
				delta := SketchPoint2{-u.Y * *op.Value, u.X * *op.Value}
				a, b := add2(*e.Start, delta), add2(*e.End, delta)
				e.Start, e.End = &a, &b
			} else {
				sign := 1.0
				if e.EndAngle < e.StartAngle {
					sign = -1
				}
				e.Radius -= sign * *op.Value
				if e.Radius <= 0 {
					return fmt.Errorf("%w: offset arc radius becomes nonpositive", ErrValidation)
				}
			}
			curves[i] = e
		}
		bridges := map[int]SketchEntity{}
		jointCount := len(curves) - 1
		if closed {
			jointCount++
		}
		for i := 0; i < jointCount; i++ {
			j := (i + 1) % len(curves)
			a, b := cornerEndpoint(curves[i], "END"), cornerEndpoint(curves[j], "START")
			if cornerDistance(a, b) <= profileTolerance {
				continue
			}
			ta, tb := offsetTangent(original[i], true), offsetTangent(original[j], false)
			turn := cornerCross(ta, tb)
			if mode == "ROUND" && turn**op.Value < 0 {
				center := cornerEndpoint(original[i], "END")
				start := math.Atan2(a.Y-center.Y, a.X-center.X)
				end := math.Atan2(b.Y-center.Y, b.X-center.X)
				sweep := math.Remainder(end-start, 2*math.Pi)
				if turn*sweep < 0 {
					sweep += math.Copysign(2*math.Pi, turn)
				}
				if math.Abs(sweep) >= math.Pi+1e-10 {
					return fmt.Errorf("%w: offset round join has an ambiguous reversing branch", ErrValidation)
				}
				bridge := SketchEntity{ID: macroID(op.OperationID, fmt.Sprintf("offset/round/%s/%s", original[i].ID, original[j].ID)), Kind: "ARC", Role: curves[i].Role, Center: &center, Radius: math.Abs(*op.Value), StartAngle: start, EndAngle: start + sweep, CreatedByOperationID: op.OperationID, SourceEntityID: original[i].ID}
				if existing[bridge.ID] {
					return fmt.Errorf("%w: duplicate offset join identity", ErrValidation)
				}
				existing[bridge.ID] = true
				bridges[i] = bridge
				constraints = append(constraints,
					SketchConstraint{ID: macroID(op.OperationID, "offset/tangent/start/"+bridge.ID), Kind: "TANGENT", Internal: true, References: []SketchGeometryRef{cornerRef(curves[i].ID, "WHOLE"), cornerRef(bridge.ID, "START")}},
					SketchConstraint{ID: macroID(op.OperationID, "offset/tangent/end/"+bridge.ID), Kind: "TANGENT", Internal: true, References: []SketchGeometryRef{cornerRef(curves[j].ID, "WHOLE"), cornerRef(bridge.ID, "END")}})
				continue
			}
			hits := offsetSupportHits(curves[i], curves[j])
			sort.Slice(hits, func(x, y int) bool {
				return cornerDistance(hits[x], a)+cornerDistance(hits[x], b) < cornerDistance(hits[y], a)+cornerDistance(hits[y], b)
			})
			found := false
			for _, p := range hits {
				aa, bb := curves[i], curves[j]
				if offsetMoveEndpoint(&aa, "END", p) && offsetMoveEndpoint(&bb, "START", p) {
					curves[i], curves[j] = aa, bb
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: offset join has no finite nondegenerate branch", ErrValidation)
			}
		}
		generated := []SketchEntity{}
		for i, e := range curves {
			generated = append(generated, e)
			if bridge, ok := bridges[i]; ok {
				generated = append(generated, bridge)
			}
		}
		for i := 0; i < len(generated)-1; i++ {
			join(generated[i], generated[i+1])
		}
		if closed {
			join(generated[len(generated)-1], generated[0])
		}
		output = append(output, generated...)
	}
	// Validate only the newly created set. Independent copies may overlap source
	// geometry by design; self-crossing/overlapping output is explicitly rejected.
	for i, a := range output {
		for j := 0; j < i; j++ {
			b := output[j]
			hits, overlap := offsetFiniteHits(a, b)
			if overlap {
				return fmt.Errorf("%w: offset output overlaps itself", ErrValidation)
			}
			for _, p := range hits {
				connected := false
				for _, c := range constraints {
					if c.Kind != "COINCIDENT" {
						continue
					}
					r, s := c.References[0], c.References[1]
					if r.EntityID == b.ID {
						r, s = s, r
					}
					if r.EntityID == a.ID && s.EntityID == b.ID {
						if cornerDistance(p, cornerEndpoint(a, r.SubElement)) <= profileTolerance && cornerDistance(p, cornerEndpoint(b, s.SubElement)) <= profileTolerance {
							connected = true
						}
					}
				}
				if !connected {
					return fmt.Errorf("%w: offset output self-intersects", ErrValidation)
				}
			}
		}
	}
	sketch.Entities = append(sketch.Entities, output...)
	sketch.Constraints = append(sketch.Constraints, constraints...)
	return nil
}
