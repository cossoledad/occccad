package thumbnail

import (
	"context"
	"github.com/occccad/occccad/internal/workspace"
	"image"
	"image/color"
	"math"
	"strconv"
)

type screenPoint struct{ x, y, z float64 }
type raster struct {
	pixels            *image.RGBA
	depth             []float64
	transparentColor  [3][]float64
	transparentWeight []float64
	transparentReveal []float64
	width, height     int
	center            vec
	scale             float64
}

func newRaster(s *renderScene) *raster {
	w, h := Width*samples, Height*samples
	r := &raster{pixels: image.NewRGBA(image.Rect(0, 0, w, h)), depth: make([]float64, w*h), width: w, height: h}
	r.center = add(s.min, mul(sub(s.max, s.min), .5))
	extent := sub(s.max, s.min)
	r.scale = min(float64(w)*.86/max(extent[0], 1e-12), float64(h)*.86/max(extent[1], 1e-12))
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h-1)
		// Quiet studio backdrop, using the viewport's cool neutral palette.
		c := color.RGBA{uint8(116 - 49*t), uint8(132 - 52*t), uint8(145 - 55*t), 255}
		for x := 0; x < w; x++ {
			r.pixels.SetRGBA(x, y, c)
			r.depth[y*w+x] = math.Inf(-1)
		}
	}
	return r
}
func (r *raster) screen(v vec) screenPoint {
	p := mul(sub(project(v), r.center), r.scale)
	return screenPoint{float64(r.width)*.5 + p[0], float64(r.height)*.5 + p[1], p[2]}
}

var keyLight = unit(vec{-3, -4, 8})
var fillLight = unit(vec{5, 2, 3})
var halfLight = unit(add(keyLight, eye))

