package api

import "testing"

func TestAssemblyAnalysisUsesCanonicalCommandCatalog(t *testing.T) {
	entries := []toolbarCatalogEntry{{ID: "constraints", Workbench: "ASSEMBLY_DESIGN", Items: []toolbarCatalogItem{{CommandID: "assembly.coincident"}}}, {ID: "move", Workbench: "ASSEMBLY_DESIGN", Items: []toolbarCatalogItem{{CommandID: "assembly.move"}}}}
	projected := canonicalAssemblyToolbars(entries)
	counts := map[string]int{}
	for _, toolbar := range canonicalAssemblyToolbars(projected) {
		for _, item := range toolbar.Items {
			counts[item.CommandID]++
		}
	}
	if counts["assembly.analyze"] != 1 || counts["assembly.move-receipt"] != 1 || counts["assembly.coincident"] != 1 {
		t.Fatal("formal command IDs must be discoverable once", counts)
	}
}
