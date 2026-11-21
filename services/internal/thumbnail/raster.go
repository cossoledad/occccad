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
	overview          bool
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
		c := color.RGBA{uint8(225 + 19*t), uint8(230 + 16*t), uint8(236 + 12*t), 255}
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

var keyLight = unit(vec{-.77, -.36, .53})
var fillLight = unit(vec{5, 2, 3})
var halfLight = unit(add(keyLight, eye))

func illumination(n vec) float64 {
	if dot(n, eye) < 0 {
		n = mul(n, -1)
	} // double-sided display for open shells
	return .38 + .72*max(0, dot(n, keyLight)) + .16*max(0, dot(n, fillLight)) + .08*math.Pow(max(0, dot(n, halfLight)), 10)
}

// Shade a surface only after a sample passes the opaque depth test. Supplied
// vertex lighting is shared by visible triangles of this placement.
func (instance *meshInstance) surfaceLighting(i int, cached []float64) ([3]float64, color.RGBA) {
	prepared := instance.mesh
	m := &prepared.mesh
	t := m.Triangles[i]
	var shades [3]float64
	var flat vec
	for corner, index := range t {
		if len(cached) > 0 && prepared.vertexNormals[index] != (vec{}) {
			if cached[index] == 0 {
				cached[index] = illumination(instance.pose.rotate(prepared.vertexNormals[index]))
			}
			shades[corner] = cached[index]
			continue
		}
		var normal vec
		if len(prepared.normals) > 0 {
			normal = prepared.normals[normalKey{m.Vertices[index], m.FaceIDs[i]}]
		}
		if normal == (vec{}) {
			if flat == (vec{}) {
				flat = unit(cross(sub(m.Vertices[t[1]], m.Vertices[t[0]]), sub(m.Vertices[t[2]], m.Vertices[t[0]])))
			}
			normal = flat
		}
		shades[corner] = illumination(instance.pose.rotate(normal))
	}
	base := instance.appearance.Color
	if base == "" && (instance.domain == "SHEET_SET" || instance.domain == "MIXED_SET") {
		base = "#819db4"
	}
	if i < len(m.FaceIDs) {
		if face := instance.appearance.Faces[uint64(m.FaceIDs[i])]; face != "" {
			base = face
		}
	}
	return shades, surfaceColor(base)
}

