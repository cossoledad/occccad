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
	maximumFaces  = 1400
)

type point struct{ x, y, depth float64 }

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
	all   []point
}

var palette = []string{"#4d9aca", "#6b8fd3", "#48a6a0", "#8b79c6", "#ca8058", "#5f9f72"}

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
			appendMesh(&drawing, artifact.Mesh, instance.Translation, colorFor(instance.ID, index))
		}
	} else if view.Artifact != nil {
		appendMesh(&drawing, view.Artifact.Mesh, [3]float64{}, colorFor(view.Artifact.GeometryKey, 0))
	} else if view.Part != nil {
		appendSketches(&drawing, view.Part.Features)
	}

	transform := fit(drawing.all)
	var body strings.Builder
	for _, item := range drawing.faces {
		body.WriteString(`<polygon points="`)
		for index, vertex := range item.points {
			if index > 0 {
				body.WriteByte(' ')
			}
			x, y := transform(vertex)
			fmt.Fprintf(&body, "%.2f,%.2f", x, y)
		}
		fmt.Fprintf(&body, `" fill="%s" stroke="#24516d" stroke-width="0.55" stroke-linejoin="round"/>`, item.fill)
	}
	for _, item := range drawing.lines {
		body.WriteString(`<polyline points="`)
		for index, vertex := range item.points {
			if index > 0 {
				body.WriteByte(' ')
			}
			x, y := transform(vertex)
			fmt.Fprintf(&body, "%.2f,%.2f", x, y)
		}
		fmt.Fprintf(&body, `" fill="none" stroke="%s" stroke-width="2" stroke-linejoin="round"/>`, item.color)
	}
	if len(drawing.all) == 0 {
		body.WriteString(emptyMark(view.Document.Type))
	}

	detail := partDetail(view)
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="320" height="200" viewBox="0 0 320 200" role="img" aria-label="%s"><defs><linearGradient id="background" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#f7fafc"/><stop offset="1" stop-color="#e4edf3"/></linearGradient></defs><rect width="320" height="200" rx="8" fill="url(#background)"/><path d="M0 164H320" stroke="#ccdbe4"/>%s<text x="16" y="181" font-family="system-ui,sans-serif" font-size="12" font-weight="600" fill="#203746">%s</text><text x="304" y="181" text-anchor="end" font-family="system-ui,sans-serif" font-size="10" fill="#647b89">%s</text></svg>`,
		html.EscapeString(view.Document.Name), body.String(), html.EscapeString(view.Document.Name), html.EscapeString(detail))
	return []byte(svg), nil
}

func appendMesh(target *scene, mesh workspace.Mesh, translation [3]float64, color string) {
	if len(mesh.Triangles) == 0 || len(mesh.Vertices) == 0 {
		return
	}
	step := int(math.Ceil(float64(len(mesh.Triangles)) / maximumFaces))
	if step < 1 {
		step = 1
	}
	for index := 0; index < len(mesh.Triangles); index += step {
		triangle := mesh.Triangles[index]
		if int(triangle[0]) >= len(mesh.Vertices) || int(triangle[1]) >= len(mesh.Vertices) || int(triangle[2]) >= len(mesh.Vertices) {
			continue
		}
		var projected [3]point
		var depth float64
		for vertexIndex, sourceIndex := range triangle {
			vertex := mesh.Vertices[sourceIndex]
			vertex[0] += translation[0]
			vertex[1] += translation[1]
			vertex[2] += translation[2]
			projected[vertexIndex] = project(vertex)
			depth += projected[vertexIndex].depth
			target.all = append(target.all, projected[vertexIndex])
		}
		shade := faceShade(mesh.Vertices[triangle[0]], mesh.Vertices[triangle[1]], mesh.Vertices[triangle[2]])
		target.faces = append(target.faces, face{points: projected, depth: depth / 3, fill: shadeColor(color, shade)})
	}
	sort.SliceStable(target.faces, func(left, right int) bool { return target.faces[left].depth < target.faces[right].depth })
}

