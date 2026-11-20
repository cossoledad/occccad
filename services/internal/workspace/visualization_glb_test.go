package workspace

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"slices"
	"testing"

	"github.com/occccad/occccad/internal/visual"
)

func TestVisualV2BinaryAuxiliaryAndReplacement(t *testing.T) {
	row := FeatureTopologyAssociation{Kind: "FACE", LocalID: 1, Status: "MAPPED",
		Origins: []string{"feature"}, Primary: []string{"feature"}, Modifiers: []string{"modifier"}, Supports: []string{"support"},
		Members: []FeatureAssociationMember{{PatternID: "pattern", Slot: 0}}}
	otherRow := row
	otherRow.Kind = "EDGE"
	index := &FeatureAssociationIndex{Features: []string{"feature", "modifier", "pattern"}, Elements: []FeatureTopologyAssociation{row, otherRow}}
	v := VisualizationManifest{FeatureAssociations: index, FeatureContributions: map[string]string{"feature": "NO_CURRENT_CONTRIBUTION"}, SchemaVersion: 1, Primitives: []VisualPrimitive{{ID: "wire", Kind: "POLYLINE", Positions: [][3]float64{{1, 2, 3}, {4, 5, 6}}, Indices: []uint32{0, 1}}, {ID: "label", Kind: "POINTS", Positions: [][3]float64{}}}}
	data, err := glbWithVisualization(nil, v, map[string]string{"geometryId": "geometry", "namingDigest": "naming"})
	if err != nil {
		t.Fatal(err)
	}
	n := binary.LittleEndian.Uint32(data[12:])
	var doc map[string]any
	if err = json.Unmarshal(data[20:20+n], &doc); err != nil {
		t.Fatal(err)
	}
	metadata := doc["extensions"].(map[string]any)[visualizationExtension].(map[string]any)
	table, ok := metadata["featureAssociationTable"].(map[string]any)
	if !ok || metadata["featureAssociations"] != nil || len(table["lists"].([]any)) != 3 || len(table["members"].([]any)) != 1 {
		t.Fatal("display ownership lists were not shared in the GLB")
	}
	p := metadata["primitives"].([]any)[0].(map[string]any)
	if _, ok := p["positions"].(float64); !ok {
		t.Fatal("JSON positions retained")
	}
	if _, ok := p["indices"].(float64); !ok {
		t.Fatal("JSON indices retained")
	}
	if len(doc["meshes"].([]any)) != 1 {
		t.Fatal("wire not exposed as standard glTF primitive")
	}
	mesh, raw, err := visual.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if err = mesh.Association.Validate("geometry", "naming"); err != nil {
		t.Fatal(err)
	}
	if mesh.Association.Validate("other", "naming") == nil || mesh.Association.Validate("geometry", "other") == nil {
		t.Fatal("mismatched artifact association accepted")
	}
	var restored VisualizationManifest
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Primitives[0].Positions) != 2 || restored.Primitives[0].Positions[1] != v.Primitives[0].Positions[1] {
		t.Fatal("binary display changed")
	}
	if restored.FeatureAssociations == nil || restored.FeatureContributions["feature"] != "NO_CURRENT_CONTRIBUTION" {
		t.Fatal("association not paired with GLB")
	}
	compare := func(got *FeatureAssociationIndex) {
		t.Helper()
		if got == nil || !slices.Equal(got.Features, index.Features) || len(got.Elements) != len(index.Elements) {
			t.Fatal("display association shape changed")
		}
		for i, want := range index.Elements {
			e := got.Elements[i]
			if e.Kind != want.Kind || e.LocalID != want.LocalID || e.Status != want.Status || !slices.Equal(e.Origins, want.Origins) ||
				!slices.Equal(e.Primary, want.Primary) || !slices.Equal(e.Modifiers, want.Modifiers) ||
				!slices.Equal(e.Supports, want.Supports) || !slices.Equal(e.Members, want.Members) {
				t.Fatal("display ownership changed", e, want)
			}
		}
	}
	compare(restored.FeatureAssociations)
	// Read a frozen prefix using only the metadata chunk. An attempt to read the
	// mesh or auxiliary payload would hit EOF with this source.
	frozen, listBytes, err := readFrozenAssociations(bytes.NewReader(data[:20+n]), int64(len(data)), "geometry", "naming")
	if err != nil {
		t.Fatal(err)
	}
	compare(frozen)
	// Three feature IDs, three singleton lists and one member share four strings.
	if want := int64(6*16 + 24 + len("featuremodifierpatternsupport")); listBytes != want {
		t.Fatalf("frozen association list charge = %d, want %d", listBytes, want)
	}
	if _, _, err := readFrozenAssociations(bytes.NewReader(data[:20+n]), int64(len(data)), "geometry", "other"); err == nil {
		t.Fatal("association reader accepted a different Naming binding")
	}
	// Repeated variants replace owned buffers, so unchanged payloads do not grow.
	v.FeatureAssociations = nil
	v.FeatureContributions = nil
	again, err := glbWithVisualization(data, v)
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err = visual.Decode(again)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.FeatureAssociations == nil || restored.FeatureContributions["feature"] != "NO_CURRENT_CONTRIBUTION" {
		t.Fatal("metadata variant lost associations")
	}
	compare(restored.FeatureAssociations)
	if len(again) != len(data) {
		t.Fatalf("retained stale auxiliary data: %d -> %d", len(data), len(again))
	}
	if _, _, err = visual.Decode(again); err != nil {
		t.Fatal(err)
	}
	table["rows"].([]any)[0].([]any)[5] = float64(99)
	corrupt, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(corrupt, &restored); err == nil {
		t.Fatal("accepted an invalid shared list ordinal")
	}
}