func illumination(n vec) float64 {
	if dot(n, eye) < 0 {
		n = mul(n, -1)
	} // double-sided display for open shells
	return .48 + .43*max(0, dot(n, keyLight)) + .15*max(0, dot(n, fillLight)) + .08*math.Pow(max(0, dot(n, halfLight)), 24)
}
func (r *raster) draw(ctx context.Context, s *renderScene) error {
	hasTransparency := false
	for _, instance := range s.meshes {
		if instance.appearance.Opacity != nil && *instance.appearance.Opacity < 1 {
			hasTransparency = true
		}
		for _, face := range instance.appearance.FaceProperties {
			if face.Opacity != nil && *face.Opacity < 1 {
				hasTransparency = true
			}
		}
	}
	for pass := 0; pass < 2; pass++ {
		if pass == 1 && !hasTransparency {
			break
		}
		for _, instance := range s.meshes {
			m := instance.mesh.mesh
			points := make([]screenPoint, len(m.Vertices))
			for i, v := range m.Vertices {
				if i%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				points[i] = r.screen(instance.pose.point(v))
			}
			for i, t := range m.Triangles {
				if i%128 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				if !validTriangle(m, t) {
					continue
				}
				opacity := 1.0
				if instance.appearance.Opacity != nil {
					opacity = *instance.appearance.Opacity
				}
				if i < len(m.FaceIDs) {
					if face := instance.appearance.FaceProperties[uint64(m.FaceIDs[i])]; face.Opacity != nil {
						opacity = *face.Opacity
					}
				}
				if opacity <= 0 || (pass == 0 && opacity < 1) || (pass == 1 && opacity >= 1) {
					continue
				}
				n := unit(cross(sub(m.Vertices[t[1]], m.Vertices[t[0]]), sub(m.Vertices[t[2]], m.Vertices[t[0]])))
				shades := [3]float64{}
				for corner, index := range t {
					normal := n
					if len(m.FaceIDs) == len(m.Triangles) {
						if smooth := instance.mesh.normals[normalKey{m.Vertices[index], m.FaceIDs[i]}]; smooth != (vec{}) {
							normal = smooth
						}
					}
					shades[corner] = illumination(instance.pose.rotate(normal))
				}
				base := instance.appearance.Color
				if i < len(m.FaceIDs) {
					if face := instance.appearance.Faces[uint64(m.FaceIDs[i])]; face != "" {
						base = face
					}
				}
				if err := r.triangleAlpha(ctx, [3]screenPoint{points[t[0]], points[t[1]], points[t[2]]}, shades, surfaceColor(base), opacity); err != nil {
					return err
				}
			}
		}
	}
	if err := r.compositeTransparency(ctx); err != nil {
		return err
	}
	// All solid depth is complete before any edge/curve pass: hidden lines cannot
	// shine through another instance or a nearer part of the same concave body.
	for _, instance := range s.meshes {
		m := instance.mesh.mesh
		edgeColor, edgeWidth, edgeOpacity := thumbnailEdgeStyle(instance.appearance)
		if edgeOpacity <= 0 {
			continue
		}
		if len(m.Edges) > 0 {
			for i, edge := range m.Edges {
				if i%128 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				for j := 1; j < len(edge.Points); j++ {
					if j%128 == 0 {
						if err := ctx.Err(); err != nil {
							return err
						}
					}
					if valid(edge.Points[j-1]) && valid(edge.Points[j]) {
						r.line(r.screen(instance.pose.point(edge.Points[j-1])), r.screen(instance.pose.point(edge.Points[j])), edgeColor, edgeWidth*samples, edgeOpacity)
					}
				}
			}
		} else {
			if err := r.meshEdges(ctx, instance); err != nil {
				return err
			}
		}
	}
	for i, line := range s.lines {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		c := color.RGBA{125, 217, 235, 255}
		thickness := 1.6 * samples
		if line.construction {
			c = color.RGBA{142, 172, 202, 255}
			thickness = 1 * samples
		}
		if len(line.points) == 1 {
			p := r.screen(line.points[0])
			r.line(p, p, c, 4*samples, 1)
		}
		for j := 1; j < len(line.points); j++ {
			if j%128 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			r.line(r.screen(line.points[j-1]), r.screen(line.points[j]), c, thickness, 1)
		}
	}
	return ctx.Err()
}
func edgeFunction(a, b screenPoint, x, y float64) float64 {
	return (b.x-a.x)*(y-a.y) - (b.y-a.y)*(x-a.x)
}
func surfaceColor(value string) color.RGBA {
	if len(value) == 7 && value[0] == '#' {
		if rgb, err := strconv.ParseUint(value[1:], 16, 24); err == nil {
			return color.RGBA{uint8(rgb >> 16), uint8(rgb >> 8), uint8(rgb), 255}
		}
	}
	return color.RGBA{193, 203, 213, 255}
}
func (r *raster) triangle(ctx context.Context, p [3]screenPoint, shade [3]float64, colors ...color.RGBA) error {
	base := surfaceColor("")
	if len(colors) > 0 {
		base = colors[0]
	}
	return r.triangleAlpha(ctx, p, shade, base, float64(base.A)/255)
}
func (r *raster) triangleAlpha(ctx context.Context, p [3]screenPoint, shade [3]float64, base color.RGBA, opacity float64) error {
	if opacity <= 0 {
		return nil
	}
	area := edgeFunction(p[0], p[1], p[2].x, p[2].y)
	if !finite(area) || math.Abs(area) < 1e-12 {
		return nil
	}
	minX := max(0, int(math.Floor(min(p[0].x, min(p[1].x, p[2].x)))))
	maxX := min(r.width-1, int(math.Ceil(max(p[0].x, max(p[1].x, p[2].x)))))
	minY := max(0, int(math.Floor(min(p[0].y, min(p[1].y, p[2].y)))))
	maxY := min(r.height-1, int(math.Ceil(max(p[0].y, max(p[1].y, p[2].y)))))
	da, db := (p[1].y-p[2].y)/area, (p[2].y-p[0].y)/area
	dz := da*(p[0].z-p[2].z) + db*(p[1].z-p[2].z)
	dl := da*(shade[0]-shade[2]) + db*(shade[1]-shade[2])
	sorted := p
	if sorted[0].y > sorted[1].y {
		sorted[0], sorted[1] = sorted[1], sorted[0]
	}
	if sorted[1].y > sorted[2].y {
		sorted[1], sorted[2] = sorted[2], sorted[1]
	}
	if sorted[0].y > sorted[1].y {
		sorted[0], sorted[1] = sorted[1], sorted[0]
	}
	slope := func(a, b screenPoint) float64 {
		if a.y == b.y {
			return 0
		}
		return (b.x - a.x) / (b.y - a.y)
	}
	longSlope, topSlope, bottomSlope := slope(sorted[0], sorted[2]), slope(sorted[0], sorted[1]), slope(sorted[1], sorted[2])
	for y := minY; y <= maxY; y++ {
		if y%32 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		py := float64(y) + .5
		if py < sorted[0].y || py > sorted[2].y {
			continue
		}
		left := sorted[0].x + (py-sorted[0].y)*longSlope
		right := sorted[0].x + (py-sorted[0].y)*topSlope
		if py >= sorted[1].y {
			right = sorted[1].x + (py-sorted[1].y)*bottomSlope
		}
		if left > right {
			left, right = right, left
		}
		start, end := max(minX, int(math.Ceil(left-.5))), min(maxX, int(math.Floor(right-.5)))
		a := edgeFunction(p[1], p[2], float64(start)+.5, py) / area
		b := edgeFunction(p[2], p[0], float64(start)+.5, py) / area
		z, light := a*p[0].z+b*p[1].z+(1-a-b)*p[2].z, a*shade[0]+b*shade[1]+(1-a-b)*shade[2]
		for x := start; x <= end; x++ {
			index := y*r.width + x
			if z > r.depth[index] {
				shaded := color.RGBA{uint8(min(255, float64(base.R)*light)), uint8(min(255, float64(base.G)*light)), uint8(min(255, float64(base.B)*light)), 255}
				if opacity >= 1 {
					r.depth[index] = z
					r.pixels.SetRGBA(x, y, shaded)
				} else {
					r.transparent(index, z, shaded, opacity)
				}
			}
			z += dz
			light += dl
		}
	}
	return nil
}
func (r *raster) line(a, b screenPoint, c color.RGBA, width, opacity float64) {
	if !finite(a.x) || !finite(a.y) || !finite(b.x) || !finite(b.y) {
		return
	}
	dx, dy := b.x-a.x, b.y-a.y
	length := dx*dx + dy*dy
	radius := width * .5
	x0 := max(0, int(math.Floor(min(a.x, b.x)-radius)))
	x1 := min(r.width-1, int(math.Ceil(max(a.x, b.x)+radius)))
	y0 := max(0, int(math.Floor(min(a.y, b.y)-radius)))
	y1 := min(r.height-1, int(math.Ceil(max(a.y, b.y)+radius)))
	plot := func(x, y int) {
		t := 0.
		if length > 0 {
			t = max(0, min(1, ((float64(x)+.5-a.x)*dx+(float64(y)+.5-a.y)*dy)/length))
		}
		distance := math.Hypot(float64(x)+.5-a.x-t*dx, float64(y)+.5-a.y-t*dy)
		alpha := min(1, max(0, radius+.5-distance)) * opacity
		if alpha <= 0 || a.z+t*(b.z-a.z) < r.depth[y*r.width+x]-2*samples {
			return
		}
		old := r.pixels.RGBAAt(x, y)
		r.pixels.SetRGBA(x, y, color.RGBA{uint8(float64(old.R)*(1-alpha) + float64(c.R)*alpha), uint8(float64(old.G)*(1-alpha) + float64(c.G)*alpha), uint8(float64(old.B)*(1-alpha) + float64(c.B)*alpha), 255})
	}
	// Traverse a narrow strip along the major axis, not the line's entire bounding box.
	if math.Abs(dx) >= math.Abs(dy) && dx != 0 {
		extent := (radius + 1) * math.Sqrt(length) / math.Abs(dx)
		for x := x0; x <= x1; x++ {
			center := a.y + (float64(x)+.5-a.x)*dy/dx
			for y := max(y0, int(math.Floor(center-extent))); y <= min(y1, int(math.Ceil(center+extent))); y++ {
				plot(x, y)
			}
		}
	} else if dy != 0 {
		extent := (radius + 1) * math.Sqrt(length) / math.Abs(dy)
		for y := y0; y <= y1; y++ {
			center := a.x + (float64(y)+.5-a.y)*dx/dy
			for x := max(x0, int(math.Floor(center-extent))); x <= min(x1, int(math.Ceil(center+extent))); x++ {
				plot(x, y)
			}
		}
	} else {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				plot(x, y)
			}
		}
	}
}

