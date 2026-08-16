package exchange

import (
	"path"
	"strings"
)

// ImportedDocumentName preserves the user-visible source file identity,
// including its extension. Component Part names may append a component label,
// but the root Part/Product keeps this value exactly.
func ImportedDocumentName(fileName string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(fileName), `\`, "/")
	return path.Base(normalized)
}
