package workspace

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// Compound edits operate on stable selections and deterministic output IDs.
// Reference changes are explicit in the persisted operation, never inferred by
// nearest endpoint. The caller stages this complete operation atomically.
func applySketchGeometryEdit(sketch *SketchFeature, op SketchOperation) error {
	selected := map[string]bool{}
	if len(op.EntityIDs) == 0 && op.EntityID != "" {
		op.EntityIDs = []string{op.EntityID}
	}
	for _, id := range op.EntityIDs {
		if id == "" || selected[id] {
			return fmt.Errorf("%w: edit requires unique stable entity identities", ErrValidation)
		}
		selected[id] = true
	}
	if len(selected) == 0 {
		return fmt.Errorf("%w: select sketch geometry", ErrValidation)
	}
	entities := map[string]SketchEntity{}
	for _, e := range sketch.Entities {
		entities[e.ID] = e
	}
	for id := range selected {
		if _, ok := entities[id]; !ok {
			return fmt.Errorf("%w: %s is not editable local geometry; detach external geometry explicitly", ErrValidation, id)
		}
	}
	detached := map[string]bool{}
	for _, id := range op.DetachConstraintIDs {
		if detached[id] {
			return fmt.Errorf("%w: duplicate constraint release", ErrValidation)
		}
		detached[id] = true
	}
	for id := range detached {
		found := false
		for _, c := range sketch.Constraints {
			if c.ID == id {
				for _, r := range c.References {
					found = found || selected[r.EntityID]
				}
			}
		}
		if !found {
			return fmt.Errorf("%w: constraint release %s is not part of selection", ErrValidation, id)
		}
	}
	if op.Type == "APPLY_DRAG_RESULT" {
		if op.OperationID == "" || op.CurveSourceDigest != sketchDragDigest(sketch) || len(op.ComputedEntities) != len(sketch.Entities) {
			return fmt.Errorf("%w: drag baseline changed", ErrValidation)
		}
		byID := make(map[string]SketchEntity, len(sketch.Entities))
		for _, e := range sketch.Entities {
			byID[e.ID] = e
		}
		for _, e := range op.ComputedEntities {
			original, ok := byID[e.ID]
			if !ok || e.Kind != original.Kind || e.Role != original.Role || e.StartPointID != original.StartPointID || e.EndPointID != original.EndPointID {
				return fmt.Errorf("%w: drag cannot change geometry identity", ErrValidation)
			}
			delete(byID, e.ID)
		}
		sketch.Entities = op.ComputedEntities
		return nil
	}
	if op.Type == "DELETE_ENTITIES" {
		kept := sketch.Entities[:0]
		for _, e := range sketch.Entities {
			if !selected[e.ID] {
				kept = append(kept, e)
			}
		}
		sketch.Entities = kept
		constraints := sketch.Constraints[:0]
		for _, c := range sketch.Constraints {
			remove := false
			for _, r := range c.References {
				remove = remove || selected[r.EntityID]
			}
			if !remove {
				constraints = append(constraints, c)
			}
		}
		sketch.Constraints = constraints
		return nil
	}
	if op.OperationID == "" {
		return fmt.Errorf("%w: geometry edit requires an operation identity", ErrValidation)
	}
	if op.Type == "REPLACE_CURVE_INTERVALS" {
		return applyComputedSketchIntervals(sketch, op, entities, detached)
	}
	if op.Type == "SPLIT_ENTITY" {
		return splitSketchEntity(sketch, op, entities, detached)
	}
	linked := op.Type == "MIRROR_ENTITIES" && op.MirrorMode == "LINKED"
	if op.Type == "MIRROR_ENTITIES" && op.MirrorMode != "LINKED" && op.MirrorMode != "INDEPENDENT" {
		return fmt.Errorf("%w: mirror mode must be explicit", ErrValidation)
	}
	copyGeometry := op.Copy || op.Type == "COPY_ENTITIES" || op.Type == "MIRROR_ENTITIES"
	if copyGeometry && !linked && op.ConstraintPolicy != "INTERNAL" && op.ConstraintPolicy != "GEOMETRY_ONLY" {
		return fmt.Errorf("%w: copy requires explicit INTERNAL or GEOMETRY_ONLY constraint policy", ErrValidation)
	}
	origin := SketchPoint2{}
	if op.Origin != nil {
		origin = *op.Origin
	}
	translation := SketchPoint2{}
	if op.Translation != nil {
		translation = *op.Translation
	}
	if op.Scale != nil {
		return fmt.Errorf("%w: sketch uniform scaling is unsupported", ErrValidation)
	}
	if !finite(origin.X) || !finite(origin.Y) || !finite(translation.X) || !finite(translation.Y) || !finite(op.Angle) {
		return fmt.Errorf("%w: invalid transform", ErrValidation)
	}
	transform := func(p SketchPoint2) SketchPoint2 {
		d := sub2(p, origin)
		c, s := math.Cos(op.Angle), math.Sin(op.Angle)
		return add2(add2(origin, translation), SketchPoint2{X: c*d.X - s*d.Y, Y: s*d.X + c*d.Y})
	}
	reflected := op.Type == "MIRROR_ENTITIES"
	axisAngle := 0.0
	if reflected {
		if op.Axis == nil {
			return fmt.Errorf("%w: mirror requires a stable sketch line or axis", ErrValidation)
		}
		a, b, ok := constraintLine(*op.Axis, entities)
		if !ok {
			return fmt.Errorf("%w: mirror axis must be a sketch line or intrinsic axis", ErrValidation)
		}
		d := sub2(b, a)
		square := d.X*d.X + d.Y*d.Y
		if square == 0 {
			return fmt.Errorf("%w: mirror axis is degenerate", ErrValidation)
		}
		axisAngle = math.Atan2(d.Y, d.X)
		transform = func(p SketchPoint2) SketchPoint2 {
			v := sub2(p, a)
			projection := add2(a, scale2(d, (v.X*d.X+v.Y*d.Y)/square))
			return sub2(scale2(projection, 2), p)
		}
	}
	ordered := append([]string(nil), op.EntityIDs...)
	sort.Strings(ordered)
	mapping := map[string]string{}
	for _, id := range ordered {
		next := id
		if copyGeometry {
			next = macroID(op.OperationID, "entity/"+id)
			if _, exists := entities[next]; exists {
				return fmt.Errorf("%w: output entity identity already exists", ErrValidation)
			}
		}
		if reflected && sketchMirrorSelfMapped(entities[id], transform, axisAngle) {
			next = id
		}
		mapping[id] = next
	}
	constraints := []SketchConstraint{}
	for _, c := range sketch.Constraints {
		if detached[c.ID] {
			continue
		}
		touches, internal := false, true
		for _, r := range c.References {
			if selected[r.EntityID] && r.Target == "ENTITY" {
				touches = true
			} else {
				internal = false
			}
		}
		if !touches {
			constraints = append(constraints, c)
			continue
		}
		if copyGeometry {
			constraints = append(constraints, c)
			if linked {
				continue
			}
			if op.ConstraintPolicy == "GEOMETRY_ONLY" || !internal {
				continue
			}
		}
		if !internal {
			return fmt.Errorf("%w: transform crosses constraint %s; explicitly release it or include its geometry", ErrValidation, c.ID)
		}
		if c.Kind == "FIXED" || c.Kind == "FIXED_POINT" {
			return fmt.Errorf("%w: fixed geometry %s cannot be transformed with INTERNAL constraints", ErrValidation, c.ID)
		}
		if (c.Kind == "HORIZONTAL" || c.Kind == "VERTICAL") && ((reflected && math.Remainder(2*axisAngle, math.Pi) != 0) || (!reflected && math.Remainder(op.Angle, math.Pi) != 0)) {
			return fmt.Errorf("%w: transform conflicts with %s; explicitly release the direction constraint", ErrValidation, c.ID)
		}
		clone := c
		clone.References = append([]SketchGeometryRef(nil), c.References...)
		if copyGeometry {
			clone.ID = macroID(op.OperationID, "constraint/"+c.ID)
			clone.ParameterID = ""
		}
		for i := range clone.References {
			oldID := clone.References[i].EntityID
			clone.References[i].EntityID = mapping[oldID]
			if copyGeometry && mapping[oldID] != oldID {
				clone.References[i].PointID = ""
			}
			if copyGeometry && mapping[oldID] != oldID && clone.References[i].ControlPointID != "" {
				clone.References[i].ControlPointID = macroID(op.OperationID, "point/"+clone.References[i].ControlPointID)
			}
		}
		if clone.LabelPosition != nil {
			p := transform(*clone.LabelPosition)
			clone.LabelPosition = &p
		}
		constraints = append(constraints, clone)
	}
	outputs := []SketchEntity{}
	for _, id := range ordered {
		raw, _ := json.Marshal(entities[id])
		var e SketchEntity
		_ = json.Unmarshal(raw, &e)
		if copyGeometry && mapping[id] == id {
			continue
		}
		e.ID = mapping[id]
		if copyGeometry {
			e.CreatedByOperationID = op.OperationID
			e.SourceEntityID = id
			e.StartPointID = macroID(op.OperationID, "endpoint/"+id+"/start")
			e.EndPointID = macroID(op.OperationID, "endpoint/"+id+"/end")
			for i, pointID := range e.ControlPointIDs {
				e.ControlPointIDs[i] = macroID(op.OperationID, "point/"+pointID)
			}
			for i, pointID := range e.PoleIDs {
				e.PoleIDs[i] = macroID(op.OperationID, "point/"+pointID)
			}
		}
		for _, p := range []*SketchPoint2{e.Point, e.Start, e.End, e.Center} {
			if p != nil {
				*p = transform(*p)
			}
		}
		for i := range e.Poles {
			e.Poles[i] = transform(e.Poles[i])
		}
		for i := range e.ControlPoints {
			e.ControlPoints[i] = transform(e.ControlPoints[i])
		}
		if reflected {
			if e.Kind == "ARC" {
				e.StartAngle = 2*axisAngle - e.StartAngle
				e.EndAngle = 2*axisAngle - e.EndAngle
			}
			if e.Kind == "ELLIPSE" || e.Kind == "ELLIPTICAL_ARC" {
				e.Rotation = 2*axisAngle - e.Rotation
				e.StartAngle = -e.StartAngle
				e.EndAngle = -e.EndAngle
			}
		} else {
			if e.Kind == "ARC" {
				e.StartAngle += op.Angle
				e.EndAngle += op.Angle
			}
			e.Rotation += op.Angle
		}
		outputs = append(outputs, e)
	}
	if linked {
		for _, id := range ordered {
			target := mapping[id]
			if target == id {
				mode := sketchSelfMirrorMode(entities[id], transform, axisAngle)
				constraints = append(constraints, SketchConstraint{ID: macroID(op.OperationID, "self-mirror/"+id), Kind: "MIRROR", Internal: true, SelfMirrorMode: mode, References: []SketchGeometryRef{{Target: "ENTITY", EntityID: id, SubElement: "WHOLE"}, *op.Axis, {Target: "ENTITY", EntityID: id, SubElement: "WHOLE"}}})
				continue
			}
			constraints = append(constraints, SketchConstraint{ID: macroID(op.OperationID, "mirror/"+id), Kind: "MIRROR", Internal: true, References: []SketchGeometryRef{{Target: "ENTITY", EntityID: id, SubElement: "WHOLE"}, *op.Axis, {Target: "ENTITY", EntityID: target, SubElement: "WHOLE"}}})
			for _, sub := range []string{"POINT", "START", "END"} {
				p, ok := constraintReferencePoint(SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: sub}, entities)
				if !ok {
					continue
				}
				reflected := transform(p)
				if math.Hypot(reflected.X-p.X, reflected.Y-p.Y) <= profileTolerance {
					constraints = append(constraints, SketchConstraint{ID: macroID(op.OperationID, "axis-join/"+id+"/"+sub), Kind: "COINCIDENT", Internal: true, References: []SketchGeometryRef{{Target: "ENTITY", EntityID: id, SubElement: sub}, {Target: "ENTITY", EntityID: target, SubElement: sub}}})
				}
			}
		}
		for _, c := range sketch.Constraints {
			if c.Kind != "COINCIDENT" || detached[c.ID] {
				continue
			}
			clone := c
			clone.ID = macroID(op.OperationID, "connection/"+c.ID)
			clone.Internal = true
			clone.References = append([]SketchGeometryRef(nil), c.References...)
			mapped := true
			changed := false
			for i, r := range clone.References {
				id, ok := mapping[r.EntityID]
				if !ok || r.Target != "ENTITY" {
					mapped = false
					break
				}
				changed = changed || id != r.EntityID
				clone.References[i].EntityID = id
				if id != r.EntityID {
					clone.References[i].PointID = ""
				}
				if id != r.EntityID && r.ControlPointID != "" {
					clone.References[i].ControlPointID = macroID(op.OperationID, "point/"+r.ControlPointID)
				}
			}
			if mapped && changed {
				constraints = append(constraints, clone)
			}
		}
	}
	if copyGeometry {
		sketch.Entities = append(sketch.Entities, outputs...)
	} else {
		for i, e := range sketch.Entities {
			if selected[e.ID] {
				for _, output := range outputs {
					if output.ID == e.ID {
						sketch.Entities[i] = output
						break
					}
				}
			}
		}
	}
	sketch.Constraints = constraints
	return nil
}

