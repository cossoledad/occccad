package workspace

import (
	"encoding/binary"
	"encoding/json"
	"github.com/occccad/occccad/internal/visual"
	"testing"
)

func TestVisualV2BinaryAuxiliaryAndReplacement(t *testing.T) {
	v := VisualizationManifest{FeatureAssociations: &FeatureAssociationIndex{Features: []string{"feature"}}, FeatureContributions: map[string]string{"feature": "NO_CURRENT_CONTRIBUTION"}, SchemaVersion: 1, Primitives: []VisualPrimitive{{ID: "wire", Kind: "POLYLINE", Positions: [][3]float64{{1, 2, 3}, {4, 5, 6}}, Indices: []uint32{0, 1}}, {ID: "label", Kind: "POINTS", Positions: [][3]float64{}}}}
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
	if len(again) != len(data) {
		t.Fatalf("retained stale auxiliary data: %d -> %d", len(data), len(again))
	}
	if _, _, err = visual.Decode(again); err != nil {
		t.Fatal(err)
	}
}
