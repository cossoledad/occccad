package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	perf "github.com/occccad/occccad/internal/performance"
	"modernc.org/sqlite"
)

const sqliteTimeLayout = "2006-01-02T15:04:05.000000000Z"

var sqliteFunctions sync.Once

func registerSQLiteFunctions() {
	sqliteFunctions.Do(func() {
		sqlite.MustRegisterDeterministicScalarFunction("lower", 1, func(_ *sqlite.FunctionContext, a []driver.Value) (driver.Value, error) {
			if a[0] == nil {
				return nil, nil
			}
			return strings.ToLower(fmt.Sprint(a[0])), nil
		})
		sqlite.MustRegisterDeterministicScalarFunction("role_level", 1, func(_ *sqlite.FunctionContext, a []driver.Value) (driver.Value, error) {
			switch a[0] {
			case "OWNER":
				return int64(30), nil
			case "EDITOR":
				return int64(20), nil
			case "VIEWER":
				return int64(10), nil
			}
			return int64(0), nil
		})
		sqlite.MustRegisterDeterministicScalarFunction("role_name", 1, func(_ *sqlite.FunctionContext, a []driver.Value) (driver.Value, error) {
			n, _ := a[0].(int64)
			switch {
			case n >= 30:
				return "OWNER", nil
			case n >= 20:
				return "EDITOR", nil
			case n >= 10:
				return "VIEWER", nil
			}
			return "NONE", nil
		})

		sqlite.MustRegisterScalarFunction("now", 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			return time.Now().UTC().Format(sqliteTimeLayout), nil
		})
		sqlite.MustRegisterScalarFunction("uuidv7", 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			v, e := uuid.NewV7()
			return v.String(), e
		})
		sqlite.MustRegisterDeterministicScalarFunction("add_interval", 2, func(_ *sqlite.FunctionContext, a []driver.Value) (driver.Value, error) {
			base, e := time.Parse(time.RFC3339Nano, fmt.Sprint(a[0]))
			if e != nil {
				return nil, e
			}
			raw := fmt.Sprint(a[1])
			var d time.Duration
			d, e = time.ParseDuration(raw)
			if e != nil {
				var n int
				var unit string
				if _, e = fmt.Sscan(raw, &n, &unit); e != nil {
					return nil, e
				}
				switch strings.TrimSuffix(unit, "s") {
				case "second":
					d = time.Duration(n) * time.Second
				case "minute":
					d = time.Duration(n) * time.Minute
				case "hour":
					d = time.Duration(n) * time.Hour
				case "day":
					d = time.Duration(n) * 24 * time.Hour
				default:
					return nil, fmt.Errorf("unsupported interval unit %q", unit)
				}
			}
			return base.Add(d).UTC().Format(sqliteTimeLayout), nil
		})
	})
}

type sqliteBackend struct {
	db   *sql.DB
	path string
}

func openSQLite(ctx context.Context, databaseURL string) (*Pool, error) {
	registerSQLiteFunctions()
	raw := strings.TrimPrefix(databaseURL, "sqlite:")
	// Only a local file is accepted. All processes must use the same absolute path.
	raw = strings.TrimPrefix(raw, "//")
	if raw == "" || strings.ContainsAny(raw, "?#") || raw == ":memory:" {
		return nil, errors.New("sqlite URL must identify a local database file without query parameters")
	}
	if !filepath.IsAbs(raw) {
		return nil, errors.New("SQLite Local Mode requires an absolute file path shared by API and Jobs")
	}
	path, err := filepath.Abs(raw)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: path}).String() + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(2000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	limits, err := schedulingLimits(1)
	if err != nil {
		db.Close()
		return nil, err
	}
	if limits.Concurrent != 1 {
		db.Close()
		return nil, errors.New("SQLite requires OCCCCAD_DB_CONCURRENCY=1 (or unset)")
	}
	p := &Pool{backend: &sqliteBackend{db: db, path: path}, scheduler: newScheduler(limits)}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	return p, nil
}
func sqliteError(err error) error {
	var e *sqlite.Error
	if errors.As(err, &e) {
		switch e.Code() {
		case 1555, 2067:
			return errors.Join(ErrUniqueViolation, err)
		case 5, 6, 261, 262, 517:
			return errors.Join(ErrBusy, err)
		}
	}
	return err
}
func sqliteArgs(args []any) ([]any, error) {
	result := make([]any, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case time.Time:
			result[i] = v.UTC().Format(sqliteTimeLayout)
		case *time.Time:
			if v != nil {
				result[i] = v.UTC().Format(sqliteTimeLayout)
			}
		case []byte:
			if v != nil {
				result[i] = string(v)
			}
		case json.RawMessage:
			if v != nil {
				result[i] = string(v)
			}
		case []string:
			b, _ := json.Marshal(v)
			result[i] = string(b)
		default:
			if a == nil {
				continue
			}
			if _, ok := a.(driver.Valuer); ok {
				result[i] = a
				continue
			}
			kind := reflect.TypeOf(a).Kind()
			if kind == reflect.Map || kind == reflect.Slice || kind == reflect.Struct || kind == reflect.Array {
				b, e := json.Marshal(a)
				if e != nil {
					return nil, e
				}
				result[i] = string(b)
			} else {
				result[i] = a
			}
		}
	}
	return result, nil
}

type sqliteExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func sqliteExec(ctx context.Context, e sqliteExecutor, q string, a ...any) (Result, error) {
	finish := perf.Start(ctx, "db-query")
	defer finish()
	query, args, err := sqliteQuery(q, a)
	if err != nil {
		return Result{}, err
	}
	r, err := e.ExecContext(ctx, query, args...)
	if err != nil {
		return Result{}, sqliteError(err)
	}
	n, err := r.RowsAffected()
	return Result{n}, err
}
func sqliteQueryRows(ctx context.Context, e sqliteExecutor, q string, a ...any) (Rows, error) {
	finish := perf.Start(ctx, "db-query")
	defer finish()
	query, args, err := sqliteQuery(q, a)
	if err != nil {
		return nil, err
	}
	r, err := e.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, sqliteError(err)
	}
	return &sqliteRows{rows: r}, nil
}
func sqliteQueryRow(ctx context.Context, e sqliteExecutor, q string, a ...any) Row {
	r, err := sqliteQueryRows(ctx, e, q, a...)
	if err != nil {
		return errorRow{err}
	}
	return sqliteRow{r}
}
func (p *sqliteBackend) Close() { _ = p.db.Close() }
func (p *sqliteBackend) Exec(ctx context.Context, q string, a ...any) (Result, error) {
	return sqliteExec(ctx, p.db, q, a...)
}
func (p *sqliteBackend) Query(ctx context.Context, q string, a ...any) (Rows, error) {
	return sqliteQueryRows(ctx, p.db, q, a...)
}
func (p *sqliteBackend) QueryRow(ctx context.Context, q string, a ...any) Row {
	return sqliteQueryRow(ctx, p.db, q, a...)
}
func (p *sqliteBackend) BeginTx(ctx context.Context, _ TxOptions) (Tx, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, sqliteError(err)
	}
	return &sqliteTx{tx: tx}, nil
}
func (p *sqliteBackend) SendBatch(ctx context.Context, b *Batch) BatchResults {
	tx, err := p.BeginTx(ctx, TxOptions{})
	if err != nil {
		return errorBatch{err}
	}
	return &sqliteBatch{ctx: ctx, tx: tx, batch: b, owns: true}
}

type sqliteTx struct {
	tx        *sql.Tx
	savepoint string
	sequence  *int
	done      bool
	failed    error
}

