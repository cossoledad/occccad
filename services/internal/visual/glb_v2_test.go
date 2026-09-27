package visual

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
)

func TestGLBV2MultiplePrimitivesAndTopologyMapping(t *testing.T) {
	var bin []byte
	var views, accessors []any
	add := func(values []float64, components, componentType int) int {
		start := len(bin)
		for _, v := range values {
			bits := uint32(v)
			if componentType == 5126 {
				bits = math.Float32bits(float32(v))
			}
			var b [4]byte
			binary.LittleEndian.PutUint32(b[:], bits)
			bin = append(bin, b[:]...)
		}
		id := len(accessors)
		kind := "SCALAR"
		if components == 3 {
			kind = "VEC3"
		}
		views = append(views, map[string]any{"buffer": 0, "byteOffset": start, "byteLength": len(bin) - start})
		accessors = append(accessors, map[string]any{"bufferView": id, "componentType": componentType, "count": len(values) / components, "type": kind})
		return id
	}
	positions := add([]float64{0, 0, 0, 1, 0, 0, 0, 1, 0}, 3, 5126)
	normals := add([]float64{0, 0, 1, 0, 0, 1, 0, 0, 1}, 3, 5126)
	indices := add([]float64{0, 1, 2}, 1, 5125)
	faces := add([]float64{1}, 1, 5125)
	edges := add([]float64{1, 1}, 1, 5125)
	vertices := add([]float64{1, 2, 3}, 1, 5125)
	primitive := func(kind string, mode, ids int) map[string]any {
		return map[string]any{"attributes": map[string]any{"POSITION": positions}, "mode": mode, "extensions": map[string]any{"OCCCCAD_cad": map[string]any{"kind": kind, "localIds": ids}}}
	}
	face := primitive("FACE", 4, faces)
	face["indices"] = indices
	face["attributes"].(map[string]any)["NORMAL"] = normals
	doc := map[string]any{"asset": map[string]string{"version": "2.0"}, "buffers": []any{map[string]any{"byteLength": len(bin)}}, "bufferViews": views, "accessors": accessors, "meshes": []any{map[string]any{"primitives": []any{face, primitive("EDGE", 3, edges)}}, map[string]any{"primitives": []any{face, primitive("VERTEX", 0, vertices)}}}, "extensions": map[string]any{"OCCCCAD_cad": map[string]any{"schemaVersion": 2, "units": "mm", "coordinateSpace": "PART_LOCAL"}}}
	jsonData, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	for len(jsonData)%4 != 0 {
		jsonData = append(jsonData, ' ')
	}
	data := make([]byte, 28+len(jsonData)+len(bin))
	for offset, v := range map[int]uint32{0: 0x46546c67, 4: 2, 8: uint32(len(data)), 12: uint32(len(jsonData)), 16: 0x4e4f534a, 20 + len(jsonData): uint32(len(bin)), 24 + len(jsonData): 0x004e4942} {
		binary.LittleEndian.PutUint32(data[offset:], v)
	}
	copy(data[20:], jsonData)
	copy(data[28+len(jsonData):], bin)
	mesh, _, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(mesh.Triangles) != 2 || mesh.Triangles[1] != [3]uint32{3, 4, 5} || len(mesh.Normals) != 6 {
		t.Fatal("multiple primitive offsets/normals")
	}
	if mesh.FaceIDs[0] != 1 || mesh.FaceIDs[1] != 1 || len(mesh.Edges) != 1 || mesh.Edges[0].LocalID != 1 || len(mesh.Edges[0].Points) != 3 || mesh.TopologyVertices[0].LocalID != 1 {
		t.Fatalf("inconsistent topology mapping: %+v", mesh)
	}
}
