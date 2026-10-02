package workspace

import "github.com/occccad/occccad/internal/geometry"

func assemblySnapHints(properties TopologyElementProperties, pose InstancePose) *AssemblySnapHints {
	hints := &AssemblySnapHints{}
	point := func(key string) *[3]float64 {
		v, ok := properties.Properties[key].([3]float64)
		if !ok || !assemblyFiniteVector(v) {
			return nil
		}
		mapped := assemblyGeometryInBody(geometry.AssemblyGeometry{Origin: v}, pose).Origin
		return &mapped
	}
	hints.Center = point("snapCenter")
	hints.EndFirst, hints.EndLast = point("snapEndFirst"), point("snapEndLast")
	if v, ok := properties.Properties["snapBoundaryDirection"].([3]float64); ok && assemblyUnitVector(v) {
		mapped := assemblyGeometryInBody(geometry.AssemblyGeometry{Direction: v}, pose).Direction
		hints.BoundaryDirection = &mapped
	}
	if properties.GeometryType == "LINE" {
		origin, o := properties.Properties["origin"].([3]float64)
		direction, d := properties.Properties["direction"].([3]float64)
		first, f := properties.Properties["firstParameter"].(float64)
		last, l := properties.Properties["lastParameter"].(float64)
		if o && d && f && l && assemblyUnitVector(direction) {
			endpoint := func(t float64) *[3]float64 {
				v := origin
				for i := range v {
					v[i] += direction[i] * t
				}
				if !assemblyFiniteVector(v) {
					return nil
				}
				v = assemblyGeometryInBody(geometry.AssemblyGeometry{Origin: v}, pose).Origin
				return &v
			}
			hints.EndFirst, hints.EndLast, hints.Center = endpoint(first), endpoint(last), endpoint((first+last)/2)
		}
	}
	return hints
}