// BIN remains a stream even when business display metadata changes. Allocation
// scales with JSON/auxiliary arrays rather than triangle count.
func BenchmarkVisualizationStream(b *testing.B) {
	const size = 32 << 20
	jsonChunk := []byte(`{"asset":{"version":"2.0"},"buffers":[{"byteLength":33554432}],"meshes":[],"nodes":[]}`)
	for len(jsonChunk)%4 != 0 {
		jsonChunk = append(jsonChunk, ' ')
	}
	prefix := make([]byte, 20+len(jsonChunk)+8)
	binary.LittleEndian.PutUint32(prefix[:4], 0x46546c67)
	binary.LittleEndian.PutUint32(prefix[4:8], 2)
	binary.LittleEndian.PutUint32(prefix[8:12], uint32(len(prefix)+size))
	binary.LittleEndian.PutUint32(prefix[12:16], uint32(len(jsonChunk)))
	binary.LittleEndian.PutUint32(prefix[16:20], 0x4e4f534a)
	copy(prefix[20:], jsonChunk)
	binary.LittleEndian.PutUint32(prefix[len(prefix)-8:], size)
	binary.LittleEndian.PutUint32(prefix[len(prefix)-4:], 0x004e4942)
	data := append(prefix, make([]byte, size)...)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, err := glbWithVisualizationReader(bytes.NewReader(data), int64(len(data)), VisualizationManifest{})
		if err != nil {
			b.Fatal(err)
		}
		if _, err = io.Copy(io.Discard, reader); err != nil {
			b.Fatal(err)
		}
	}
}

func TestVisualizationStreamRetainsMeshPrefixAndReplacesAuxiliarySuffix(t *testing.T) {
	v := VisualizationManifest{SchemaVersion: 1, Primitives: []VisualPrimitive{{ID: "curve", Kind: "POLYLINE", Positions: [][3]float64{{1, 2, 3}, {4, 5, 6}}}}}
	original, err := glbWithVisualization(nil, v)
	if err != nil {
		t.Fatal(err)
	}
	next := v
	next.Primitives = []VisualPrimitive{{ID: "point", Kind: "POINTS", Positions: [][3]float64{{7, 8, 9}}}}
	reader, err := glbWithVisualizationReader(bytes.NewReader(original), int64(len(original)), next)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err := visual.Decode(actual)
	if err != nil {
		t.Fatal(err)
	}
	var restored VisualizationManifest
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Primitives) != 1 || restored.Primitives[0].ID != "point" || len(restored.Primitives[0].Positions) != 1 || restored.Primitives[0].Positions[0] != [3]float64{7, 8, 9} {
		t.Fatal("old auxiliary arrays survived replacement", restored)
	}
}
