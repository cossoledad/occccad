package workspace

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
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
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return ""
	}
	if object, ok := value.(map[string]any); ok {
		object["digest"] = ""
	}
	// JSONB expands exponent-form doubles into decimal integers. Exact integer
	// domain fields must retain uint64/int64 identity; coordinates/quantities
	// must instead canonicalize as doubles, even when their text looks integral.
	var normalize func(any, string) any
	normalize = func(v any, field string) any {
		switch x := v.(type) {
		case json.Number:
			text := x.String()
			if !strings.ContainsAny(text, ".eE") {
				if text == "-0" {
					return json.Number("0")
				}
				if len(text) <= 15 {
					return x
				}
				switch field {
				case "winding", "Winding", "targetSequence", "interactionGoalSequence", "maxIterations", "MaxIterations",
					"maxPreferenceIterations", "MaxPreferenceIterations", "maxConflictProbes", "MaxConflictProbes":
					return x
				}
			}
			f, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return x
			}
			if f == 0 {
				return json.Number("0")
			}
			encoded, err := json.Marshal(f)
			if err != nil {
				return x
			}
			return json.Number(encoded)
		case []any:
			for i := range x {
				x[i] = normalize(x[i], "")
			}
		case map[string]any:
			for k := range x {
				x[k] = normalize(x[k], k)
			}
		}
		return v
	}
	return resolvedDigest(normalize(value, ""))
}

// Only the current canonical format is accepted; JSONB numeric normalization
// remains part of current identity, not an experimental-format adapter.
func storedAssemblyManifestDigestMatches(raw []byte, manifest AssemblySolveManifest, digest string) bool {
	return manifest.DigestPolicy == assemblyManifestCanonicalJSON && canonicalAssemblyManifestJSONDigest(raw) == digest && assemblyManifestDigest(manifest) == digest
}