func splitSketchEntity(sketch *SketchFeature, op SketchOperation, entities map[string]SketchEntity, detached map[string]bool) error {
	if len(op.EntityIDs) != 1 || len(op.Parameters) == 0 {
		return fmt.Errorf("%w: split requires one entity and curve parameters", ErrValidation)
	}
	source := entities[op.EntityIDs[0]]
	if source.Kind != "LINE" && source.Kind != "ARC" && source.Kind != "ELLIPTICAL_ARC" {
		return fmt.Errorf("%w: this curve requires the precise curve-edit evaluator", ErrValidation)
	}
	params := append([]float64{0}, op.Parameters...)
	params = append(params, 1)
	sort.Float64s(params)
	for i, p := range params {
		if !finite(p) || p < 0 || p > 1 || (i > 0 && p == params[i-1]) {
			return fmt.Errorf("%w: split parameters must be distinct and inside (0,1)", ErrValidation)
		}
	}
	outputs := []SketchEntity{}
	for i := 0; i < len(params)-1; i++ {
		raw, _ := json.Marshal(source)
		var e SketchEntity
		_ = json.Unmarshal(raw, &e)
		e.CreatedByOperationID = op.OperationID
		e.SourceEntityID = source.ID
		if i > 0 {
			e.ID = macroID(op.OperationID, fmt.Sprintf("segment/%s/%d", source.ID, i))
			if _, ok := entities[e.ID]; ok {
				return fmt.Errorf("%w: split output identity collision", ErrValidation)
			}
		}
		if source.Kind == "LINE" {
			a := add2(*source.Start, scale2(sub2(*source.End, *source.Start), params[i]))
			b := add2(*source.Start, scale2(sub2(*source.End, *source.Start), params[i+1]))
			e.Start = &a
			e.End = &b
		} else {
			e.StartAngle = source.StartAngle + (source.EndAngle-source.StartAngle)*params[i]
			e.EndAngle = source.StartAngle + (source.EndAngle-source.StartAngle)*params[i+1]
		}
		e.StartPointID = macroID(op.OperationID, fmt.Sprintf("segment/%d/start", i))
		e.EndPointID = macroID(op.OperationID, fmt.Sprintf("segment/%d/end", i))
		if i == 0 {
			e.StartPointID = source.StartPointID
		}
		if i == len(params)-2 {
			e.EndPointID = source.EndPointID
		}
		outputs = append(outputs, e)
	}
	constraints := []SketchConstraint{}
	for _, c := range sketch.Constraints {
		if detached[c.ID] {
			continue
		}
		clone := c
		clone.References = append([]SketchGeometryRef(nil), c.References...)
		for i, r := range clone.References {
			if r.Target != "ENTITY" || r.EntityID != source.ID {
				continue
			}
			if r.SubElement == "END" {
				clone.References[i].EntityID = outputs[len(outputs)-1].ID
			}
			if r.SubElement == "WHOLE" || r.SubElement == "DIRECTION" {
				return fmt.Errorf("%w: split changes whole-curve constraint %s; explicitly release it", ErrValidation, c.ID)
			}
		}
		constraints = append(constraints, clone)
	}
	for i := 1; i < len(outputs); i++ {
		constraints = append(constraints, SketchConstraint{ID: macroID(op.OperationID, fmt.Sprintf("join/%d", i)), Kind: "COINCIDENT", Internal: true, References: []SketchGeometryRef{{Target: "ENTITY", EntityID: outputs[i-1].ID, SubElement: "END"}, {Target: "ENTITY", EntityID: outputs[i].ID, SubElement: "START"}}})
	}
	for i := 1; i < len(outputs); i++ {
		refs := []SketchGeometryRef{{Target: "ENTITY", EntityID: outputs[0].ID, SubElement: "WHOLE"}, {Target: "ENTITY", EntityID: outputs[i].ID, SubElement: "WHOLE"}}
		if source.Kind == "LINE" {
			constraints = append(constraints, SketchConstraint{ID: macroID(op.OperationID, fmt.Sprintf("support/%d", i)), Kind: "PARALLEL", Internal: true, References: refs})
		} else {
			constraints = append(constraints, SketchConstraint{ID: macroID(op.OperationID, fmt.Sprintf("center/%d", i)), Kind: "CONCENTRIC", Internal: true, References: refs}, SketchConstraint{ID: macroID(op.OperationID, fmt.Sprintf("shape/%d", i)), Kind: "EQUAL", Internal: true, References: refs})
		}
	}
	for i, e := range sketch.Entities {
		if e.ID == source.ID {
			sketch.Entities[i] = outputs[0]
			break
		}
	}
	sketch.Entities = append(sketch.Entities, outputs[1:]...)
	sketch.Constraints = constraints
	return nil
}

