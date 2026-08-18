package thumbnail

import (
	"fmt"
	"hash/fnv"
	"html"
	"math"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/workspace"
)

const (
	width         = 320.0
	height        = 200.0
	drawingTop    = 16.0
	drawingBottom = 158.0
)

type point struct {
	x     float64
	y     float64
	depth float64
}

type face struct {
	points [3]point
	depth  float64
	fill   string
}

type line struct {
	points []point
	color  string
}

type scene struct {
	faces []face
	lines []line
	dots  []point

	minX float64
	maxX float64
	minY float64
	maxY float64

	hasBounds bool
}

var palette = []string{
	"#4d9aca",
	"#6b8fd3",
	"#48a6a0",
	"#8b79c6",
	"#ca8058",
	"#5f9f72",
}

// Render produces a deterministic, dependency-free SVG preview from the same
// mesh data used by the browser. It supports Part solids, Part sketches and
// flattened Product instances.
func Render(view workspace.DocumentView) ([]byte, error) {
	var drawing scene

	if view.Document.Type == "PRODUCT" {
		for index, instance := range view.ResolvedInstances {
			artifact, exists := view.Artifacts[instance.GeometryKey]
			if !exists {
				continue
			}

			appendMesh(
				&drawing,
				artifact.Mesh,
				instance.Translation,
				colorFor(instance.ID, index),
			)
			appendVisualization(&drawing, artifact.Visualization, instance.Translation)
		}
	} else if view.Artifact != nil {
		appendMesh(
			&drawing,
			view.Artifact.Mesh,
			[3]float64{},
			colorFor(view.Artifact.GeometryKey, 0),
		)
		appendVisualization(&drawing, view.Artifact.Visualization, [3]float64{})
	} else if view.Part != nil {
		appendSketches(&drawing, view.Part.Features)
	}

	transform := fit(&drawing)

	var body strings.Builder

	for _, item := range drawing.faces {
		body.WriteString(`<polygon points="`)

		for index, vertex := range item.points {
			if index > 0 {
				body.WriteByte(' ')
			}

			x, y := transform(vertex)

			fmt.Fprintf(
				&body,
				"%.2f,%.2f",
				x,
				y,
			)
		}

		fmt.Fprintf(
			&body,
			`" fill="%s" stroke="#24516d" stroke-width="0.16" stroke-opacity="0.35" stroke-linejoin="round"/>`,
			item.fill,
		)
	}

	for _, item := range drawing.lines {
		body.WriteString(`<polyline points="`)

		for index, vertex := range item.points {
			if index > 0 {
				body.WriteByte(' ')
			}

			x, y := transform(vertex)

			fmt.Fprintf(
				&body,
				"%.2f,%.2f",
				x,
				y,
			)
		}

		fmt.Fprintf(
			&body,
			`" fill="none" stroke="%s" stroke-width="2" stroke-linejoin="round"/>`,
			item.color,
		)
	}
	for _, item := range drawing.dots {
		x, y := transform(item)
		fmt.Fprintf(&body, `<circle cx="%.2f" cy="%.2f" r="2.4" fill="#f4f7f8" stroke="#32556b" stroke-width="1.2"/>`, x, y)
	}

	if !drawing.hasBounds {
		body.WriteString(emptyMark(view.Document.Type))
	}

	detail := partDetail(view)

	svg := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="320" height="200" viewBox="0 0 320 200" role="img" aria-label="%s"><defs><linearGradient id="background" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#f7fafc"/><stop offset="1" stop-color="#e4edf3"/></linearGradient></defs><rect width="320" height="200" rx="8" fill="url(#background)"/><path d="M0 164H320" stroke="#ccdbe4"/>%s<text x="16" y="181" font-family="system-ui,sans-serif" font-size="12" font-weight="600" fill="#203746">%s</text><text x="304" y="181" text-anchor="end" font-family="system-ui,sans-serif" font-size="10" fill="#647b89">%s</text></svg>`,
		html.EscapeString(view.Document.Name),
		body.String(),
		html.EscapeString(view.Document.Name),
		html.EscapeString(detail),
	)

	return []byte(svg), nil
}

func appendVisualization(target *scene, visualization workspace.VisualizationManifest, translation [3]float64) {
	for _, primitive := range visualization.Primitives {
		if len(primitive.Positions) == 0 {
			continue
		}
		switch primitive.Kind {
		case "POINTS":
			for _, position := range primitive.Positions {
				projected := project(translate(position, translation))
				target.dots = append(target.dots, projected)
				target.addPoint(projected)
			}
		case "POLYLINE":
			item := line{color: "#315b72", points: make([]point, 0, len(primitive.Positions))}
			if primitive.Role == "CONSTRUCTION" {
				item.color = "#7b8c94"
			}
			for _, position := range primitive.Positions {
				projected := project(translate(position, translation))
				item.points = append(item.points, projected)
				target.addPoint(projected)
			}
			target.lines = append(target.lines, item)
		case "TRIANGLES":
			mesh := workspace.Mesh{Vertices: primitive.Positions, Triangles: make([][3]uint32, 0, len(primitive.Indices)/3)}
			for index := 0; index+2 < len(primitive.Indices); index += 3 {
				mesh.Triangles = append(mesh.Triangles, [3]uint32{
					primitive.Indices[index], primitive.Indices[index+1], primitive.Indices[index+2],
				})
			}
			appendMesh(target, mesh, translation, "#65a6b8")
		}
	}
}

