package api

import "testing"

func TestAssemblyToolbarProjectsSixPublicFamiliesWithoutParallelModels(t *testing.T) {
	entries := []toolbarCatalogEntry{{Workbench: "ASSEMBLY_DESIGN", Items: []toolbarCatalogItem{{CommandID: "assembly.rigid"}, {CommandID: "assembly.coincident"}, {CommandID: "assembly.distance"}}}}
	result := canonicalAssemblyToolbars(entries)
	ids := map[string]bool{}
	for _, item := range result[0].Items {
		ids[item.CommandID] = true
	}
	if ids["assembly.rigid"] || !ids["assembly.fix_together"] || !ids["assembly.contact"] {
		t.Fatal("parallel public model remained")
	}
	if result[0].Items[2].Name != "偏移" {
		t.Fatal("internal Distance leaked")
	}
	if len(canonicalAssemblyToolbars(result)[0].Items) != len(result[0].Items) {
		t.Fatal("catalog projection duplicates Contact")
	}
}
