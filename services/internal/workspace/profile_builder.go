package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
)

const profileTolerance = 1e-6

type profileEdge struct {
	entity             SketchEntity
	start, end         SketchPoint2
	startNode, endNode string
}
type profileLoop struct {
	value   geometry.ProfileLoop
	polygon []SketchPoint2
	area    float64
}
type disjointSet struct{ parent map[string]string }

func newDisjointSet() *disjointSet { return &disjointSet{parent: map[string]string{}} }
func (d *disjointSet) find(value string) string {
	parent, ok := d.parent[value]
	if !ok {
		d.parent[value] = value
		return value
	}
	if parent != value {
		d.parent[value] = d.find(parent)
	}
	return d.parent[value]
}
func (d *disjointSet) union(a, b string) {
	a, b = d.find(a), d.find(b)
	if a != b {
		if a > b {
			a, b = b, a
		}
		d.parent[b] = a
	}
}

func profileReferenceNode(reference SketchGeometryRef) (string, bool) {
	if reference.Target == "SKETCH_ORIGIN" && reference.SubElement == "POINT" {
		return "origin", true
	}
	if reference.Target != "ENTITY" {
		return "", false
	}
	switch reference.SubElement {
	case "POINT", "START", "END":
		return reference.EntityID + "/" + reference.SubElement, true
	}
	return "", false
}

func entityProfileEndpoints(entity SketchEntity) (SketchPoint2, SketchPoint2, bool) {
	switch entity.Kind {
	case "LINE":
		if entity.Start != nil && entity.End != nil {
			return *entity.Start, *entity.End, true
		}
	case "ARC":
		if entity.Center != nil {
			return SketchPoint2{X: entity.Center.X + entity.Radius*math.Cos(entity.StartAngle), Y: entity.Center.Y + entity.Radius*math.Sin(entity.StartAngle)}, SketchPoint2{X: entity.Center.X + entity.Radius*math.Cos(entity.EndAngle), Y: entity.Center.Y + entity.Radius*math.Sin(entity.EndAngle)}, true
		}
	case "SPLINE":
		if !entity.Closed && len(entity.ControlPoints) > 1 {
			return entity.ControlPoints[0], entity.ControlPoints[len(entity.ControlPoints)-1], true
		}
	}
	return SketchPoint2{}, SketchPoint2{}, false
}

