package database

import (
	"context"
	"database/sql"
	"errors"
)

// DB is the application-facing database contract. Driver types stay in adapters.
// Every transaction is one admission unit; callers must finish transactions and
// consume or close results. Implementations must never retry a write implicitly.
type DB interface {
	Executor
	Begin(context.Context) (Tx, error)
	BeginTx(context.Context, TxOptions) (Tx, error)
	SendBatch(context.Context, *Batch) BatchResults
	Ping(context.Context) error
	Snapshot() Snapshot
	Close()
}
type Executor interface {
	Exec(context.Context, string, ...any) (Result, error)
	Query(context.Context, string, ...any) (Rows, error)
	QueryRow(context.Context, string, ...any) Row
}
type Tx interface {
	Executor
	Begin(context.Context) (Tx, error)
	SendBatch(context.Context, *Batch) BatchResults
	Commit(context.Context) error
	Rollback(context.Context) error
}

// TxOptions intentionally exposes only the portable transaction contract.
// Write transactions use the backend's locking implementation.
type TxOptions struct{}
type Result struct{ affected int64 }

// NewResult constructs a driver-independent affected-row result.
func NewResult(affected int64) Result { return Result{affected: affected} }

func (r Result) RowsAffected() int64 { return r.affected }

type Row interface{ Scan(...any) error }
type CollectableRow = Row
type Rows interface {
	Row
	Next() bool
	Close()
	Err() error
	Values() ([]any, error)
}
type QueuedQuery struct {
	SQL       string
	Arguments []any
}
type Batch struct{ QueuedQueries []*QueuedQuery }

func (b *Batch) Queue(query string, args ...any) {
	b.QueuedQueries = append(b.QueuedQueries, &QueuedQuery{query, args})
}

type BatchResults interface {
	Exec() (Result, error)
	Query() (Rows, error)
	QueryRow() Row
	Close() error
}

var ErrNoRows = sql.ErrNoRows
var ErrTxAborted = errors.New("database transaction is aborted")
var ErrUniqueViolation = errors.New("database unique constraint violation")

func IsUniqueViolation(err error) bool { return errors.Is(err, ErrUniqueViolation) }
func CollectRows[T any](rows Rows, scan func(CollectableRow) (T, error)) ([]T, error) {
	defer rows.Close()
	values := []T{}
	for rows.Next() {
		value, err := scan(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
