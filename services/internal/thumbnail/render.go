// Package thumbnail renders immutable display artifacts without a GPU or browser.
package thumbnail

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"sync"
	"time"

	"github.com/occccad/occccad/internal/workspace"
)

const (
	Width           = 640
	Height          = 400
	RendererVersion = "png-v4"
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

// RenderContext is synchronous: cancellation never leaves a renderer goroutine
// consuming CPU after a job lease has ended. Budget/deadline exhaustion uses a
// cached placeholder; parent cancellation remains an error for the job runner.
func RenderContext(parent context.Context, view workspace.DocumentView, timeout time.Duration) ([]byte, bool, error) {
	if err := parent.Err(); err != nil {
		return nil, false, err
	}
	if timeout <= 0 {
		return Default(view), true, nil
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	payload, err := render(ctx, view)
	if parent.Err() != nil {
		return nil, false, parent.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errSceneBudget) {
		return Default(view), true, nil
	}
	return payload, false, err
}

func render(ctx context.Context, view workspace.DocumentView) ([]byte, error) {
	scene, err := collectScene(ctx, view)
	if err != nil {
		return nil, err
	}
	if !scene.hasBounds || (len(scene.meshes) == 0 && len(scene.lines) == 0) {
		return Default(view), nil
	}
	raster := newRaster(scene)
	if err := raster.draw(ctx, scene); err != nil {
		return nil, err
	}
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
		for y := 0; y < Height; y++ {
			for x := 0; x < Width; x++ {
				img.SetRGBA(x, y, color.RGBA{235, 240, 245, 255})
			}
		}
		// A quiet wire cube is deliberately distinct from a rendered model.
		r := &raster{pixels: img, width: Width, height: Height, depth: make([]float64, Width*Height)}
		for i := range r.depth {
			r.depth[i] = -1e300
		}
		points := []screenPoint{{250, 170, 0}, {320, 130, 0}, {390, 170, 0}, {320, 210, 0}, {250, 245, 0}, {320, 285, 0}, {390, 245, 0}}
		for _, pair := range [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}, {0, 4}, {4, 5}, {5, 6}, {6, 2}, {3, 5}} {
			r.line(points[pair[0]], points[pair[1]], color.RGBA{128, 151, 174, 255}, 2, 1)
		}
		defaultImage, _ = encodePNG(context.Background(), img)
	})
	return bytes.Clone(defaultImage)
}
