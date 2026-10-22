package assistant

import "encoding/json"

// This is a per-result byte boundary, not a Run-wide token or call limit.
const MaxToolResultBytes = 32 << 10

// NormalizeToolResult never slices JSON. The same value must be persisted,
// appended to an online session, and replayed after recovery.
func NormalizeToolResult(result Result) Result {
	if len(result.Content) > MaxToolResultBytes {
		result.IsError = true
		result.Content = marshalJSON(map[string]any{
			"code":        "TOOL_PAGE_RESOURCE",
			"message":     "Read smaller fields/page; result not included. The Run can continue.",
			"sourceBytes": len(result.Content), "limitBytes": MaxToolResultBytes,
		})
	} else if !json.Valid(result.Content) {
		result.IsError = true
		result.Content = json.RawMessage(`{"code":"TOOL_RESULT_INVALID","message":"The tool did not return valid JSON; no partial result was included."}`)
	}
	return result
}
