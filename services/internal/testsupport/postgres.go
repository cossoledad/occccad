// Package testsupport provides explicitly configured integration resources.
package testsupport

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/database"
)

// OpenPostgres never substitutes SQLite or resets existing data. Integration
// fixtures use fresh document IDs and their own temporary ArtifactStore.
func OpenPostgres(t *testing.T) *database.Pool {
	t.Helper()
	dsn := os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ENVIRONMENT_BLOCKED: set OCCCCAD_TEST_DATABASE_URL to a dedicated PostgreSQL test database")
	}
	expected, err := postgresTestDatabase(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var actual string
	if err := db.QueryRow(t.Context(), "SELECT current_database()").Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatal("integration database identity differs from configured PostgreSQL target")
	}
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func postgresTestDatabase(dsn string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return "", errors.New("invalid OCCCCAD_TEST_DATABASE_URL: expected a PostgreSQL URL; SQLite is not a substitute for PostgreSQL integration")
	}
	name := strings.TrimPrefix(parsed.Path, "/")
	if parsed.Hostname() == "" || !strings.HasPrefix(name, "occccad_") || !strings.HasSuffix(name, "_test") || strings.Contains(name, "/") {
		return "", errors.New("invalid OCCCCAD_TEST_DATABASE_URL: requires a dedicated occccad_*_test database; application databases are rejected")
	}
	return name, nil
}
