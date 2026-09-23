package database

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	perf "github.com/occccad/occccad/internal/performance"
)

// Pool is the sole runtime database entry point. It schedules a complete
// transaction or result stream as one unit, never its individual statements.
// There is no write-behind, automatic retry, or successful acknowledgement before
// PostgreSQL confirms the operation. Callers must Close rows / end transactions.
type Pool struct {
	raw       *pgxpool.Pool // bootstrap migrations only; not exposed to business callers
	backend   poolBackend
	scheduler *scheduler
}
type poolBackend interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
	SendBatch(context.Context, *pgx.Batch) pgx.BatchResults
	Close()
}

func (p *Pool) Close() { p.backend.Close() }
func (p *Pool) Ping(ctx context.Context) error {
	var one int
	return p.QueryRow(ctx, "SELECT 1").Scan(&one)
}
func (p *Pool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	release, err := p.scheduler.acquire(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	defer release()
	return p.backend.Exec(ctx, sql, args...)
}
func (p *Pool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	release, err := p.scheduler.acquire(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := p.backend.Query(ctx, sql, args...)
	if err != nil {
		release()
		return nil, err
	}
	return &scheduledRows{Rows: rows, release: release}, nil
}
func (p *Pool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	release, err := p.scheduler.acquire(ctx)
	if err != nil {
		return errorRow{err}
	}
	return &scheduledRow{row: p.backend.QueryRow(ctx, sql, args...), release: release}
}
func (p *Pool) Begin(ctx context.Context) (pgx.Tx, error) { return p.BeginTx(ctx, pgx.TxOptions{}) }
func (p *Pool) BeginTx(ctx context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	release, err := p.scheduler.acquire(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := p.backend.BeginTx(ctx, options)
	if err != nil {
		release()
		return nil, err
	}
	return &scheduledTx{Tx: tx, release: release, finish: perf.Start(ctx, "db-transaction")}, nil
}
func (p *Pool) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	release, err := p.scheduler.acquire(ctx)
	if err != nil {
		return errorBatch{err}
	}
	return &scheduledBatch{BatchResults: p.backend.SendBatch(ctx, batch), release: release}
}

type scheduledRows struct {
	pgx.Rows
	release func()
}

func (r *scheduledRows) Close() { r.Rows.Close(); r.release() }
func (r *scheduledRows) Next() bool {
	ok := r.Rows.Next()
	if !ok {
		r.release()
	}
	return ok
}
func (r *scheduledRows) Scan(dest ...any) error {
	err := r.Rows.Scan(dest...)
	if err != nil {
		r.Close()
	}
	return err
}
func (r *scheduledRows) Values() ([]any, error) {
	v, err := r.Rows.Values()
	if err != nil {
		r.Close()
	}
	return v, err
}

type scheduledRow struct {
	row     pgx.Row
	release func()
}

func (r *scheduledRow) Scan(dest ...any) error { defer r.release(); return r.row.Scan(dest...) }

type errorRow struct{ err error }

func (r errorRow) Scan(...any) error { return r.err }

type scheduledTx struct {
	pgx.Tx
	release, finish func()
	once            sync.Once
}

func (t *scheduledTx) done()                              { t.once.Do(func() { t.finish(); t.release() }) }
func (t *scheduledTx) Commit(ctx context.Context) error   { defer t.done(); return t.Tx.Commit(ctx) }
func (t *scheduledTx) Rollback(ctx context.Context) error { defer t.done(); return t.Tx.Rollback(ctx) }

// Nested Begin is pgx's savepoint transaction on the same connection. Its end
// does not release the outer transaction's admission slot.

type scheduledBatch struct {
	pgx.BatchResults
	release func()
	once    sync.Once
}

func (b *scheduledBatch) Close() error { defer b.once.Do(b.release); return b.BatchResults.Close() }

type errorBatch struct{ err error }

func (b errorBatch) Close() error                     { return b.err }
func (b errorBatch) Exec() (pgconn.CommandTag, error) { return pgconn.CommandTag{}, b.err }
func (b errorBatch) Query() (pgx.Rows, error)         { return nil, b.err }
func (b errorBatch) QueryRow() pgx.Row                { return errorRow{b.err} }

// ExecBatch pipelines writes on the caller's existing transaction. Bounded
// chunks limit protocol buffering; all chunks remain in the SAME transaction.
// This helper is for writes with no returned rows or result callbacks.
func ExecBatch(ctx context.Context, tx pgx.Tx, batch *pgx.Batch) error {
	const chunkSize = 128
	for start := 0; start < len(batch.QueuedQueries); start += chunkSize {
		end := min(start+chunkSize, len(batch.QueuedQueries))
		chunk := &pgx.Batch{QueuedQueries: batch.QueuedQueries[start:end]}
		results := tx.SendBatch(ctx, chunk)
		for range chunk.QueuedQueries {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return err
			}
		}
		if err := results.Close(); err != nil {
			return err
		}
	}
	return nil
}

type Snapshot struct {
	Active               int64  `json:"active"`
	Waiting              int64  `json:"waiting"`
	Rejected             uint64 `json:"rejected"`
	MaxConcurrent        int    `json:"maxConcurrent"`
	BackgroundConcurrent int    `json:"backgroundConcurrent"`
	AcquiredConnections  int32  `json:"acquiredConnections"`
	IdleConnections      int32  `json:"idleConnections"`
}

func (p *Pool) Snapshot() Snapshot {
	result := Snapshot{Active: p.scheduler.running.Load(), Waiting: p.scheduler.waiting.Load(), Rejected: p.scheduler.rejected.Load(), MaxConcurrent: p.scheduler.limits.Concurrent, BackgroundConcurrent: p.scheduler.limits.BackgroundConcurrent}
	if p.raw != nil {
		stat := p.raw.Stat()
		result.AcquiredConnections = stat.AcquiredConns()
		result.IdleConnections = stat.IdleConns()
	}
	return result
}
