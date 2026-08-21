package thumbnail

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"html"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/occccad/occccad/internal/workspace"
)

const (
	Width           = 320
	Height          = 200
	RendererVersion = "svg-v3"
	width           = float64(Width)
	drawingTop      = 16.0
	drawingBottom   = 184.0
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
	points  []point
	color   string
	width   float64
	opacity float64
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
	return render(context.Background(), view)
}

// RenderWithTimeout bounds thumbnail generation. A timeout produces a valid
// fixed-size default thumbnail so callers never need to turn a missing image
// into a layout decision.
func RenderWithTimeout(view workspace.DocumentView, timeout time.Duration) ([]byte, bool, error) {
	if timeout <= 0 {
		return Default(view), true, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	resultChannel := make(chan struct {
		result []byte
		err    error
	}, 1)
	go func() {
		result, err := render(ctx, view)
		resultChannel <- struct {
			result []byte
			err    error
		}{result: result, err: err}
	}()
	select {
	case result := <-resultChannel:
		if errors.Is(result.err, context.DeadlineExceeded) || errors.Is(result.err, context.Canceled) {
			return Default(view), true, nil
		}
		return result.result, false, result.err
	case <-ctx.Done():
		return Default(view), true, nil
	}
}

// Default returns the same fixed-size SVG contract used when a preview is not
// ready, unavailable, or exceeds its generation deadline.
func Default(view workspace.DocumentView) []byte {
	return defaultSVG(view.Document.Type, triangleCount(view))
}

// DefaultForType returns a fixed-size placeholder when the API only has the
// document type available and the current model has not been loaded.
func DefaultForType(documentType string) []byte {
	return defaultSVG(documentType, 0)
}

func render(ctx context.Context, view workspace.DocumentView) ([]byte, error) {
	var drawing scene

	if view.Document.Type == "PRODUCT" {
		for index, instance := range view.ResolvedInstances {
			artifact, exists := view.Artifacts[instance.GeometryKey]
			if !exists {
				continue
			}

			if err := appendMesh(ctx,
				&drawing,
				artifact.Mesh,
				instance.Translation,
				colorFor(instance.ID, index),
			); err != nil {
				return nil, err
			}
			if err := appendVisualization(ctx, &drawing, artifact.Visualization, instance.Translation); err != nil {
				return nil, err
			}
		}
	} else if view.Artifact != nil {
		if err := appendMesh(ctx,
			&drawing,
			view.Artifact.Mesh,
			[3]float64{},
			colorFor(view.Artifact.GeometryKey, 0),
		); err != nil {
			return nil, err
		}
		if err := appendVisualization(ctx, &drawing, view.Artifact.Visualization, [3]float64{}); err != nil {
			return nil, err
		}
	} else if view.Part != nil {
		if err := appendSketches(ctx, &drawing, view.Part.Features); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(drawing.faces, func(left, right int) bool {
		return drawing.faces[left].depth < drawing.faces[right].depth
	})

	transform := fit(&drawing)

	var body strings.Builder

	for _, item := range drawing.faces {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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

		fmt.Fprintf(&body, `" fill="%s" stroke="none"/>`, item.fill)
	}

	for _, item := range drawing.lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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

		strokeWidth := item.width
		if strokeWidth <= 0 {
			strokeWidth = 2
		}
		strokeOpacity := item.opacity
		if strokeOpacity <= 0 {
			strokeOpacity = 1
		}
		fmt.Fprintf(&body, `" fill="none" stroke="%s" stroke-width="%.2f" stroke-opacity="%.2f" stroke-linejoin="round"/>`,
			item.color, strokeWidth, strokeOpacity)
	}
	for _, item := range drawing.dots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		x, y := transform(item)
		fmt.Fprintf(&body, `<circle cx="%.2f" cy="%.2f" r="2.4" fill="#f4f7f8" stroke="#32556b" stroke-width="1.2"/>`, x, y)
	}

	if !drawing.hasBounds {
		body.WriteString(emptyMark(view.Document.Type))
	}

	svg := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="triangle count %d"><defs><linearGradient id="background" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#f7fafc"/><stop offset="1" stop-color="#e4edf3"/></linearGradient></defs><rect width="%d" height="%d" rx="8" fill="url(#background)"/>%s<text x="304" y="190" text-anchor="end" font-family="system-ui,sans-serif" font-size="10" fill="#647b89">△ %d</text></svg>`,
		Width, Height, Width, Height, triangleCount(view), Width, Height, body.String(), triangleCount(view),
	)

	return []byte(svg), nil
}