func appendMesh(
	target *scene,
	mesh workspace.Mesh,
	translation [3]float64,
	color string,
) {
	if len(mesh.Triangles) == 0 || len(mesh.Vertices) == 0 {
		return
	}

	//
	// Important:
	//
	// Do not sample triangles using:
	//
	//     index += step
	//
	// Doing that literally removes triangles from the surface and therefore
	// creates visible holes on finely tessellated curved surfaces.
	//
	// For the SVG renderer we preserve mesh connectivity and render every
	// triangle.
	//
	for _, triangle := range mesh.Triangles {
		i0 := int(triangle[0])
		i1 := int(triangle[1])
		i2 := int(triangle[2])

		if i0 < 0 || i0 >= len(mesh.Vertices) ||
			i1 < 0 || i1 >= len(mesh.Vertices) ||
			i2 < 0 || i2 >= len(mesh.Vertices) {
			continue
		}

		sourceA := mesh.Vertices[i0]
		sourceB := mesh.Vertices[i1]
		sourceC := mesh.Vertices[i2]

		//
		// Skip degenerate triangles.
		//
		if triangleAreaSquared(sourceA, sourceB, sourceC) < 1e-24 {
			continue
		}

		vertices := [3][3]float64{
			translate(sourceA, translation),
			translate(sourceB, translation),
			translate(sourceC, translation),
		}

		var projected [3]point
		var depth float64

		for index, vertex := range vertices {
			p := project(vertex)

			projected[index] = p
			depth += p.depth

			target.addPoint(p)
		}

		shade := faceShade(
			sourceA,
			sourceB,
			sourceC,
		)

		target.faces = append(
			target.faces,
			face{
				points: projected,
				depth:  depth / 3.0,
				fill:   shadeColor(color, shade),
			},
		)
	}

	//
	// SVG has no depth buffer, so use a painter-style depth sort.
	//
	// Smaller depth values are rendered first, larger values later.
	//
	sort.SliceStable(
		target.faces,
		func(left, right int) bool {
			return target.faces[left].depth < target.faces[right].depth
		},
	)
}

func appendSketches(
	target *scene,
	features []workspace.Feature,
) {
	for _, feature := range features {
		if !strings.Contains(
			strings.ToUpper(feature.Type),
			"SKETCH",
		) {
			continue
		}

		if feature.Sketch == nil {
			continue
		}
		for _, entity := range feature.Sketch.Entities {
			if entity.Kind != "LINE" || entity.Start == nil || entity.End == nil {
				continue
			}
			item := line{color: "#2679aa"}
			for _, coordinate := range [][2]float64{{entity.Start.X, entity.Start.Y}, {entity.End.X, entity.End.Y}} {
				projected := project(sketchPoint(feature.Sketch.Support.Plane, coordinate))
				item.points = append(item.points, projected)
				target.addPoint(projected)
			}
			target.lines = append(target.lines, item)
		}
	}
}

func (s *scene) addPoint(p point) {
	if !s.hasBounds {
		s.minX = p.x
		s.maxX = p.x
		s.minY = p.y
		s.maxY = p.y
		s.hasBounds = true

		return
	}

	s.minX = math.Min(s.minX, p.x)
	s.maxX = math.Max(s.maxX, p.x)
	s.minY = math.Min(s.minY, p.y)
	s.maxY = math.Max(s.maxY, p.y)
}

func translate(
	vertex [3]float64,
	translation [3]float64,
) [3]float64 {
	return [3]float64{
		vertex[0] + translation[0],
		vertex[1] + translation[1],
		vertex[2] + translation[2],
	}
}

func sketchPoint(
	plane string,
	coordinate [2]float64,
) [3]float64 {
	switch strings.ToUpper(plane) {
	case "XZ":
		return [3]float64{
			coordinate[0],
			0,
			coordinate[1],
		}

	case "YZ":
		return [3]float64{
			0,
			coordinate[0],
			coordinate[1],
		}

	default:
		return [3]float64{
			coordinate[0],
			coordinate[1],
			0,
		}
	}
}

// project performs a fixed axonometric/isometric-style projection.
//
// Screen-space X:
//
//	sqrt(3) / 2 * (X - Y)
//
// Screen-space Y:
//
//	1 / 2 * (X + Y) - Z
//
// depth is retained separately for painter-style SVG ordering.
func project(vertex [3]float64) point {
	return point{
		x: 0.8660254037844386 *
			(vertex[0] - vertex[1]),

		y: 0.5*
			(vertex[0]+vertex[1]) -
			vertex[2],

		depth: vertex[0] +
			vertex[1] +
			vertex[2],
	}
}