func buildProfileRegions(feature Feature) ([]geometry.ProfileRegion, error) {
	if feature.Sketch == nil {
		return nil, fmt.Errorf("%w: sketch model is missing", ErrValidation)
	}
	dsu := newDisjointSet()
	entities := map[string]SketchEntity{}
	for _, entity := range feature.Sketch.Entities {
		entities[entity.ID] = entity
	}
	for _, constraint := range feature.Sketch.Constraints {
		if constraint.Kind != "COINCIDENT" || len(constraint.References) != 2 {
			continue
		}
		a, aok := profileReferenceNode(constraint.References[0])
		b, bok := profileReferenceNode(constraint.References[1])
		if aok && bok {
			dsu.union(a, b)
		}
	}
	edges := []profileEdge{}
	loops := []profileLoop{}
	for _, entity := range feature.Sketch.Entities {
		if entity.Role != "PROFILE" || entity.Kind == "POINT" {
			continue
		}
		if entity.Kind == "CIRCLE" || (entity.Kind == "SPLINE" && entity.Closed) {
			loop, err := closedEntityLoop(entity)
			if err != nil {
				return nil, err
			}
			loops = append(loops, loop)
			continue
		}
		start, end, ok := entityProfileEndpoints(entity)
		if !ok {
			return nil, fmt.Errorf("%w: unsupported open profile entity %s", ErrValidation, entity.ID)
		}
		edges = append(edges, profileEdge{entity: entity, start: start, end: end, startNode: dsu.find(entity.ID + "/START"), endNode: dsu.find(entity.ID + "/END")})
	}
	if len(edges) == 0 && len(loops) == 0 {
		return nil, fmt.Errorf("%w: sketch has no closed PROFILE curves", ErrValidation)
	}
	adjacency := map[string][]int{}
	nodePoints := map[string]SketchPoint2{}
	for index, edge := range edges {
		for _, endpoint := range []struct {
			node  string
			point SketchPoint2
		}{{edge.startNode, edge.start}, {edge.endNode, edge.end}} {
			if prior, ok := nodePoints[endpoint.node]; ok && math.Hypot(prior.X-endpoint.point.X, prior.Y-endpoint.point.Y) > profileTolerance {
				return nil, fmt.Errorf("%w: coincident profile endpoints did not solve to the same point", ErrValidation)
			}
			nodePoints[endpoint.node] = endpoint.point
		}
		adjacency[edge.startNode] = append(adjacency[edge.startNode], index)
		adjacency[edge.endNode] = append(adjacency[edge.endNode], index)
	}
	for node, incident := range adjacency {
		if len(incident) != 2 {
			return nil, fmt.Errorf("%w: profile is open or has a T-junction at %s (degree %d)", ErrValidation, node, len(incident))
		}
	}
	visited := make([]bool, len(edges))
	for {
		seed := -1
		for index, edge := range edges {
			if !visited[index] && (seed < 0 || edge.entity.ID < edges[seed].entity.ID) {
				seed = index
			}
		}
		if seed < 0 {
			break
		}
		startNode := edges[seed].startNode
		if edges[seed].endNode < startNode {
			startNode = edges[seed].endNode
		}
		current := startNode
		ordered := []geometry.ProfileCurve{}
		for {
			candidate := -1
			for _, index := range adjacency[current] {
				if !visited[index] && (candidate < 0 || edges[index].entity.ID < edges[candidate].entity.ID) {
					candidate = index
				}
			}
			if candidate < 0 {
				if current == startNode {
					break
				}
				return nil, fmt.Errorf("%w: profile traversal ended before closing", ErrValidation)
			}
			edge := edges[candidate]
			reversed := edge.endNode == current
			ordered = append(ordered, profileCurve(edge.entity, reversed))
			visited[candidate] = true
			if reversed {
				current = edge.startNode
			} else {
				current = edge.endNode
			}
		}
		loop, err := makeProfileLoop(ordered)
		if err != nil {
			return nil, err
		}
		loops = append(loops, loop)
	}
	return classifyProfileLoops(loops)
}

