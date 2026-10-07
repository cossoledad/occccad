package testsupport

import "testing"

func TestPostgresIntegrationTargetRequiresExplicitIsolation(t *testing.T) {
	for _, input := range []string{"", "sqlite:test", "sqlite:/tmp/test.db", "postgres://host/ganjb", "postgres://host/occccad", "postgres://host/unrelated_test", "postgres:///occccad_assembly_contract_test"} {
		if _, err := postgresTestDatabase(input); err == nil {
			t.Fatal("unsafe or alternate integration target accepted")
		}
	}
	for _, name := range []string{"occccad_offset_contract_test", "occccad_assembly_contract_test"} {
		for _, scheme := range []string{"postgres", "postgresql"} {
			got, err := postgresTestDatabase(scheme + "://host/" + name + "?sslmode=disable")
			if err != nil || got != name {
				t.Fatal("dedicated PostgreSQL target rejected", err)
			}
		}
	}
}
