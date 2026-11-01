package workspace

import (
	"fmt"
	"math"

	"github.com/occccad/occccad/internal/modelcore"
)

type referenceParameterSlot struct {
	slot, label, unit string
	value             float64
}

func normalizeReferenceConstructions(model *PartModel) {
	if model.OriginVisible != nil && !*model.OriginVisible {
		for i := range model.DatumPlanes {
			model.DatumPlanes[i].Visible = boolPointer(false)
		}
		for i := range model.DatumAxes {
			model.DatumAxes[i].Visible = boolPointer(false)
		}
		for i := range model.AxisSystems {
			model.AxisSystems[i].Visible = boolPointer(false)
		}
		model.OriginVisible = nil
	}
	for i := range model.DatumPlanes {
		p := &model.DatumPlanes[i]
		if !isStandardDatumPlane(p.ID) && p.Definition == nil && p.Construction == nil {
			p.Construction = &ReferenceConstruction{Mode: "PLANE_COORDINATES", Coordinates: p.Origin, Direction: p.Normal}
		}
	}
	for i := range model.DatumAxes {
		p := &model.DatumAxes[i]
		if p.Definition == nil && p.Construction == nil {
			p.Construction = &ReferenceConstruction{Mode: "LINE_COORDINATES", Coordinates: p.Origin, Direction: p.Direction}
		}
	}
}

func referenceParameterSlots(c *ReferenceConstruction) []referenceParameterSlot {
	if c == nil {
		return nil
	}
	out := []referenceParameterSlot{}
	add := func(slot, label, unit string, value float64) {
		out = append(out, referenceParameterSlot{slot, label, unit, value})
	}
	switch c.Mode {
	case "POINT_COORDINATES", "LINE_COORDINATES", "PLANE_COORDINATES":
		for i, axis := range []string{"x", "y", "z"} {
			add("reference:"+axis, axis, "mm", c.Coordinates[i])
		}
		if c.Mode != "POINT_COORDINATES" {
			for i, axis := range []string{"x", "y", "z"} {
				add("reference:d"+axis, "方向 "+axis, "", c.Direction[i])
			}
		}
	case "POINT_BETWEEN":
		add("reference:ratio", "比例", "", c.Ratio)
	case "POINT_ON_PLANE":
		add("reference:u", "U", "mm", c.Coordinates[0])
		add("reference:v", "V", "mm", c.Coordinates[1])
	case "PLANE_OFFSET":
		add("reference:offset", "偏移", "mm", c.Offset)
	case "PLANE_ANGLE":
		add("reference:angle", "角度", "deg", c.Angle)
	case "PLANE_EQUATION":
		for i, axis := range []string{"a", "b", "c", "d"} {
			unit := ""
			if i == 3 {
				unit = "mm"
			}
			add("reference:"+axis, axis, unit, c.Coefficients[i])
		}
	}
	return out
}

func referenceConstructions(model *PartModel) map[string]*ReferenceConstruction {
	out := map[string]*ReferenceConstruction{}
	for _, p := range model.DatumPoints {
		if p.Construction != nil {
			out[p.ID] = p.Construction
		}
	}
	for _, p := range model.DatumAxes {
		if p.Construction != nil {
			out[p.ID] = p.Construction
		}
	}
	for _, p := range model.DatumPlanes {
		if p.Construction != nil {
			out[p.ID] = p.Construction
		}
	}
	return out
}

func referenceParameterSources(model PartModel, c *ReferenceConstruction, expressions map[string]string) (map[string]modelcore.ValueSource, error) {
	bindings := map[string]modelcore.ParameterBinding{}
	for _, p := range model.Parameters {
		bindings[p.Key] = modelcore.ParameterBinding{ParameterID: p.ParameterID, Dimension: p.Dimension}
	}
	sources := map[string]modelcore.ValueSource{}
	for _, slot := range referenceParameterSlots(c) {
		q, err := modelcore.NewQuantity(slot.value, slot.unit)
		if err != nil {
			return nil, err
		}
		sources[slot.slot] = modelcore.ValueSource{Literal: &q}
		if source := expressions[slot.slot]; source != "" {
			expression, err := modelcore.CompileExpression(source, bindings, q.Dimension, slot.unit)
			if err != nil {
				return nil, fmt.Errorf("%w: %w", ErrValidation, err)
			}
			sources[slot.slot] = modelcore.ValueSource{Expression: &expression}
		}
	}
	for key := range expressions {
		if _, ok := sources[key]; !ok || expressions[key] == "" {
			return nil, fmt.Errorf("%w: unknown or empty reference parameter slot %s", ErrValidation, key)
		}
	}
	return sources, nil
}

