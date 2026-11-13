package thumbnail

import (
	"context"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/occccad/occccad/internal/visual"
	"github.com/occccad/occccad/internal/workspace"
)

func TestSuppliedSurfaceNormalsAndLegacyFallback(t *testing.T) {
	m := workspace.Mesh{Vertices: []vec{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}, Triangles: [][3]uint32{{0, 1, 2}}, FaceIDs: []uint32{1}, Normals: []vec{{1, 0, 1}, {0, 1, 1}, {0, 0, 2}}}
	p, err := prepareMesh(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.normals) != 0 || p.vertexNormals[0] != unit(m.Normals[0]) {
		t.Fatal("provided surface normals were averaged from triangles")
	}
	m.Normals[1] = vec{math.NaN(), 0, 1}
	p, err = prepareMesh(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if p.vertexNormals[0] != unit(m.Normals[0]) || p.vertexNormals[1] != (vec{}) || p.normals[normalKey{m.Vertices[1], 1}] != (vec{0, 0, 1}) {
		t.Fatal("partial normals must retain valid samples and fall back within the face")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = prepareMesh(ctx, m); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}
func TestThumbnailNaturalEdgesKeepCurvatureBoundaryAndColour(t *testing.T) {
	profile := &visual.EdgeProfile{Positions: []vec{{0, 0, 0}, {1, 0, 0}}, NormalA: []vec{{0, 0, 1}, {0, 0, 1}}, NormalB: []vec{{0, 0, 1}, {0, 0, 1}}, Metadata: [][4]uint32{{1, 1, 2, 3}}}
	styles, err := prepareEdgeStyles(context.Background(), profile)
	if err != nil {
		t.Fatal(err)
	}
	if styles[0].sharp != [2]float64{} || styles[0].suppressed != [2]float64{} {
		t.Fatal("a smooth connection is soft, not absent")
	}
	profile.Metadata[0][3] = 4
	styles, _ = prepareEdgeStyles(context.Background(), profile)
	if styles[0].suppressed != [2]float64{1, 1} {
		t.Fatal("a coplanar seam must be suppressed")
	}
	profile.Metadata[0][3] = 1
	styles, _ = prepareEdgeStyles(context.Background(), profile)
	if styles[0].sharp != [2]float64{1, 1} {
		t.Fatal("open boundary must remain structural")
	}
	profile.NormalA = nil
	styles, err = prepareEdgeStyles(context.Background(), profile)
	if err != nil || len(styles) > 0 {
		t.Fatal("incomplete profile must use legacy topology")
	}
	for _, base := range []color.RGBA{{245, 245, 245, 255}, {20, 27, 36, 255}, {207, 102, 77, 255}} {
		r := &raster{pixels: image.NewRGBA(image.Rect(0, 0, 12, 12)), depth: make([]float64, 144), width: 12, height: 12}
		for i := range r.depth {
			r.depth[i] = 0
			r.pixels.SetRGBA(i%12, i/12, base)
		}
		r.stroke(screenPoint{2, 6, 1}, screenPoint{10, 6, 1}, studioEdgeColor, 1, 1, &lineProfile{})
		got := r.pixels.RGBAAt(6, 6)
		if got.R >= base.R || got.G >= base.G || got.B >= base.B {
			t.Fatalf("soft ink must darken the underlying lit hue: %v -> %v", base, got)
		}
		if math.Abs(float64(got.R)/float64(got.G)-float64(base.R)/float64(base.G)) > .08 {
			t.Fatalf("soft ink changed hue: %v -> %v", base, got)
		}
	}
}

func TestLinearStudioIlluminationTransfer(t *testing.T) {
	for _, base := range []uint8{0, 20, 77, 142, 175, 245, 255} {
		for i := 0; i <= 1400; i++ {
			light := float64(i) / 1000
			got := surfaceDisplay[max(0, min(4096, int(surfaceLinear[base]*light+.5)))]
			want := math.Round(min(1, linearSRGB(srgbLinear(float64(base)/255)*light)) * 255)
			if math.Abs(float64(got)-want) > 1 {
				t.Fatalf("display transfer error: base=%d light=%g got=%d want=%g", base, light, got, want)
			}
		}
	}
	if got := surfaceDisplay[int(surfaceLinear[200]*.5+.5)]; got < 145 || got > 147 {
		t.Fatalf("half linear light must not halve sRGB bytes: %d", got)
	}
}
