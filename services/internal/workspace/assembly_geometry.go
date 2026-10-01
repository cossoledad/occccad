package workspace

import (
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
	"math"
)

// Values originate in frozen precise BREP/Datum descriptors. Coordinates and
// radii are millimetres; scalar Quantity conversion happens before this layer.
func assemblyGeometryFromProperties(value geometry.AssemblyGeometry, p TopologyElementProperties) (geometry.AssemblyGeometry, error) {
	fail := func(reason string) (geometry.AssemblyGeometry, error) {
		return value, fmt.Errorf("%w: ASSEMBLY_DESCRIPTOR_MISSING: %s", ErrValidation, reason)
	}
	value.LengthUnit = "mm"
	if unit, ok := p.Properties["lengthUnit"].(string); ok && unit != "mm" {
		return fail("precise topology lengthUnit must be mm")
	}
	vector := func(name string) ([3]float64, bool) {
		v, ok := p.Properties[name].([3]float64)
		return v, ok && assemblyFiniteVector(v)
	}
	number := func(name string) (float64, bool) {
		v, ok := p.Properties[name].(float64)
		return v, ok && !math.IsNaN(v) && !math.IsInf(v, 0)
	}
	side := func() (int32, bool) {
		switch s := p.Properties["materialSide"].(type) {
		case int64:
			return int32(s), s == 1 || s == -1
		case int32:
			return s, s == 1 || s == -1
		case float64:
			return int32(s), s == 1 || s == -1
		}
		return 0, false
	}
	if p.GeometryType == "POINT" || p.Kind == "VERTEX" {
		if p.Point == nil || !assemblyFiniteVector(*p.Point) {
			return fail("point position")
		}
		value.Kind = "POINT"
		value.Origin = *p.Point
		return value, nil
	}
	var ok bool
	switch p.GeometryType {
	case "LINE":
		value.Kind = "AXIS"
		value.Origin, ok = vector("origin")
		if !ok {
			return fail("line origin")
		}
		value.Direction, ok = vector("direction")
	case "PLANE":
		value.Kind = "PLANE"
		value.Origin, ok = vector("origin")
		if !ok {
			return fail("plane origin")
		}
		value.Direction, ok = vector("normal")
	case "CYLINDER":
		value.Kind = "CYLINDER"
		value.Origin, ok = vector("origin")
		if !ok {
			return fail("cylinder axis origin")
		}
		value.Direction, ok = vector("axis")
	case "CIRCLE":
		value.Kind = "CIRCLE"
		value.Origin, ok = vector("center")
		if !ok {
			return fail("circle center")
		}
		value.Direction, ok = vector("normal")
		value.XDirection, _ = vector("xDirection")
		if !assemblyUnitVector(value.XDirection) || math.Abs(assemblyDot(value.Direction, value.XDirection)) > 1e-9 {
			return fail("circle oriented orthogonal frame")
		}
	case "SPHERE":
		value.Kind = "SPHERE"
		value.Origin, ok = vector("center")
		if !ok {
			return fail("sphere center")
		}
	case "CONE":
		if unit, exists := p.Properties["angleUnit"].(string); exists && unit != "rad" {
			return fail("cone angleUnit must be rad")
		}
		value.Kind = "CONE"
		value.Origin, ok = vector("apex")
		if !ok {
			return fail("cone apex")
		}
		value.Direction, ok = vector("axis")
		angle, valid := number("semiAngle")
		if !valid || math.Abs(angle) <= 0 || math.Abs(angle) >= math.Pi/2 {
			return fail("cone half angle")
		}
		value.HalfAngle = math.Abs(angle)
		leaf, valid := number("coneLeaf")
		if valid {
			if leaf != 1 && leaf != -1 {
				return fail("cone leaf")
			}
			value.ConeLeaf = int32(leaf)
		} else {
			switch l := p.Properties["coneLeaf"].(type) {
			case int64:
				if l != 1 && l != -1 {
					return fail("cone leaf")
				}
				value.ConeLeaf = int32(l)
			case int32:
				value.ConeLeaf = l
			}
		}
		if value.ConeLeaf != 1 && value.ConeLeaf != -1 {
			return fail("cone leaf")
		}
	default:
		return fail("unsupported exact geometry " + p.GeometryType)
	}
	if value.Kind != "SPHERE" && (!ok || !assemblyUnitVector(value.Direction)) {
		return fail("unit support direction")
	}
	if value.Kind == "CIRCLE" || value.Kind == "SPHERE" || value.Kind == "CYLINDER" {
		value.Radius, ok = number("radius")
		if !ok || value.Radius <= 0 {
			return fail("positive analytic radius")
		}
	}
	if value.Kind == "SPHERE" || value.Kind == "CONE" || value.Kind == "CYLINDER" {
		value.MaterialSide, ok = side()
		if !ok {
			return fail("material side")
		}
	}
	if value.Kind == "CIRCLE" {
		if unit, exists := p.Properties["parameterUnit"].(string); exists && unit != "rad" {
			return fail("circle parameterUnit must be rad")
		}
		start, validStart := number("firstParameter")
		end, validEnd := number("lastParameter")
		if !validStart || !validEnd || end <= start || end-start > 2*math.Pi+1e-9 {
			return fail("circle radian parameter range")
		}
		value.ParameterStart = &start
		value.ParameterEnd = &end
	}
	return value, nil
}
func assemblyFiniteVector(v [3]float64) bool {
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return false
		}
	}
	return true
}