func applyReferenceParameterValues(model *PartModel, values map[string]modelcore.Quantity) error {
	blocked := blockedParameterOwners(*model)
	for id, c := range referenceConstructions(model) {
		if blocked[id] {
			continue
		}
		read := func(slot, unit string) (float64, error) {
			value, ok := values["parameter:"+id+":"+slot]
			if !ok {
				return 0, fmt.Errorf("%w: PARAMETER_SOURCE_NOT_CURRENT: %s", ErrValidation, id+"/"+slot)
			}
			return quantityInDisplayUnit(value, unit)
		}
		for _, p := range referenceParameterSlots(c) {
			value, err := read(p.slot, p.unit)
			if err != nil {
				return err
			}
			switch p.slot {
			case "reference:x":
				c.Coordinates[0] = value
			case "reference:y":
				c.Coordinates[1] = value
			case "reference:z":
				c.Coordinates[2] = value
			case "reference:dx":
				c.Direction[0] = value
			case "reference:dy":
				c.Direction[1] = value
			case "reference:dz":
				c.Direction[2] = value
			case "reference:u":
				c.Coordinates[0] = value
			case "reference:v":
				c.Coordinates[1] = value
			case "reference:ratio":
				c.Ratio = value
			case "reference:offset":
				c.Offset = value
			case "reference:angle":
				c.Angle = value
			case "reference:a":
				c.Coefficients[0] = value
			case "reference:b":
				c.Coefficients[1] = value
			case "reference:c":
				c.Coefficients[2] = value
			case "reference:d":
				c.Coefficients[3] = value
			}
		}
	}
	return nil
}

