package workspace

import (
	"fmt"
	"math"
	"sort"
)

type sketchCornerCandidate struct {
	center, first, second SketchPoint2
	start, end, score     float64
}

func cornerCross(a, b SketchPoint2) float64    { return a.X*b.Y - a.Y*b.X }
func cornerDot(a, b SketchPoint2) float64      { return a.X*b.X + a.Y*b.Y }
func cornerDistance(a, b SketchPoint2) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }
func cornerRef(id, sub string) SketchGeometryRef {
	return SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: sub}
}
func cornerLineIntersection(a, ad, b, bd SketchPoint2) (SketchPoint2, bool) {
	d := cornerCross(ad, bd)
	if d == 0 {
		return SketchPoint2{}, false
	}
	t := cornerCross(sub2(b, a), bd) / d
	return add2(a, scale2(ad, t)), true
}
func cornerLineCircle(origin, direction, center SketchPoint2, radius float64) []SketchPoint2 {
	unit := scale2(direction, 1/math.Hypot(direction.X, direction.Y))
	t := cornerDot(sub2(center, origin), unit)
	foot := add2(origin, scale2(unit, t))
	h2 := radius*radius - cornerDot(sub2(foot, center), sub2(foot, center))
	if h2 < 0 {
		return nil
	}
	if h2 == 0 {
		return []SketchPoint2{foot}
	}
	h := math.Sqrt(h2)
	return []SketchPoint2{add2(foot, scale2(unit, h)), add2(foot, scale2(unit, -h))}
}
func cornerCircleCircle(a SketchPoint2, ar float64, b SketchPoint2, br float64) []SketchPoint2 {
	d := cornerDistance(a, b)
	if d == 0 || d > ar+br || d < math.Abs(ar-br) {
		return nil
	}
	u := scale2(sub2(b, a), 1/d)
	x := (ar*ar - br*br + d*d) / (2 * d)
	h2 := ar*ar - x*x
	if h2 < 0 {
		return nil
	}
	base := add2(a, scale2(u, x))
	if h2 == 0 {
		return []SketchPoint2{base}
	}
	n := SketchPoint2{X: -u.Y, Y: u.X}
	h := math.Sqrt(h2)
	return []SketchPoint2{add2(base, scale2(n, h)), add2(base, scale2(n, -h))}
}
func cornerEndpoint(e SketchEntity, sub string) SketchPoint2 {
	if e.Kind == "LINE" {
		if sub == "START" {
			return *e.Start
		}
		return *e.End
	}
	angle := e.StartAngle
	if sub == "END" {
		angle = e.EndAngle
	}
	return add2(*e.Center, SketchPoint2{X: e.Radius * math.Cos(angle), Y: e.Radius * math.Sin(angle)})
}
func cornerContains(e SketchEntity, p SketchPoint2) bool {
	if e.Kind == "LINE" {
		d := sub2(*e.End, *e.Start)
		l2 := cornerDot(d, d)
		if l2 == 0 {
			return false
		}
		t := cornerDot(sub2(p, *e.Start), d) / l2
		return t >= -1e-10 && t <= 1+1e-10 && math.Abs(cornerCross(sub2(p, *e.Start), d))/math.Sqrt(l2) <= profileTolerance
	}
	u := math.Atan2(p.Y-e.Center.Y, p.X-e.Center.X)
	lo, hi := e.StartAngle, e.EndAngle
	if hi < lo {
		lo, hi = hi, lo
	}
	u += math.Round(((lo+hi)/2-u)/(2*math.Pi)) * 2 * math.Pi
	return u >= lo-1e-10 && u <= hi+1e-10 && math.Abs(cornerDistance(p, *e.Center)-e.Radius) <= profileTolerance
}
func cornerCutAllowed(active SketchEntity, sub string, p SketchPoint2) bool {
	other := "START"
	if sub == "START" {
		other = "END"
	}
	return cornerContains(active, p) && cornerDistance(p, cornerEndpoint(active, other)) > profileTolerance
}
func cornerArcDistance(p, center SketchPoint2, radius, start, end float64) float64 {
	phi := math.Atan2(p.Y-center.Y, p.X-center.X)
	lo, hi := start, end
	if hi < lo {
		lo, hi = hi, lo
	}
	phi += math.Round(((lo+hi)/2-phi)/(2*math.Pi)) * 2 * math.Pi
	if phi >= lo && phi <= hi {
		return math.Abs(cornerDistance(p, center) - radius)
	}
	a := add2(center, SketchPoint2{X: radius * math.Cos(start), Y: radius * math.Sin(start)})
	b := add2(center, SketchPoint2{X: radius * math.Cos(end), Y: radius * math.Sin(end)})
	return math.Min(cornerDistance(p, a), cornerDistance(p, b))
}
func cornerFilletCandidates(first, second SketchEntity, firstActive, secondActive SketchEntity, firstSub, secondSub string, r float64, click *SketchPoint2) []sketchCornerCandidate {
	candidates := []sketchCornerCandidate{}
	appendCandidate := func(center, a, b SketchPoint2) {
		if !cornerCutAllowed(firstActive, firstSub, a) || !cornerCutAllowed(secondActive, secondSub, b) || cornerDistance(a, b) <= profileTolerance {
			return
		}
		start := math.Atan2(a.Y-center.Y, a.X-center.X)
		sweep := math.Remainder(math.Atan2(b.Y-center.Y, b.X-center.X)-start, 2*math.Pi)
		sweeps := []float64{sweep}
		if click != nil {
			if sweep > 0 {
				sweeps = append(sweeps, sweep-2*math.Pi)
			} else {
				sweeps = append(sweeps, sweep+2*math.Pi)
			}
		}
		for _, s := range sweeps {
			score := cornerDistance(a, cornerEndpoint(firstActive, firstSub)) + cornerDistance(b, cornerEndpoint(secondActive, secondSub))
			if click != nil {
				score = cornerArcDistance(*click, center, r, start, start+s)
			}
			candidates = append(candidates, sketchCornerCandidate{center, a, b, start, start + s, score})
		}
	}
	if first.Kind == "LINE" && second.Kind == "LINE" {
		ad, bd := sub2(*first.End, *first.Start), sub2(*second.End, *second.Start)
		au, bu := scale2(ad, 1/math.Hypot(ad.X, ad.Y)), scale2(bd, 1/math.Hypot(bd.X, bd.Y))
		an, bn := SketchPoint2{X: -au.Y, Y: au.X}, SketchPoint2{X: -bu.Y, Y: bu.X}
		for _, as := range []float64{-1, 1} {
			for _, bs := range []float64{-1, 1} {
				center, ok := cornerLineIntersection(add2(*first.Start, scale2(an, as*r)), au, add2(*second.Start, scale2(bn, bs*r)), bu)
				if !ok {
					continue
				}
				appendCandidate(center, sub2(center, scale2(an, as*r)), sub2(center, scale2(bn, bs*r)))
			}
		}
	} else if first.Kind == "LINE" || second.Kind == "LINE" {
		line, arc := first, second
		flipped := false
		if first.Kind != "LINE" {
			line, arc = second, first
			flipped = true
		}
		d := sub2(*line.End, *line.Start)
		u := scale2(d, 1/math.Hypot(d.X, d.Y))
		n := SketchPoint2{X: -u.Y, Y: u.X}
		for _, side := range []float64{-1, 1} {
			for _, offset := range []float64{arc.Radius + r, arc.Radius - r} {
				if offset == 0 {
					continue
				}
				for _, center := range cornerLineCircle(add2(*line.Start, scale2(n, side*r)), u, *arc.Center, math.Abs(offset)) {
					radial := scale2(sub2(center, *arc.Center), arc.Radius/offset)
					a, b := sub2(center, scale2(n, side*r)), add2(*arc.Center, radial)
					if flipped {
						a, b = b, a
					}
					appendCandidate(center, a, b)
				}
			}
		}
	} else {
		for _, aoffset := range []float64{first.Radius + r, first.Radius - r} {
			for _, boffset := range []float64{second.Radius + r, second.Radius - r} {
				if aoffset == 0 || boffset == 0 {
					continue
				}
				for _, center := range cornerCircleCircle(*first.Center, math.Abs(aoffset), *second.Center, math.Abs(boffset)) {
					a := add2(*first.Center, scale2(sub2(center, *first.Center), first.Radius/aoffset))
					b := add2(*second.Center, scale2(sub2(center, *second.Center), second.Radius/boffset))
					appendCandidate(center, a, b)
				}
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.score != b.score {
			return a.score < b.score
		}
		if a.center.X != b.center.X {
			return a.center.X < b.center.X
		}
		if a.center.Y != b.center.Y {
			return a.center.Y < b.center.Y
		}
		return a.end-a.start < b.end-b.start
	})
	return candidates
}

// The transaction owner applies this to a staged clone. Supports keep their
// identity and drivers; only the display/profile role and explicit descendants change.
func applySketchCornerEdit(sketch *SketchFeature, op SketchOperation) error {
	if op.OperationID == "" || len(op.EntityIDs) != 2 || op.EntityIDs[0] == op.EntityIDs[1] {
		return fmt.Errorf("%w: corner requires operation identity and two distinct local curves", ErrValidation)
	}
	trim := op.TrimMode
	if trim == "" {
		trim = "TRIM"
	}
	if trim != "TRIM" && trim != "KEEP" {
		return fmt.Errorf("%w: corner trimMode must be TRIM or KEEP", ErrValidation)
	}
	if op.Point != nil && (!finite(op.Point.X) || !finite(op.Point.Y)) {
		return fmt.Errorf("%w: invalid corner branch point", ErrValidation)
	}
	refs := []*SketchGeometryRef{op.FirstReference, op.SecondReference}
	sources := make([]SketchEntity, 2)
	actives := make([]SketchEntity, 2)
	indices := make([]int, 2)
	subs := make([]string, 2)
	for i, id := range op.EntityIDs {
		found := -1
		for j, e := range sketch.Entities {
			if e.ID == id {
				found = j
				break
			}
		}
		if found < 0 {
			return fmt.Errorf("%w: corner source is not local editable geometry; detach references explicitly", ErrValidation)
		}
		source := sketch.Entities[found]
		if source.SourceEntityID != "" {
			for j, e := range sketch.Entities {
				if e.ID == source.SourceEntityID && cornerHasSupport(sketch, source, e.ID) {
					source = e
					found = j
					break
				}
			}
		}
		if source.Suppressed || (source.Kind != "LINE" && source.Kind != "ARC") {
			return fmt.Errorf("%w: corner supports Line and Arc only", ErrValidation)
		}
		if source.Kind == "LINE" && (source.Start == nil || source.End == nil || cornerDistance(*source.Start, *source.End) <= profileTolerance) {
			return fmt.Errorf("%w: degenerate corner line", ErrValidation)
		}
		if source.Kind == "ARC" && (source.Center == nil || !positiveFinite(source.Radius) || source.StartAngle == source.EndAngle) {
			return fmt.Errorf("%w: degenerate corner arc", ErrValidation)
		}
		if refs[i] == nil || refs[i].Target != "ENTITY" || (refs[i].EntityID != id && refs[i].EntityID != source.ID) || (refs[i].SubElement != "START" && refs[i].SubElement != "END") {
			return fmt.Errorf("%w: corner requires explicit stable START/END selection for each curve", ErrValidation)
		}
		active := source
		activeCount := 0
		for _, e := range sketch.Entities {
			if e.Role == "PROFILE" && e.SourceEntityID == source.ID && cornerHasSupport(sketch, e, source.ID) {
				active = e
				activeCount++
			}
		}
		if activeCount > 1 {
			return fmt.Errorf("%w: ambiguous corner descendant selection", ErrValidation)
		}
		if trim == "TRIM" && active.ID != source.ID {
			uncutID := macroID(active.CreatedByOperationID, "corner/uncut/"+source.ID+"/"+refs[i].SubElement)
			uncut := false
			for _, c := range sketch.Constraints {
				if c.ID == uncutID {
					uncut = true
					break
				}
			}
			if !uncut {
				return fmt.Errorf("%w: this endpoint already has a corner; edit its dimensions", ErrValidation)
			}
		}
		sources[i], actives[i], indices[i], subs[i] = source, active, found, refs[i].SubElement
	}
	if sources[0].ID == sources[1].ID {
		return fmt.Errorf("%w: corner resolves to the same source curve", ErrValidation)
	}
	var chosen sketchCornerCandidate
	var virtual SketchPoint2
	chamferMode := ""
	firstLength, secondLength := 0.0, 0.0
	angleRefsReversed, bridgeReversed := false, false
	if op.Type == "FILLET_ENTITIES" {
		if op.Value == nil || !positiveFinite(*op.Value) {
			return fmt.Errorf("%w: fillet radius must be positive finite", ErrValidation)
		}
		candidates := cornerFilletCandidates(sources[0], sources[1], actives[0], actives[1], subs[0], subs[1], *op.Value, op.Point)
		if len(candidates) == 0 {
			return fmt.Errorf("%w: no finite fillet branch; radius too large, parallel support or unsupported contact", ErrValidation)
		}
		chosen = candidates[0]
	} else if op.Type == "CHAMFER_ENTITIES" {
		if sources[0].Kind != "LINE" || sources[1].Kind != "LINE" {
			return fmt.Errorf("%w: chamfer requires two lines", ErrValidation)
		}
		var ok bool
		virtual, ok = cornerLineIntersection(*sources[0].Start, sub2(*sources[0].End, *sources[0].Start), *sources[1].Start, sub2(*sources[1].End, *sources[1].Start))
		if !ok {
			return fmt.Errorf("%w: parallel lines have no finite virtual corner", ErrValidation)
		}
		rays := make([]SketchPoint2, 2)
		for i, e := range actives {
			other := "START"
			if subs[i] == "START" {
				other = "END"
			}
			rays[i] = sub2(cornerEndpoint(e, other), virtual)
			length := math.Hypot(rays[i].X, rays[i].Y)
			if length == 0 {
				return fmt.Errorf("%w: degenerate corner ray", ErrValidation)
			}
			rays[i] = scale2(rays[i], 1/length)
		}
		firstLength = op.ChamferFirst
		if !positiveFinite(firstLength) {
			return fmt.Errorf("%w: chamfer first length must be positive finite", ErrValidation)
		}
		chamferMode = op.ChamferMode
		if chamferMode == "" {
			chamferMode = "EQUAL"
		}
		switch chamferMode {
		case "EQUAL":
			secondLength = firstLength
		case "TWO_LENGTHS":
			secondLength = op.ChamferSecond
			if !positiveFinite(secondLength) {
				return fmt.Errorf("%w: chamfer second length must be positive finite", ErrValidation)
			}
		case "LENGTH_ANGLE":
			theta := op.ChamferAngle * math.Pi / 180
			if !finite(theta) || theta <= 0 || theta >= math.Pi {
				return fmt.Errorf("%w: chamfer angle must be between 0 and 180 degrees", ErrValidation)
			}
			phi := math.Acos(math.Max(-1, math.Min(1, cornerDot(rays[0], rays[1]))))
			if phi == 0 || phi+theta >= math.Pi {
				return fmt.Errorf("%w: chamfer angle has no positive second branch", ErrValidation)
			}
			secondLength = firstLength * math.Sin(theta) / math.Sin(phi+theta)
		default:
			return fmt.Errorf("%w: unknown chamfer mode", ErrValidation)
		}
		chosen.first = add2(virtual, scale2(rays[0], firstLength))
		chosen.second = add2(virtual, scale2(rays[1], secondLength))
		if !cornerCutAllowed(actives[0], subs[0], chosen.first) || !cornerCutAllowed(actives[1], subs[1], chosen.second) {
			return fmt.Errorf("%w: chamfer exceeds the available finite edges", ErrValidation)
		}
		if chamferMode == "LENGTH_ANGLE" {
			direction := sub2(chosen.second, chosen.first)
			sourceDirection := sub2(*sources[0].End, *sources[0].Start)
			delta := math.Atan2(cornerCross(sourceDirection, direction), cornerDot(sourceDirection, direction)) * 180 / math.Pi
			found := false
			for _, flip := range []bool{false, true} {
				for _, swap := range []bool{false, true} {
					d := delta
					if flip {
						d = math.Remainder(d+180, 360)
					}
					if swap {
						d = -d
					}
					if math.Abs(d-op.ChamferAngle) <= 1e-8 {
						bridgeReversed, angleRefsReversed, found = flip, swap, true
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: chamfer angle branch cannot be represented consistently", ErrValidation)
			}
		}
	} else {
		return fmt.Errorf("%w: unknown corner operation", ErrValidation)
	}
	bridgeID := macroID(op.OperationID, "corner/bridge")
	for _, e := range sketch.Entities {
		if e.ID == bridgeID {
			return fmt.Errorf("%w: corner operation identity already exists", ErrValidation)
		}
	}
	appendConstraint := func(label, kind string, refs ...SketchGeometryRef) {
		sketch.Constraints = append(sketch.Constraints, SketchConstraint{ID: macroID(op.OperationID, "corner/"+label), Kind: kind, References: refs, Internal: true})
	}
	appendDimension := func(label, kind, unit string, value float64, refs ...SketchGeometryRef) {
		sketch.Constraints = append(sketch.Constraints, SketchConstraint{ID: macroID(op.OperationID, "corner/"+label), Kind: kind, References: refs, Value: &value, Unit: unit, ParameterID: macroID(op.OperationID, "corner/parameter/"+label)})
	}
	childIDs := make([]string, 2)
	role := "PROFILE"
	if trim == "KEEP" {
		role = "CONSTRUCTION"
	}
	for i, source := range sources {
		child := actives[i]
		if trim == "KEEP" || child.ID == source.ID {
			child.ID = macroID(op.OperationID, "corner/trim/"+source.ID)
			child.StartPointID = ""
			child.EndPointID = ""
			child.CreatedByOperationID = op.OperationID
			child.SourceEntityID = source.ID
			child.Role = role
		}
		if subs[i] == "START" {
			child.StartPointID = macroID(op.OperationID, "corner/cut-point/"+source.ID+"/START")
		} else {
			child.EndPointID = macroID(op.OperationID, "corner/cut-point/"+source.ID+"/END")
		}
		point := chosen.first
		if i == 1 {
			point = chosen.second
		}
		if child.Kind == "LINE" {
			if subs[i] == "START" {
				child.Start = &point
			} else {
				child.End = &point
			}
		} else {
			angle := math.Atan2(point.Y-source.Center.Y, point.X-source.Center.X)
			angle += math.Round(((source.StartAngle+source.EndAngle)/2-angle)/(2*math.Pi)) * 2 * math.Pi
			if subs[i] == "START" {
				child.StartAngle = angle
			} else {
				child.EndAngle = angle
			}
		}
		found := -1
		for j, e := range sketch.Entities {
			if e.ID == child.ID {
				found = j
				break
			}
		}
		if found < 0 {
			sketch.Entities = append(sketch.Entities, child)
			appendConstraint("support/"+source.ID, "SAME_SUPPORT", cornerRef(source.ID, "WHOLE"), cornerRef(child.ID, "WHOLE"))
			other := "START"
			if subs[i] == "START" {
				other = "END"
			}
			appendConstraint("uncut/"+source.ID+"/"+other, "COINCIDENT", cornerRef(child.ID, other), cornerRef(source.ID, other))
		} else {
			sketch.Entities[found] = child
			uncutID := macroID(child.CreatedByOperationID, "corner/uncut/"+source.ID+"/"+subs[i])
			kept := sketch.Constraints[:0]
			for _, c := range sketch.Constraints {
				if c.ID != uncutID {
					kept = append(kept, c)
				}
			}
			sketch.Constraints = kept
		}
		appendConstraint("cut-on/"+source.ID, "POINT_ON_OBJECT", cornerRef(child.ID, subs[i]), cornerRef(source.ID, "WHOLE"))
		childIDs[i] = child.ID
		if trim == "TRIM" {
			sketch.Entities[indices[i]].Role = "CONSTRUCTION"
		}
	}
	bridge := SketchEntity{ID: bridgeID, Role: role, CreatedByOperationID: op.OperationID, SourceEntityID: sources[0].ID}
	firstBridgeSub, secondBridgeSub := "START", "END"
	if op.Type == "FILLET_ENTITIES" {
		bridge.Kind = "ARC"
		bridge.Center = &chosen.center
		bridge.Radius = *op.Value
		bridge.StartAngle = chosen.start
		bridge.EndAngle = chosen.end
		appendConstraint("tangent/first", "TANGENT", cornerRef(sources[0].ID, "WHOLE"), cornerRef(bridgeID, "START"))
		appendConstraint("tangent/second", "TANGENT", cornerRef(sources[1].ID, "WHOLE"), cornerRef(bridgeID, "END"))
		appendDimension("radius", "RADIUS", "mm", *op.Value, cornerRef(bridgeID, "WHOLE"))
	} else {
		bridge.Kind = "LINE"
		a, b := chosen.first, chosen.second
		if bridgeReversed {
			a, b = b, a
			firstBridgeSub, secondBridgeSub = "END", "START"
		}
		bridge.Start = &a
		bridge.End = &b
		virtualID := macroID(op.OperationID, "corner/virtual")
		sketch.Entities = append(sketch.Entities, SketchEntity{ID: virtualID, Kind: "POINT", Role: "CONSTRUCTION", Point: &virtual, CreatedByOperationID: op.OperationID, SourceEntityID: sources[0].ID})
		appendConstraint("virtual/first", "POINT_ON_OBJECT", cornerRef(virtualID, "POINT"), cornerRef(sources[0].ID, "WHOLE"))
		appendConstraint("virtual/second", "POINT_ON_OBJECT", cornerRef(virtualID, "POINT"), cornerRef(sources[1].ID, "WHOLE"))
		rays := make([]string, 2)
		for i, p := range []SketchPoint2{chosen.first, chosen.second} {
			rays[i] = macroID(op.OperationID, fmt.Sprintf("corner/ray/%d", i))
			start, end := virtual, p
			sketch.Entities = append(sketch.Entities, SketchEntity{ID: rays[i], Kind: "LINE", Role: "CONSTRUCTION", Start: &start, End: &end, CreatedByOperationID: op.OperationID, SourceEntityID: sources[i].ID})
			appendConstraint(fmt.Sprintf("ray/%d/start", i), "COINCIDENT", cornerRef(rays[i], "START"), cornerRef(virtualID, "POINT"))
			appendConstraint(fmt.Sprintf("ray/%d/end", i), "COINCIDENT", cornerRef(rays[i], "END"), cornerRef(childIDs[i], subs[i]))
		}
		appendDimension("first-length", "LENGTH", "mm", firstLength, cornerRef(rays[0], "WHOLE"))
		if chamferMode == "EQUAL" {
			appendConstraint("equal-lengths", "EQUAL", cornerRef(rays[0], "WHOLE"), cornerRef(rays[1], "WHOLE"))
		} else if chamferMode == "TWO_LENGTHS" {
			appendDimension("second-length", "LENGTH", "mm", secondLength, cornerRef(rays[1], "WHOLE"))
		} else {
			a, b := cornerRef(sources[0].ID, "DIRECTION"), cornerRef(bridgeID, "DIRECTION")
			if angleRefsReversed {
				a, b = b, a
			}
			appendDimension("angle", "ANGLE", "deg", op.ChamferAngle, a, b)
		}
	}
	sketch.Entities = append(sketch.Entities, bridge)
	appendConstraint("join/first", "COINCIDENT", cornerRef(childIDs[0], subs[0]), cornerRef(bridgeID, firstBridgeSub))
	appendConstraint("join/second", "COINCIDENT", cornerRef(childIDs[1], subs[1]), cornerRef(bridgeID, secondBridgeSub))
	return validateSketchCornerGeometry(sketch)
}
func cornerHasSupport(sketch *SketchFeature, child SketchEntity, sourceID string) bool {
	id := macroID(child.CreatedByOperationID, "corner/support/"+sourceID)
	for _, c := range sketch.Constraints {
		if c.ID == id && len(c.References) == 2 && c.References[0].EntityID == sourceID && c.References[1].EntityID == child.ID {
			return true
		}
	}
	return false
}

// Run after authoritative coordinates are accepted, before Profile or persistence.
func validateSketchCornerGeometry(sketch *SketchFeature) error {
	byID := map[string]SketchEntity{}
	for _, e := range sketch.Entities {
		byID[e.ID] = e
	}
	for _, e := range sketch.Entities {
		source, ok := byID[e.SourceEntityID]
		if !ok || !cornerHasSupport(sketch, e, source.ID) {
			continue
		}
		if !cornerContains(source, cornerEndpoint(e, "START")) || !cornerContains(source, cornerEndpoint(e, "END")) || cornerDistance(cornerEndpoint(e, "START"), cornerEndpoint(e, "END")) <= profileTolerance {
			return fmt.Errorf("%w: corner cut exceeds source finite range or collapses edge %s", ErrValidation, e.ID)
		}
		if e.Kind == "ARC" && ((e.EndAngle-e.StartAngle)*(source.EndAngle-source.StartAngle) <= 0 || math.Abs(e.EndAngle-e.StartAngle) > math.Abs(source.EndAngle-source.StartAngle)+1e-10) {
			return fmt.Errorf("%w: corner changes source arc branch %s", ErrValidation, e.ID)
		}
	}
	return nil
}