func (t *sqliteTx) Exec(ctx context.Context, q string, a ...any) (Result, error) {
	if t.done {
		return Result{}, sql.ErrTxDone
	}
	if t.failed != nil {
		return Result{}, errors.Join(ErrTxAborted, t.failed)
	}
	r, err := sqliteExec(ctx, t.tx, q, a...)
	if err != nil {
		t.failed = err
	}
	return r, err
}
func (t *sqliteTx) Query(ctx context.Context, q string, a ...any) (Rows, error) {
	if t.done {
		return nil, sql.ErrTxDone
	}
	if t.failed != nil {
		return nil, errors.Join(ErrTxAborted, t.failed)
	}
	r, err := sqliteQueryRows(ctx, t.tx, q, a...)
	if err != nil {
		t.failed = err
		return nil, err
	}
	return &sqliteTxRows{Rows: r, owner: t}, nil
}
func (t *sqliteTx) QueryRow(ctx context.Context, q string, a ...any) Row {
	if t.done {
		return errorRow{sql.ErrTxDone}
	}
	r, err := t.Query(ctx, q, a...)
	if err != nil {
		return errorRow{err}
	}
	return sqliteRow{r}
}
func (t *sqliteTx) Begin(ctx context.Context) (Tx, error) {
	if t.failed != nil {
		return nil, errors.Join(ErrTxAborted, t.failed)
	}
	if t.done {
		return nil, sql.ErrTxDone
	}
	if t.sequence == nil {
		t.sequence = new(int)
	}
	*t.sequence++
	name := fmt.Sprintf("nested_%d", *t.sequence)
	if _, err := t.tx.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return nil, sqliteError(err)
	}
	return &sqliteTx{tx: t.tx, savepoint: name, sequence: t.sequence}, nil
}
func (t *sqliteTx) Commit(ctx context.Context) error {
	if t.failed != nil && !t.done {
		err := errors.Join(ErrTxAborted, t.failed)
		_ = t.Rollback(ctx)
		return err
	}
	if t.done {
		return sql.ErrTxDone
	}
	t.done = true
	if t.savepoint != "" {
		_, err := t.tx.ExecContext(ctx, "RELEASE SAVEPOINT "+t.savepoint)
		return sqliteError(err)
	}
	if err := ctx.Err(); err != nil {
		_ = t.tx.Rollback()
		return err
	}
	return sqliteError(t.tx.Commit())
}
func (t *sqliteTx) Rollback(ctx context.Context) error {
	if t.done {
		return sql.ErrTxDone
	}
	t.done = true
	if t.savepoint != "" {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_, err := t.tx.ExecContext(cleanup, "ROLLBACK TO SAVEPOINT "+t.savepoint)
		if err != nil {
			return sqliteError(err)
		}
		_, err = t.tx.ExecContext(cleanup, "RELEASE SAVEPOINT "+t.savepoint)
		return sqliteError(err)
	}
	return sqliteError(t.tx.Rollback())
}
func (t *sqliteTx) SendBatch(ctx context.Context, b *Batch) BatchResults {
	return &sqliteBatch{ctx: ctx, tx: t, batch: b}
}

type sqliteRow struct{ Rows }

func (r sqliteRow) Scan(dest ...any) error {
	defer r.Close()
	if !r.Next() {
		if err := r.Err(); err != nil {
			return err
		}
		return ErrNoRows
	}
	return r.Rows.Scan(dest...)
}

type sqliteRows struct{ rows *sql.Rows }

func (r *sqliteRows) Next() bool { return r.rows.Next() }
func (r *sqliteRows) Close()     { _ = r.rows.Close() }
func (r *sqliteRows) Err() error { return sqliteError(r.rows.Err()) }
func (r *sqliteRows) Values() ([]any, error) {
	columns, err := r.rows.Columns()
	if err != nil {
		return nil, err
	}
	v := make([]any, len(columns))
	d := make([]any, len(v))
	for i := range v {
		d[i] = &v[i]
	}
	err = r.rows.Scan(d...)
	return v, sqliteError(err)
}