func sketchMirrorSelfMapped(e SketchEntity, transform func(SketchPoint2) SketchPoint2, axisAngle float64) bool {
	same := func(a, b SketchPoint2) bool { return math.Hypot(a.X-b.X, a.Y-b.Y) <= profileTolerance }
	switch e.Kind {
	case "POINT":
		return same(*e.Point, transform(*e.Point))
	case "LINE":
		return (same(*e.Start, transform(*e.Start)) && same(*e.End, transform(*e.End))) || (same(*e.Start, transform(*e.End)) && same(*e.End, transform(*e.Start)))
	case "CIRCLE":
		return same(*e.Center, transform(*e.Center))
	case "ELLIPSE":
		return same(*e.Center, transform(*e.Center)) && math.Abs(math.Sin(2*(axisAngle-e.Rotation))) <= 1e-12
	case "ARC", "ELLIPTICAL_ARC":
		if e.Kind == "ELLIPTICAL_ARC" && math.Abs(math.Sin(2*(axisAngle-e.Rotation))) > 1e-12 {
			return false
		}
		a, b, ok := entityProfileEndpoints(e)
		return ok && same(*e.Center, transform(*e.Center)) && same(a, transform(b)) && same(b, transform(a))
	case "SPLINE":
		points := e.ControlPoints
		if e.Mode == "CONTROL" {
			points = e.Poles
		}
		if len(points) == 0 {
			return false
		}
		direct, reverse := true, true
		for i, p := range points {
			direct = direct && same(p, transform(p))
			reverse = reverse && same(p, transform(points[len(points)-1-i]))
		}
		if reverse && e.Mode == "CONTROL" {
			reverse = sketchSplineReflectionBasisSymmetric(e)
		}
		return direct || reverse
	}
	return false
}