func assemblyGeometryFromPublication(value geometry.AssemblyGeometry, r PublicationResolution) (geometry.AssemblyGeometry, error) {
	if r.LengthUnit != "" && r.LengthUnit != "mm" {
		return value, fmt.Errorf("%w: Publication descriptor lengthUnit must be mm", ErrValidation)
	}
	if value.Kind == "FRAME" {
		return assemblyDatumFrame(value, AxisSystem{Origin: r.Origin, XDirection: r.XDirection, YDirection: r.YDirection, ZDirection: r.ZDirection})
	}
	if value.Kind == "POINT" || value.Kind == "VERTEX" {
		if !assemblyFiniteVector(r.Origin) {
			return value, fmt.Errorf("%w: invalid Publication point", ErrValidation)
		}
		value.Kind = "POINT"
		value.Origin = r.Origin
		value.LengthUnit = "mm"
		return value, nil
	}
	if value.Kind == "AXIS" || value.Kind == "PLANE" {
		value.Origin = r.Origin
		value.Direction = r.ZDirection
		value.LengthUnit = "mm"
		if !assemblyFiniteVector(value.Origin) || !assemblyUnitVector(value.Direction) {
			return value, fmt.Errorf("%w: invalid Publication support frame", ErrValidation)
		}
		return value, nil
	}
	p := TopologyElementProperties{GeometryType: r.GeometryKind, Properties: map[string]any{"origin": r.Origin, "center": r.Origin, "apex": r.Origin, "direction": r.ZDirection, "normal": r.ZDirection, "axis": r.ZDirection, "xDirection": r.XDirection, "radius": r.Radius, "semiAngle": r.HalfAngle, "coneLeaf": r.ConeLeaf, "materialSide": r.MaterialSide}}
	if r.LengthUnit != "" {
		p.Properties["lengthUnit"] = r.LengthUnit
	}
	if r.ParameterStart != nil {
		p.Properties["firstParameter"] = *r.ParameterStart
	}
	if r.ParameterEnd != nil {
		p.Properties["lastParameter"] = *r.ParameterEnd
	}
	return assemblyGeometryFromProperties(value, p)
}
func assemblyDot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func assemblyUnitVector(v [3]float64) bool {
	return assemblyFiniteVector(v) && math.Abs(assemblyDot(v, v)-1) <= 1e-9
}