func constructReference(c *ReferenceConstruction, resolve func(DatumReference) (datumFrame, error)) (datumFrame, error) {
	bad := func() (datumFrame, error) {
		return datumFrame{}, fmt.Errorf("%w: REFERENCE_CONSTRUCTION_INVALID: %s", ErrValidation, c.Mode)
	}
	counts := map[string]int{"POINT_COORDINATES": 0, "POINT_BETWEEN": 2, "POINT_CENTER": 1, "POINT_ON_PLANE": 1, "LINE_COORDINATES": 0, "LINE_TWO_POINTS": 2, "LINE_POINT_DIRECTION": 2, "LINE_NORMAL_PLANE": 2, "PLANE_COORDINATES": 0, "PLANE_OFFSET": 1, "PLANE_PARALLEL_POINT": 2, "PLANE_THREE_POINTS": 3, "PLANE_ANGLE": 2, "PLANE_EQUATION": 0}
	count, known := counts[c.Mode]
	if !known || len(c.Inputs) != count {
		return bad()
	}
	for _, p := range referenceParameterSlots(c) {
		if !finite(p.value) {
			return bad()
		}
	}
	inputs := []datumFrame{}
	for _, ref := range c.Inputs {
		frame, err := resolve(ref)
		if err != nil {
			return datumFrame{}, err
		}
		inputs = append(inputs, frame)
	}
	point := func(p [3]float64) (datumFrame, error) { return datumFrame{origin: p, point: true}, nil }
	line := func(p, d [3]float64) (datumFrame, error) {
		direction, ok := normalize3(d)
		if !ok {
			return bad()
		}
		return datumFrame{origin: p, direction: direction}, nil
	}
	plane := func(p, n [3]float64) (datumFrame, error) {
		normal, ok := normalize3(n)
		if !ok {
			return bad()
		}
		u, ok := stableSupportX(normal, [3]float64{1, 0, 0})
		if !ok {
			return bad()
		}
		return datumFrame{origin: p, direction: normal, u: u, plane: true}, nil
	}
	sub := func(a, b [3]float64) (out [3]float64) {
		for i := range a {
			out[i] = a[i] - b[i]
		}
		return
	}
	switch c.Mode {
	case "POINT_COORDINATES":
		if len(inputs) != 0 {
			return bad()
		}
		return point(c.Coordinates)
	case "POINT_BETWEEN":
		if len(inputs) != 2 || !inputs[0].point || !inputs[1].point {
			return bad()
		}
		p := inputs[0].origin
		for i := range p {
			p[i] += (inputs[1].origin[i] - p[i]) * c.Ratio
		}
		return point(p)
	case "POINT_CENTER":
		if len(inputs) != 1 || !inputs[0].point {
			return bad()
		}
		return point(inputs[0].origin)
	case "POINT_ON_PLANE":
		if len(inputs) != 1 || !inputs[0].plane {
			return bad()
		}
		p := inputs[0].origin
		v := patternCross(inputs[0].direction, inputs[0].u)
		for i := range p {
			p[i] += inputs[0].u[i]*c.Coordinates[0] + v[i]*c.Coordinates[1]
		}
		return point(p)
	case "LINE_COORDINATES":
		if len(inputs) != 0 {
			return bad()
		}
		return line(c.Coordinates, c.Direction)
	case "LINE_TWO_POINTS":
		if len(inputs) != 2 || !inputs[0].point || !inputs[1].point {
			return bad()
		}
		return line(inputs[0].origin, sub(inputs[1].origin, inputs[0].origin))
	case "LINE_POINT_DIRECTION":
		if len(inputs) != 2 || !inputs[0].point || inputs[1].plane || inputs[1].point {
			return bad()
		}
		return line(inputs[0].origin, inputs[1].direction)
	case "LINE_NORMAL_PLANE":
		if len(inputs) != 2 || !inputs[0].point || !inputs[1].plane {
			return bad()
		}
		return line(inputs[0].origin, inputs[1].direction)
	case "PLANE_COORDINATES":
		if len(inputs) != 0 {
			return bad()
		}
		return plane(c.Coordinates, c.Direction)
	case "PLANE_OFFSET":
		if len(inputs) != 1 || !inputs[0].plane {
			return bad()
		}
		p := inputs[0].origin
		for i := range p {
			p[i] += inputs[0].direction[i] * c.Offset
		}
		return plane(p, inputs[0].direction)
	case "PLANE_PARALLEL_POINT":
		if len(inputs) != 2 || !inputs[0].plane || !inputs[1].point {
			return bad()
		}
		return plane(inputs[1].origin, inputs[0].direction)
	case "PLANE_THREE_POINTS":
		if len(inputs) != 3 || !inputs[0].point || !inputs[1].point || !inputs[2].point {
			return bad()
		}
		return plane(inputs[0].origin, patternCross(sub(inputs[1].origin, inputs[0].origin), sub(inputs[2].origin, inputs[0].origin)))
	case "PLANE_ANGLE":
		if len(inputs) != 2 || !inputs[0].plane || inputs[1].plane || inputs[1].point {
			return bad()
		}
		direction, ok := normalize3(inputs[1].direction)
		if !ok {
			return bad()
		}
		angle := c.Angle * math.Pi / 180
		p := datumRotate(sub(inputs[0].origin, inputs[1].origin), direction, angle)
		for i := range p {
			p[i] += inputs[1].origin[i]
		}
		return plane(p, datumRotate(inputs[0].direction, direction, angle))
	case "PLANE_EQUATION":
		if len(inputs) != 0 {
			return bad()
		}
		n := [3]float64{c.Coefficients[0], c.Coefficients[1], c.Coefficients[2]}
		den := dot3(n, n)
		if !finite(den) || den < 1e-18 {
			return bad()
		}
		p := n
		for i := range p {
			p[i] *= c.Coefficients[3] / den
		}
		return plane(p, n)
	}
	return bad()
}
