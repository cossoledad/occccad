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
			"meshes":     []any{map[string]any{"primitives": []any{map[string]any{"attributes": map[string]any{"POSITION": 0}, "indices": 1, "extensions": map[string]any{"OCCCCAD_cad": map[string]any{"kind": "FACE", "localIds": 2}}}}}},
			"extensions": map[string]any{"OCCCCAD_cad": map[string]any{"schemaVersion": 2, "units": units, "coordinateSpace": "PART_LOCAL", "faceIds": 2}},
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
		binary.LittleEndian.PutUint32(result[28+len(raw)+24:], 1)
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

func TestDecodeSurfaceNormalsAndEdgeProfile(t *testing.T) {
	fixture := func(mutate func(map[string]any)) []byte {
		var bin []byte
		var views, accessors []any
		attr := func(kind string, ctype int, values ...float64) int {
			offset := len(bin)
			for _, v := range values {
				bits := uint32(v)
				if ctype == 5126 {
					bits = math.Float32bits(float32(v))
				}
				bin = binary.LittleEndian.AppendUint32(bin, bits)
			}
			n := 1
			if kind == "VEC3" {
				n = 3
			}
			if kind == "VEC4" {
				n = 4
			}
			views = append(views, map[string]any{"buffer": 0, "byteOffset": offset, "byteLength": len(bin) - offset})
			accessors = append(accessors, map[string]any{"bufferView": len(views) - 1, "count": len(values) / n, "type": kind, "componentType": ctype})
			return len(accessors) - 1
		}
		pos := attr("VEC3", 5126, 0, 0, 0, 1, 0, 0, 0, 1, 0)
		norm := attr("VEC3", 5126, 0, 0, 1, 0, 0, 1, 0, 0, 1)
		tri := attr("SCALAR", 5125, 0, 1, 2)
		face := attr("SCALAR", 5125, 1)
		edgePos := attr("VEC3", 5126, 0, 0, 0, 1, 0, 0)
		edgeID := attr("SCALAR", 5125, 2)
		normalA := attr("VEC3", 5126, 0, 0, 1, 0, 0, 1)
		metadata := attr("VEC4", 5125, 2, 1, 1, 5)
		profile := map[string]any{"schemaVersion": 1, "source": "OCCT_EDGE_USE_V1", "positions": edgePos, "normalA": normalA, "normalB": normalA, "metadata": metadata}
		if mutate != nil {
			mutate(profile)
		}
		document := map[string]any{"asset": map[string]any{"version": "2.0"}, "accessors": accessors, "bufferViews": views, "extensions": map[string]any{"OCCCCAD_cad": map[string]any{"schemaVersion": 2, "units": "mm", "coordinateSpace": "PART_LOCAL", "edgeProfile": profile}}, "meshes": []any{map[string]any{"primitives": []any{
			map[string]any{"attributes": map[string]any{"POSITION": pos, "NORMAL": norm}, "indices": tri, "extensions": map[string]any{"OCCCCAD_cad": map[string]any{"kind": "FACE", "localIds": face}}},
			map[string]any{"attributes": map[string]any{"POSITION": edgePos}, "mode": 3, "extensions": map[string]any{"OCCCCAD_cad": map[string]any{"kind": "EDGE", "localIds": edgeID}}},
		}}}}
		raw, _ := json.Marshal(document)
		for len(raw)%4 != 0 {
			raw = append(raw, ' ')
		}
		result := make([]byte, 28+len(raw)+len(bin))
		for offset, value := range map[int]uint32{0: 0x46546c67, 4: 2, 8: uint32(len(result)), 12: uint32(len(raw)), 16: 0x4e4f534a, 20 + len(raw): uint32(len(bin)), 24 + len(raw): 0x004e4942} {
			binary.LittleEndian.PutUint32(result[offset:], value)
		}
		copy(result[20:], raw)
		copy(result[28+len(raw):], bin)
		return result
	}
	mesh, _, err := Decode(fixture(nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(mesh.Normals) != 3 || mesh.EdgeProfile == nil || len(mesh.EdgeProfile.Metadata) != 1 || mesh.EdgeProfile.Metadata[0][0] != 2 {
		t.Fatal("surface evidence lost")
	}
	for _, mutate := range []func(map[string]any){func(p map[string]any) { delete(p, "normalA") }, func(p map[string]any) { p["metadata"] = 0 }, func(p map[string]any) { p["normalB"] = -1 }} {
		if _, _, err := Decode(fixture(mutate)); err == nil {
			t.Fatal("accepted corrupt edge evidence")
		}
	}
	mesh, _, err = Decode(fixture(func(p map[string]any) { p["source"] = "UNKNOWN" }))
	if err != nil || mesh.EdgeProfile != nil {
		t.Fatalf("unknown evidence should retain topological fallback: %v", err)
	}
}