func sketchSelfMirrorMode(e SketchEntity, transform func(SketchPoint2) SketchPoint2, axisAngle float64) string {
	switch e.Kind {
	case "POINT", "CIRCLE":
		return "ON_AXIS"
	case "ELLIPSE", "ELLIPTICAL_ARC":
		if math.Abs(math.Sin(axisAngle-e.Rotation)) <= 1e-12 {
			return "MAJOR_PARALLEL"
		}
		return "MAJOR_PERPENDICULAR"
	case "LINE":
		if math.Hypot(transform(*e.Start).X-e.Start.X, transform(*e.Start).Y-e.Start.Y) <= profileTolerance {
			return "ON_AXIS"
		}
		return "PAIRED"
	case "ARC":
		return "PAIRED"
	case "SPLINE":
		points := e.ControlPoints
		if e.Mode == "CONTROL" {
			points = e.Poles
		}
		direct := true
		for _, p := range points {
			q := transform(p)
			direct = direct && math.Hypot(q.X-p.X, q.Y-p.Y) <= profileTolerance
		}
		if direct {
			return "ON_AXIS"
		}
		return "PAIRED"
	}
	return ""
}
func sketchSplineReflectionBasisSymmetric(e SketchEntity) bool {
	if len(e.Weights) != 0 && len(e.Weights) != len(e.Poles) {
		return false
	}
	for i, w := range e.Weights {
		other := e.Weights[len(e.Weights)-1-i]
		if math.Abs(w-other) > 1e-12*math.Max(1, math.Max(w, other)) {
			return false
		}
	}
	if len(e.Knots) != len(e.Multiplicities) || len(e.Knots) < 2 {
		return false
	}
	sum := e.ParameterStart + e.ParameterEnd
	for i, k := range e.Knots {
		j := len(e.Knots) - 1 - i
		if e.Multiplicities[i] != e.Multiplicities[j] || math.Abs(k+e.Knots[j]-sum) > 1e-12*math.Max(1, math.Abs(sum)) {
			return false
		}
	}
	return true
}