// Derived supports preserve their source identity. Role only selects an
// explicitly named mathematical subelement; no implicit Frame/Surface cast.
func assemblyDerivedGeometry(value geometry.AssemblyGeometry, role string) (geometry.AssemblyGeometry, error) {
	if role == "" {
		return value, nil
	}
	switch role {
	case "underlying-circle":
		if value.Kind == "CIRCLE" {
			return value, nil
		}
	case "circle-axis":
		if value.Kind == "CIRCLE" {
			value.Kind = "AXIS"
			return value, nil
		}
	case "circle-plane":
		if value.Kind == "CIRCLE" {
			value.Kind = "PLANE"
			return value, nil
		}
	case "sphere-center":
		if value.Kind == "SPHERE" {
			value.Kind = "POINT"
			return value, nil
		}
	case "circle-center":
		if value.Kind == "CIRCLE" {
			value.Kind = "POINT"
			return value, nil
		}
	case "cone-apex":
		if value.Kind == "CONE" {
			value.Kind = "POINT"
			return value, nil
		}
	case "cylinder-axis":
		if value.Kind == "CYLINDER" {
			value.Kind = "AXIS"
			return value, nil
		}
	case "cone-axis":
		if value.Kind == "CONE" {
			value.Kind = "AXIS"
			return value, nil
		}
	case "frame-origin":
		if value.Kind == "FRAME" {
			value.Kind = "POINT"
			return value, nil
		}
	case "frame-axis-x", "frame-axis-y", "frame-axis-z", "frame-plane-xy", "frame-plane-yz", "frame-plane-zx":
		if value.Kind == "FRAME" {
			axis := [3]float64{0, 0, 1}
			switch role {
			case "frame-axis-x", "frame-plane-yz":
				axis = [3]float64{1, 0, 0}
			case "frame-axis-y", "frame-plane-zx":
				axis = [3]float64{0, 1, 0}
			}
			value.Direction = rotateByPose(InstancePose{Rotation: value.Rotation}, axis)
			value.Kind = "AXIS"
			if role == "frame-plane-xy" || role == "frame-plane-yz" || role == "frame-plane-zx" {
				value.Kind = "PLANE"
			}
			return value, nil
		}
	}
	return value, fmt.Errorf("%w: derived role %s is incompatible with %s", ErrValidation, role, value.Kind)
}
func assemblyDatumFrame(value geometry.AssemblyGeometry, system AxisSystem) (geometry.AssemblyGeometry, error) {
	x, y, z := system.XDirection, system.YDirection, system.ZDirection
	cross := [3]float64{x[1]*y[2] - x[2]*y[1], x[2]*y[0] - x[0]*y[2], x[0]*y[1] - x[1]*y[0]}
	if !assemblyFiniteVector(system.Origin) || !assemblyUnitVector(x) || !assemblyUnitVector(y) || !assemblyUnitVector(z) || math.Abs(assemblyDot(x, y)) > 1e-9 || assemblyDot(cross, z) < 1-1e-9 {
		return value, fmt.Errorf("%w: Frame must be right-handed and orthonormal", ErrValidation)
	}
	// Matrix columns are the persistent local axes, quaternion is xyzw.
	trace := x[0] + y[1] + z[2]
	q := [4]float64{}
	if trace > 0 {
		s := 2 * math.Sqrt(trace+1)
		q = [4]float64{(y[2] - z[1]) / s, (z[0] - x[2]) / s, (x[1] - y[0]) / s, s / 4}
	} else if x[0] > y[1] && x[0] > z[2] {
		s := 2 * math.Sqrt(1+x[0]-y[1]-z[2])
		q = [4]float64{s / 4, (y[0] + x[1]) / s, (z[0] + x[2]) / s, (y[2] - z[1]) / s}
	} else if y[1] > z[2] {
		s := 2 * math.Sqrt(1+y[1]-x[0]-z[2])
		q = [4]float64{(y[0] + x[1]) / s, s / 4, (z[1] + y[2]) / s, (z[0] - x[2]) / s}
	} else {
		s := 2 * math.Sqrt(1+z[2]-x[0]-y[1])
		q = [4]float64{(z[0] + x[2]) / s, (z[1] + y[2]) / s, s / 4, (x[1] - y[0]) / s}
	}
	value.Kind = "FRAME"
	value.Origin = system.Origin
	value.Rotation = normalizedInstanceRotation(q)
	value.XDirection = x
	value.Direction = z
	value.LengthUnit = "mm"
	return value, nil
}