func fit(
	drawing *scene,
) func(point) (float64, float64) {
	if !drawing.hasBounds {
		return func(point) (float64, float64) {
			return width / 2,
				(drawingTop + drawingBottom) / 2
		}
	}

	dx := math.Max(
		drawing.maxX-drawing.minX,
		1e-9,
	)

	dy := math.Max(
		drawing.maxY-drawing.minY,
		1e-9,
	)

	//
	// Keep a horizontal margin of 20 px on both sides.
	//
	scale := math.Min(
		(width-40.0)/dx,
		(drawingBottom-drawingTop)/dy,
	)

	centerX := (drawing.minX + drawing.maxX) / 2.0
	centerY := (drawing.minY + drawing.maxY) / 2.0

	targetCenterX := width / 2.0
	targetCenterY := (drawingTop + drawingBottom) / 2.0

	return func(value point) (float64, float64) {
		x := targetCenterX +
			(value.x-centerX)*scale

		//
		// SVG's Y axis points downward.
		//
		y := targetCenterY -
			(value.y-centerY)*scale

		return x, y
	}
}

func faceShade(
	a [3]float64,
	b [3]float64,
	c [3]float64,
) float64 {
	u := [3]float64{
		b[0] - a[0],
		b[1] - a[1],
		b[2] - a[2],
	}

	v := [3]float64{
		c[0] - a[0],
		c[1] - a[1],
		c[2] - a[2],
	}

	n := [3]float64{
		u[1]*v[2] - u[2]*v[1],
		u[2]*v[0] - u[0]*v[2],
		u[0]*v[1] - u[1]*v[0],
	}

	length := math.Sqrt(
		n[0]*n[0] +
			n[1]*n[1] +
			n[2]*n[2],
	)

	if length < 1e-12 {
		return 0.8
	}

	//
	// Normalized directional light:
	//
	//     approximately (0.3, -0.5, 0.8)
	//
	// The vector below has already been normalized.
	//
	const (
		lightX = 0.30304576336566325
		lightY = -0.5050762722761054
		lightZ = 0.8081220356417685
	)

	diffuse := (n[0]*lightX +
		n[1]*lightY +
		n[2]*lightZ) / length

	//
	// Do not use abs(diffuse). Opposite-facing surfaces should not receive
	// exactly the same illumination.
	//
	diffuse = math.Max(0, diffuse)

	//
	// Keep sufficient ambient light because this is a small thumbnail rather
	// than a physically based renderer.
	//
	return 0.62 + 0.32*diffuse
}

// triangleAreaSquared returns a value proportional to the squared area of the
// triangle. It is used only for detecting degenerate triangles, so dividing by
// four is unnecessary.
func triangleAreaSquared(
	a [3]float64,
	b [3]float64,
	c [3]float64,
) float64 {
	u := [3]float64{
		b[0] - a[0],
		b[1] - a[1],
		b[2] - a[2],
	}

	v := [3]float64{
		c[0] - a[0],
		c[1] - a[1],
		c[2] - a[2],
	}

	cross := [3]float64{
		u[1]*v[2] - u[2]*v[1],
		u[2]*v[0] - u[0]*v[2],
		u[0]*v[1] - u[1]*v[0],
	}

	return cross[0]*cross[0] +
		cross[1]*cross[1] +
		cross[2]*cross[2]
}

func colorFor(
	identifier string,
	index int,
) string {
	hasher := fnv.New32a()

	_, _ = hasher.Write(
		[]byte(identifier),
	)

	return palette[(int(hasher.Sum32())+index)%len(palette)]
}

func shadeColor(
	hexColor string,
	shade float64,
) string {
	var red int
	var green int
	var blue int

	if _, err := fmt.Sscanf(
		hexColor,
		"#%02x%02x%02x",
		&red,
		&green,
		&blue,
	); err != nil {
		return hexColor
	}

	shade = math.Max(
		0,
		math.Min(1, shade),
	)

	return fmt.Sprintf(
		"#%02x%02x%02x",
		min(
			255,
			int(float64(red)*shade),
		),
		min(
			255,
			int(float64(green)*shade),
		),
		min(
			255,
			int(float64(blue)*shade),
		),
	)
}

func partDetail(
	view workspace.DocumentView,
) string {
	if view.Document.Type == "PRODUCT" {
		return fmt.Sprintf(
			"Product · %d instances",
			len(view.ResolvedInstances),
		)
	}

	if view.Artifact != nil {
		return fmt.Sprintf(
			"Part · %d faces",
			len(view.Artifact.Mesh.Triangles),
		)
	}

	count := 0

	if view.Part != nil {
		count = len(view.Part.Features)
	}

	return fmt.Sprintf(
		"Part · %d features",
		count,
	)
}

func emptyMark(
	documentType string,
) string {
	if documentType == "PRODUCT" {
		return `<g fill="none" stroke="#6f91a6" stroke-width="2"><rect x="112" y="48" width="45" height="36"/><rect x="164" y="77" width="45" height="36"/><path d="M157 66h27v11"/></g>`
	}

	return `<g fill="none" stroke="#6f91a6" stroke-width="2"><path d="M112 104l48-56 48 56-48 31z"/><path d="M112 104h96M160 48v87"/></g>`
}
