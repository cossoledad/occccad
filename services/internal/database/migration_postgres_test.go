//go:build integration

package database

import (
	"os"
	"strings"
	"testing"
)

func testConfiguredMigrationBaseline(t *testing.T) {
	t.Run("PostgreSQL", func(t *testing.T) {
		url := os.Getenv("OCCCCAD_DATABASE_URL")
		if strings.HasPrefix(url, "sqlite:") {
			t.Skip("NOT_APPLICABLE: configured backend is SQLite")
		}
		testMigrationBaseline(t, url, "postgres_migrations")
	})
}
