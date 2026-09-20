package thumbnail

import (
	"context"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"sort"
	"strings"
)

type vec = [3]float64

func add(a, b vec) vec         { return vec{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func sub(a, b vec) vec         { return vec{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func mul(a vec, s float64) vec { return vec{a[0] * s, a[1] * s, a[2] * s} }
func dot(a, b vec) float64     { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func cross(a, b vec) vec {
	return vec{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func unit(v vec) vec {
	n := math.Hypot(math.Hypot(v[0], v[1]), v[2])
	if n == 0 || !finite(n) {
		return vec{}
	}
	return mul(v, 1/n)
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func valid(v vec) bool      { return finite(v[0]) && finite(v[1]) && finite(v[2]) }

// Same right-handed camera as viewport ISO: eye=(1,-1,1), world up=+Z.
var eye = unit(vec{1, -1, 1})
var right = unit(cross(vec{0, 0, 1}, eye))
var up = cross(eye, right)

func project(v vec) vec { return vec{dot(v, right), -dot(v, up), dot(v, eye)} }

type pose struct {
	translation vec
	q           [4]float64
}

func newPose(t vec, q [4]float64) pose {
	length := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
	if length == 0 || !finite(length) {
		q = [4]float64{0, 0, 0, 1}
	} else {
		for i := range q {
			q[i] /= length
		}
	}
	return pose{t, q}
}
func (p pose) rotate(v vec) vec {
	q := vec{p.q[0], p.q[1], p.q[2]}
	t := mul(cross(q, v), 2)
	return add(v, add(mul(t, p.q[3]), cross(q, t)))
}
func (p pose) point(v vec) vec { return add(p.rotate(v), p.translation) }

type normalKey struct {
	point vec
	face  uint32
}
type preparedMesh struct {
	mesh    workspace.Mesh
	normals map[normalKey]vec
}
type meshInstance struct {
	mesh *preparedMesh
	pose pose
}
type sceneLine struct {
	points       []vec
	construction bool
}
type renderScene struct {
	meshes    []meshInstance
	lines     []sceneLine
	min, max  vec
	hasBounds bool
}

func (s *renderScene) include(v vec) {
	if !valid(v) {
		return
	}
	p := project(v)
	if !valid(p) {
		return
	}
	if !s.hasBounds {
		s.min = p
		s.max = p
		s.hasBounds = true
		return
	}
	for i := range p {
		s.min[i] = math.Min(s.min[i], p[i])
		s.max[i] = math.Max(s.max[i], p[i])
	}
}
func collectScene(ctx context.Context, view workspace.DocumentView) (*renderScene, error) {
	s := &renderScene{}
	triangles, vertices := 0, 0
	cache := map[string]*preparedMesh{}
	appendArtifact := func(a workspace.Artifact, p pose) error {
		triangles += len(a.Mesh.Triangles)
		vertices += len(a.Mesh.Vertices)
		for _, edge := range a.Mesh.Edges {
			vertices += len(edge.Points)
		}
		for _, primitive := range a.Visualization.Primitives {
			vertices += len(primitive.Positions)
			triangles += len(primitive.Indices) / 3
		}
		if triangles > maxTriangles || vertices > maxVertices {
			return errSceneBudget
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(a.Mesh.Triangles) > 0 {
			prepared := cache[a.GeometryKey]
			if prepared == nil || a.GeometryKey == "" {
				var err error
				prepared, err = prepareMesh(ctx, a.Mesh)
				if err != nil {
					return err
				}
				cache[a.GeometryKey] = prepared
			}
			s.meshes = append(s.meshes, meshInstance{prepared, p})
			// Only referenced finite vertices influence framing; unused mesh vertices do not.
			for i, t := range a.Mesh.Triangles {
				if i%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				if !validTriangle(a.Mesh, t) {
					continue
				}
				for _, index := range t {
					s.include(p.point(a.Mesh.Vertices[index]))
				}
			}
		}
		for _, primitive := range a.Visualization.Primitives {
			if err := ctx.Err(); err != nil {
				return err
			}
			// Thumbnails show the resulting solid, not its sketch editing overlays.
			if len(a.Mesh.Triangles) > 0 && strings.HasPrefix(primitive.Semantic, "SKETCH_") {
				continue
			}
			if primitive.Kind == "TRIANGLES" {
				mesh := workspace.Mesh{Vertices: primitive.Positions}
				for i := 0; i+2 < len(primitive.Indices); i += 3 {
					mesh.Triangles = append(mesh.Triangles, [3]uint32{primitive.Indices[i], primitive.Indices[i+1], primitive.Indices[i+2]})
				}
				prepared, err := prepareMesh(ctx, mesh)
				if err != nil {
					return err
				}
				s.meshes = append(s.meshes, meshInstance{prepared, p})
				for _, t := range mesh.Triangles {
					if validTriangle(mesh, t) {
						for _, index := range t {
							s.include(p.point(mesh.Vertices[index]))
						}
					}
				}
			} else if primitive.Kind == "POINTS" || primitive.Kind == "POLYLINE" {
				points := make([]vec, 0, len(primitive.Positions))
				flush := func() {
					if len(points) > 0 {
						s.lines = append(s.lines, sceneLine{points, primitive.Role == "CONSTRUCTION"})
						points = nil
					}
				}
				for i, v := range primitive.Positions {
					if i%1024 == 0 {
						if err := ctx.Err(); err != nil {
							return err
						}
					}
					v = p.point(v)
					if !valid(v) {
						flush()
						continue
					}
					s.include(v)
					points = append(points, v)
					if primitive.Kind == "POINTS" {
						flush()
					}
				}
				flush()
			}
		}
		return nil
	}
	if view.Document.Type == "PRODUCT" {
		if len(view.ResolvedInstances) > maxInstances {
			return nil, errSceneBudget
		}
		// Stable ordering makes coplanar tie handling independent of map/API order.
		instances := append([]workspace.ResolvedInstance(nil), view.ResolvedInstances...)
		sort.SliceStable(instances, func(i, j int) bool { return instances[i].ID < instances[j].ID })
		for _, instance := range instances {
			if a, ok := view.Artifacts[instance.GeometryKey]; ok {
				if err := appendArtifact(a, newPose(instance.Translation, instance.Rotation)); err != nil {
					return nil, err
				}
			}
		}
	} else if view.Artifact != nil {
		if err := appendArtifact(*view.Artifact, newPose(vec{}, [4]float64{})); err != nil {
			return nil, err
		}
	} else if view.Part != nil {
		if err := appendSketchFallback(ctx, s, view.Part.Features); err != nil {
			return nil, err
		}
	}
	return s, ctx.Err()
}
func validTriangle(m workspace.Mesh, t [3]uint32) bool {
	for _, i := range t {
		if uint64(i) >= uint64(len(m.Vertices)) || !valid(m.Vertices[i]) {
			return false
		}
	}
	return unit(cross(sub(m.Vertices[t[1]], m.Vertices[t[0]]), sub(m.Vertices[t[2]], m.Vertices[t[0]]))) != vec{}
}
func prepareMesh(ctx context.Context, m workspace.Mesh) (*preparedMesh, error) {
	result := &preparedMesh{mesh: m, normals: map[normalKey]vec{}}
	// Smooth within an exact B-Rep face, including duplicated tessellation vertices;
	// never blend normals across a CAD face boundary or infer persistent identity.
	if len(m.FaceIDs) != len(m.Triangles) {
		return result, nil
	}
	for i, t := range m.Triangles {
		if i%512 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if !validTriangle(m, t) {
			continue
		}
		normal := unit(cross(sub(m.Vertices[t[1]], m.Vertices[t[0]]), sub(m.Vertices[t[2]], m.Vertices[t[0]])))
		for _, index := range t {
			key := normalKey{m.Vertices[index], m.FaceIDs[i]}
			result.normals[key] = add(result.normals[key], normal)
		}
	}
	count := 0
	for key, v := range result.normals {
		count++
		if count%512 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		result.normals[key] = unit(v)
	}
	return result, nil
}