func (r *raster) draw(ctx context.Context, s *renderScene) error {
	type drawMesh struct {
		points                    []screenPoint
		shades                    []float64
		opacity                   float64
		hasOpaque, hasTransparent bool
	}
	draws := make([]drawMesh, len(s.meshes))
	for k, instance := range s.meshes {
		m := &instance.mesh.mesh
		draw := &draws[k]
		draw.opacity = 1
		if instance.appearance.Opacity != nil {
			draw.opacity = *instance.appearance.Opacity
		}
		draw.hasOpaque = draw.opacity >= 1
		draw.hasTransparent = draw.opacity > 0 && draw.opacity < 1
		for _, face := range instance.appearance.FaceProperties {
			if face.Opacity != nil {
				draw.hasOpaque = draw.hasOpaque || *face.Opacity >= 1
				draw.hasTransparent = draw.hasTransparent || (*face.Opacity > 0 && *face.Opacity < 1)
			}
		}
		if !draw.hasOpaque && !draw.hasTransparent || len(instance.mesh.triangles) == 0 {
			continue
		}
		draw.points = make([]screenPoint, len(m.Vertices))
		if len(instance.mesh.vertexNormals) > 0 {
			draw.shades = make([]float64, len(m.Vertices))
		}
		for i, index := range instance.mesh.vertices {
			if i%1024 == 0 && ctx.Err() != nil {
				return ctx.Err()
			}
			draw.points[index] = r.screen(instance.pose.point(m.Vertices[index]))
		}
	}
	for pass := 0; pass < 2; pass++ {
		for k := range s.meshes {
			instance, draw := &s.meshes[k], &draws[k]
			if pass == 0 && !draw.hasOpaque || pass == 1 && !draw.hasTransparent {
				continue
			}
			m := &instance.mesh.mesh
			for ordinal, i := range instance.mesh.triangles {
				if ordinal%128 == 0 && ctx.Err() != nil {
					return ctx.Err()
				}
				opacity := draw.opacity
				if int(i) < len(m.FaceIDs) {
					if face := instance.appearance.FaceProperties[uint64(m.FaceIDs[i])]; face.Opacity != nil {
						opacity = *face.Opacity
					}
				}
				if opacity <= 0 || (pass == 0 && opacity < 1) || (pass == 1 && opacity >= 1) {
					continue
				}
				t := m.Triangles[i]
				points := [3]screenPoint{draw.points[t[0]], draw.points[t[1]], draw.points[t[2]]}
				if err := r.drawTriangle(ctx, points, opacity, func() ([3]float64, color.RGBA) {
					return instance.surfaceLighting(int(i), draw.shades)
				}); err != nil {
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
		if r.overview && len(m.Triangles) > 0 && instance.domain != "WIRE_SET" {
			continue
		}
		edgeColor, edgeWidth, edgeOpacity := thumbnailEdgeStyle(instance.appearance)
		if instance.domain == "WIRE_SET" {
			edgeColor = color.RGBA{169, 155, 255, 255}
			if instance.appearance.Color != "" {
				edgeColor = surfaceColor(instance.appearance.Color)
			}
			if instance.appearance.EdgeColor != "" {
				edgeColor = surfaceColor(instance.appearance.EdgeColor)
			}
			edgeWidth = 1.8
			if instance.appearance.EdgeWidth != nil {
				edgeWidth = *instance.appearance.EdgeWidth
			}
			edgeOpacity = 1
			if instance.appearance.Opacity != nil {
				edgeOpacity = *instance.appearance.Opacity
			}
		}
		if edgeOpacity <= 0 {
			continue
		}
		if m.EdgeProfile != nil && len(instance.mesh.edgeStyles) > 0 && len(instance.mesh.edgeStyles) == len(m.EdgeProfile.Metadata) && instance.domain != "WIRE_SET" {
			phase := 0.0
			var previous uint32
			for i, meta := range m.EdgeProfile.Metadata {
				if i%128 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				if meta[0] != previous {
					phase = 0
					previous = meta[0]
				}
				style := instance.mesh.edgeStyles[i]
				style.explicitColor = instance.appearance.EdgeColor != ""
				if style.suppressed == [2]float64{1, 1} {
					continue
				}
				r.styledLine(r.screen(instance.pose.point(m.EdgeProfile.Positions[i*2])), r.screen(instance.pose.point(m.EdgeProfile.Positions[i*2+1])), edgeColor, edgeWidth*samples, edgeOpacity/.88, instance.appearance.LineStyle == "DASHED", &phase, &style)
			}
		} else if len(m.Edges) > 0 {
			for i, edge := range m.Edges {
				dashPhase := 0.0
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
						r.styledLine(r.screen(instance.pose.point(edge.Points[j-1])), r.screen(instance.pose.point(edge.Points[j])), edgeColor, edgeWidth*samples, edgeOpacity, instance.appearance.LineStyle == "DASHED", &dashPhase)
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
			if line.engineering {
				radius := 3.5 * float64(samples)
				opacity := 1.0
				if line.appearance.Opacity != nil {
					opacity = *line.appearance.Opacity
				}
				if line.appearance.PointSize != nil {
					radius *= *line.appearance.PointSize / 11
				}
				pointColor := color.RGBA{255, 255, 255, 255}
				if line.appearance.Color != "" {
					pointColor = surfaceColor(line.appearance.Color)
				}
				for pass := 0; pass < 2; pass++ {
					width := 2.7 * float64(samples)
					value := color.RGBA{27, 37, 48, 255}
					if pass == 1 {
						width = 1.2 * float64(samples)
						value = pointColor
					}
					if line.appearance.PointSymbol == "DOT" {
						r.line(p, p, value, radius*1.7, opacity)
						continue
					}
					if line.appearance.PointSymbol == "PLUS" {
						r.line(screenPoint{p.x - radius, p.y, p.z}, screenPoint{p.x + radius, p.y, p.z}, value, width, opacity)
						r.line(screenPoint{p.x, p.y - radius, p.z}, screenPoint{p.x, p.y + radius, p.z}, value, width, opacity)
						continue
					}
					r.line(screenPoint{p.x - radius, p.y - radius, p.z}, screenPoint{p.x + radius, p.y + radius, p.z}, value, width, opacity)
					r.line(screenPoint{p.x - radius, p.y + radius, p.z}, screenPoint{p.x + radius, p.y - radius, p.z}, value, width, opacity)
				}
			} else {
				r.line(p, p, c, 4*samples, 1)
			}
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
func (r *raster) styledLine(a, b screenPoint, c color.RGBA, width, opacity float64, dashed bool, phase *float64, profiles ...*lineProfile) {
	var profile *lineProfile
	if len(profiles) > 0 {
		profile = profiles[0]
	}
	if !dashed {
		r.stroke(a, b, c, width, opacity, profile)
		return
	}
	length := math.Hypot(b.x-a.x, b.y-a.y)
	if length < 1e-9 {
		return
	}
	period := 10.0 * samples
	dash := 6.0 * samples
	interpolate := func(t float64) screenPoint {
		return screenPoint{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t, a.z + (b.z-a.z)*t}
	}
	for offset := 0.0; offset < length; {
		position := math.Mod(*phase+offset, period)
		step := period - position
		if position < dash {
			step = dash - position
		}
		end := math.Min(length, offset+step)
		if end <= offset {
			break
		}
		if position < dash {
			local := profile
			if profile != nil {
				value := *profile
				for k, t := range []float64{offset / length, end / length} {
					value.sharp[k] = profile.sharp[0]*(1-t) + profile.sharp[1]*t
					value.suppressed[k] = profile.suppressed[0]*(1-t) + profile.suppressed[1]*t
				}
				local = &value
			}
			r.stroke(interpolate(offset/length), interpolate(end/length), c, width, opacity, local)
		}
		offset = end
	}
	*phase = math.Mod(*phase+length, period)
}
func surfaceColor(value string) color.RGBA {
	if len(value) == 7 && value[0] == '#' {
		if rgb, err := strconv.ParseUint(value[1:], 16, 24); err == nil {
			return color.RGBA{uint8(rgb >> 16), uint8(rgb >> 8), uint8(rgb), 255}
		}
	}
	return color.RGBA{142, 159, 175, 255}
}
func (r *raster) drawTriangle(ctx context.Context, p [3]screenPoint, opacity float64, lighting func() ([3]float64, color.RGBA)) error {
	if opacity <= 0 {
		return nil
	}
	var shade, linear [3]float64
	var dl float64
	lit := false
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
		if start > end {
			continue
		}
		a := edgeFunction(p[1], p[2], float64(start)+.5, py) / area
		b := edgeFunction(p[2], p[0], float64(start)+.5, py) / area
		z := a*p[0].z + b*p[1].z + (1-a-b)*p[2].z
		if !lit {
			// Use the same stepped depth and tie rule as the color pass.
			probe := z
			for x := start; x <= end; x++ {
				if probe > r.depth[y*r.width+x] {
					var base color.RGBA
					shade, base = lighting()
					linear = [3]float64{surfaceLinear[base.R], surfaceLinear[base.G], surfaceLinear[base.B]}
					dl = da*(shade[0]-shade[2]) + db*(shade[1]-shade[2])
					lit = true
					break
				}
				probe += dz
			}
			if !lit {
				continue
			}
		}
		light := a*shade[0] + b*shade[1] + (1-a-b)*shade[2]
		for x := start; x <= end; x++ {
			index := y*r.width + x
			if z > r.depth[index] {
				shaded := color.RGBA{surfaceDisplay[max(0, min(4096, int(linear[0]*light+.5)))], surfaceDisplay[max(0, min(4096, int(linear[1]*light+.5)))], surfaceDisplay[max(0, min(4096, int(linear[2]*light+.5)))], 255}
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
	r.stroke(a, b, c, width, opacity, nil)
}
func (r *raster) stroke(a, b screenPoint, c color.RGBA, width, opacity float64, profile *lineProfile) {
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
		if a.z+t*(b.z-a.z) < r.depth[y*r.width+x]-2*samples {
			return
		}
		distance := math.Hypot(float64(x)+.5-a.x-t*dx, float64(y)+.5-a.y-t*dy)
		alpha := min(1, max(0, radius+.5-distance)) * opacity
		ink := c
		if profile != nil {
			sharp := max(0, min(1, profile.sharp[0]*(1-t)+profile.sharp[1]*t))
			suppressed := max(0, min(1, profile.suppressed[0]*(1-t)+profile.suppressed[1]*t))
			alpha *= (.12 + .76*sharp) * (1 - suppressed)
			if !profile.explicitColor {
				ink = studioEdgeInk[int(math.Round(sharp*256))]
			}
		}
		if alpha <= 0 {
			return
		}
		old := r.pixels.RGBAAt(x, y)
		r.pixels.SetRGBA(x, y, color.RGBA{uint8(float64(old.R)*(1-alpha) + float64(ink.R)*alpha), uint8(float64(old.G)*(1-alpha) + float64(ink.G)*alpha), uint8(float64(old.B)*(1-alpha) + float64(ink.B)*alpha), 255})
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
	c := studioEdgeColor
	if a.EdgeColor != "" {
		c = surfaceColor(a.EdgeColor)
	}
	width := 1.35
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
	return c, width, .88 * opacity
}
