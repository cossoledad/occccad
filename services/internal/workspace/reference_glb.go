package workspace

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
)

const referenceGeometryExtension = "OCCCCAD_reference_geometry"

// glbWithReferenceGeometry mirrors the persisted Part reference geometry into
// a vendor GLB extension. The extension remains useful even when the GLB has no
// triangle mesh, which makes an empty Part a real display artifact.
func glbWithReferenceGeometry(source []byte, reference ReferenceGeometry) ([]byte, error) {
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
		if len(source) < 20 || binary.LittleEndian.Uint32(source[0:4]) != 0x46546c67 {
			return nil, fmt.Errorf("invalid GLB header")
		}
		offset := 12
		for offset+8 <= len(source) {
			length := int(binary.LittleEndian.Uint32(source[offset : offset+4]))
			kind := binary.LittleEndian.Uint32(source[offset+4 : offset+8])
			offset += 8
			if length < 0 || offset+length > len(source) {
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
	extensions[referenceGeometryExtension] = reference
	used, _ := document["extensionsUsed"].([]any)
	found := false
	for _, entry := range used {
		if entry == referenceGeometryExtension {
			found = true
		}
	}
	if !found {
		used = append(used, referenceGeometryExtension)
	}
	document["extensionsUsed"] = used
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
