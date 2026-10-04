package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresBackend struct{ *pgxpool.Pool }

func postgresError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoRows
	}
	if errors.Is(err, pgx.ErrTxCommitRollback) {
		return errors.Join(ErrTxAborted, err)
	}
	var e *pgconn.PgError
	if errors.As(err, &e) && e.Code == "23505" {
		return errors.Join(ErrUniqueViolation, err)
	}
	return err
}
func (p *postgresBackend) Exec(ctx context.Context, q string, a ...any) (Result, error) {
	r, e := p.Pool.Exec(ctx, q, a...)
	return Result{r.RowsAffected()}, postgresError(e)
}
func (p *postgresBackend) Query(ctx context.Context, q string, a ...any) (Rows, error) {
	r, e := p.Pool.Query(ctx, q, a...)
	return wrapPostgresRows(r, e)
}
func (p *postgresBackend) QueryRow(ctx context.Context, q string, a ...any) Row {
	return postgresRow{p.Pool.QueryRow(ctx, q, a...)}
}
func (p *postgresBackend) BeginTx(ctx context.Context, _ TxOptions) (Tx, error) {
	t, e := p.Pool.BeginTx(ctx, pgx.TxOptions{})
	if e != nil {
		return nil, postgresError(e)
	}
	return &postgresTx{t}, nil
}
func (p *postgresBackend) SendBatch(ctx context.Context, b *Batch) BatchResults {
	return postgresBatch{p.Pool.SendBatch(ctx, postgresQueries(b))}
}

type postgresTx struct{ pgx.Tx }

func (t *postgresTx) Begin(ctx context.Context) (Tx, error) {
	v, e := t.Tx.Begin(ctx)
	if e != nil {
		return nil, postgresError(e)
	}
	return &postgresTx{v}, nil
}
func (t *postgresTx) Exec(ctx context.Context, q string, a ...any) (Result, error) {
	r, e := t.Tx.Exec(ctx, q, a...)
	return Result{r.RowsAffected()}, postgresError(e)
}
func (t *postgresTx) Query(ctx context.Context, q string, a ...any) (Rows, error) {
	r, e := t.Tx.Query(ctx, q, a...)
	return wrapPostgresRows(r, e)
}
func (t *postgresTx) QueryRow(ctx context.Context, q string, a ...any) Row {
	return postgresRow{t.Tx.QueryRow(ctx, q, a...)}
}
func (t *postgresTx) SendBatch(ctx context.Context, b *Batch) BatchResults {
	return postgresBatch{t.Tx.SendBatch(ctx, postgresQueries(b))}
}
func (t *postgresTx) Commit(ctx context.Context) error   { return postgresError(t.Tx.Commit(ctx)) }
func (t *postgresTx) Rollback(ctx context.Context) error { return postgresError(t.Tx.Rollback(ctx)) }

type postgresRow struct{ pgx.Row }

func (r postgresRow) Scan(a ...any) error { return postgresError(r.Row.Scan(a...)) }

type postgresRows struct{ pgx.Rows }

func wrapPostgresRows(r pgx.Rows, e error) (Rows, error) {
	if e != nil {
		return nil, postgresError(e)
	}
	return postgresRows{r}, nil
}
func (r postgresRows) Scan(a ...any) error { return postgresError(r.Rows.Scan(a...)) }
func (r postgresRows) Err() error          { return postgresError(r.Rows.Err()) }

type postgresBatch struct{ pgx.BatchResults }

func (b postgresBatch) Exec() (Result, error) {
	r, e := b.BatchResults.Exec()
	return Result{r.RowsAffected()}, postgresError(e)
}
func (b postgresBatch) Query() (Rows, error) {
	r, e := b.BatchResults.Query()
	return wrapPostgresRows(r, e)
}
func (b postgresBatch) QueryRow() Row { return postgresRow{b.BatchResults.QueryRow()} }
func (b postgresBatch) Close() error  { return postgresError(b.BatchResults.Close()) }
func postgresQueries(b *Batch) *pgx.Batch {
	r := &pgx.Batch{}
	for _, q := range b.QueuedQueries {
		r.Queue(q.SQL, q.Arguments...)
	}
	return r
}

func (p *postgresBackend) connections() (int32, int32) {
	s := p.Pool.Stat()
	return s.AcquiredConns(), s.IdleConns()
}
