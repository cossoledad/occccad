package assemblycontract

import "testing"

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
