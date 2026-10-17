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

	"github.com/occccad/occccad/internal/workspace"
)

const (
	Width           = 640
	Height          = 400
	RendererVersion = "png-v8-empty-scene"
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
		// An empty viewport, never a placeholder solid that could be mistaken for content.
		camera := mul(eye, 420)
		tanY := math.Tan(35 * math.Pi / 360)
		for y := 0; y < Height; y++ {
			blend := float64(y) / float64(Height-1)
			base := [3]float64{35 + 77*blend, 51 + 83*blend, 70 + 85*blend}
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
				img.SetRGBA(x, y, color.RGBA{uint8(base[0] + (203-base[0])*intensity), uint8(base[1] + (220-base[1])*intensity), uint8(base[2] + (237-base[2])*intensity), 255})
			}
		}
		defaultImage, _ = encodePNG(context.Background(), img)
	})
	return bytes.Clone(defaultImage)
}
