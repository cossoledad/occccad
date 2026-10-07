package api

import (
	"github.com/occccad/occccad/internal/workbenchconfig"
	"testing"
)

func TestAssemblyToolbarProjectsSixPublicFamiliesWithoutParallelModels(t *testing.T) {
	catalog, err := workbenchconfig.Read()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, command := range catalog.Commands {
		ids[command.ID] = true
		if command.ID == "assembly.distance" && command.Name != "偏移" {
			t.Fatal("internal Distance leaked")
		}
	}
	if ids["assembly.rigid"] || !ids["assembly.fix_together"] || !ids["assembly.contact"] {
		t.Fatal("public assembly declarations incomplete")
	}
}
