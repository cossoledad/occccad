package workspace

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/geometry"
	"math"
	"testing"
)

func TestAssemblyManifestCanonicalJSONBIdentity(t *testing.T) {
	m := AssemblySolveManifest{DigestPolicy: assemblyManifestCanonicalJSON, Geometry: []geometry.AssemblyGeometry{{ID: "circle", Kind: "CIRCLE", Radius: 3, XDirection: [3]float64{1, 0, math.Copysign(0, -1)}}}}
	m.Digest = assemblyManifestDigest(m)
	raw, _ := json.Marshal(m)
	var stored map[string]any
	_ = json.Unmarshal(raw, &stored)
	g := stored["geometry"].([]any)[0].(map[string]any)
	g["XDirection"].([]any)[2] = float64(0)
	persisted, _ := json.Marshal(stored)
	var decoded AssemblySolveManifest
	_ = json.Unmarshal(persisted, &decoded)
	if !storedAssemblyManifestDigestMatches(persisted, decoded, m.Digest) {
		t.Fatal("JSONB altered frozen identity")
	}
	g["Radius"] = float64(4)
	tampered, _ := json.Marshal(stored)
	_ = json.Unmarshal(tampered, &decoded)
	if storedAssemblyManifestDigestMatches(tampered, decoded, m.Digest) {
		t.Fatal("geometry tampering accepted")
	}
}

func TestAssemblyManifestLegacyAppendedDefaultsAndTampering(t *testing.T) {
	m := AssemblySolveManifest{Constraints: []geometry.AssemblyConstraint{{ID: "old", Kind: "DISTANCE"}}}
	raw, _ := json.Marshal(m)
	if storedAssemblyManifestDigestMatches(raw, m, resolvedDigest(m)) {
		t.Fatal("unsupported experimental digest accepted")
	}
	m.DigestPolicy = assemblyManifestCanonicalJSON
	m.Digest = assemblyManifestDigest(m)
	raw, _ = json.Marshal(m)
	m.Constraints[0].Value = 12
	if storedAssemblyManifestDigestMatches(raw, m, m.Digest) {
		t.Fatal("typed input tampering accepted")
	}
}
