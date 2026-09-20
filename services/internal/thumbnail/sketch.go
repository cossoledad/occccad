package thumbnail

import (
	"context"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"strings"
)

// Normally sketches arrive as evaluated Visualization primitives. This fallback
// is for a Part without a display artifact; it must never invent a solid.
func appendSketchFallback(ctx context.Context, s *renderScene, features []workspace.Feature) error {
	count := 0
	for _, feature := range features {
		if feature.Sketch == nil {
			continue
		}
		support := feature.Sketch.Support
		u, n, origin := unit(support.XDirection), unit(support.Normal), support.Origin
		if u == (vec{}) || n == (vec{}) {
			plane := support.Plane
			if plane == "" {
				plane = feature.Plane
			}
			switch strings.ToUpper(plane) {
			case "XZ":
				u, n = vec{1, 0, 0}, vec{0, -1, 0}
			case "YZ":
				u, n = vec{0, 1, 0}, vec{1, 0, 0}
			default:
				u, n = vec{1, 0, 0}, vec{0, 0, 1}
			}
		}
		v := unit(cross(n, u))
		world := func(p workspace.SketchPoint2) vec { return add(origin, add(mul(u, p.X), mul(v, p.Y))) }
		for _, entity := range feature.Sketch.Entities {
			if err := ctx.Err(); err != nil {
				return err
			}
			count++
			if count > maxVertices/100 {
				return errSceneBudget
			}
			if entity.Suppressed {
				continue
			}
			var points []vec
			switch entity.Kind {
			case "LINE":
				if entity.Start != nil && entity.End != nil {
					points = []vec{world(*entity.Start), world(*entity.End)}
				}
			case "POINT":
				if entity.Point != nil {
					points = []vec{world(*entity.Point)}
				}
			case "CIRCLE", "ARC":
				if entity.Center == nil || entity.Radius <= 0 || !finite(entity.Radius) {
					continue
				}
				start, end := 0., 2*math.Pi
				if entity.Kind == "ARC" {
					start, end = entity.StartAngle, entity.EndAngle
					if !finite(start) || !finite(end) {
						continue
					}
					if end <= start {
						end += 2 * math.Pi
					}
				}
				for i := 0; i <= 96; i++ {
					angle := start + (end-start)*float64(i)/96
					points = append(points, world(workspace.SketchPoint2{X: entity.Center.X + entity.Radius*math.Cos(angle), Y: entity.Center.Y + entity.Radius*math.Sin(angle)}))
				}
			}
			allValid := true
			for _, p := range points {
				if !valid(p) {
					allValid = false
					break
				}
			}
			if !allValid || len(points) == 0 {
				continue
			}
			for _, p := range points {
				s.include(p)
			}
			s.lines = append(s.lines, sceneLine{points, entity.Role == "CONSTRUCTION"})
		}
	}
	return nil
}
