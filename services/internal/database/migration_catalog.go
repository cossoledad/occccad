package database

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

//go:embed postgres_migrations/*.sql sqlite_migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	name, sql, checksum string
}

var migrationName = regexp.MustCompile(`^[0-9]{4}_[a-z0-9_]+\.sql$`)

// Matching filenames give both providers the same ordered schema milestones.
// SQL and checksums remain provider-specific.
func readMigrations(directory string) ([]migration, error) {
	postgres, err := loadMigrations(migrationFiles, "postgres_migrations")
	if err != nil {
		return nil, err
	}
	sqlite, err := loadMigrations(migrationFiles, "sqlite_migrations")
	if err != nil {
		return nil, err
	}
	if len(postgres) != len(sqlite) {
		return nil, fmt.Errorf("PostgreSQL and SQLite migration counts differ")
	}
	for i := range postgres {
		if postgres[i].name != sqlite[i].name {
			return nil, fmt.Errorf("PostgreSQL and SQLite migration names differ: %s / %s", postgres[i].name, sqlite[i].name)
		}
	}
	switch directory {
	case "postgres_migrations":
		return postgres, nil
	case "sqlite_migrations":
		return sqlite, nil
	default:
		return nil, fmt.Errorf("unknown migration directory %s", directory)
	}
}

func loadMigrations(files fs.FS, directory string) ([]migration, error) {
	entries, err := fs.ReadDir(files, directory) // fs.ReadDir sorts by filename.
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", directory, err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("empty migration directory %s", directory)
	}
	result := make([]migration, 0, len(entries))
	for i, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !migrationName.MatchString(name) || !strings.HasPrefix(name, fmt.Sprintf("%04d_", i+1)) {
			return nil, fmt.Errorf("invalid migration sequence: %s/%s", directory, name)
		}
		data, err := fs.ReadFile(files, directory+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			return nil, fmt.Errorf("empty migration %s/%s", directory, name)
		}
		digest := sha256.Sum256(data)
		result = append(result, migration{name: name, sql: string(data), checksum: hex.EncodeToString(digest[:])})
	}
	return result, nil
}

func validateAppliedMigrations(migrations []migration, applied []string) error {
	known := make(map[string]bool, len(migrations))
	for _, migration := range migrations {
		known[migration.name] = true
	}
	for i, version := range applied {
		if !known[version] {
			return fmt.Errorf("applied migration %s is absent from the current migration chain; the unpublished development schema requires an explicit rebuild", version)
		}
		if i >= len(migrations) || migrations[i].name != version {
			return fmt.Errorf("applied migrations are not a prefix of the current migration chain: %s", version)
		}
	}
	return nil
}
