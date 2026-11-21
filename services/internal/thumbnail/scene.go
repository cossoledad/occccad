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
	mesh          workspace.Mesh
	triangles     []uint32 // original ordinals, preserving per-face appearance
	vertices      []uint32 // referenced by a valid surface triangle
	normals       map[normalKey]vec
	vertexNormals []vec
	edgeStyles    []lineProfile
}
type meshInstance struct {
	mesh       *preparedMesh
	pose       pose
	appearance workspace.ResolvedAppearance
	domain     string
}
type sceneLine struct {
	appearance   workspace.ResolvedAppearance
	points       []vec
	construction bool
	engineering  bool
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
	withoutSketches := func(a workspace.Artifact) workspace.Artifact {
		var primitives []workspace.VisualPrimitive
		for _, p := range a.Visualization.Primitives {
			if !strings.HasPrefix(p.Semantic, "SKETCH_") {
				primitives = append(primitives, p)
			}
		}
		a.Visualization.Primitives = primitives
		return a
	}
	appendArtifact := func(a workspace.Artifact, p pose, appearance workspace.ResolvedAppearance) error {
		triangles += len(a.Mesh.Triangles)
		vertices += len(a.Mesh.Vertices)
		for _, edge := range a.Mesh.Edges {
			vertices += len(edge.Points)
		}
		if a.Mesh.EdgeProfile != nil {
			vertices += len(a.Mesh.EdgeProfile.Positions)
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
		if len(a.Mesh.Triangles) > 0 || len(a.Mesh.Edges) > 0 {
			prepared := cache[a.GeometryKey]
			if prepared == nil || a.GeometryKey == "" {
				var err error
				prepared, err = prepareMesh(ctx, a.Mesh)
				if err != nil {
					return err
				}
				cache[a.GeometryKey] = prepared
			}
			if err := s.addMesh(ctx, meshInstance{mesh: prepared, pose: p, appearance: appearance, domain: a.GeometryDomain}); err != nil {
				return err
			}
		}
		for _, edge := range a.Mesh.Edges {
			for i, point := range edge.Points {
				if i%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				s.include(p.point(point))
			}
		}
		if a.GeometryDomain == "POINT_SET" {
			vertices += len(a.Mesh.TopologyVertices)
			if vertices > maxVertices {
				return errSceneBudget
			}
			for i, point := range a.Mesh.TopologyVertices {
				if i%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				v := p.point(point.Point)
				if !valid(v) {
					continue
				}
				s.include(v)
				s.lines = append(s.lines, sceneLine{points: []vec{v}, engineering: true, appearance: appearance})
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
				if err := s.addMesh(ctx, meshInstance{mesh: prepared, pose: p}); err != nil {
					return err
				}
			} else if primitive.Kind == "POINTS" || primitive.Kind == "POLYLINE" {
				points := make([]vec, 0, len(primitive.Positions))
				flush := func() {
					if len(points) > 0 {
						s.lines = append(s.lines, sceneLine{points: points, construction: primitive.Role == "CONSTRUCTION"})
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
		// Stable ordering makes coplanar tie handling independent of map/API order.
		instances := make([]workspace.ResolvedInstance, 0, min(maxInstances, len(view.ResolvedInstances)))
		for _, inst := range view.ResolvedInstances {
			if visibleInstance(view, inst) {
				if len(instances) == maxInstances {
					return nil, errSceneBudget
				}
				instances = append(instances, inst)
			}
		}
		sort.SliceStable(instances, func(i, j int) bool { return instances[i].ID < instances[j].ID })
		sketchPaths := map[string]bool{}
		for _, scope := range view.ReferenceDisplayScopes {
			if scope.SketchVisualization != nil {
				sketchPaths[scope.InstancePath.Canonical] = true
			}
		}
		for _, instance := range instances {
			a := view.Artifacts[instance.GeometryKey]
			if sketchPaths[instance.OccurrencePath] {
				a = withoutSketches(a)
			}
			if err := appendArtifact(a, newPose(instance.Translation, instance.Rotation), instance.Appearance); err != nil {
				return nil, err
			}
		}
		for _, scope := range view.ReferenceDisplayScopes {
			if scope.SketchVisualization != nil {
				if err := appendArtifact(workspace.Artifact{Visualization: *scope.SketchVisualization}, newPose(scope.Translation, scope.Rotation), workspace.ResolvedAppearance{}); err != nil {
					return nil, err
				}
			}
			for _, result := range scope.Results {
				if !result.Visible {
					continue
				}
				if a, ok := view.Artifacts[result.GeometryKey]; ok {
					a.GeometryDomain = result.Domain
					if err := appendArtifact(a, newPose(scope.Translation, scope.Rotation), result.Appearance); err != nil {
						return nil, err
					}
				}
			}
		}
	} else if view.Part != nil {
		for _, body := range view.Part.Bodies {
			if !body.Visible {
				continue
			}
			if a, ok := view.Artifacts[body.GeometryKey]; ok {
				if view.SketchVisualization != nil {
					a = withoutSketches(a)
				}
				if err := appendArtifact(a, newPose(vec{}, [4]float64{}), view.BodyAppearances[body.ID]); err != nil {
					return nil, err
				}
			}
		}
		for _, feature := range view.Part.Features {
			if feature.Type != "GEOMETRY" || feature.Result == nil || feature.Suppressed || feature.EvaluationStatus != "" || feature.Visible != nil && !*feature.Visible || !workspace.GeometryFeatureVisible(*view.Part, feature) {
				continue
			}
			if a, ok := view.Artifacts[feature.Result.GeometryKey]; ok {
				a.GeometryDomain = feature.Result.Domain
				if err := appendArtifact(a, newPose(vec{}, [4]float64{}), view.ResultAppearances[feature.ID]); err != nil {
					return nil, err
				}
			}
		}
		if view.SketchVisualization != nil {
			if err := appendArtifact(workspace.Artifact{Visualization: *view.SketchVisualization}, newPose(vec{}, [4]float64{}), workspace.ResolvedAppearance{}); err != nil {
				return nil, err
			}
		}
		hasModelDisplay := len(s.meshes)+len(s.lines) > 0
		for _, point := range view.Part.DatumPoints {
			if point.EvaluationStatus != "" || point.Visible != nil && !*point.Visible || !workspace.GeometryFeatureVisible(*view.Part, workspace.Feature{GeometricalSetID: point.GeometricalSetID}) || !valid(point.Position) {
				continue
			}
			vertices++
			if vertices > maxVertices {
				return nil, errSceneBudget
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			s.include(point.Position)
			s.lines = append(s.lines, sceneLine{points: []vec{point.Position}, engineering: true, appearance: view.ObjectAppearances["REFERENCE_POINT:"+point.ID]})
		}
		if !hasModelDisplay && view.SketchVisualization == nil {
			if err := appendSketchFallback(ctx, s, view.Part.Features); err != nil {
				return nil, err
			}
		}
	}
	return s, ctx.Err()
}
func (s *renderScene) addMesh(ctx context.Context, instance meshInstance) error {
	s.meshes = append(s.meshes, instance)
	for i, index := range instance.mesh.vertices {
		if i%1024 == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		s.include(instance.pose.point(instance.mesh.mesh.Vertices[index]))
	}
	return nil
}

func validTriangle(m workspace.Mesh, t [3]uint32) bool {
	return triangleNormal(m.Vertices, t) != (vec{})
}

func triangleNormal(vertices []vec, t [3]uint32) vec {
	for _, i := range t {
		if uint64(i) >= uint64(len(vertices)) || !valid(vertices[i]) {
			return vec{}
		}
	}
	return unit(cross(sub(vertices[t[1]], vertices[t[0]]), sub(vertices[t[2]], vertices[t[0]])))
}
func prepareMesh(ctx context.Context, m workspace.Mesh) (*preparedMesh, error) {
	result := &preparedMesh{mesh: m, normals: map[normalKey]vec{}}
	if m.EdgeProfile != nil {
		var err error
		result.edgeStyles, err = prepareEdgeStyles(ctx, m.EdgeProfile)
		if err != nil {
			return nil, err
		}
	}
	// The GLB carries oriented surface normals. Rebuilding them from triangles
	// loses curvature and repeats a large coordinate/face hash unnecessarily.
	complete := len(m.Normals) == len(m.Vertices) && len(m.Vertices) > 0
	if complete {
		result.vertexNormals = make([]vec, len(m.Normals))
		for i, n := range m.Normals {
			if i%512 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			result.vertexNormals[i] = unit(n)
			if result.vertexNormals[i] == (vec{}) {
				complete = false
			}
		}
	}
	// Smooth within an exact B-Rep face, including duplicated tessellation vertices;
	// never blend normals across a CAD face boundary or infer persistent identity.
	smooth := !complete && len(m.FaceIDs) == len(m.Triangles)
	used := make([]bool, len(m.Vertices))
	result.triangles = make([]uint32, 0, len(m.Triangles))
	for i, t := range m.Triangles {
		if i%512 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		normal := triangleNormal(m.Vertices, t)
		if normal == (vec{}) {
			continue
		}
		result.triangles = append(result.triangles, uint32(i))
		for _, index := range t {
			if !used[index] {
				used[index] = true
				result.vertices = append(result.vertices, index)
			}
		}
		if !smooth {
			continue
		}
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
