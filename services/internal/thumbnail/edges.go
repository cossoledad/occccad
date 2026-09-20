package thumbnail

import (
	"context"
	"image/color"
)

type edgeKey struct{ a, b vec }
type edgeNormals struct {
	first, second vec
	count         int
}

func less(a, b vec) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// Fallback for meshes without B-Rep edge polylines. Weld exact positions so
// duplicate face vertices don't expose every tessellation seam.
func (r *raster) meshEdges(ctx context.Context, instance meshInstance) error {
	m := instance.mesh.mesh
	edges := make(map[edgeKey]edgeNormals)
	for i, t := range m.Triangles {
		if i%512 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if !validTriangle(m, t) {
			continue
		}
		normal := instance.pose.rotate(unit(cross(sub(m.Vertices[t[1]], m.Vertices[t[0]]), sub(m.Vertices[t[2]], m.Vertices[t[0]]))))
		for j := 0; j < 3; j++ {
			a, b := m.Vertices[t[j]], m.Vertices[t[(j+1)%3]]
			if less(b, a) {
				a, b = b, a
			}
			key := edgeKey{a, b}
			value := edges[key]
			if value.count == 0 {
				value.first = normal
			} else {
				value.second = normal
			}
			value.count++
			edges[key] = value
		}
	}
	count := 0
	for key, n := range edges {
		count++
		if count%512 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if n.count > 1 && dot(n.first, n.second) > .82 && dot(n.first, eye)*dot(n.second, eye) > 0 {
			continue
		}
		r.line(r.screen(instance.pose.point(key.a)), r.screen(instance.pose.point(key.b)), color.RGBA{30, 48, 65, 255}, 1.15*samples, .82)
	}
	return nil
}