func profileCurve(entity SketchEntity, reversed bool) geometry.ProfileCurve {
	value := geometry.ProfileCurve{EntityID: entity.ID, Kind: entity.Kind, Reversed: reversed, Radius: entity.Radius, StartAngle: entity.StartAngle, EndAngle: entity.EndAngle, Degree: entity.Degree, Closed: entity.Closed}
	if entity.Start != nil {
		value.Start = [2]float64{entity.Start.X, entity.Start.Y}
	}
	if entity.End != nil {
		value.End = [2]float64{entity.End.X, entity.End.Y}
	}
	if entity.Center != nil {
		value.Center = [2]float64{entity.Center.X, entity.Center.Y}
	}
	for _, point := range entity.ControlPoints {
		value.ControlPoints = append(value.ControlPoints, [2]float64{point.X, point.Y})
	}
	return value
}
func closedEntityLoop(entity SketchEntity) (profileLoop, error) {
	return makeProfileLoop([]geometry.ProfileCurve{profileCurve(entity, false)})
}
func loopID(curves []geometry.ProfileCurve) string {
	parts := make([]string, len(curves))
	for i, curve := range curves {
		direction := "+"
		if curve.Reversed {
			direction = "-"
		}
		parts[i] = curve.EntityID + direction
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "profile-loop:" + hex.EncodeToString(digest[:12])
}
func makeProfileLoop(curves []geometry.ProfileCurve) (profileLoop, error) {
	value := geometry.ProfileLoop{ID: loopID(curves), Curves: curves}
	polygon := sampleProfileLoop(value)
	area := polygonArea(polygon)
	if math.Abs(area) < profileTolerance {
		return profileLoop{}, fmt.Errorf("%w: profile loop %s has no area", ErrValidation, value.ID)
	}
	if selfIntersects(polygon) {
		return profileLoop{}, fmt.Errorf("%w: profile loop %s self-intersects", ErrValidation, value.ID)
	}
	return profileLoop{value: value, polygon: polygon, area: area}, nil
}

func sampleProfileLoop(loop geometry.ProfileLoop) []SketchPoint2 {
	result := []SketchPoint2{}
	for _, curve := range loop.Curves {
		points := sampleProfileCurve(curve)
		if curve.Reversed {
			for left, right := 0, len(points)-1; left < right; left, right = left+1, right-1 {
				points[left], points[right] = points[right], points[left]
			}
		}
		if len(result) > 0 && len(points) > 0 {
			points = points[1:]
		}
		result = append(result, points...)
	}
	if len(result) > 0 && (math.Hypot(result[0].X-result[len(result)-1].X, result[0].Y-result[len(result)-1].Y) > profileTolerance) {
		result = append(result, result[0])
	}
	return result
}
func sampleProfileCurve(curve geometry.ProfileCurve) []SketchPoint2 {
	switch curve.Kind {
	case "LINE":
		return []SketchPoint2{{X: curve.Start[0], Y: curve.Start[1]}, {X: curve.End[0], Y: curve.End[1]}}
	case "CIRCLE", "ARC":
		start, end := 0.0, 2*math.Pi
		if curve.Kind == "ARC" {
			start, end = curve.StartAngle, curve.EndAngle
		}
		count := 64
		if curve.Kind == "ARC" {
			count = int(math.Max(8, math.Ceil(64*math.Abs(end-start)/(2*math.Pi))))
		}
		points := make([]SketchPoint2, count+1)
		for i := range points {
			angle := start + (end-start)*float64(i)/float64(count)
			points[i] = SketchPoint2{X: curve.Center[0] + curve.Radius*math.Cos(angle), Y: curve.Center[1] + curve.Radius*math.Sin(angle)}
		}
		return points
	case "SPLINE":
		points := make([]SketchPoint2, len(curve.ControlPoints), len(curve.ControlPoints)+1)
		for i, value := range curve.ControlPoints {
			points[i] = SketchPoint2{X: value[0], Y: value[1]}
		}
		if curve.Closed && len(points) > 0 {
			points = append(points, points[0])
		}
		return sampleBSpline(points, int(curve.Degree), 64)
	}
	return nil
}

func sampleBSpline(control []SketchPoint2, degree, segments int) []SketchPoint2 {
	n := len(control) - 1
	if n < 1 {
		return control
	}
	p := degree
	if p < 1 {
		p = 1
	}
	if p > n {
		p = n
	}
	maximum := n - p + 1
	knots := make([]int, n+p+2)
	for index := range knots {
		switch {
		case index <= p:
			knots[index] = 0
		case index > n:
			knots[index] = maximum
		default:
			knots[index] = index - p
		}
	}
	evaluate := func(parameter float64) SketchPoint2 {
		u := math.Min(parameter, float64(maximum)-math.SmallestNonzeroFloat64)
		span := int(math.Floor(u)) + p
		if parameter >= float64(maximum) || span > n {
			span = n
		}
		values := make([]SketchPoint2, p+1)
		copy(values, control[span-p:span+1])
		for level := 1; level <= p; level++ {
			for index := p; index >= level; index-- {
				source := span - p + index
				denominator := knots[source+p-level+1] - knots[source]
				alpha := 0.0
				if denominator != 0 {
					alpha = (u - float64(knots[source])) / float64(denominator)
				}
				values[index] = SketchPoint2{X: values[index-1].X*(1-alpha) + values[index].X*alpha, Y: values[index-1].Y*(1-alpha) + values[index].Y*alpha}
			}
		}
		return values[p]
	}
	result := make([]SketchPoint2, segments+1)
	for index := range result {
		result[index] = evaluate(float64(maximum) * float64(index) / float64(segments))
	}
	return result
}
func polygonArea(points []SketchPoint2) float64 {
	area := 0.0
	for i := 1; i < len(points); i++ {
		area += points[i-1].X*points[i].Y - points[i].X*points[i-1].Y
	}
	return area / 2
}
func orientation(a, b, c SketchPoint2) float64 { return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X) }
func segmentsIntersect(a, b, c, d SketchPoint2) bool {
	o1, o2, o3, o4 := orientation(a, b, c), orientation(a, b, d), orientation(c, d, a), orientation(c, d, b)
	if o1*o2 < -profileTolerance && o3*o4 < -profileTolerance {
		return true
	}
	onSegment := func(first, point, last SketchPoint2) bool {
		return math.Abs(orientation(first, last, point)) <= profileTolerance && point.X >= math.Min(first.X, last.X)-profileTolerance && point.X <= math.Max(first.X, last.X)+profileTolerance && point.Y >= math.Min(first.Y, last.Y)-profileTolerance && point.Y <= math.Max(first.Y, last.Y)+profileTolerance
	}
	return onSegment(a, c, b) || onSegment(a, d, b) || onSegment(c, a, d) || onSegment(c, b, d)
}
func selfIntersects(points []SketchPoint2) bool {
	segments := len(points) - 1
	for i := 0; i < segments; i++ {
		for j := i + 1; j < segments; j++ {
			if j == i+1 || (i == 0 && j == segments-1) {
				continue
			}
			if segmentsIntersect(points[i], points[i+1], points[j], points[j+1]) {
				return true
			}
		}
	}
	return false
}
func polygonsIntersect(a, b []SketchPoint2) bool {
	for i := 1; i < len(a); i++ {
		for j := 1; j < len(b); j++ {
			if segmentsIntersect(a[i-1], a[i], b[j-1], b[j]) {
				return true
			}
		}
	}
	return false
}
func pointInPolygon(point SketchPoint2, polygon []SketchPoint2) bool {
	inside := false
	for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
		a, b := polygon[i], polygon[j]
		if (a.Y > point.Y) != (b.Y > point.Y) && point.X < (b.X-a.X)*(point.Y-a.Y)/(b.Y-a.Y)+a.X {
			inside = !inside
		}
	}
	return inside
}
func reverseProfileLoop(loop geometry.ProfileLoop) geometry.ProfileLoop {
	curves := make([]geometry.ProfileCurve, len(loop.Curves))
	for i := range loop.Curves {
		curve := loop.Curves[len(loop.Curves)-1-i]
		curve.Reversed = !curve.Reversed
		curves[i] = curve
	}
	return geometry.ProfileLoop{ID: loop.ID, Curves: curves}
}
func classifyProfileLoops(loops []profileLoop) ([]geometry.ProfileRegion, error) {
	sort.Slice(loops, func(i, j int) bool { return math.Abs(loops[i].area) > math.Abs(loops[j].area) })
	parent := make([]int, len(loops))
	depth := make([]int, len(loops))
	for i := range parent {
		parent[i] = -1
	}
	for i := range loops {
		for j := 0; j < i; j++ {
			if polygonsIntersect(loops[i].polygon, loops[j].polygon) {
				return nil, fmt.Errorf("%w: profile loops intersect", ErrValidation)
			}
			if pointInPolygon(loops[i].polygon[0], loops[j].polygon) {
				parent[i] = j
			}
		}
		if parent[i] >= 0 {
			depth[i] = depth[parent[i]] + 1
		}
	}
	regions := []geometry.ProfileRegion{}
	regionForLoop := map[int]int{}
	for i, loop := range loops {
		if depth[i]%2 != 0 {
			continue
		}
		outer := loop.value
		if loop.area < 0 {
			outer = reverseProfileLoop(outer)
		}
		digest := sha256.Sum256([]byte(outer.ID))
		regions = append(regions, geometry.ProfileRegion{ID: "profile-region:" + hex.EncodeToString(digest[:12]), Outer: outer})
		regionForLoop[i] = len(regions) - 1
	}
	for i, loop := range loops {
		if depth[i]%2 == 0 {
			continue
		}
		owner := parent[i]
		for owner >= 0 && depth[owner]%2 != 0 {
			owner = parent[owner]
		}
		index, ok := regionForLoop[owner]
		if !ok {
			return nil, fmt.Errorf("%w: profile hole has no outer loop", ErrValidation)
		}
		hole := loop.value
		if loop.area > 0 {
			hole = reverseProfileLoop(hole)
		}
		regions[index].Holes = append(regions[index].Holes, hole)
	}
	return regions, nil
}
