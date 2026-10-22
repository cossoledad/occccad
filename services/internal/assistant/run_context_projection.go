package assistant

import (
	"encoding/base64"
	"encoding/json"

	"github.com/occccad/occccad/internal/modelcore"
)

func selectionSetContext(token string) map[string]any {
	out := map[string]any{"digest": modelcore.ValueDigest([]byte(token)), "bytes": len(token)}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return out
	}
	var set map[string]any
	if json.Unmarshal(raw, &set) != nil {
		return out
	}
	for _, key := range []string{"document", "revision", "body", "kind", "count", "complete", "filter", "matchDigest"} {
		if v, ok := set[key]; ok {
			out[key] = v
		}
	}
	return out
}

// Deterministic projections are for rebuilt context only. The canonical tool
// payload, provider request/response trace and online tool reply stay intact.
func compactArguments(raw json.RawMessage) map[string]any {
	out := map[string]any{"digest": modelcore.ValueDigest(raw)}
	var args map[string]any
	if json.Unmarshal(raw, &args) != nil {
		return out
	}
	for key, value := range args {
		if key == "selectionSet" {
			if token, ok := value.(string); ok {
				out[key] = selectionSetContext(token)
			}
			continue
		}
		switch v := value.(type) {
		case string:
			if len(v) <= 160 {
				out[key] = v
			}
		case float64, bool, nil:
			out[key] = v
		default:
			if key == "filter" || key == "resultBindings" || key == "expectedHead" {
				out[key] = v
			} else {
				bytes := marshalJSON(v)
				out[key] = map[string]any{"digest": modelcore.ValueDigest(bytes), "bytes": len(bytes)}
			}
		}
	}
	return out
}

func recentResultContext(callID, name string, result Result, currentRevision string) map[string]any {
	out := map[string]any{"digest": modelcore.ValueDigest(result.Content), "bytes": len(result.Content), "isError": result.IsError}
	var value map[string]json.RawMessage
	if json.Unmarshal(result.Content, &value) != nil {
		return out
	}
	if result.IsError {
		for _, key := range []string{"code", "message", "retryable", "sourceBytes", "limitBytes"} {
			if v, ok := value[key]; ok {
				out[key] = v
			}
		}
		return out
	}
	for _, key := range []string{"scope", "filter", "scopeCount", "candidateCount", "matchedCount", "returnedCount", "unknownCount", "matchComplete", "selectionSetCount", "selectionSetComplete", "pageComplete", "hasMore", "nextCursor", "snapshot", "id", "status", "revisionId", "passed"} {
		if v, ok := value[key]; ok {
			out[key] = v
		}
	}
	var scope struct {
		RevisionID string `json:"revisionId"`
	}
	_ = json.Unmarshal(value["scope"], &scope)
	if scope.RevisionID == "" {
		_ = json.Unmarshal(value["snapshot"], &scope)
	}
	if scope.RevisionID != "" {
		out["historical"] = scope.RevisionID != currentRevision
	}
	if ref := queryResultReference(callID, result.CallID, name, result); ref != nil {
		out["selectionSetRef"] = ref
	}
	if name == "cad_query_topology" {
		var rows []struct {
			ID           string          `json:"id"`
			GeometryType string          `json:"geometryType"`
			Geometry     json.RawMessage `json:"geometry"`
		}
		if json.Unmarshal(value["items"], &rows) == nil {
			out["preview"] = rows[:min(len(rows), 3)]
			out["previewComplete"] = len(rows) <= 3
		}
	} else if name == "cad_create_task" || name == "cad_get_task" {
		// Durable Task can be read again; only identities/phases belong in facts.
		var task struct {
			Steps []struct{ ID, OperationID, Phase string }
		}
		if json.Unmarshal(result.Content, &task) == nil {
			out["steps"] = task.Steps
		}
	}
	return out
}

func selectionTargetsContext(values map[string]string, currentRevision string) map[string]any {
	out := map[string]any{}
	for key, token := range values {
		descriptor := selectionSetContext(token)
		if revision, ok := descriptor["revision"].(string); ok {
			descriptor["historical"] = revision != currentRevision
		}
		out[key] = descriptor
	}
	return out
}
