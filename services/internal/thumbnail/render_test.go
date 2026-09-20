package thumbnail

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/workspace"
)

func boxMesh(x, y, z float64) workspace.Mesh {
	m := workspace.Mesh{Vertices: []vec{{0, 0, 0}, {x, 0, 0}, {x, y, 0}, {0, y, 0}, {0, 0, z}, {x, 0, z}, {x, y, z}, {0, y, z}}, Triangles: [][3]uint32{
		{0, 2, 1}, {0, 3, 2}, {4, 5, 6}, {4, 6, 7}, {0, 1, 5}, {0, 5, 4}, {1, 2, 6}, {1, 6, 5}, {2, 3, 7}, {2, 7, 6}, {3, 0, 4}, {3, 4, 7}}}
	for i := range m.Triangles {
		m.FaceIDs = append(m.FaceIDs, uint32(i/2))
	}
	return m
}
func ringMesh(segments int) workspace.Mesh {
	m := workspace.Mesh{}
	for i := 0; i <= segments; i++ {
		a := float64(i%segments) * 2 * math.Pi / float64(segments)
		for _, v := range [][2]float64{{32, 0}, {32, 18}, {17, 0}, {17, 18}} {
			m.Vertices = append(m.Vertices, vec{v[0] * math.Cos(a), v[0] * math.Sin(a), v[1]})
		}
	}
	for i := 0; i < segments; i++ {
		a, b := uint32(i*4), uint32((i+1)*4)
		for face, ts := range [][2][3]uint32{{{a, b, b + 1}, {a, b + 1, a + 1}}, {{a + 2, a + 3, b + 3}, {a + 2, b + 3, b + 2}}, {{a + 1, b + 1, b + 3}, {a + 1, b + 3, a + 3}}, {{a, b + 2, b}, {a, a + 2, b + 2}}} {
			for _, triangle := range ts {
				m.Triangles = append(m.Triangles, triangle)
				m.FaceIDs = append(m.FaceIDs, uint32(face))
			}
		}
	}
	for _, corner := range []int{0, 1, 2, 3} {
		edge := workspace.MeshEdge{}
		for i := 0; i <= segments; i++ {
			edge.Points = append(edge.Points, m.Vertices[i*4+corner])
		}
		m.Edges = append(m.Edges, edge)
	}
	return m
}
func partView(m workspace.Mesh) workspace.DocumentView {
	return workspace.DocumentView{Document: workspace.DocumentSummary{Type: "PART"}, Artifact: &workspace.Artifact{GeometryKey: "fixture", Mesh: m}}
}
func decode(t *testing.T, payload []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, Width, Height) {
		t.Fatalf("wrong canvas: %v", img.Bounds())
	}
	return img
}
func TestRenderPNGAndStableMaterial(t *testing.T) {
	view := partView(ringMesh(96))
	first, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	decode(t, first)
	if len(first) > 250_000 {
		t.Fatalf("thumbnail too large: %d", len(first))
	}
	view.Artifact.GeometryKey = "new-revision-same-shape"
	second, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("geometry cache key changed material appearance")
	}
}
func TestCameraIsOrthonormalAndMatchesViewportISO(t *testing.T) {
	for _, axis := range []vec{right, up, eye} {
		if math.Abs(dot(axis, axis)-1) > 1e-12 {
			t.Fatal("non-unit camera basis")
		}
	}
	if math.Abs(dot(right, up))+math.Abs(dot(right, eye))+math.Abs(dot(up, eye)) > 1e-12 {
		t.Fatal("camera basis is not orthogonal")
	}
	if project(vec{0, 0, 1})[1] >= 0 {
		t.Fatal("world Z must point up on screen")
	}
	if project(vec{1, 0, 0})[0] <= 0 || project(vec{0, 1, 0})[0] <= 0 {
		t.Fatal("ISO axes disagree with viewport (1,-1,1)")
	}
}
func TestDepthBufferCrossingTrianglesAndHiddenLines(t *testing.T) {
	r := &raster{pixels: image.NewRGBA(image.Rect(0, 0, 64, 64)), depth: make([]float64, 64*64), width: 64, height: 64}
	for i := range r.depth {
		r.depth[i] = math.Inf(-1)
	}
	front := [3]screenPoint{{4, 4, 10}, {60, 4, -10}, {4, 60, 10}}
	back := [3]screenPoint{{4, 4, 0}, {60, 4, 0}, {4, 60, 0}}
	if err := r.triangle(context.Background(), front, [3]float64{1, 1, 1}); err != nil {
		t.Fatal(err)
	}
	if err := r.triangle(context.Background(), back, [3]float64{.5, .5, .5}); err != nil {
		t.Fatal(err)
	}
	if r.pixels.RGBAAt(8, 8).R < 180 || r.pixels.RGBAAt(48, 8).R > 110 {
		t.Fatal("crossing triangles incorrectly sorted by mean depth")
	}
	before := r.pixels.RGBAAt(8, 8)
	r.line(screenPoint{4, 8, -100}, screenPoint{60, 8, -100}, color.RGBA{255, 0, 0, 255}, 2, 1)
	if r.pixels.RGBAAt(8, 8) != before {
		t.Fatal("hidden edge leaked through foreground surface")
	}
}
func TestProductRotationAndTranslation(t *testing.T) {
	a := workspace.Artifact{GeometryKey: "box", Mesh: boxMesh(40, 12, 9)}
	view := workspace.DocumentView{Document: workspace.DocumentSummary{Type: "PRODUCT"}, Artifacts: map[string]workspace.Artifact{"box": a}, ResolvedInstances: []workspace.ResolvedInstance{{ID: "a", GeometryKey: "box"}, {ID: "b", GeometryKey: "box", Translation: vec{60, 0, 0}}}}
	first, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	view.ResolvedInstances[1].Rotation = [4]float64{0, 0, math.Sin(math.Pi / 4), math.Cos(math.Pi / 4)}
	second, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("occurrence rotation ignored")
	}
	view.ResolvedInstances[1].Translation = vec{60, 30, 0}
	third, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(second, third) {
		t.Fatal("occurrence translation ignored")
	}
	// Applying the same rigid transform to mesh positions must produce the same result.
	transformed := a
	p := newPose(view.ResolvedInstances[1].Translation, view.ResolvedInstances[1].Rotation)
	transformed.GeometryKey = "transformed"
	transformed.Mesh.Vertices = append([]vec(nil), a.Mesh.Vertices...)
	for i, v := range transformed.Mesh.Vertices {
		transformed.Mesh.Vertices[i] = p.point(v)
	}
	view.Artifacts["transformed"] = transformed
	view.ResolvedInstances[1] = workspace.ResolvedInstance{ID: "b", GeometryKey: "transformed"}
	fourth, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	// Rounding in transformed normals may differ by one RGB level.
	left, right := decode(t, third), decode(t, fourth)
	different := 0
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			if left.At(x, y) != right.At(x, y) {
				different++
			}
		}
	}
	if different > Width*Height/100 {
		t.Fatalf("rigid transforms disagree at %d pixels", different)
	}
}
func TestSketchAndVisualizationPose(t *testing.T) {
	a := workspace.Artifact{GeometryKey: "wire", Visualization: workspace.VisualizationManifest{Primitives: []workspace.VisualPrimitive{{Kind: "POLYLINE", Positions: []vec{{0, 0, 0}, {10, 0, 0}, {10, 20, 0}}}, {Kind: "POINTS", Positions: []vec{{5, 5, 0}}}}}}
	view := workspace.DocumentView{Document: workspace.DocumentSummary{Type: "PRODUCT"}, Artifacts: map[string]workspace.Artifact{"wire": a}, ResolvedInstances: []workspace.ResolvedInstance{{ID: "wire", GeometryKey: "wire"}}}
	first, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	decode(t, first)
	view.ResolvedInstances[0].Rotation = [4]float64{math.Sin(math.Pi / 4), 0, 0, math.Cos(math.Pi / 4)}
	second, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("wire rotation ignored")
	}
	view = workspace.DocumentView{Document: workspace.DocumentSummary{Type: "PART"}, Part: &workspace.PartModel{Features: []workspace.Feature{{Type: "SKETCH", Plane: "XY", Sketch: &workspace.SketchFeature{Entities: []workspace.SketchEntity{{Kind: "CIRCLE", Center: &workspace.SketchPoint2{}, Radius: 20}}}}}}}
	sketch, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(sketch, Default(view)) {
		t.Fatal("sketch-only view lost")
	}
}
func TestInvalidGeometryAndCancellation(t *testing.T) {
	view := partView(boxMesh(10, 10, 10))
	view.Artifact.Mesh.Triangles = append(view.Artifact.Mesh.Triangles, [3]uint32{999, 1, 2})
	view.Artifact.Mesh.Vertices = append(view.Artifact.Mesh.Vertices, vec{math.NaN(), 0, 0})
	result, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	decode(t, result)
	for _, duration := range []time.Duration{0, time.Nanosecond} {
		result, fallback, err := RenderWithTimeout(view, duration)
		if err != nil || !fallback {
			t.Fatalf("deadline: fallback=%v err=%v", fallback, err)
		}
		decode(t, result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = RenderContext(ctx, view, time.Second)
	if err != context.Canceled {
		t.Fatalf("parent cancellation lost: %v", err)
	}
}
func TestFramingTranslationAndScaleInvariant(t *testing.T) {
	baseline, err := Render(partView(boxMesh(40, 20, 10)))
	if err != nil {
		t.Fatal(err)
	}
	for _, scale := range []float64{1e-6, 1e6} {
		mesh := boxMesh(40, 20, 10)
		for i, v := range mesh.Vertices {
			mesh.Vertices[i] = add(mul(v, scale), vec{1000 * scale, -1000 * scale, 500 * scale})
		}
		payload, err := Render(partView(mesh))
		if err != nil {
			t.Fatal(err)
		}
		a, b := decode(t, baseline), decode(t, payload)
		difference := 0
		for y := 0; y < Height; y++ {
			for x := 0; x < Width; x++ {
				if a.At(x, y) != b.At(x, y) {
					difference++
				}
			}
		}
		if difference > Width*Height/100 {
			t.Fatalf("framing changed with world scale %g: %d pixels", scale, difference)
		}
	}
}
func TestThumbnailGallery(t *testing.T) {
	directory := os.Getenv("OCCCCAD_THUMBNAIL_GALLERY")
	if directory == "" {
		t.Skip("set OCCCCAD_THUMBNAIL_GALLERY to export visual acceptance fixtures")
	}
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	ring := workspace.Artifact{GeometryKey: "ring", Mesh: ringMesh(128)}
	assembly := workspace.DocumentView{Document: workspace.DocumentSummary{Type: "PRODUCT"}, Artifacts: map[string]workspace.Artifact{"ring": ring}, ResolvedInstances: []workspace.ResolvedInstance{{ID: "a", GeometryKey: "ring"}, {ID: "b", GeometryKey: "ring", Translation: vec{75, 0, 0}, Rotation: [4]float64{0, math.Sin(math.Pi / 4), 0, math.Cos(math.Pi / 4)}}}}
	for name, view := range map[string]workspace.DocumentView{"ring": partView(ring.Mesh), "plate": partView(boxMesh(80, 45, 8)), "rotated-assembly": assembly} {
		data, err := Render(view)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name+".png"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
func BenchmarkThumbnailRing(b *testing.B) {
	view := partView(ringMesh(2048))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Render(view); err != nil {
			b.Fatal(err)
		}
	}
}
