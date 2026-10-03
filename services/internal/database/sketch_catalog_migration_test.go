package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"
)

// This test consumes an explicitly configured fresh database and leaves its
// evidence intact. It never drops schemas/tables or falls back to the app URL.
func TestSketchCatalogIncrementalMigrationPreservesDocuments(t *testing.T) {
	url := os.Getenv("OCCCCAD_TEST_MIGRATION_DATABASE_URL")
	if url == "" {
		t.Skip("OCCCCAD_TEST_MIGRATION_DATABASE_URL must name an isolated fresh database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal("cannot open explicitly configured migration test database")
	}
	defer pool.Close()
	var existingSchema bool
	var relations int
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='occccad')`).Scan(&existingSchema); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname <> 'information_schema' AND c.relkind IN ('r','p','v','m','S','f')`).Scan(&relations); err != nil {
		t.Fatal(err)
	}
	if existingSchema || relations != 0 {
		t.Fatal("migration test refuses a non-fresh database; no data was deleted")
	}
	if _, err = pool.Exec(ctx, `CREATE SCHEMA occccad; CREATE TABLE occccad.schema_migrations(version text PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now(),checksum text,execution_ms bigint)`); err != nil {
		t.Fatal(err)
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	expectedBaseline := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name >= "0031_" {
			continue
		}
		sql, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(sql)
		checksum := hex.EncodeToString(digest[:])
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, string(sql)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO occccad.schema_migrations(version,checksum,execution_ms) VALUES($1,$2,0)`, name, checksum)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("baseline migration %s: %v", name, err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		expectedBaseline[name] = checksum
	}
	if len(expectedBaseline) != 30 {
		t.Fatalf("expected real embedded migrations 0001..0030, got %d", len(expectedBaseline))
	}
	const documentID = "00000000-0000-7000-8000-00000000a031"
	const versionID = "00000000-0000-7000-8000-00000000b031"
	if _, err = pool.Exec(ctx, `INSERT INTO occccad.documents(id,document_type,name,description,owner_user_id) VALUES($1,'PART','Migration sentinel','Do not modify migration sentinel','00000000-0000-7000-8000-000000000001');`, documentID); err != nil {
		t.Fatal(err)
	}
	sentinelModel := `{"bodies":[],"features":[],"sentinel":"preserve-exact-model"}`
	sentinelDigest := sha256.Sum256([]byte(sentinelModel))
	if _, err = pool.Exec(ctx, `INSERT INTO occccad.document_versions(id,document_id,sequence,model_json,model_hash,state) VALUES($1,$2,1,$3::jsonb,$4,'READY')`, versionID, documentID, sentinelModel, hex.EncodeToString(sentinelDigest[:])); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE occccad.documents SET head_version_id=$1 WHERE id=$2`, versionID, documentID); err != nil {
		t.Fatal(err)
	}
	snapshot := func() (string, string, int) {
		t.Helper()
		var document, version string
		var count int
		if err := pool.QueryRow(ctx, `SELECT to_jsonb(d)::text FROM occccad.documents d WHERE id=$1`, documentID).Scan(&document); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT to_jsonb(v)::text FROM occccad.document_versions v WHERE id=$1`, versionID).Scan(&version); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM occccad.documents`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return document, version, count
	}
	checksums := func() map[string]string {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT version,checksum FROM occccad.schema_migrations ORDER BY version`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		result := map[string]string{}
		for rows.Next() {
			var name, checksum string
			if err := rows.Scan(&name, &checksum); err != nil {
				t.Fatal(err)
			}
			result[name] = checksum
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	beforeDocument, beforeVersion, beforeCount := snapshot()
	if !reflect.DeepEqual(checksums(), expectedBaseline) {
		t.Fatal("baseline checksums do not match exact embedded content")
	}
	var oldSlotCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occccad.ui_toolbar_items WHERE command_id='sketch.slot'`).Scan(&oldSlotCount); err != nil || oldSlotCount != 1 {
		t.Fatalf("original 0023 Slot catalog was not installed: %v count=%d", err, oldSlotCount)
	}
	if err = Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	afterDocument, afterVersion, afterCount := snapshot()
	if beforeDocument != afterDocument || beforeVersion != afterVersion || beforeCount != afterCount {
		t.Fatal("incremental catalog migration changed persistent documents")
	}
	upgraded := checksums()
	for name, want := range expectedBaseline {
		if upgraded[name] != want {
			t.Fatalf("baseline checksum changed: %s", name)
		}
	}
	sql, _ := migrationFiles.ReadFile("migrations/0031_sketch_workflow.sql")
	digest := sha256.Sum256(sql)
	if upgraded["0031_sketch_workflow.sql"] != hex.EncodeToString(digest[:]) {
		t.Fatal("new migration checksum missing or incorrect")
	}
	var slotCount, editCount, newCount, helpCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occccad.ui_toolbar_items WHERE command_id='sketch.slot'`).Scan(&slotCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occccad.ui_toolbar_items WHERE toolbar_id='sketch-edit'`).Scan(&editCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occccad.ui_toolbar_items WHERE command_id IN ('sketch.circle.three_point','sketch.arc.three_point','sketch.rectangle.center','sketch.rectangle.oriented','sketch.ellipse','sketch.elliptical_arc','sketch.spline.control','sketch.constraint.collinear','sketch.constraint.horizontal_distance','sketch.constraint.vertical_distance','sketch.constraint.major_radius','sketch.constraint.minor_radius')`).Scan(&newCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occccad.ui_toolbar_items WHERE (command_id='sketch.point' AND help_text LIKE '%C 切换%') OR (command_id='sketch.line' AND help_text LIKE '%Tab%') OR (command_id='sketch.polyline' AND help_text LIKE '%T 切换%') OR (command_id='sketch.spline' AND help_text LIKE '%拟合点%')`).Scan(&helpCount); err != nil {
		t.Fatal(err)
	}
	if slotCount != 0 || editCount != 17 || newCount != 12 || helpCount != 4 {
		t.Fatalf("catalog mismatch: slot=%d edit=%d new=%d updatedHelp=%d", slotCount, editCount, newCount, helpCount)
	}
	var removed int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occccad.ui_toolbar_items WHERE command_id IN ('sketch.edit.scale','sketch.edit.quick_trim','sketch.edit.rotate','sketch.normal') OR group_key='more'`).Scan(&removed); err != nil || removed != 0 {
		t.Fatalf("obsolete commands or overflow groups remain: %v count=%d", err, removed)
	}
	var families, polygonModes, arcVariants int
	if err = pool.QueryRow(ctx, `SELECT count(DISTINCT group_key) FROM occccad.ui_toolbar_items WHERE toolbar_id='sketch-dimensional-constraints'`).Scan(&families); err != nil || families != 3 {
		t.Fatalf("dimension families: %d %v", families, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occccad.ui_toolbar_items WHERE group_key='variants:polygon'`).Scan(&polygonModes); err != nil || polygonModes != 2 {
		t.Fatalf("polygon modes: %d %v", polygonModes, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM occccad.ui_toolbar_items WHERE toolbar_id='sketch-profiles' AND group_key='variants:arc'`).Scan(&arcVariants); err != nil || arcVariants != 3 {
		t.Fatalf("arc variants: %d %v", arcVariants, err)
	}
	var catalogBefore, catalogAfter string
	if err = pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(i) ORDER BY toolbar_id,command_id)::text FROM occccad.ui_toolbar_items i`).Scan(&catalogBefore); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(i) ORDER BY toolbar_id,command_id)::text FROM occccad.ui_toolbar_items i`).Scan(&catalogAfter); err != nil {
		t.Fatal(err)
	}
	if catalogBefore != catalogAfter || !reflect.DeepEqual(checksums(), upgraded) {
		t.Fatal("repeated production Migrate was not idempotent")
	}
	repeatedDocument, repeatedVersion, repeatedCount := snapshot()
	if beforeDocument != repeatedDocument || beforeVersion != repeatedVersion || beforeCount != repeatedCount {
		t.Fatal("repeated migration changed documents")
	}
}
