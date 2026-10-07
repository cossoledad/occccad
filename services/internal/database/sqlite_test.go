package database

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func localTestPool(t *testing.T) *Pool {
	t.Helper()
	p, e := Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "local.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(p.Close)
	if e = Migrate(t.Context(), p); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestSQLiteMigrationsReopenAndConstraints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.db")
	p, e := Open(t.Context(), "sqlite:"+path)
	if e != nil {
		t.Fatal(e)
	}
	if e = Migrate(t.Context(), p); e != nil {
		t.Fatal(e)
	}
	if e = Migrate(t.Context(), p); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = p.QueryRow(t.Context(), `SELECT count(*) FROM sqlite_master WHERE name IN ('ui_toolbars','ui_toolbar_items')`).Scan(&count); e != nil || count != 0 {
		t.Fatalf("obsolete presentation storage remains %d: %v", count, e)
	}
	var id string
	e = p.QueryRow(t.Context(), `INSERT INTO occccad.folders(name,owner_user_id) VALUES('root','00000000-0000-7000-8000-000000000001') RETURNING id::text`).Scan(&id)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.Exec(t.Context(), `INSERT INTO occccad.folders(name,owner_user_id) VALUES('root','00000000-0000-7000-8000-000000000001')`); !IsUniqueViolation(e) {
		t.Fatalf("root uniqueness: %v", e)
	}
	if _, e = p.Exec(t.Context(), `INSERT INTO occccad.folders(name,owner_user_id) VALUES('bad','11111111-1111-1111-1111-111111111111')`); e == nil {
		t.Fatal("foreign key accepted")
	}
	p.Close()
	p, e = Open(t.Context(), "sqlite:"+path)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	var name string
	if e = p.QueryRow(t.Context(), `SELECT name FROM occccad.folders WHERE id=$1`, id).Scan(&name); e != nil || name != "root" {
		t.Fatalf("reopen: %s %v", name, e)
	}
}
func TestSQLiteAtomicBatchCancellationAndSavepoint(t *testing.T) {
	p := localTestPool(t)
	ctx := t.Context()
	if _, e := p.Exec(ctx, `CREATE TABLE test_atomic(id INTEGER PRIMARY KEY,value INTEGER NOT NULL)`); e != nil {
		t.Fatal(e)
	}
	tx, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	b := &Batch{}
	for i := 0; i < 140; i++ {
		b.Queue(`INSERT INTO test_atomic VALUES($1,$1)`, i)
	}
	b.Queue(`INSERT INTO test_atomic VALUES(0,9)`)
	if e = ExecBatch(ctx, tx, b); !IsUniqueViolation(e) {
		t.Fatal(e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	var n int
	if e = p.QueryRow(ctx, `SELECT count(*) FROM test_atomic`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("partial batch %d %v", n, e)
	}
	tx, e = p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO test_atomic VALUES(1,10)`); e != nil {
		t.Fatal(e)
	}
	child, e := tx.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = child.Exec(ctx, `UPDATE test_atomic SET value=20`); e != nil {
		t.Fatal(e)
	}
	if e = child.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	cancelCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if _, e = p.Exec(cancelCtx, `UPDATE test_atomic SET value=30`); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = p.QueryRow(ctx, `SELECT value FROM test_atomic`).Scan(&n); e != nil || n != 10 {
		t.Fatalf("value=%d %v", n, e)
	}
	if e = p.QueryRow(ctx, `SELECT id FROM test_atomic WHERE id=2`).Scan(&n); !errors.Is(e, ErrNoRows) {
		t.Fatal(e)
	}
	if p.Snapshot().Active != 0 {
		t.Fatal("leaked admission")
	}
}
func TestSQLiteIndependentPoolsSerializeCAS(t *testing.T) {
	p := localTestPool(t)
	backend := p.backend.(*sqliteBackend)
	other, e := Open(t.Context(), "sqlite:"+backend.path)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	if _, e = p.Exec(t.Context(), `CREATE TABLE test_cas(id INTEGER PRIMARY KEY,head INTEGER)`); e != nil {
		t.Fatal(e)
	}
	p.Exec(t.Context(), `INSERT INTO test_cas VALUES(1,0)`)
	tx, e := p.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	result := make(chan error, 1)
	go func() {
		t2, e := other.Begin(t.Context())
		if e == nil {
			defer t2.Rollback(t.Context())
			var r Result
			r, e = t2.Exec(t.Context(), `UPDATE test_cas SET head=2 WHERE id=1 AND head=0`)
			if e == nil && r.RowsAffected() != 0 {
				e = fmt.Errorf("stale head advanced")
			}
			if e == nil {
				e = t2.Commit(t.Context())
			}
		}
		result <- e
	}()
	if _, e = tx.Exec(t.Context(), `UPDATE test_cas SET head=1 WHERE id=1 AND head=0`); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(t.Context()); e != nil {
		t.Fatal(e)
	}
	if e = <-result; e != nil {
		t.Fatal(e)
	}
}

// Compile the complete static application queries against the real empty schema.
// This catches drift in the dialect vocabulary, joins, columns and JSON paths.
// Runtime-composed Jobs statements are exercised by service contract tests.
func TestSQLiteApplicationQueriesCompile(t *testing.T) {
	p := localTestPool(t)
	raw := p.backend.(*sqliteBackend).db
	tested := 0
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == "../database" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, e := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if e != nil {
			return e
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			q, e := strconv.Unquote(lit.Value)
			if e != nil || !strings.Contains(q, "occccad.") {
				return true
			}
			trim := strings.TrimSpace(q)
			upper := strings.ToUpper(trim)
			if !(strings.HasPrefix(upper, "SELECT ") || strings.HasPrefix(upper, "WITH ") || strings.HasPrefix(upper, "UPDATE ") || strings.HasPrefix(upper, "INSERT ") || strings.HasPrefix(upper, "DELETE ")) || (strings.HasSuffix(upper, "RETURNING") || strings.HasSuffix(upper, "ORDER BY")) {
				return true
			}
			tokens, _ := sqlTokens(q)
			argc := 0
			for _, v := range tokens {
				if strings.HasPrefix(v, "$") {
					n, _ := strconv.Atoi(v[1:])
					argc = max(argc, n)
				}
			}
			args := make([]any, argc)
			for i := range args {
				args[i] = ""
			}
			lowered, a, e := sqliteQuery(q, args)
			if e == nil {
				var rows Rows
				rows, e = sqliteQueryRows(t.Context(), raw, "EXPLAIN "+q, a...)
				if rows != nil {
					rows.Close()
				}
			}
			if e != nil {
				t.Errorf("%s: %v\nSQL: %s", path, e, lowered)
			}
			tested++
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if tested < 150 {
		t.Fatalf("only %d queries", tested)
	}
	t.Logf("compiled %d application queries", tested)
}

func TestSQLiteFailedStatementCannotCommitEarlierWrites(t *testing.T) {
	p := localTestPool(t)
	ctx := t.Context()
	if _, err := p.Exec(ctx, `CREATE TABLE test_abort(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO test_abort VALUES(1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO test_abort VALUES(1)`); !IsUniqueViolation(err) {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); !errors.Is(err, ErrTxAborted) {
		t.Fatalf("failed transaction committed: %v", err)
	}
	var n int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM test_abort`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("partial commit: %d %v", n, err)
	}
}

func TestSQLiteCrossProcessTransactions(t *testing.T) {
	if path := os.Getenv("OCCCCAD_SQLITE_PROCESS_TEST"); path != "" {
		p, err := Open(t.Context(), "sqlite:"+path)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		for range 12 {
			tx, err := p.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			var head int
			if err = tx.QueryRow(t.Context(), `SELECT head FROM process_cas WHERE id=1`).Scan(&head); err != nil {
				t.Fatal(err)
			}
			result, err := tx.Exec(t.Context(), `UPDATE process_cas SET head=$1 WHERE id=1 AND head=$2`, head+1, head)
			if err != nil || result.RowsAffected() != 1 {
				t.Fatalf("CAS: %v", err)
			}
			if err = tx.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	p := localTestPool(t)
	if _, err := p.Exec(t.Context(), `CREATE TABLE process_cas(id INTEGER PRIMARY KEY,head INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(t.Context(), `INSERT INTO process_cas VALUES(1,0)`); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestSQLiteCrossProcessTransactions$", "-test.count=1")
			cmd.Env = append(os.Environ(), "OCCCCAD_SQLITE_PROCESS_TEST="+p.backend.(*sqliteBackend).path)
			output, err := cmd.CombinedOutput()
			if err != nil {
				err = fmt.Errorf("subprocess: %w: %s", err, output)
			}
			results <- err
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var head int
	if err := p.QueryRow(t.Context(), `SELECT head FROM process_cas`).Scan(&head); err != nil || head != 24 {
		t.Fatalf("lost cross-process write: %d %v", head, err)
	}
}

func TestSQLiteDialectValuesAndQuotedText(t *testing.T) {
	p := localTestPool(t)
	var literal, number string
	var value float64
	var raw []byte
	err := p.QueryRow(t.Context(), `SELECT 'occccad.a::text FOR UPDATE $2', 1.25e-2,42::text,
 jsonb_build_object('nested','{"x":1}'::jsonb,'flag','true'::jsonb)`).Scan(&literal, &value, &number, &raw)
	if err != nil || literal != "occccad.a::text FOR UPDATE $2" || value != 0.0125 || number != "42" || string(raw) != `{"nested":{"x":1},"flag":true}` {
		t.Fatalf("values: %q %v %q %s: %v", literal, value, number, raw, err)
	}
	var a, b, c string
	if err = p.QueryRow(t.Context(), `SELECT $2,$1,$2`, "first", "second").Scan(&a, &b, &c); err != nil || a != "second" || b != "first" || c != a {
		t.Fatalf("numbered binds: %s %s %s: %v", a, b, c, err)
	}
}

func TestSQLiteResetRestoresForeignKeysOnSameConnection(t *testing.T) {
	pool := localTestPool(t)
	ctx := t.Context()
	if _, err := ResetDevelopmentSchema(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var enabled int
	if err := pool.QueryRow(ctx, `PRAGMA foreign_keys`).Scan(&enabled); err != nil || enabled != 1 {
		t.Fatalf("foreign_keys after reset = %d: %v", enabled, err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO occccad.account_audit_events(actor_user_id,action) VALUES('00000000-0000-0000-0000-000000000001','invalid actor')`); err == nil {
		t.Fatal("reset left foreign keys unenforced")
	}
}