func appendSketches(target *scene, features []workspace.Feature) {
	for _, feature := range features {
		if !strings.Contains(strings.ToUpper(feature.Type), "SKETCH") {
			continue
		}
		rectangle := feature.Rectangle
		if rectangle == nil {
			continue
		}
		u0, v0 := rectangle.Origin[0], rectangle.Origin[1]
		coordinates := [][2]float64{{u0, v0}, {u0 + rectangle.Width, v0}, {u0 + rectangle.Width, v0 + rectangle.Height}, {u0, v0 + rectangle.Height}, {u0, v0}}
		item := line{color: "#2679aa"}
		for _, coordinate := range coordinates {
			world := sketchPoint(feature.Plane, coordinate)
			projected := project(world)
			item.points = append(item.points, projected)
			target.all = append(target.all, projected)
		}
		target.lines = append(target.lines, item)
	}
}

func sketchPoint(plane string, coordinate [2]float64) [3]float64 {
	switch strings.ToUpper(plane) {
	case "XZ":
		return [3]float64{coordinate[0], 0, coordinate[1]}
	case "YZ":
		return [3]float64{0, coordinate[0], coordinate[1]}
	default:
		return [3]float64{coordinate[0], coordinate[1], 0}
	}
}

func project(vertex [3]float64) point {
	return point{x: .8660254 * (vertex[0] - vertex[1]), y: .5*(vertex[0]+vertex[1]) - vertex[2],
		depth: vertex[0] + vertex[1] + vertex[2]}
}

func fit(points []point) func(point) (float64, float64) {
	if len(points) == 0 {
		return func(point) (float64, float64) { return width / 2, (drawingTop + drawingBottom) / 2 }
	}
	minX, maxX, minY, maxY := points[0].x, points[0].x, points[0].y, points[0].y
	for _, item := range points[1:] {
		minX, maxX = math.Min(minX, item.x), math.Max(maxX, item.x)
		minY, maxY = math.Min(minY, item.y), math.Max(maxY, item.y)
	}
	dx, dy := math.Max(maxX-minX, 1e-9), math.Max(maxY-minY, 1e-9)
	scale := math.Min((width-40)/dx, (drawingBottom-drawingTop)/dy)
	offsetX := (width - scale*(minX+maxX)) / 2
	offsetY := (drawingTop + drawingBottom + scale*(minY+maxY)) / 2
	return func(value point) (float64, float64) { return offsetX + scale*value.x, offsetY - scale*value.y }
}

func faceShade(a, b, c [3]float64) float64 {
	u := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	v := [3]float64{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
	n := [3]float64{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
	length := math.Sqrt(n[0]*n[0] + n[1]*n[1] + n[2]*n[2])
	if length == 0 {
		return .8
	}
	light := math.Abs((n[0]*.3 + n[1]*-.5 + n[2]*.8) / length)
	return .62 + .32*light
}

func colorFor(identifier string, index int) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(identifier))
	return palette[(int(hasher.Sum32())+index)%len(palette)]
}

func shadeColor(hexColor string, shade float64) string {
	var red, green, blue int
	if _, err := fmt.Sscanf(hexColor, "#%02x%02x%02x", &red, &green, &blue); err != nil {
		return hexColor
	}
	return fmt.Sprintf("#%02x%02x%02x", min(255, int(float64(red)*shade)),
		min(255, int(float64(green)*shade)), min(255, int(float64(blue)*shade)))
}

func partDetail(view workspace.DocumentView) string {
	if view.Document.Type == "PRODUCT" {
		return fmt.Sprintf("Product · %d instances", len(view.ResolvedInstances))
	}
	if view.Artifact != nil {
		return fmt.Sprintf("Part · %d faces", len(view.Artifact.Mesh.Triangles))
	}
	count := 0
	if view.Part != nil {
		count = len(view.Part.Features)
	}
	return fmt.Sprintf("Part · %d features", count)
}

func emptyMark(documentType string) string {
	if documentType == "PRODUCT" {
		return `<g fill="none" stroke="#6f91a6" stroke-width="2"><rect x="112" y="48" width="45" height="36"/><rect x="164" y="77" width="45" height="36"/><path d="M157 66h27v11"/></g>`
	}
	return `<g fill="none" stroke="#6f91a6" stroke-width="2"><path d="M112 104l48-56 48 56-48 31z"/><path d="M112 104h96M160 48v87"/></g>`
}
