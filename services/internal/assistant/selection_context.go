package assistant

import (
	"encoding/json"

	"github.com/occccad/occccad/internal/modelcore"
)

// The full formal reference stays in FrozenScope and remains the execution
// input. This is a read projection, not a new identity or handle registry.
func compactSelections(scope FrozenScope, after, end int) []map[string]any {
	out := make([]map[string]any, 0, end-after)
	digest := modelcore.ValueDigest(marshalJSON(scope))
	for i := after; i < end; i++ {
		ref := scope.Selections[i]
		ref.Selection = nil
		var item map[string]any
		_ = json.Unmarshal(marshalJSON(ref), &item)
		item["index"] = i
		item["scopeDigest"] = digest
		item["hasFormalSelection"] = scope.Selections[i].Selection != nil
		if scope.Selections[i].Selection != nil {
			item["referenceDigest"] = modelcore.ValueDigest(marshalJSON(scope.Selections[i].Selection))
		}
		out = append(out, item)
	}
	return out
}
