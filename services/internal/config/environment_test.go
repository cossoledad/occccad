package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentLoadsExplicitFileAndAnchorsPaths(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(file, []byte("OCCCCAD_DATABASE_URL=sqlite:build/test-resources/example_test.db\nOCCCCAD_ASSEMBLY_FIXTURE_DIR=build/custom-fixtures\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OCCCCAD_ENV_FILE", file)
	for _, key := range []string{"OCCCCAD_TEST_RESOURCES_DIR", "OCCCCAD_TEST_IMPORT_BREP", "OCCCCAD_GEOMETRY_WORKER_BIN", "OCCCCAD_TEST_CURVED_GLB", "OCCCCAD_TEST_GLB_FIXTURE", "OCCCCAD_TEST_ASSEMBLY_REPLAY_OUTPUT"} {
		t.Setenv(key, "")
	}
	for _, key := range []string{"OCCCCAD_DATABASE_URL", "OCCCCAD_ASSEMBLY_FIXTURE_DIR"} {
		old, exists := os.LookupEnv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if exists {
				os.Setenv(key, old)
			} else {
				os.Unsetenv(key)
			}
		})
	}
	if _, err := LoadEnvironment(); err != nil {
		t.Fatal(err)
	}
	dsn := os.Getenv("OCCCCAD_DATABASE_URL")
	if !filepath.IsAbs(strings.TrimPrefix(dsn, "sqlite:")) || !strings.HasSuffix(dsn, "example_test.db") {
		t.Fatal("unresolved SQLite test path")
	}
	if !filepath.IsAbs(os.Getenv("OCCCCAD_ASSEMBLY_FIXTURE_DIR")) {
		t.Fatal("unresolved fixture path")
	}
	t.Setenv("OCCCCAD_DATABASE_URL", "sqlite:/tmp/exported_test.db")
	if _, err := LoadEnvironment(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("OCCCCAD_DATABASE_URL") != "sqlite:/tmp/exported_test.db" {
		t.Fatal("exported value overridden")
	}
}

func TestExplicitRelativeEnvironmentIsRootAnchoredAndLiteral(t *testing.T) {
	root, e := ProjectRoot()
	if e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(t.TempDir(), "literal.env")
	if e = os.WriteFile(file, []byte("export OCCCCAD_LITERAL_PROBE=\"$(false)\"\nOCCCCAD_LITERAL_PROBE=duplicate\nOCCCCAD_OVERRIDE_PROBE=file\n"), 0600); e != nil {
		t.Fatal(e)
	}
	relative, e := filepath.Rel(root, file)
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("OCCCCAD_ENV_FILE", relative)
	t.Setenv("OCCCCAD_OVERRIDE_PROBE", "exported")
	old, exists := os.LookupEnv("OCCCCAD_LITERAL_PROBE")
	os.Unsetenv("OCCCCAD_LITERAL_PROBE")
	t.Cleanup(func() {
		if exists {
			os.Setenv("OCCCCAD_LITERAL_PROBE", old)
		} else {
			os.Unsetenv("OCCCCAD_LITERAL_PROBE")
		}
	})
	selected, e := LoadProjectEnv()
	if e != nil || selected != file {
		t.Fatal(selected, e)
	}
	if os.Getenv("OCCCCAD_LITERAL_PROBE") != "$(false)" || os.Getenv("OCCCCAD_OVERRIDE_PROBE") != "exported" {
		t.Fatal("literal/export precedence differs")
	}
	t.Setenv("OCCCCAD_ENV_FILE", relative+".missing")
	if _, e = LoadProjectEnv(); e == nil {
		t.Fatal("missing explicit file fell back")
	}
}
