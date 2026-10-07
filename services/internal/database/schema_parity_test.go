package database

import (
	"os"
	"reflect"
	"sort"
	"testing"
)

// Read-only against an explicitly supplied migrated PostgreSQL database; never
// migrates or clears it. The SQLite side is always a fresh temporary file.
func TestSQLitePostgresSchemaParity(t *testing.T) {
	url := os.Getenv("OCCCCAD_TEST_SCHEMA_PARITY_URL")
	if url == "" {
		t.Skip("requires explicitly migrated PostgreSQL database")
	}
	pg, err := Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer pg.Close()
	local := localTestPool(t)
	rows, err := pg.Query(t.Context(), `SELECT table_name,column_name FROM information_schema.columns WHERE table_schema='occccad' AND table_name<>'schema_migrations' ORDER BY table_name,ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string][]string{}
	for rows.Next() {
		var table, column string
		if err = rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		expected[table] = append(expected[table], column)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows, err = local.Query(t.Context(), `SELECT m.name,p.name FROM sqlite_schema m JOIN pragma_table_info(m.name) p WHERE m.type='table' AND m.name NOT IN ('schema_migrations','sqlite_sequence') ORDER BY m.name,p.cid`)
	if err != nil {
		t.Fatal(err)
	}
	actual := map[string][]string{}
	for rows.Next() {
		var table, column string
		if err = rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		actual[table] = append(actual[table], column)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("schema column drift:\nSQLite: %v\nPostgreSQL: %v", actual, expected)
	}
	for _, obsolete := range []string{"ui_toolbars", "ui_toolbar_items"} {
		if _, exists := actual[obsolete]; exists {
			t.Fatalf("obsolete presentation table %s remains", obsolete)
		}
	}
	readStrings := func(db DB, query string) []string {
		t.Helper()
		rows, err := db.Query(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}
		result, err := CollectRows(rows, func(row CollectableRow) (string, error) {
			var item string
			err := row.Scan(&item)
			return item, err
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(result)
		return result
	}
	compare := func(name, pgSQL, sqliteSQL string) {
		t.Helper()
		a, b := readStrings(pg, pgSQL), readStrings(local, sqliteSQL)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("%s drift:\nPostgreSQL: %v\nSQLite: %v", name, a, b)
		}
	}
	compare("primary keys", `SELECT r.relname||':'||string_agg(a.attname,',' ORDER BY k.position)
        FROM pg_constraint c JOIN pg_class r ON r.oid=c.conrelid JOIN pg_namespace n ON n.oid=r.relnamespace
        JOIN LATERAL unnest(c.conkey) WITH ORDINALITY k(attnum,position) ON true JOIN pg_attribute a ON a.attrelid=r.oid AND a.attnum=k.attnum
        WHERE n.nspname='occccad' AND r.relname<>'schema_migrations' AND c.contype='p' GROUP BY r.relname,c.oid`,
		`SELECT name||':'||group_concat(col,',') FROM (SELECT m.name,p.name col FROM sqlite_schema m JOIN pragma_table_info(m.name) p WHERE m.type='table' AND m.name NOT IN ('schema_migrations','sqlite_sequence') AND p.pk>0 ORDER BY m.name,p.pk) GROUP BY name`)
	compare("unique constraints", `SELECT r.relname||':'||string_agg(a.attname,',' ORDER BY k.position)
        FROM pg_constraint c JOIN pg_class r ON r.oid=c.conrelid JOIN pg_namespace n ON n.oid=r.relnamespace
        JOIN LATERAL unnest(c.conkey) WITH ORDINALITY k(attnum,position) ON true JOIN pg_attribute a ON a.attrelid=r.oid AND a.attnum=k.attnum
        WHERE n.nspname='occccad' AND r.relname<>'schema_migrations' AND c.contype='u' GROUP BY r.relname,c.oid`,
		`SELECT tbl||':'||group_concat(col,',') FROM (SELECT m.name tbl,i.name idx,p.name col FROM sqlite_schema m JOIN pragma_index_list(m.name) i JOIN pragma_index_info(i.name) p WHERE m.type='table' AND m.name<>'schema_migrations' AND i.origin='u' ORDER BY m.name,i.name,p.seqno) GROUP BY tbl,idx`)
	compare("foreign keys", `SELECT r.relname||':'||a.attname||':'||f.relname||':'||b.attname||':'||
        CASE c.confdeltype WHEN 'a' THEN 'NO ACTION' WHEN 'r' THEN 'RESTRICT' WHEN 'c' THEN 'CASCADE' WHEN 'n' THEN 'SET NULL' WHEN 'd' THEN 'SET DEFAULT' END||':'||
        CASE c.confupdtype WHEN 'a' THEN 'NO ACTION' WHEN 'r' THEN 'RESTRICT' WHEN 'c' THEN 'CASCADE' WHEN 'n' THEN 'SET NULL' WHEN 'd' THEN 'SET DEFAULT' END
        FROM pg_constraint c JOIN pg_class r ON r.oid=c.conrelid JOIN pg_namespace n ON n.oid=r.relnamespace JOIN pg_class f ON f.oid=c.confrelid
        JOIN LATERAL unnest(c.conkey) WITH ORDINALITY k(attnum,position) ON true JOIN pg_attribute a ON a.attrelid=r.oid AND a.attnum=k.attnum
        JOIN LATERAL unnest(c.confkey) WITH ORDINALITY j(attnum,position) ON j.position=k.position JOIN pg_attribute b ON b.attrelid=f.oid AND b.attnum=j.attnum
        WHERE n.nspname='occccad' AND c.contype='f'`,
		`SELECT m.name||':'||p."from"||':'||p."table"||':'||p."to"||':'||p.on_delete||':'||p.on_update FROM sqlite_schema m JOIN pragma_foreign_key_list(m.name) p WHERE m.type='table'`)
	compare("explicit indexes", `SELECT r.relname||':'||i.relname FROM pg_index x JOIN pg_class i ON i.oid=x.indexrelid JOIN pg_class r ON r.oid=x.indrelid JOIN pg_namespace n ON n.oid=r.relnamespace WHERE n.nspname='occccad' AND NOT EXISTS(SELECT 1 FROM pg_constraint c WHERE c.conindid=i.oid)`,
		`SELECT tbl_name||':'||name FROM sqlite_schema WHERE type='index' AND sql IS NOT NULL`)
	compare("bootstrap administrator", `SELECT id::text||':'||email||':'||display_name||':'||status||':'||platform_role FROM occccad.users WHERE id='00000000-0000-7000-8000-000000000001'`,
		`SELECT id||':'||email||':'||display_name||':'||status||':'||platform_role FROM users WHERE id='00000000-0000-7000-8000-000000000001'`)
	var violations int
	if err := local.QueryRow(t.Context(), `SELECT count(*) FROM pragma_foreign_key_check`).Scan(&violations); err != nil || violations != 0 {
		t.Fatalf("SQLite foreign key integrity: %d %v", violations, err)
	}
	t.Logf("matched %d domain tables, primary/unique keys, foreign keys, explicit indexes and administrator seed", len(actual))
}
