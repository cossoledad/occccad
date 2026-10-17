package thumbnail

import (
	"context"
	"image"
	"image/color"
	"math"
	"testing"
)

func transparencyRaster() *raster {
	r := &raster{pixels: image.NewRGBA(image.Rect(0, 0, 32, 32)), depth: make([]float64, 32*32), width: 32, height: 32}
	for i := range r.depth {
		r.depth[i] = math.Inf(-1)
	}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			r.pixels.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	return r
}

func TestThumbnailTransparencyDepthZeroAndOrder(t *testing.T) {
	ctx := context.Background()
	shade := [3]float64{1, 1, 1}
	triangle := func(z float64) [3]screenPoint { return [3]screenPoint{{2, 2, z}, {30, 2, z}, {2, 30, z}} }
	var first color.RGBA
	for iteration, order := range [][2]float64{{1, 2}, {2, 1}} {
		r := transparencyRaster()
		if err := r.triangleAlpha(ctx, triangle(0), shade, color.RGBA{0, 0, 255, 255}, 1); err != nil {
			t.Fatal(err)
		}
		for _, z := range order {
			c := color.RGBA{255, 0, 0, 255}
			if z == 2 {
				c = color.RGBA{0, 255, 0, 255}
			}
			if err := r.triangleAlpha(ctx, triangle(z), shade, c, .4); err != nil {
				t.Fatal(err)
			}
		}
		if r.depth[8*32+8] != 0 {
			t.Fatal("translucent surface wrote opaque depth")
		}
		if err := r.compositeTransparency(ctx); err != nil {
			t.Fatal(err)
		}
		pixel := r.pixels.RGBAAt(8, 8)
		if iteration == 0 {
			first = pixel
		} else if pixel != first {
			t.Fatalf("transparency depends on draw order: %v != %v", pixel, first)
		}
		if pixel.R == 0 || pixel.G == 0 || pixel.B == 0 {
			t.Fatal("alpha lost a visible layer", pixel)
		}
		other := transparencyRaster()
		if err := other.triangleAlpha(ctx, triangle(0), shade, color.RGBA{0, 0, 255, 255}, 1); err != nil {
			t.Fatal(err)
		}
		if err := other.triangleAlpha(ctx, triangle(3), shade, color.RGBA{255, 0, 0, 255}, 0); err != nil {
			t.Fatal(err)
		}
		if err := other.triangleAlpha(ctx, triangle(-1), shade, color.RGBA{255, 0, 0, 255}, .5); err != nil {
			t.Fatal(err)
		}
		if err := other.compositeTransparency(ctx); err != nil {
			t.Fatal(err)
		}
		if other.pixels.RGBAAt(8, 8) != (color.RGBA{0, 0, 255, 255}) {
			t.Fatal("zero alpha or occluded surface changed the image")
		}
	}
	r := transparencyRaster()
	if err := r.compositeTransparency(ctx); err != nil {
		t.Fatal(err)
	}
	if r.transparentWeight != nil {
		t.Fatal("opaque scenes allocated transparency buffers")
	}
}
