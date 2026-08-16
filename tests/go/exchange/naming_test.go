package exchange_test

import (
	"testing"

	"github.com/occccad/occccad/internal/exchange"
)

func TestImportedDocumentNameKeepsFileExtension(t *testing.T) {
	t.Parallel()
	for _, fileName := range []string{"a.step", "Bottom Support - Bottom Support.step", "gear.brep"} {
		if got := exchange.ImportedDocumentName(fileName); got != fileName {
			t.Fatalf("ImportedDocumentName(%q) = %q", fileName, got)
		}
	}
}

func TestImportedDocumentNameRemovesClientPath(t *testing.T) {
	t.Parallel()
	for _, fileName := range []string{"unsafe/path/a.step", `C:\fakepath\a.step`} {
		if got := exchange.ImportedDocumentName(fileName); got != "a.step" {
			t.Fatalf("ImportedDocumentName(%q) = %q", fileName, got)
		}
	}
}