// Downsample linear pixel coverage at a fixed cost independent of mesh complexity.
func (r *raster) image() *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, Width, Height))
	for y := 0; y < Height; y++ {
		for x := 0; x < Width; x++ {
			red, green, blue := 0, 0, 0
			for dy := 0; dy < samples; dy++ {
				for dx := 0; dx < samples; dx++ {
					c := r.pixels.RGBAAt(x*samples+dx, y*samples+dy)
					red += int(c.R)
					green += int(c.G)
					blue += int(c.B)
				}
			}
			result.SetRGBA(x, y, color.RGBA{uint8(red / (samples * samples)), uint8(green / (samples * samples)), uint8(blue / (samples * samples)), 255})
		}
	}
	return result
}

// Weighted blending keeps thumbnail memory bounded by its fixed pixel count,
// independent of triangle order and transparent surface count.
func (r *raster) transparent(index int, z float64, c color.RGBA, alpha float64) {
	if r.transparentWeight == nil {
		size := r.width * r.height
		r.transparentWeight = make([]float64, size)
		r.transparentReveal = make([]float64, size)
		for i := range r.transparentColor {
			r.transparentColor[i] = make([]float64, size)
		}
	}
	if r.transparentWeight[index] == 0 {
		r.transparentReveal[index] = 1
	}
	weight := alpha * math.Pow(max(.1, min(1, .5+z/float64(max(1, r.height)*2))), 3)
	r.transparentColor[0][index] += float64(c.R) * weight
	r.transparentColor[1][index] += float64(c.G) * weight
	r.transparentColor[2][index] += float64(c.B) * weight
	r.transparentWeight[index] += weight
	r.transparentReveal[index] *= 1 - alpha
}
func (r *raster) compositeTransparency(ctx context.Context) error {
	if r.transparentWeight == nil {
		return nil
	}
	for y := 0; y < r.height; y++ {
		if y%32 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		for x := 0; x < r.width; x++ {
			i := y*r.width + x
			weight := r.transparentWeight[i]
			if weight == 0 {
				continue
			}
			reveal := r.transparentReveal[i]
			old := r.pixels.RGBAAt(x, y)
			blend := func(channel int, background uint8) uint8 {
				return uint8(max(0, min(255, float64(background)*reveal+r.transparentColor[channel][i]/weight*(1-reveal))))
			}
			r.pixels.SetRGBA(x, y, color.RGBA{blend(0, old.R), blend(1, old.G), blend(2, old.B), 255})
		}
	}
	return nil
}
func thumbnailEdgeStyle(a workspace.ResolvedAppearance) (color.RGBA, float64, float64) {
	c := color.RGBA{30, 48, 65, 255}
	if a.EdgeColor != "" {
		c = surfaceColor(a.EdgeColor)
	}
	width := 1.15
	if a.EdgeWidth != nil {
		width = *a.EdgeWidth
	}
	opacity := 1.0
	if a.Opacity != nil {
		opacity = *a.Opacity
	}
	if a.EdgeOpacityMode == "CLEAR" {
		opacity = 1
	}
	if a.Edges != nil && !*a.Edges {
		opacity = 0
	}
	return c, width, .82 * opacity
}
