// Package visual decodes the single CAD display interchange: *.mesh.glb.
// Decoded meshes are consumer-local working data, never database/API payloads.
package visual

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

type Mesh struct {
	Vertices         [][3]float64      `json:"vertices"`
	Triangles        [][3]uint32       `json:"triangles"`
	FaceIDs          []uint32          `json:"faceIds"`
	Edges            []Edge            `json:"edges"`
	TopologyVertices []Point           `json:"topologyVertices"`
	StableIDs        map[string]string `json:"stableIds"`
}
type Edge struct {
	LocalID uint64       `json:"localId"`
	Points  [][3]float64 `json:"points"`
}
type Point struct {
	LocalID uint64     `json:"localId"`
	Point   [3]float64 `json:"point"`
}

func Decode(data []byte) (Mesh, json.RawMessage, error) {
	empty := Mesh{Vertices: [][3]float64{}, Triangles: [][3]uint32{}, FaceIDs: []uint32{}, Edges: []Edge{}, TopologyVertices: []Point{}}
	fail := func(message string) (Mesh, json.RawMessage, error) {
		return empty, nil, fmt.Errorf("invalid CAD GLB: %s", message)
	}
	if len(data) < 20 || binary.LittleEndian.Uint32(data) != 0x46546c67 || binary.LittleEndian.Uint32(data[4:]) != 2 || uint64(binary.LittleEndian.Uint32(data[8:])) != uint64(len(data)) {
		return fail("header")
	}
	var document struct {
		Asset     struct{ Version string }
		Accessors []struct {
			BufferView    int
			ByteOffset    int
			ComponentType int
			Count         int
			Type          string
		}
		BufferViews []struct {
			Buffer     int
			ByteOffset int
			ByteLength int
			ByteStride int
		}
		Meshes []struct {
			Primitives []struct {
				Attributes struct{ Position int }
				Indices    int
			}
		}
		Extensions map[string]json.RawMessage
	}
	var bin []byte
	for offset := 12; offset < len(data); {
		if offset+8 > len(data) {
			return fail("chunk header")
		}
		n := int(binary.LittleEndian.Uint32(data[offset:]))
		kind := binary.LittleEndian.Uint32(data[offset+4:])
		offset += 8
		if n > len(data)-offset || n%4 != 0 {
			return fail("chunk length")
		}
		switch kind {
		case 0x4e4f534a:
			if err := json.Unmarshal(data[offset:offset+n], &document); err != nil {
				return fail("JSON")
			}
		case 0x004e4942:
			bin = data[offset : offset+n]
		}
		offset += n
	}
	if document.Asset.Version != "2.0" {
		return fail("asset version")
	}
	visualization := document.Extensions["OCCCCAD_visualization"]
	if len(document.Meshes) == 0 {
		return empty, visualization, nil
	}
	var cad struct {
		Units           string
		CoordinateSpace string
		SchemaVersion   int
		FaceIDs         int
		Edges           []struct {
			LocalID   uint64
			Positions int
		}
		Vertices  []Point
		StableIDs map[string]string
	}
	if err := json.Unmarshal(document.Extensions["OCCCCAD_cad"], &cad); err != nil || cad.SchemaVersion != 1 || cad.Units != "mm" || cad.CoordinateSpace != "PART_LOCAL" {
		return fail("CAD extension")
	}
	read := func(index, components, ctype int) ([]float64, error) {
		if index < 0 || index >= len(document.Accessors) {
			return nil, fmt.Errorf("accessor index")
		}
		a := document.Accessors[index]
		if a.BufferView < 0 || a.BufferView >= len(document.BufferViews) || a.ComponentType != ctype || a.Count < 0 {
			return nil, fmt.Errorf("accessor layout")
		}
		v := document.BufferViews[a.BufferView]
		expected := "SCALAR"
		if components == 3 {
			expected = "VEC3"
		}
		if a.Type != expected || v.Buffer != 0 || v.ByteStride != 0 || v.ByteOffset < 0 || v.ByteLength < 0 || a.ByteOffset < 0 {
			return nil, fmt.Errorf("unsupported layout")
		}
		// Bound each term before multiplication/addition, including malicious JSON counts.
		if a.Count > len(bin)/components/4 || v.ByteOffset > len(bin) || a.ByteOffset > len(bin)-v.ByteOffset || a.ByteOffset > v.ByteLength {
			return nil, fmt.Errorf("accessor bounds")
		}
		size := a.Count * components * 4
		offset := v.ByteOffset + a.ByteOffset
		if size > v.ByteLength-a.ByteOffset || size > len(bin)-offset {
			return nil, fmt.Errorf("accessor bounds")
		}
		result := make([]float64, a.Count*components)
		for i := range result {
			bits := binary.LittleEndian.Uint32(bin[int(offset)+i*4:])
			if ctype == 5126 {
				result[i] = float64(math.Float32frombits(bits))
				if math.IsNaN(result[i]) || math.IsInf(result[i], 0) {
					return nil, fmt.Errorf("nonfinite position")
				}
			} else {
				result[i] = float64(bits)
			}
		}
		return result, nil
	}
	if len(document.Meshes[0].Primitives) != 1 {
		return fail("solid primitive")
	}
	p := document.Meshes[0].Primitives[0]
	positions, err := read(p.Attributes.Position, 3, 5126)
	if err != nil {
		return fail(err.Error())
	}
	indices, err := read(p.Indices, 1, 5125)
	if err != nil {
		return fail(err.Error())
	}
	faces, err := read(cad.FaceIDs, 1, 5125)
	if err != nil {
		return fail(err.Error())
	}
	if len(indices)%3 != 0 || len(faces) != len(indices)/3 {
		return fail("face mapping cardinality")
	}
	for i := 0; i < len(positions); i += 3 {
		empty.Vertices = append(empty.Vertices, [3]float64{positions[i], positions[i+1], positions[i+2]})
	}
	for i := 0; i < len(indices); i += 3 {
		t := [3]uint32{uint32(indices[i]), uint32(indices[i+1]), uint32(indices[i+2])}
		for _, id := range t {
			if uint64(id) >= uint64(len(empty.Vertices)) {
				return fail("triangle index")
			}
		}
		empty.Triangles = append(empty.Triangles, t)
	}
	for _, id := range faces {
		empty.FaceIDs = append(empty.FaceIDs, uint32(id))
	}
	for _, edge := range cad.Edges {
		if edge.LocalID == 0 {
			return fail("edge identity")
		}
		points, err := read(edge.Positions, 3, 5126)
		if err != nil {
			return fail(err.Error())
		}
		e := Edge{LocalID: edge.LocalID}
		for i := 0; i < len(points); i += 3 {
			e.Points = append(e.Points, [3]float64{points[i], points[i+1], points[i+2]})
		}
		empty.Edges = append(empty.Edges, e)
	}
	for _, vertex := range cad.Vertices {
		if vertex.LocalID == 0 {
			return fail("vertex identity")
		}
		for _, value := range vertex.Point {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fail("vertex coordinate")
			}
		}
	}
	empty.TopologyVertices = cad.Vertices
	empty.StableIDs = cad.StableIDs
	return empty, visualization, nil
}
