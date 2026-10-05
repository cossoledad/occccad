package workspace

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

const visualizationExtension = "OCCCCAD_visualization"

// glbWithVisualization mirrors the complete Part display manifest into a
// vendor GLB extension. It remains valid without a triangle mesh, so an empty
// Part or sketch-only Part is still a real visualization artifact.
func glbWithVisualization(source []byte, visualization VisualizationManifest, association ...map[string]string) ([]byte, error) {
	var document map[string]any
	var binaryChunk []byte
	if len(source) == 0 {
		document = map[string]any{
			"asset":  map[string]any{"version": "2.0", "generator": "occccad metadata service"},
			"scene":  0,
			"scenes": []any{map[string]any{"nodes": []any{}}},
			"nodes":  []any{},
		}
	} else {
		if len(source) < 20 || binary.LittleEndian.Uint32(source[0:4]) != 0x46546c67 || binary.LittleEndian.Uint32(source[4:8]) != 2 || uint64(binary.LittleEndian.Uint32(source[8:12])) != uint64(len(source)) {
			return nil, fmt.Errorf("invalid GLB header")
		}
		offset := 12
		for offset+8 <= len(source) {
			length := int(binary.LittleEndian.Uint32(source[offset : offset+4]))
			kind := binary.LittleEndian.Uint32(source[offset+4 : offset+8])
			offset += 8
			if length < 0 || length%4 != 0 || length > len(source)-offset {
				return nil, fmt.Errorf("invalid GLB chunk length")
			}
			chunk := source[offset : offset+length]
			switch kind {
			case 0x4e4f534a:
				if err := json.Unmarshal(chunk, &document); err != nil {
					return nil, fmt.Errorf("decode GLB JSON: %w", err)
				}
			case 0x004e4942:
				binaryChunk = append([]byte(nil), chunk...)
			}
			offset += length
		}
	}
	extensions, _ := document["extensions"].(map[string]any)
	if extensions == nil {
		extensions = map[string]any{}
		document["extensions"] = extensions
	}
	// Auxiliary buffers are owned as a suffix, so a visualization variant replaces
	// its previous suffix instead of retaining obsolete sketch arrays indefinitely.
	if previous, ok := extensions[visualizationExtension].(map[string]any); ok {
		for _, field := range []string{"bufferStart", "accessorStart", "bufferViewStart", "meshStart", "nodeStart"} {
			n, ok := previous[field].(float64)
			if !ok || n < 0 || n != math.Trunc(n) {
				return nil, fmt.Errorf("invalid visualization ownership range")
			}
			if field == "bufferStart" {
				if n > float64(len(binaryChunk)) {
					return nil, fmt.Errorf("invalid visualization binary suffix")
				}
				binaryChunk = binaryChunk[:int(n)]
				continue
			}
			key := map[string]string{"accessorStart": "accessors", "bufferViewStart": "bufferViews", "meshStart": "meshes", "nodeStart": "nodes"}[field]
			entries, _ := document[key].([]any)
			if n > float64(len(entries)) {
				return nil, fmt.Errorf("invalid visualization suffix")
			}
			document[key] = entries[:int(n)]
		}
	}
	// The runtime manifest is materialized into binary accessors, never JSON numeric arrays.
	raw, err := json.Marshal(visualization)
	if err != nil {
		return nil, err
	}
	var metadata map[string]any
	if err = json.Unmarshal(raw, &metadata); err != nil {
		return nil, err
	}
	if visualization.FeatureAssociations == nil {
		if previous, ok := extensions[visualizationExtension].(map[string]any); ok {
			for _, key := range []string{"featureAssociations", "featureContributions"} {
				if derived := previous[key]; derived != nil {
					metadata[key] = derived
				}
			}
		}
	}
	metadata["schemaVersion"] = 2
	views, _ := document["bufferViews"].([]any)
	accessors, _ := document["accessors"].([]any)
	metadata["bufferStart"] = len(binaryChunk)
	metadata["accessorStart"] = len(accessors)
	metadata["bufferViewStart"] = len(views)
	meshes, _ := document["meshes"].([]any)
	nodes, _ := document["nodes"].([]any)
	metadata["meshStart"] = len(meshes)
	metadata["nodeStart"] = len(nodes)
	add := func(values []float64, components int, floating bool) int {
		start := len(binaryChunk)
		for _, value := range values {
			var bits uint32
			if floating {
				bits = math.Float32bits(float32(value))
			} else {
				bits = uint32(value)
			}
			var bytes [4]byte
			binary.LittleEndian.PutUint32(bytes[:], bits)
			binaryChunk = append(binaryChunk, bytes[:]...)
		}
		view := len(views)
		views = append(views, map[string]any{"buffer": 0, "byteOffset": start, "byteLength": len(binaryChunk) - start})
		component, kind := 5125, "SCALAR"
		if floating {
			component = 5126
		}
		if components == 3 {
			kind = "VEC3"
		}
		id := len(accessors)
		a := map[string]any{"bufferView": view, "componentType": component, "count": len(values) / components, "type": kind}
		if components == 3 && len(values) > 0 {
			lo, hi := [3]float64{values[0], values[1], values[2]}, [3]float64{values[0], values[1], values[2]}
			for i, v := range values {
				lo[i%3] = math.Min(lo[i%3], v)
				hi[i%3] = math.Max(hi[i%3], v)
			}
			a["min"] = lo
			a["max"] = hi
		}
		accessors = append(accessors, a)
		return id
	}
	primitives, _ := metadata["primitives"].([]any)
	var displayPrimitives []any
	for _, entry := range primitives {
		p := entry.(map[string]any)
		points, _ := p["positions"].([]any)
		var values []float64
		for _, point := range points {
			for _, value := range point.([]any) {
				values = append(values, value.(float64))
			}
		}
		if len(values) == 0 {
			delete(p, "positions")
			delete(p, "indices")
			continue
		}
		p["positions"] = add(values, 3, true)
		if indices, ok := p["indices"].([]any); ok {
			values = nil
			for _, i := range indices {
				values = append(values, i.(float64))
			}
			p["indices"] = add(values, 1, false)
		}
		mode := 0
		switch p["kind"] {
		case "POLYLINE":
			mode = 3
		case "TRIANGLES":
			mode = 4
		}
		display := map[string]any{"attributes": map[string]any{"POSITION": p["positions"]}, "mode": mode}
		if indices, ok := p["indices"]; ok {
			display["indices"] = indices
		}
		displayPrimitives = append(displayPrimitives, display)
	}
	if len(displayPrimitives) > 0 {
		nodes = append(nodes, map[string]any{"mesh": len(meshes)})
		meshes = append(meshes, map[string]any{"primitives": displayPrimitives})
	}
	document["meshes"] = meshes
	document["nodes"] = nodes
	sceneNodes := make([]int, len(nodes))
	for i := range sceneNodes {
		sceneNodes[i] = i
	}
	document["scenes"] = []any{map[string]any{"nodes": sceneNodes}}
	document["scene"] = 0

	document["bufferViews"] = views
	document["accessors"] = accessors
	document["buffers"] = []any{map[string]any{"byteLength": len(binaryChunk)}}
	if extensions["OCCCCAD_cad"] == nil {
		extensions["OCCCCAD_cad"] = map[string]any{"schemaVersion": 2, "units": "mm", "coordinateSpace": "PART_LOCAL"}
	}
	if len(association) > 0 {
		extensions["OCCCCAD_cad"].(map[string]any)["association"] = association[0]
	}
	extensions[visualizationExtension] = metadata
	used, _ := document["extensionsUsed"].([]any)
	found := false
	for _, entry := range used {
		if entry == visualizationExtension {
			found = true
		}
	}
	if extensions["OCCCCAD_cad"] != nil {
		hasCAD := false
		for _, entry := range used {
			if entry == "OCCCCAD_cad" {
				hasCAD = true
			}
		}
		if !hasCAD {
			used = append(used, "OCCCCAD_cad")
		}
	}
	if !found {
		used = append(used, visualizationExtension)
	}
	document["extensionsUsed"] = used
	if len(binaryChunk) == 0 {
		delete(document, "buffers")
	}
	for _, key := range []string{"meshes", "nodes", "accessors", "bufferViews"} {
		if values, _ := document[key].([]any); len(values) == 0 {
			delete(document, key)
		}
	}
	if len(nodes) == 0 {
		document["scenes"] = []any{map[string]any{}}
	}
	jsonChunk, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	for len(jsonChunk)%4 != 0 {
		jsonChunk = append(jsonChunk, ' ')
	}
	for len(binaryChunk)%4 != 0 {
		binaryChunk = append(binaryChunk, 0)
	}
	total := 12 + 8 + len(jsonChunk)
	if len(binaryChunk) > 0 {
		total += 8 + len(binaryChunk)
	}
	if uint64(total) > math.MaxUint32 {
		return nil, fmt.Errorf("GLB exceeds 32-bit container limit")
	}
	output := make([]byte, 12, total)
	binary.LittleEndian.PutUint32(output[0:4], 0x46546c67)
	binary.LittleEndian.PutUint32(output[4:8], 2)
	binary.LittleEndian.PutUint32(output[8:12], uint32(total))
	appendChunk := func(kind uint32, data []byte) {
		header := make([]byte, 8)
		binary.LittleEndian.PutUint32(header[0:4], uint32(len(data)))
		binary.LittleEndian.PutUint32(header[4:8], kind)
		output = append(output, header...)
		output = append(output, data...)
	}
	appendChunk(0x4e4f534a, jsonChunk)
	if len(binaryChunk) > 0 {
		appendChunk(0x004e4942, binaryChunk)
	}
	return output, nil
}
