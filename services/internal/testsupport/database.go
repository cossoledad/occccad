// Package testsupport provides explicitly configured integration resources.
package testsupport

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
)

// OpenTestDatabase uses the selected dedicated backend without resetting data.
func OpenTestDatabase(t *testing.T) *database.Pool {
	t.Helper()
	if err := config.LoadTestEnvironment(); err != nil {
		t.Fatal(err)
	}
	dsn := os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	expected, err := testDatabaseTarget(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if !strings.HasPrefix(dsn, "sqlite:") {
		var actual string
		if err := db.QueryRow(t.Context(), "SELECT current_database()").Scan(&actual); err != nil {
			t.Fatal(err)
		}
		if actual != expected {
			t.Fatal("integration database identity differs from configured PostgreSQL target")
		}
	}
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func testDatabaseTarget(dsn string) (string, error) {
	if strings.HasPrefix(dsn, "sqlite:") {
		path := strings.TrimPrefix(dsn, "sqlite:")
		if !filepath.IsAbs(path) || !strings.HasSuffix(filepath.Base(path), "_test.db") {
			return "", errors.New("test SQLite requires an absolute dedicated *_test.db file")
		}
		canonical := func(path string) string {
			path = filepath.Clean(path)
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				return resolved
			}
			return path
		}
		app := os.Getenv("OCCCCAD_DATABASE_URL")
		if strings.HasPrefix(app, "sqlite:") && canonical(path) == canonical(strings.TrimPrefix(app, "sqlite:")) {
			return "", errors.New("test database must differ from the application database")
		}
		return path, nil
	}
	return postgresTestDatabase(dsn)
}

// Run initializes package-level integration configuration before any test reads
// an environment variable. Individual fixtures open/migrate their database.
func Run(m *testing.M) int {
	if err := config.LoadTestEnvironment(); err != nil {
		fmt.Fprintln(os.Stderr, "test environment:", err)
		return 1
	}
	dsn := os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if _, err := testDatabaseTarget(dsn); err != nil {
		fmt.Fprintln(os.Stderr, "test database:", err)
		return 1
	}
	return m.Run()
}

func PrepareAssemblyFixtures(t *testing.T) {
	t.Helper()
	if err := config.LoadTestEnvironment(); err != nil {
		t.Fatal(err)
	}
	directory := os.Getenv("OCCCCAD_ASSEMBLY_FIXTURE_DIR")
	complete := true
	for _, name := range []string{"sphere-r6", "sphere-r3", "sphere-r075", "cylinder-r6-h12", "cylinder-r3-h12", "cone-r9-r3-h12", "cone-r6-r2-h8"} {
		for _, extension := range []string{".brep", ".step"} {
			if _, err := os.Stat(filepath.Join(directory, name+extension)); err != nil {
				complete = false
			}
		}
	}
	if complete {
		return
	}
	worker := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	binary := filepath.Clean(filepath.Join(filepath.Dir(worker), "..", "..", "kernel", "occt", "tests", "occcad_geometry_scenarios"))
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("analytic fixture generator missing: %s", binary)
	}
	command := exec.CommandContext(t.Context(), binary, "--gtest_filter=AssemblyExactSupport.ExportAnalyticRouterFixtures")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("analytic fixture generation failed: %v\n%s", err, output)
	}
}

func postgresTestDatabase(dsn string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return "", errors.New("invalid OCCCCAD_TEST_DATABASE_URL: expected a dedicated PostgreSQL URL")
	}
	name := strings.TrimPrefix(parsed.Path, "/")
	if parsed.Hostname() == "" || !strings.HasPrefix(name, "occccad_") || !strings.HasSuffix(name, "_test") || strings.Contains(name, "/") {
		return "", errors.New("invalid OCCCCAD_TEST_DATABASE_URL: requires a dedicated occccad_*_test database; application databases are rejected")
	}
	return name, nil
}
