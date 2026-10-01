package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

const assemblyManifestCanonicalJSON = "CANONICAL_JSON_V1"

// JSONB preserves numeric values, not object order or IEEE negative-zero bits.
// This explicitly versioned identity is assembly-manifest-local: it does not
// change parameters, solver inputs, stable references, or legacy stored bytes.
func assemblyManifestDigest(manifest AssemblySolveManifest) string {
	manifest.Digest = ""
	if manifest.DigestPolicy != assemblyManifestCanonicalJSON {
		return resolvedDigest(manifest)
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

// Appended optional fields must not invalidate older frozen snapshots merely
// because a newer Go struct emits defaults. Preserve legacy serialized field
// order while projecting only fields actually present in the stored snapshot.
func projectLegacyManifestJSON(typed []byte, shape any) ([]byte, error) {
	d := json.NewDecoder(bytes.NewReader(typed))
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return typed, nil
	}
	var out bytes.Buffer
	out.WriteByte(byte(delim))
	first := true
	if delim == '{' {
		fields, _ := shape.(map[string]any)
		for d.More() {
			keyToken, e := d.Token()
			if e != nil {
				return nil, e
			}
			key := keyToken.(string)
			var raw json.RawMessage
			if e = d.Decode(&raw); e != nil {
				return nil, e
			}
			original, present := fields[key]
			if !present {
				continue
			}
			projected, e := projectLegacyManifestJSON(raw, original)
			if e != nil {
				return nil, e
			}
			if !first {
				out.WriteByte(',')
			}
			first = false
			encoded, _ := json.Marshal(key)
			out.Write(encoded)
			out.WriteByte(':')
			out.Write(projected)
		}
		out.WriteByte('}')
	} else if delim == '[' {
		items, _ := shape.([]any)
		i := 0
		for d.More() {
			var raw json.RawMessage
			if err = d.Decode(&raw); err != nil {
				return nil, err
			}
			var item any
			if i < len(items) {
				item = items[i]
			}
			i++
			projected, e := projectLegacyManifestJSON(raw, item)
			if e != nil {
				return nil, e
			}
			if !first {
				out.WriteByte(',')
			}
			first = false
			out.Write(projected)
		}
		out.WriteByte(']')
	}
	return out.Bytes(), nil
}

func storedAssemblyManifestDigestMatches(raw []byte, manifest AssemblySolveManifest, digest string) bool {
	if manifest.DigestPolicy == assemblyManifestCanonicalJSON {
		return canonicalAssemblyManifestJSONDigest(raw) == digest
	}
	if assemblyManifestDigest(manifest) == digest {
		return true
	}
	if manifest.DigestPolicy != "" {
		return false
	}
	manifest.Digest = ""
	typed, _ := json.Marshal(manifest)
	var shape any
	if json.Unmarshal(raw, &shape) != nil {
		return false
	}
	projected, err := projectLegacyManifestJSON(typed, shape)
	sum := sha256.Sum256(projected)
	return err == nil && hex.EncodeToString(sum[:]) == digest
}
