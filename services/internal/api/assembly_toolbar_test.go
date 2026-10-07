package api

import (
	"github.com/occccad/occccad/internal/workbenchconfig"
	"testing"
)

func TestAssemblyAnalysisUsesCanonicalCommandCatalog(t *testing.T) {
	catalog, err := workbenchconfig.Read()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, bar := range catalog.Toolbars() {
		for _, item := range bar.Items {
			counts[item.CommandID]++
		}
	}
	for _, id := range []string{"assembly.analyze", "assembly.move-receipt", "assembly.coincident", "assembly.contact", "assembly.fix_together"} {
		if counts[id] != 1 {
			t.Fatalf("formal command %s count %d", id, counts[id])
		}
	}
}
