package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
)

func TestMigrationCatalogValidation(t *testing.T) {
	for _, directory := range []string{"postgres_migrations", "sqlite_migrations"} {
		migrations, err := readMigrations(directory)
		if err != nil {
			t.Fatal(err)
		}
		if len(migrations) != 2 || migrations[0].name != "0001_baseline.sql" || migrations[1].name != "0002_automation_tasks.sql" {
			t.Fatalf("unexpected baseline: %v", migrations)
		}
	}
	for _, files := range []fstest.MapFS{
		{},
		{"m/0002_gap.sql": {Data: []byte("SELECT 1;")}},
		{"m/0001_a.sql": {Data: []byte("SELECT 1;")}, "m/0001_b.sql": {Data: []byte("SELECT 1;")}},
		{"m/readme.sql": {Data: []byte("SELECT 1;")}},
		{"m/0001_empty.sql": {Data: []byte(" \n")}},
	} {
		if _, err := loadMigrations(files, "m"); err == nil {
			t.Fatalf("invalid migration catalog accepted: %v", files)
		}
	}
	migrations := []migration{{name: "0001_baseline.sql"}, {name: "0002_example.sql"}}
	for _, applied := range [][]string{{"0001_demo01.sql"}, {"0002_example.sql"}, {"0001_baseline.sql", "0001_baseline.sql"}} {
		if err := validateAppliedMigrations(migrations, applied); err == nil {
			t.Fatalf("invalid history accepted: %v", applied)
		}
	}
}

// PostgreSQL uses the configured development database. SQLite corruption cases
// use a unit-test file so deliberate migration damage has a local lifetime.
func TestMigrationBaselinePreservesDocuments(t *testing.T) {
	t.Run("SQLite", func(t *testing.T) {
		url := "sqlite:" + filepath.Join(t.TempDir(), "baseline.db")
		testMigrationBaseline(t, url, "sqlite_migrations")
	})
	t.Run("PostgreSQL", func(t *testing.T) {
		url := os.Getenv("OCCCCAD_DATABASE_URL")
		if strings.HasPrefix(url, "sqlite:") {
			t.Skip("NOT_APPLICABLE: configured backend is SQLite")
		}
		testMigrationBaseline(t, url, "postgres_migrations")
	})
}

func testMigrationBaseline(t *testing.T, url, directory string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	if err = Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	migrations, err := readMigrations(directory)
	if err != nil {
		t.Fatal(err)
	}
	baseline := migrations[0]
	var version, checksum string
	var migrationCount int
	if err = pool.QueryRow(ctx, `SELECT version,checksum FROM occccad.schema_migrations WHERE version=$1`, baseline.name).Scan(&version, &checksum); err != nil {
		t.Fatal(err)
	}
	if version != baseline.name || checksum != baseline.checksum {
		t.Fatalf("incorrect migration metadata: %s %s", version, checksum)
	}
	documentID := uuid.NewString()
	versionID := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO occccad.documents(id,document_type,name,description,owner_user_id) VALUES($1,'PART','Migration sentinel','Preserve this document','00000000-0000-7000-8000-000000000001')`, documentID); err != nil {
		t.Fatal(err)
	}
	model := `{"bodies":[],"features":[],"sentinel":"preserve-exact-model"}`
	hash := sha256.Sum256([]byte(model))
	if _, err = pool.Exec(ctx, `INSERT INTO occccad.document_versions(id,document_id,sequence,model_json,model_hash,state) VALUES($1,$2,1,$3::jsonb,$4,'READY')`, versionID, documentID, model, hex.EncodeToString(hash[:])); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE occccad.documents SET head_version_id=$1 WHERE id=$2`, versionID, documentID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO occccad.document_history(document_id,position,version_id) VALUES($1,1,$2)`, documentID, versionID); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var name, description, head, persistedModel, persistedHash string
		var sequence, historyCount int
		if err := pool.QueryRow(ctx, `SELECT d.name,d.description,d.head_version_id::text,v.model_json::text,v.model_hash,v.sequence FROM occccad.documents d JOIN occccad.document_versions v ON v.id=d.head_version_id WHERE d.id=$1`, documentID).Scan(&name, &description, &head, &persistedModel, &persistedHash, &sequence); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM occccad.document_history WHERE document_id=$1`, documentID).Scan(&historyCount); err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d", name, description, head, persistedModel, persistedHash, sequence, historyCount)
	}
	before := snapshot()
	for range 2 {
		if err = Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if snapshot() != before {
			t.Fatal("repeated migration changed document, revision or history")
		}
	}
	pool.Close()
	pool, err = Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if snapshot() != before {
		t.Fatal("reopen changed persistent document state")
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occccad.schema_migrations`).Scan(&migrationCount); err != nil || migrationCount != len(migrations) {
		t.Fatalf("migration count=%d err=%v", migrationCount, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE occccad.schema_migrations SET checksum='tampered' WHERE version=$1`, baseline.name); err != nil {
		t.Fatal(err)
	}
	// Restore deliberately damaged shared metadata even if an assertion fails.
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanup, `UPDATE occccad.schema_migrations SET checksum=$1 WHERE version=$2`, baseline.checksum, baseline.name); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(cleanup, `DELETE FROM occccad.schema_migrations WHERE version='0001_demo01.sql'`); err != nil {
			t.Error(err)
		}
	})
	if err = Migrate(ctx, pool); err == nil || !strings.Contains(err.Error(), "checksum changed") {
		t.Fatalf("checksum corruption accepted: %v", err)
	}
	if snapshot() != before {
		t.Fatal("checksum rejection changed document state")
	}
	if _, err = pool.Exec(ctx, `UPDATE occccad.schema_migrations SET checksum=$1 WHERE version=$2`, baseline.checksum, baseline.name); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO occccad.schema_migrations(version,checksum,execution_ms) VALUES('0001_demo01.sql','legacy',0)`); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, pool); err == nil || !strings.Contains(err.Error(), "explicit rebuild") {
		t.Fatalf("obsolete history accepted: %v", err)
	}
	if snapshot() != before {
		t.Fatal("obsolete history rejection changed document state")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM occccad.schema_migrations WHERE version='0001_demo01.sql'`); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
}
