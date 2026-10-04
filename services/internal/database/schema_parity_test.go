package database

import (
	"os"
	"reflect"
	"testing"
)

// Read-only against an explicitly supplied migrated PostgreSQL database; never
// migrates or clears it. The SQLite side is always a fresh temporary file.
func TestSQLitePostgresSchemaAndCatalogParity(t *testing.T) {
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
	type catalogItem struct {
		Toolbar, Command, Name, Help, Icon, Group string
		Order                                     int
		Repeat                                    bool
	}
	read := func(db DB) []catalogItem {
		rows, err := db.Query(t.Context(), `SELECT toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable FROM occccad.ui_toolbar_items ORDER BY toolbar_id,command_id`)
		if err != nil {
			t.Fatal(err)
		}
		result, err := CollectRows(rows, func(row CollectableRow) (catalogItem, error) {
			var item catalogItem
			err := row.Scan(&item.Toolbar, &item.Command, &item.Name, &item.Help, &item.Icon, &item.Group, &item.Order, &item.Repeat)
			return item, err
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if a, b := read(local), read(pg); !reflect.DeepEqual(a, b) {
		t.Fatalf("presentation catalog differs: SQLite=%d PostgreSQL=%d", len(a), len(b))
	}
	t.Logf("matched %d domain tables and presentation catalog", len(actual))
}
