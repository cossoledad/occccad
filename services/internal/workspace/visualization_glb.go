package workspace

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

const visualizationExtension = "OCCCCAD_visualization"

// glbWithVisualization mirrors the complete Part display manifest into a
// vendor GLB extension. It remains valid without a triangle mesh, so an empty
// Part or sketch-only Part is still a real visualization artifact.
func glbWithVisualization(source []byte, visualization VisualizationManifest, association ...map[string]string) ([]byte, error) {
	reader, err := glbWithVisualizationReader(bytes.NewReader(source), int64(len(source)), visualization, association...)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(reader)
}

// Only JSON and newly owned auxiliary arrays are assembled. The immutable mesh
// BIN prefix streams from the verified staging reader without another full copy.
func glbWithVisualizationReader(source io.Reader, size int64, visualization VisualizationManifest, association ...map[string]string) (io.Reader, error) {
	var document map[string]any
	var binarySize int
	var suffix []byte
	if size == 0 {
		document = map[string]any{"asset": map[string]any{"version": "2.0", "generator": "occccad metadata service"}, "scene": 0, "scenes": []any{map[string]any{"nodes": []any{}}}, "nodes": []any{}}
	} else {
		var header [20]byte
		if _, err := io.ReadFull(source, header[:]); err != nil {
			return nil, fmt.Errorf("invalid GLB header: %w", err)
		}
		jsonSize := int64(binary.LittleEndian.Uint32(header[12:16]))
		if binary.LittleEndian.Uint32(header[:4]) != 0x46546c67 || binary.LittleEndian.Uint32(header[4:8]) != 2 || int64(binary.LittleEndian.Uint32(header[8:12])) != size || binary.LittleEndian.Uint32(header[16:20]) != 0x4e4f534a || jsonSize%4 != 0 || jsonSize > size-20 {
			return nil, fmt.Errorf("invalid GLB header")
		}
		chunk := make([]byte, int(jsonSize))
		if _, err := io.ReadFull(source, chunk); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(chunk, &document); err != nil {
			return nil, fmt.Errorf("decode GLB JSON: %w", err)
		}
		if document == nil {
			return nil, fmt.Errorf("missing GLB JSON object")
		}
		remaining := size - 20 - jsonSize
		if remaining > 0 {
			var chunk [8]byte
			if _, err := io.ReadFull(source, chunk[:]); err != nil {
				return nil, fmt.Errorf("invalid GLB binary header: %w", err)
			}
			binarySize = int(binary.LittleEndian.Uint32(chunk[:4]))
			if binary.LittleEndian.Uint32(chunk[4:]) != 0x004e4942 || binarySize%4 != 0 || int64(binarySize) != remaining-8 {
				return nil, fmt.Errorf("invalid GLB binary chunk")
			}
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
				if n > float64(binarySize+len(suffix)) {
					return nil, fmt.Errorf("invalid visualization binary suffix")
				}
				binarySize = int(n)
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
	metadata["bufferStart"] = binarySize + len(suffix)
	metadata["accessorStart"] = len(accessors)
	metadata["bufferViewStart"] = len(views)
	meshes, _ := document["meshes"].([]any)
	nodes, _ := document["nodes"].([]any)
	metadata["meshStart"] = len(meshes)
	metadata["nodeStart"] = len(nodes)
	add := func(values []float64, components int, floating bool) int {
		start := binarySize + len(suffix)
		for _, value := range values {
			var bits uint32
			if floating {
				bits = math.Float32bits(float32(value))
			} else {
				bits = uint32(value)
			}
			var bytes [4]byte
			binary.LittleEndian.PutUint32(bytes[:], bits)
			suffix = append(suffix, bytes[:]...)
		}
		view := len(views)
		views = append(views, map[string]any{"buffer": 0, "byteOffset": start, "byteLength": binarySize + len(suffix) - start})
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
	document["buffers"] = []any{map[string]any{"byteLength": binarySize + len(suffix)}}
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
	if binarySize+len(suffix) == 0 {
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
	for (binarySize+len(suffix))%4 != 0 {
		suffix = append(suffix, 0)
	}
	total := 12 + 8 + len(jsonChunk)
	if binarySize+len(suffix) > 0 {
		total += 8 + binarySize + len(suffix)
	}
	if uint64(total) > math.MaxUint32 {
		return nil, fmt.Errorf("GLB exceeds 32-bit container limit")
	}
	prefix := make([]byte, 20+len(jsonChunk))
	binary.LittleEndian.PutUint32(prefix[:4], 0x46546c67)
	binary.LittleEndian.PutUint32(prefix[4:8], 2)
	binary.LittleEndian.PutUint32(prefix[8:12], uint32(total))
	binary.LittleEndian.PutUint32(prefix[12:16], uint32(len(jsonChunk)))
	binary.LittleEndian.PutUint32(prefix[16:20], 0x4e4f534a)
	copy(prefix[20:], jsonChunk)
	if binarySize+len(suffix) == 0 {
		return bytes.NewReader(prefix), nil
	}
	var chunkHeader [8]byte
	binary.LittleEndian.PutUint32(chunkHeader[:4], uint32(binarySize+len(suffix)))
	binary.LittleEndian.PutUint32(chunkHeader[4:], 0x004e4942)
	return io.MultiReader(bytes.NewReader(prefix), bytes.NewReader(chunkHeader[:]), io.LimitReader(source, int64(binarySize)), bytes.NewReader(suffix)), nil
}
