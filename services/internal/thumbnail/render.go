// Package thumbnail renders immutable display artifacts without a GPU or browser.
package thumbnail

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
	"time"

	perf "github.com/occccad/occccad/internal/performance"
	"github.com/occccad/occccad/internal/workspace"
)

const (
	Width           = 640
	Height          = 400
	RendererVersion = "png-v13-clustered-surfaces"
	ContentType     = "image/png"
	samples         = 2
	// Bound scene preparation independently of pixel count and the job deadline.
	maxTriangles = 1_000_000
	maxVertices  = 3_000_000
	maxInstances = 10_000
)

var errSceneBudget = errors.New("thumbnail scene exceeds render budget")

func Render(view workspace.DocumentView) ([]byte, error) { return render(context.Background(), view) }
func RenderWithTimeout(view workspace.DocumentView, timeout time.Duration) ([]byte, bool, error) {
	return RenderContext(context.Background(), view, timeout)
}

// RenderContext works with an already hydrated view.
func RenderContext(ctx context.Context, view workspace.DocumentView, base time.Duration) ([]byte, bool, error) {
	return RenderDisplay(ctx, view, base, nil)
}

// RenderDisplay selects detail from the visible, placed workload. The return
// flag identifies a reduced rendering of the model. Errors remain job failures.
func RenderDisplay(parent context.Context, view workspace.DocumentView, base time.Duration, hydrate func(context.Context, *workspace.Artifact) error) ([]byte, bool, error) {
	if err := parent.Err(); err != nil {
		return nil, false, err
	}
	load := estimateScene(view)
	budget := load.budget(base)
	overview := base <= 0 || load.triangles > maxTriangles || load.vertices > maxVertices || load.instances > maxInstances
	if !overview {
		loaded := make(map[string]workspace.Artifact, len(load.uses))
		for key := range load.uses {
			a, ok := view.Artifacts[key]
			if !ok {
				continue
			}
			if hydrate != nil {
				if err := hydrate(parent, &a); err != nil {
					return nil, false, err
				}
			}
			loaded[key] = a
		}
		view.Artifacts = loaded
		ctx, cancel := context.WithTimeout(parent, budget)
		data, err := render(ctx, view)
		cancel()
		if parent.Err() != nil {
			return nil, false, parent.Err()
		}
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, errSceneBudget) {
			return data, false, err
		}
		hydrate = nil
	}

	// Retain a bounded mesh for each unique geometry. Give short interactive
	// deadlines a separate bounded overview budget after the detailed pass.
	ctx, cancel := context.WithTimeout(parent, max(5*time.Second, min(2*time.Minute, budget)))
	defer cancel()
	if load.instances > maxInstances {
		// Spread representative placements over the entire ordered occurrence list.
		selected := make([]workspace.ResolvedInstance, 0, maxInstances)
		index := 0
		for _, inst := range view.ResolvedInstances {
			if !visibleInstance(view, inst) {
				continue
			}
			if index == len(selected)*(load.instances-1)/(maxInstances-1) {
				selected = append(selected, inst)
			}
			index++
		}
		view.ResolvedInstances = selected
		load = estimateScene(view)
	}
	reduced := make(map[string]workspace.Artifact, len(load.uses))
	for key, n := range load.uses {
		a, ok := view.Artifacts[key]
		if !ok {
			continue
		}
		if hydrate != nil {
			if err := hydrate(ctx, &a); err != nil {
				return nil, true, err
			}
		}
		size := len(a.Mesh.Triangles)
		denominator := load.triangles
		if size == 0 {
			size = max(1, len(a.Mesh.Vertices), len(a.Mesh.Edges), len(a.Mesh.TopologyVertices))
			denominator = load.vertices
		}
		target := int(int64(overviewTriangles) * int64(size) / max(int64(1), denominator, int64(size)*int64(n)))
		var err error
		finish := perf.Start(ctx, "simplify")
		a.Mesh, err = compactMesh(ctx, a.Mesh, max(12, min(overviewTriangles, target)))
		finish()
		if err != nil {
			return nil, true, err
		}
		reduced[key] = a
	}
	view.Artifacts = reduced
	data, err := renderScenePNG(ctx, view, true)
	return data, true, err
}

func render(ctx context.Context, view workspace.DocumentView) ([]byte, error) {
	return renderScenePNG(ctx, view, false)
}

func renderScenePNG(ctx context.Context, view workspace.DocumentView, overview bool) ([]byte, error) {
	finish := perf.Start(ctx, "scene")
	scene, err := collectScene(ctx, view)
	finish()
	if err != nil {
		return nil, err
	}
	if !scene.hasBounds || (len(scene.meshes) == 0 && len(scene.lines) == 0) {
		return Default(view), nil
	}
	finish = perf.Start(ctx, "raster")
	raster := newRaster(scene)
	raster.overview = overview
	err = raster.draw(ctx, scene)
	finish()
	if err != nil {
		return nil, err
	}
	defer perf.Start(ctx, "resolve-png")()
	return encodePNG(ctx, raster.image())
}

type contextWriter struct {
	ctx context.Context
	bytes.Buffer
}

func (w *contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.Buffer.Write(p)
}
func encodePNG(ctx context.Context, img image.Image) ([]byte, error) {
	w := &contextWriter{ctx: ctx}
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(w, img); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

var defaultOnce sync.Once
var defaultImage []byte

func Default(view workspace.DocumentView) []byte { return DefaultForType(view.Document.Type) }
func DefaultForType(_ string) []byte {
	defaultOnce.Do(func() {
		img := image.NewRGBA(image.Rect(0, 0, Width, Height))
		// An empty viewport, never a placeholder solid that could be mistaken for content.
		camera := mul(eye, 420)
		tanY := math.Tan(35 * math.Pi / 360)
		for y := 0; y < Height; y++ {
			blend := float64(y) / float64(Height-1)
			base := [3]float64{225 + 19*blend, 230 + 16*blend, 236 + 12*blend}
			for x := 0; x < Width; x++ {
				ray := add(mul(eye, -1), add(mul(right, (2*(float64(x)+.5)/Width-1)*tanY*Width/Height), mul(up, (1-2*(float64(y)+.5)/Height)*tanY)))
				intensity := 0.0
				if ray[2] < -1e-6 {
					distance := (-60 - camera[2]) / ray[2]
					point := add(camera, mul(ray, distance))
					footprint := math.Max(.25, distance*tanY*2/Height)
					line := math.Min(math.Abs(math.Remainder(point[0], 25)), math.Abs(math.Remainder(point[1], 25)))
					intensity = math.Max(0, 1-line/footprint) * .12 * math.Min(1, 500/distance)
				}
				img.SetRGBA(x, y, color.RGBA{uint8(base[0] + (149-base[0])*intensity), uint8(base[1] + (163-base[1])*intensity), uint8(base[2] + (178-base[2])*intensity), 255})
			}
		}
		defaultImage, _ = encodePNG(context.Background(), img)
	})
	return bytes.Clone(defaultImage)
}