func defaultSVG(documentType string, triangles int) []byte {
	label := "PART"
	if documentType == "PRODUCT" {
		label = "PRODUCT"
	}
	return []byte(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="default %s thumbnail"><defs><linearGradient id="background" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#f7fafc"/><stop offset="1" stop-color="#e4edf3"/></linearGradient></defs><rect width="%d" height="%d" rx="8" fill="url(#background)"/><path d="M120 118L160 82L200 118L160 142Z" fill="#8eb6c9" fill-opacity="0.55" stroke="#47788f" stroke-width="2"/><path d="M120 118V78L160 48L200 78V118" fill="none" stroke="#47788f" stroke-width="2" stroke-linejoin="round"/><path d="M160 82V142" stroke="#47788f" stroke-width="2"/><text x="304" y="190" text-anchor="end" font-family="system-ui,sans-serif" font-size="10" fill="#647b89">△ %d</text></svg>`,
		Width, Height, Width, Height, html.EscapeString(label), Width, Height, triangles,
	))
}

func appendVisualization(ctx context.Context, target *scene, visualization workspace.VisualizationManifest, translation [3]float64) error {
	for _, primitive := range visualization.Primitives {
		if err := ctx.Err(); err != nil {
			return err
		}
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
			item := line{color: "#315b72", width: 1.5, points: make([]point, 0, len(primitive.Positions))}
			if primitive.Role == "CONSTRUCTION" {
				item.color = "#7b8c94"
				item.width = 1
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
			if err := appendMesh(ctx, target, mesh, translation, "#65a6b8"); err != nil {
				return err
			}
		}
	}
	return nil
}

