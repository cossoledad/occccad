package workspace

import (
	"encoding/json"
)

const assemblyManifestCanonicalJSON = "CANONICAL_JSON_V1"

// JSONB preserves numeric values, not object order or IEEE negative-zero bits.
// This explicitly versioned identity is assembly-manifest-local: it does not
// change parameters, solver inputs or stable references; old formats are rejected.
func assemblyManifestDigest(manifest AssemblySolveManifest) string {
	manifest.Digest = ""
	if manifest.DigestPolicy != assemblyManifestCanonicalJSON {
		return ""
	}
	raw, _ := json.Marshal(manifest)
	return canonicalAssemblyManifestJSONDigest(raw)
}

func canonicalAssemblyManifestJSONDigest(raw []byte) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	if object, ok := value.(map[string]any); ok {
		object["digest"] = ""
	}
	var normalize func(any) any
	normalize = func(v any) any {
		switch x := v.(type) {
		case float64:
			if x == 0 {
				return float64(0)
			}
		case []any:
			for i := range x {
				x[i] = normalize(x[i])
			}
		case map[string]any:
			for k := range x {
				x[k] = normalize(x[k])
			}
		}
		return v
	}
	return resolvedDigest(normalize(value))
}

// Only the current canonical format is accepted; JSONB numeric normalization
// remains part of current identity, not an experimental-format adapter.
func storedAssemblyManifestDigestMatches(raw []byte, manifest AssemblySolveManifest, digest string) bool {
	return manifest.DigestPolicy == assemblyManifestCanonicalJSON && canonicalAssemblyManifestJSONDigest(raw) == digest && assemblyManifestDigest(manifest) == digest
}
