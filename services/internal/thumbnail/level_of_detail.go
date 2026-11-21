package thumbnail

import (
	"context"
	"math"
	"time"

	"github.com/occccad/occccad/internal/workspace"
)

// The raster is 640x400. Reduce display geometry only, before retaining another
// decoded definition. Placements and per-face appearance use their original IDs.
const overviewTriangles = 80_000

type sceneLoad struct {
	instances           int
	uses                map[string]int
	triangles, vertices int64
}

func visibleInstance(view workspace.DocumentView, instance workspace.ResolvedInstance) bool {
	_, ok := view.Artifacts[instance.GeometryKey]
	return ok && (instance.BodyID == "" || instance.BodyVisible)
}

func estimateScene(view workspace.DocumentView) sceneLoad {
	load := sceneLoad{uses: map[string]int{}}
	add := func(key string) {
		if key != "" {
			load.uses[key]++
		}
	}
	if view.Document.Type == "PRODUCT" {
		for _, inst := range view.ResolvedInstances {
			if visibleInstance(view, inst) {
				load.instances++
				add(inst.GeometryKey)
			}
		}
		for _, scope := range view.ReferenceDisplayScopes {
			for _, result := range scope.Results {
				if result.Visible {
					add(result.GeometryKey)
				}
			}
		}
	} else if view.Part != nil {
		for _, body := range view.Part.Bodies {
			if body.Visible {
				add(body.GeometryKey)
			}
		}
		for _, f := range view.Part.Features {
			if f.Type == "GEOMETRY" && f.Result != nil && !f.Suppressed && f.EvaluationStatus == "" && (f.Visible == nil || *f.Visible) && workspace.GeometryFeatureVisible(*view.Part, f) {
				add(f.Result.GeometryKey)
			}
		}
	}
	for key, n := range load.uses {
		a := view.Artifacts[key]
		load.triangles += int64(n) * int64(max(a.TriangleCount, uint64(len(a.Mesh.Triangles))))
		load.vertices += int64(n) * int64(max(a.DisplayVertexCount, uint64(len(a.Mesh.Vertices))))
	}
	return load
}

func (load sceneLoad) budget(base time.Duration) time.Duration {
	// Scale in quarter-million placed triangles (or equivalent vertex work).
	factor := 1 + min(int64(5), max(load.triangles/250_000, load.vertices/750_000))
	return min(base, time.Duration(math.MaxInt64/factor)) * time.Duration(factor)
}

func compactMesh(ctx context.Context, m workspace.Mesh, target int) (workspace.Mesh, error) {
	// Keep outlines for wire geometry. Solid silhouettes come from the reduced mesh.
	out := workspace.Mesh{}
	if len(m.Triangles) == 0 {
		stride := max(1, (len(m.Edges)+target-1)/target)
		for i := 0; i < len(m.Edges); i += stride {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			e := m.Edges[i]
			points := e.Points
			capPoints := max(2, target/max(1, len(m.Edges)/stride))
			if len(points) > capPoints {
				e.Points = make([]vec, capPoints)
				for j := range e.Points {
					e.Points[j] = points[j*(len(points)-1)/(capPoints-1)]
				}
			}
			out.Edges = append(out.Edges, e)
		}
		stride = max(1, (len(m.TopologyVertices)+target-1)/target)
		for i := 0; i < len(m.TopologyVertices); i += stride {
			out.TopologyVertices = append(out.TopologyVertices, m.TopologyVertices[i])
		}
		return out, ctx.Err()
	}
	if len(m.Triangles) <= target && len(m.Vertices) <= target*3 {
		out = m
		out.Edges = nil
		out.EdgeProfile = nil
		return out, ctx.Err()
	}
	cells := max(2, int(math.Sqrt(float64(target)/10)))
	for {
		reduced, err := cluster(ctx, m, cells)
		if err != nil || len(reduced.Triangles) <= target || cells == 2 {
			return reduced, err
		}
		cells = max(2, cells*2/3)
	}
}

func cluster(ctx context.Context, m workspace.Mesh, cells int) (workspace.Mesh, error) {
	out := workspace.Mesh{}
	var lo, hi vec
	hasBounds := false
	for i, v := range m.Vertices {
		if i%2048 == 0 && ctx.Err() != nil {
			return out, ctx.Err()
		}
		if !valid(v) {
			continue
		}
		if !hasBounds {
			lo, hi = v, v
			hasBounds = true
		}
		for k := range v {
			lo[k] = min(lo[k], v[k])
			hi[k] = max(hi[k], v[k])
		}
	}
	// Preserve thin parts by quantizing each nonzero axis independently.

	bins := map[[3]int]uint32{}
	remap := make([]uint32, len(m.Vertices))
	for i, v := range m.Vertices {
		if i%2048 == 0 && ctx.Err() != nil {
			return out, ctx.Err()
		}
		if !valid(v) {
			continue
		}
		var cell [3]int
		for k := range v {
			if hi[k] > lo[k] {
				cell[k] = min(cells-1, int((v[k]-lo[k])/(hi[k]-lo[k])*float64(cells)))
			}
		}
		idx, ok := bins[cell]
		if !ok {
			idx = uint32(len(out.Vertices))
			bins[cell] = idx
			out.Vertices = append(out.Vertices, v)
		}
		remap[i] = idx
	}
	seen := map[[3]uint32]bool{}
	for i, t := range m.Triangles {
		if i%2048 == 0 && ctx.Err() != nil {
			return out, ctx.Err()
		}
		if !validTriangle(m, t) {
			continue
		}
		face := [3]uint32{remap[t[0]], remap[t[1]], remap[t[2]]}
		if face[0] == face[1] || face[1] == face[2] || face[2] == face[0] || seen[face] {
			continue
		}
		seen[face] = true
		out.Triangles = append(out.Triangles, face)
		if len(m.FaceIDs) == len(m.Triangles) {
			out.FaceIDs = append(out.FaceIDs, m.FaceIDs[i])
		}
	}
	// A collection of tiny disconnected faces may fall inside single cells.
	// Preserve an actual face when quantization removes the entire surface.
	if len(out.Triangles) == 0 {
		for i, t := range m.Triangles {
			if i%2048 == 0 && ctx.Err() != nil {
				return out, ctx.Err()
			}
			if !validTriangle(m, t) {
				continue
			}
			out.Vertices = []vec{m.Vertices[t[0]], m.Vertices[t[1]], m.Vertices[t[2]]}
			out.Triangles = [][3]uint32{{0, 1, 2}}
			if len(m.FaceIDs) == len(m.Triangles) {
				out.FaceIDs = []uint32{m.FaceIDs[i]}
			}
			break
		}
	}
	return out, ctx.Err()
}
