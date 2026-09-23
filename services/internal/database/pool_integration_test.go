package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Uses only connection-local temporary tables. No migrations or persistent
// application data changes; safe against the explicitly configured dev database.
func TestPoolPostgresBatchAtomicityAndCancellation(t *testing.T) {
	url := os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("OCCCCAD_TEST_DATABASE_URL is not set")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	cfg.MaxConns = 1
	cfg.ConnConfig.Tracer = timingTracer{}
	raw, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal("cannot connect test database")
	}
	p := &Pool{raw: raw, backend: raw, scheduler: testScheduler(1, 1)}
	defer p.Close()
	if _, err = p.Exec(t.Context(), `CREATE TEMP TABLE data_access_atomicity(id int PRIMARY KEY,value int NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	batch := &pgx.Batch{}
	batch.Queue(`INSERT INTO data_access_atomicity VALUES(1,10)`)
	batch.Queue(`INSERT INTO data_access_atomicity VALUES(2,20)`)
	if err = ExecBatch(t.Context(), tx, batch); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(t.Context())
	tx, err = p.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	batch = &pgx.Batch{}
	batch.Queue(`UPDATE data_access_atomicity SET value=99 WHERE id=1`)
	// Failure in the second chunk must roll back the completed first chunk too.
	for i := 3; i < 135; i++ {
		batch.Queue(`INSERT INTO data_access_atomicity VALUES($1,$1)`, i)
	}
	batch.Queue(`INSERT INTO data_access_atomicity VALUES(1,0)`)
	if err = ExecBatch(t.Context(), tx, batch); err == nil {
		t.Fatal("duplicate key accepted")
	}
	if err = tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count, value int
	if err = p.QueryRow(t.Context(), `SELECT count(*),min(value) FROM data_access_atomicity`).Scan(&count, &value); err != nil {
		t.Fatal(err)
	}
	if count != 2 || value != 10 {
		t.Fatalf("partial commit: rows=%d value=%d", count, value)
	}
	// Queue cancellation must not send SQL or affect the held transaction.
	tx, err = p.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	_, err = p.Exec(ctx, `UPDATE data_access_atomicity SET value=100`)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancel: %v", err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = p.QueryRow(t.Context(), `SELECT min(value) FROM data_access_atomicity`).Scan(&value); err != nil || value != 10 {
		t.Fatalf("canceled SQL executed: %v value=%d", err, value)
	}
	// QueryRow error, early Rows.Close, exhausted Rows and Batch.Close all return capacity.
	if err = p.QueryRow(t.Context(), `SELECT value FROM data_access_atomicity WHERE id=999`).Scan(&value); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	rows, err := p.Query(t.Context(), `SELECT * FROM data_access_atomicity`)
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
	rows, err = p.Query(t.Context(), `SELECT * FROM data_access_atomicity`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	results := p.SendBatch(t.Context(), &pgx.Batch{})
	if err = results.Close(); err != nil {
		t.Fatal(err)
	}
	if p.Snapshot().Active != 0 {
		t.Fatal("leaked admission after real operations")
	}
}
