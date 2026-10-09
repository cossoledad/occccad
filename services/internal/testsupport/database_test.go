package testsupport

import (
	"path/filepath"
	"testing"
)

func TestSelectedTestDatabaseSupportsDedicatedSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "occccad_test.db")
	if got, err := testDatabaseTarget("sqlite:" + path); err != nil || got != path {
		t.Fatal(got, err)
	}
	for _, dsn := range []string{"sqlite:relative_test.db", "sqlite:" + filepath.Join(t.TempDir(), "app.db")} {
		if _, err := testDatabaseTarget(dsn); err == nil {
			t.Fatal("unsafe SQLite test target accepted")
		}
	}
	t.Setenv("OCCCCAD_DATABASE_URL", "sqlite:"+path)
	if _, err := testDatabaseTarget("sqlite:" + path); err == nil {
		t.Fatal("application database accepted")
	}
}

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
