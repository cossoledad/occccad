package assemblycontract

import (
	"encoding/json"
	"os/exec"
	"sync"
	"testing"
)

func TestEmbeddedPublicContractHasSixFamiliesAndStableRoles(t *testing.T) {
	c := Read()
	if len(c.Families) != 6 || c.ContractVersion == "" || len(c.Capabilities) < 58 {
		t.Fatal("incomplete public contract")
	}
	seen := map[string]bool{}
	for _, x := range c.Capabilities {
		if x.ID == "" || seen[x.ID] || x.Family == "" || len(x.Roles) == 0 {
			t.Fatalf("invalid capability %+v", x)
		}
		seen[x.ID] = true
	}
}

func TestCatalogReadOnceDetached(t *testing.T) {
	initialize()
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			c := Read()
			c.Families[0] = "mutated"
			c.Capabilities[0].Roles[0].Descriptor = "mutated"
			for k, v := range c.Policies {
				if len(v) > 0 {
					v[0] = '!'
					c.Policies[k] = v
				}
				break
			}
			f := ForFamily("Offset")
			if len(f) > 0 {
				f[0].Roles[0].Descriptor = "mutated"
			}
		})
	}
	wg.Wait()
	c := Read()
	if parseCount != 1 || c.Families[0] == "mutated" || c.Capabilities[0].Roles[0].Descriptor == "mutated" {
		t.Fatal("shared authority mutated or reparsed")
	}
	for _, v := range c.Policies {
		if !json.Valid(v) {
			t.Fatal("raw policy mutation leaked")
		}
	}
	if len(c.Capabilities) != 58 {
		t.Fatal("capabilities lost")
	}
	copy, ok := ForCapability(c.Capabilities[0].ID)
	if !ok {
		t.Fatal("capability index missing")
	}
	copy.Roles[0].Descriptor = "mutated"
	current, _ := ForCapability(c.Capabilities[0].ID)
	if current.Roles[0].Descriptor == "mutated" {
		t.Fatal("single capability mutation leaked")
	}
	if _, ok := ForCapability("missing"); ok {
		t.Fatal("unknown capability matched")
	}
}

func BenchmarkCatalogFamily(b *testing.B) {
	initialize()
	b.ReportAllocs()
	for b.Loop() {
		_ = ForFamily("Offset")
	}
}
func BenchmarkCatalogParse(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		var c Catalog
		if err := json.Unmarshal(catalogJSON, &c); err != nil {
			b.Fatal(err)
		}
	}
}

// Comparison input comes from Git, not another maintained capability matrix.
// Parsing historical bytes here is only a benchmark, never a product adapter.
func BenchmarkCatalogBaselineMixedParse(b *testing.B) {
	raw, err := exec.Command("git", "show", "fb456981:services/internal/assemblycontract/catalog.json").Output()
	if err != nil {
		b.Skip("review baseline unavailable")
	}
	b.ReportAllocs()
	for b.Loop() {
		var c Catalog
		if err := json.Unmarshal(raw, &c); err != nil {
			b.Fatal(err)
		}
	}
}