// database/sql handles scalar conversion. JSON containers and UTC timestamps
// have explicit portable encodings rather than relying on driver-specific Scan.
func (r *sqliteRows) Scan(dest ...any) error {
	converted := append([]any(nil), dest...)
	after := []func() error{}
	for i, d := range dest {
		rv := reflect.ValueOf(d)
		if rv.Kind() != reflect.Pointer || rv.IsNil() {
			continue
		}
		typ := rv.Elem().Type()
		if _, ok := d.(sql.Scanner); ok {
			continue
		}
		if typ == reflect.TypeOf(json.RawMessage{}) {
			var raw []byte
			converted[i] = &raw
			after = append(after, func() error { rv.Elem().SetBytes(raw); return nil })
			continue
		}
		if typ == reflect.TypeOf(time.Time{}) || typ == reflect.TypeOf((*time.Time)(nil)) {
			var raw sql.NullString
			converted[i] = &raw
			after = append(after, func() error {
				if !raw.Valid {
					if typ.Kind() == reflect.Pointer {
						rv.Elem().SetZero()
						return nil
					}
					return errors.New("NULL timestamp")
				}
				v, e := time.Parse(time.RFC3339Nano, raw.String)
				if e != nil {
					return e
				}
				if typ.Kind() == reflect.Pointer {
					rv.Elem().Set(reflect.ValueOf(&v))
				} else {
					rv.Elem().Set(reflect.ValueOf(v))
				}
				return nil
			})
		} else if typ.Kind() == reflect.Map || typ.Kind() == reflect.Struct || (typ.Kind() == reflect.Slice && typ.Elem().Kind() != reflect.Uint8) {
			var raw sql.NullString
			converted[i] = &raw
			after = append(after, func() error {
				if !raw.Valid {
					rv.Elem().SetZero()
					return nil
				}
				return json.Unmarshal([]byte(raw.String), d)
			})
		}
	}
	if err := r.rows.Scan(converted...); err != nil {
		return sqliteError(err)
	}
	for _, f := range after {
		if err := f(); err != nil {
			return err
		}
	}
	return nil
}

type sqliteBatch struct {
	ctx          context.Context
	tx           Tx
	batch        *Batch
	index        int
	owns, closed bool
	err          error
	open         Rows
}

func (b *sqliteBatch) next() (*QueuedQuery, error) {
	if b.closed {
		return nil, sql.ErrTxDone
	}
	if b.open != nil {
		b.open.Close()
		b.open = nil
	}
	if b.err != nil {
		return nil, b.err
	}
	if b.index >= len(b.batch.QueuedQueries) {
		return nil, errors.New("batch exhausted")
	}
	q := b.batch.QueuedQueries[b.index]
	b.index++
	return q, nil
}
func (b *sqliteBatch) Exec() (Result, error) {
	q, e := b.next()
	if e != nil {
		return Result{}, e
	}
	r, e := b.tx.Exec(b.ctx, q.SQL, q.Arguments...)
	b.err = e
	return r, e
}
func (b *sqliteBatch) Query() (Rows, error) {
	q, e := b.next()
	if e != nil {
		return nil, e
	}
	r, e := b.tx.Query(b.ctx, q.SQL, q.Arguments...)
	b.err = e
	b.open = r
	return r, e
}
func (b *sqliteBatch) QueryRow() Row {
	r, e := b.Query()
	if e != nil {
		return errorRow{e}
	}
	return batchRow{sqliteRow{r}, b}
}

type batchRow struct {
	Row
	b *sqliteBatch
}

func (r batchRow) Scan(d ...any) error {
	e := r.Row.Scan(d...)
	if e != nil {
		r.b.err = e
	}
	return e
}
func (b *sqliteBatch) Close() error {
	if b.closed {
		return b.err
	}
	if b.open != nil {
		b.open.Close()
		b.open = nil
	}
	for b.err == nil && b.index < len(b.batch.QueuedQueries) {
		_, _ = b.Exec()
	}
	b.closed = true
	if b.owns {
		if b.err != nil {
			_ = b.tx.Rollback(b.ctx)
		} else {
			b.err = b.tx.Commit(b.ctx)
		}
	}
	return b.err
}

func (p *sqliteBackend) connections() (int32, int32) {
	s := p.db.Stats()
	return int32(s.InUse), int32(s.Idle)
}

type sqliteTxRows struct {
	Rows
	owner *sqliteTx
}

func (r *sqliteTxRows) Next() bool {
	ok := r.Rows.Next()
	if !ok {
		if err := r.Rows.Err(); err != nil {
			r.owner.failed = err
		}
	}
	return ok
}
func (r *sqliteTxRows) Err() error {
	err := r.Rows.Err()
	if err != nil {
		r.owner.failed = err
	}
	return err
}
