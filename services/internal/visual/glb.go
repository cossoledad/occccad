// Package visual decodes the single CAD display interchange: *.mesh.glb.
// Decoded meshes are consumer-local working data, never database/API payloads.
package visual

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

type Association struct {
	GeometryID   string `json:"geometryId,omitempty"`
	NamingDigest string `json:"namingDigest,omitempty"`
}

func (a Association) Validate(geometryID, namingDigest string) error {
	if a.GeometryID != "" && a.GeometryID != geometryID {
		return fmt.Errorf("visual geometry identity mismatch")
	}
	if a.NamingDigest != "" && a.NamingDigest != namingDigest {
		return fmt.Errorf("visual naming digest mismatch")
	}
	return nil
}

type Mesh struct {
	Association      Association  `json:"-"`
	Normals          [][3]float64 `json:"normals,omitempty"`
	Vertices         [][3]float64 `json:"vertices"`
	Triangles        [][3]uint32  `json:"triangles"`
	FaceIDs          []uint32     `json:"faceIds"`
	Edges            []Edge       `json:"edges"`
	TopologyVertices []Point      `json:"topologyVertices"`
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
				Attributes struct {
					Position *int
					Normal   *int
				}
				Indices    *int
				Mode       *int
				Extensions map[string]json.RawMessage
			}
		}
		Extensions         map[string]json.RawMessage
		ExtensionsRequired []string
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
	for _, required := range document.ExtensionsRequired {
		if required != "OCCCCAD_cad" && required != "OCCCCAD_visualization" {
			return fail("unsupported required extension")
		}
	}
	var cad struct {
		Units, CoordinateSpace string
		SchemaVersion          int
		Association            Association
	}
	if err := json.Unmarshal(document.Extensions["OCCCCAD_cad"], &cad); err != nil || cad.SchemaVersion != 2 || cad.Units != "mm" || cad.CoordinateSpace != "PART_LOCAL" {
		return fail("CAD extension")
	}
	empty.Association = cad.Association

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
		if a.Count > len(bin)/components/4 || v.ByteOffset > len(bin) || v.ByteLength > len(bin)-v.ByteOffset || a.ByteOffset > len(bin)-v.ByteOffset || a.ByteOffset > v.ByteLength {
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
	completeNormals := true
	for _, mesh := range document.Meshes {
		for _, p := range mesh.Primitives {
			var mapping struct {
				Kind     string
				LocalIDs int
			}
			if len(p.Extensions["OCCCCAD_cad"]) == 0 {
				continue
			} // non-topological sketch/wire primitive
			if err := json.Unmarshal(p.Extensions["OCCCCAD_cad"], &mapping); err != nil {
				return fail("primitive mapping")
			}
			if p.Attributes.Position == nil {
				return fail("missing POSITION attribute")
			}
			positions, err := read(*p.Attributes.Position, 3, 5126)
			if err != nil {
				return fail(err.Error())
			}
			ids, err := read(mapping.LocalIDs, 1, 5125)
			if err != nil {
				return fail(err.Error())
			}
			for _, id := range ids {
				if id == 0 {
					return fail("zero topology locator")
				}
			}
			indices := make([]float64, len(positions)/3)
			for i := range indices {
				indices[i] = float64(i)
			}
			if p.Indices != nil {
				indices, err = read(*p.Indices, 1, 5125)
				if err != nil {
					return fail(err.Error())
				}
			}
			for _, id := range indices {
				if id >= float64(len(positions)/3) {
					return fail("primitive index")
				}
			}
			point := func(i float64) [3]float64 {
				j := int(i) * 3
				return [3]float64{positions[j], positions[j+1], positions[j+2]}
			}
			mode := 4
			if p.Mode != nil {
				mode = *p.Mode
			}
			switch mapping.Kind {
			case "FACE":
				if mode != 4 || len(indices)%3 != 0 || len(ids) != len(indices)/3 {
					return fail("face mapping cardinality")
				}
				if p.Attributes.Normal != nil {
					values, err := read(*p.Attributes.Normal, 3, 5126)
					if err != nil {
						return fail(err.Error())
					}
					if len(values) != len(positions) {
						return fail("normal count")
					}
					for i := 0; i < len(values); i += 3 {
						empty.Normals = append(empty.Normals, [3]float64{values[i], values[i+1], values[i+2]})
					}
				} else {
					completeNormals = false
				}
				base := uint32(len(empty.Vertices))
				for i := 0; i < len(positions)/3; i++ {
					empty.Vertices = append(empty.Vertices, point(float64(i)))
				}
				for i := 0; i < len(indices); i += 3 {
					empty.Triangles = append(empty.Triangles, [3]uint32{base + uint32(indices[i]), base + uint32(indices[i+1]), base + uint32(indices[i+2])})
					empty.FaceIDs = append(empty.FaceIDs, uint32(ids[i/3]))
				}
			case "EDGE":
				count := len(indices) - 1
				if mode == 1 {
					count = len(indices) / 2
				}
				if (mode != 1 && mode != 3) || (mode == 1 && len(indices)%2 != 0) || count != len(ids) {
					return fail("edge mapping cardinality")
				}
				for i, id := range ids {
					j := i
					if mode == 1 {
						j = i * 2
					}
					if i > 0 && len(empty.Edges) > 0 && empty.Edges[len(empty.Edges)-1].LocalID == uint64(id) && empty.Edges[len(empty.Edges)-1].Points[len(empty.Edges[len(empty.Edges)-1].Points)-1] == point(indices[j]) {
						last := &empty.Edges[len(empty.Edges)-1]
						last.Points = append(last.Points, point(indices[j+1]))
					} else {
						empty.Edges = append(empty.Edges, Edge{LocalID: uint64(id), Points: [][3]float64{point(indices[j]), point(indices[j+1])}})
					}
				}
			case "VERTEX":
				if mode != 0 || len(ids) != len(indices) {
					return fail("vertex mapping cardinality")
				}
				for i, id := range ids {
					empty.TopologyVertices = append(empty.TopologyVertices, Point{LocalID: uint64(id), Point: point(indices[i])})
				}
			default:
				return fail("unknown topology kind")
			}
		}
	}
	if !completeNormals {
		empty.Normals = nil
	}
	var visualization map[string]any
	if raw := document.Extensions["OCCCCAD_visualization"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &visualization); err != nil {
			return fail("visualization metadata")
		}
		if visualization["schemaVersion"] != float64(2) {
			return fail("visualization schema")
		}
		primitives, _ := visualization["primitives"].([]any)
		for _, value := range primitives {
			p, ok := value.(map[string]any)
			if !ok {
				return fail("visualization primitive")
			}
			for _, field := range []string{"positions", "indices"} {
				index, exists := p[field]
				if !exists {
					if field == "positions" {
						p[field] = [][3]float64{}
					}
					continue
				}
				n, ok := index.(float64)
				if !ok || n != math.Trunc(n) || n < 0 || n >= float64(len(document.Accessors)) {
					return fail("visualization accessor")
				}
				components, ctype := 1, 5125
				if field == "positions" {
					components, ctype = 3, 5126
				}
				values, err := read(int(n), components, ctype)
				if err != nil {
					return fail(err.Error())
				}
				if components == 3 {
					points := make([][3]float64, 0, len(values)/3)
					for i := 0; i < len(values); i += 3 {
						points = append(points, [3]float64{values[i], values[i+1], values[i+2]})
					}
					p[field] = points
				} else {
					p[field] = values
				}
			}
		}
	}
	var raw json.RawMessage
	if visualization != nil {
		raw, _ = json.Marshal(visualization)
	}
	return empty, raw, nil
}
