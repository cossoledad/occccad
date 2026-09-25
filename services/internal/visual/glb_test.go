package visual

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
)

func TestDecodeRejectsCorruptCADBuffers(t *testing.T) {
	fixture := func(count, offset int, units string) []byte {
		document := map[string]any{
			"asset":       map[string]any{"version": "2.0"},
			"bufferViews": []any{map[string]any{"buffer": 0, "byteOffset": offset, "byteLength": 28}},
			"accessors": []any{
				map[string]any{"bufferView": 0, "count": count, "type": "VEC3", "componentType": 5126},
				map[string]any{"bufferView": 0, "byteOffset": 12, "count": 3, "type": "SCALAR", "componentType": 5125},
				map[string]any{"bufferView": 0, "byteOffset": 24, "count": 1, "type": "SCALAR", "componentType": 5125},
			},
			"meshes":     []any{map[string]any{"primitives": []any{map[string]any{"attributes": map[string]any{"POSITION": 0}, "indices": 1}}}},
			"extensions": map[string]any{"OCCCCAD_cad": map[string]any{"schemaVersion": 1, "units": units, "coordinateSpace": "PART_LOCAL", "faceIds": 2}},
		}
		raw, _ := json.Marshal(document)
		for len(raw)%4 != 0 {
			raw = append(raw, ' ')
		}
		result := make([]byte, 28+len(raw)+28)
		for offset, value := range map[int]uint32{0: 0x46546c67, 4: 2, 8: uint32(len(result)), 12: uint32(len(raw)), 16: 0x4e4f534a, 20 + len(raw): 28, 24 + len(raw): 0x004e4942} {
			binary.LittleEndian.PutUint32(result[offset:], value)
		}
		copy(result[20:], raw)
		return result
	}
	if mesh, _, err := Decode(fixture(1, 0, "mm")); err != nil || len(mesh.Triangles) != 1 {
		t.Fatalf("valid fixture: %v", err)
	}
	for name, data := range map[string][]byte{"overflow count": fixture(math.MaxInt, 0, "mm"), "negative offset": fixture(1, -1, "mm"), "overflow offset": fixture(1, math.MaxInt, "mm"), "wrong units": fixture(1, 0, "m")} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Decode(data); err == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
}