func appendMesh(
	ctx context.Context,
	target *scene,
	mesh workspace.Mesh,
	translation [3]float64,
	color string,
) error {
	if len(mesh.Triangles) == 0 || len(mesh.Vertices) == 0 {
		return nil
	}

	type triangleRecord struct {
		indices [3]int
		points  [3]point
		normal  [3]float64
		depth   float64
	}
	type edgeRecord struct {
		vertices [2][3]float64
		normals  [][3]float64
	}

	triangles := make([]triangleRecord, 0, len(mesh.Triangles))
	vertexNormals := make([][][3]float64, len(mesh.Vertices))
	edges := make(map[[2]int]*edgeRecord, len(mesh.Triangles)*3)

	// Preserve every valid triangle. The first pass also builds averaged
	// vertex normals and adjacency information for smooth light and visible
	// outline/crease edges.
	for _, triangle := range mesh.Triangles {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		normal, ok := unitNormal(sourceA, sourceB, sourceC)
		if !ok {
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

		record := triangleRecord{indices: [3]int{i0, i1, i2}, points: projected,
			normal: normal, depth: depth / 3.0}
		triangles = append(triangles, record)
		for _, index := range record.indices {
			vertexNormals[index] = append(vertexNormals[index], normal)
		}
		for _, pair := range [][2]int{{i0, i1}, {i1, i2}, {i2, i0}} {
			key := pair
			if key[0] > key[1] {
				key[0], key[1] = key[1], key[0]
			}
			item := edges[key]
			if item == nil {
				item = &edgeRecord{vertices: [2][3]float64{mesh.Vertices[key[0]], mesh.Vertices[key[1]]}}
				edges[key] = item
			}
			item.normals = append(item.normals, normal)
		}
	}

	for _, item := range triangles {
		smoothNormal := [3]float64{}
		for _, index := range item.indices {
			for _, candidate := range vertexNormals[index] {
				// Do not smooth across a real crease. The threshold still
				// averages finely tessellated curved surfaces.
				if dot(candidate, item.normal) < 0.82 {
					continue
				}
				smoothNormal[0] += candidate[0]
				smoothNormal[1] += candidate[1]
				smoothNormal[2] += candidate[2]
			}
		}
		smoothNormal, ok := normalize(smoothNormal)
		if !ok {
			smoothNormal = item.normal
		}
		target.faces = append(target.faces, face{points: item.points, depth: item.depth,
			fill: shadeColor(color, faceShadeNormal(smoothNormal))})
	}

	// Draw only boundary, silhouette, and strong crease edges. Drawing every
	// tessellation seam makes curved surfaces look noisy; this keeps the shape
	// legible while retaining the useful CAD edge cues.
	viewDirection := [3]float64{0.577350269, 0.577350269, 0.577350269}
	for _, edge := range edges {
		if !visibleEdge(edge.normals, viewDirection) {
			continue
		}
		first := project(translate(edge.vertices[0], translation))
		second := project(translate(edge.vertices[1], translation))
		target.lines = append(target.lines, line{points: []point{first, second}, color: "#264e67", width: 0.8, opacity: 0.78})
	}
	return nil
}

func appendSketches(
	ctx context.Context,
	target *scene,
	features []workspace.Feature,
) error {
	for _, feature := range features {
		if err := ctx.Err(); err != nil {
			return err
		}
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
			if err := ctx.Err(); err != nil {
				return err
			}
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
	return nil
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

func unitNormal(
	a [3]float64,
	b [3]float64,
	c [3]float64,
) ([3]float64, bool) {
	u := [3]float64{b[0] - a[0], b[1] - a[1], b[2] - a[2]}
	v := [3]float64{c[0] - a[0], c[1] - a[1], c[2] - a[2]}
	return normalize([3]float64{
		u[1]*v[2] - u[2]*v[1],
		u[2]*v[0] - u[0]*v[2],
		u[0]*v[1] - u[1]*v[0],
	})
}

func normalize(value [3]float64) ([3]float64, bool) {
	length := math.Sqrt(value[0]*value[0] + value[1]*value[1] + value[2]*value[2])
	if length < 1e-12 {
		return [3]float64{}, false
	}
	return [3]float64{value[0] / length, value[1] / length, value[2] / length}, true
}

func visibleEdge(normals [][3]float64, viewDirection [3]float64) bool {
	if len(normals) <= 1 {
		return len(normals) == 1
	}
	for index, left := range normals {
		leftFacing := dot(left, viewDirection)
		for _, right := range normals[index+1:] {
			rightFacing := dot(right, viewDirection)
			if leftFacing*rightFacing <= 0 || dot(left, right) < 0.82 {
				return true
			}
		}
	}
	return false
}

func dot(left [3]float64, right [3]float64) float64 {
	return left[0]*right[0] + left[1]*right[1] + left[2]*right[2]
}

func faceShadeNormal(normal [3]float64) float64 {

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

	diffuse := normal[0]*lightX + normal[1]*lightY + normal[2]*lightZ

	//
	// Do not use abs(diffuse). Opposite-facing surfaces should not receive
	// exactly the same illumination.
	//
	diffuse = math.Max(0, diffuse)

	//
	// Keep sufficient ambient light because this is a small thumbnail rather
	// than a physically based renderer.
	//
	return 0.48 + 0.52*diffuse
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

func triangleCount(view workspace.DocumentView) int {
	if view.Document.Type == "PRODUCT" {
		count := 0
		for _, instance := range view.ResolvedInstances {
			if artifact, ok := view.Artifacts[instance.GeometryKey]; ok {
				count += len(artifact.Mesh.Triangles)
			}
		}
		return count
	}
	if view.Artifact != nil {
		return len(view.Artifact.Mesh.Triangles)
	}
	return 0
}

func emptyMark(
	documentType string,
) string {
	if documentType == "PRODUCT" {
		return `<g fill="none" stroke="#6f91a6" stroke-width="2"><rect x="112" y="48" width="45" height="36"/><rect x="164" y="77" width="45" height="36"/><path d="M157 66h27v11"/></g>`
	}

	return `<g fill="none" stroke="#6f91a6" stroke-width="2"><path d="M112 104l48-56 48 56-48 31z"/><path d="M112 104h96M160 48v87"/></g>`
}
