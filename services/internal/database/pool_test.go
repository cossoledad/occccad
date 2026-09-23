package database

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func testScheduler(concurrent, queued int) *scheduler {
	return newScheduler(Limits{Concurrent: concurrent, Queued: queued, BackgroundConcurrent: 1, BackgroundQueued: 1, WaitTimeout: time.Second})
}
func await(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !predicate() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for scheduler")
		}
		runtime.Gosched()
	}
}

func TestSchedulerBackpressureCancellationAndBackgroundIsolation(t *testing.T) {
	s := testScheduler(2, 0)
	bg, err := s.acquire(Background(t.Context()))
	if err != nil {
		t.Fatal(err)
	}
	defer bg()
	ctx, cancel := context.WithCancel(Background(t.Context()))
	outcome := make(chan error, 1)
	go func() {
		release, err := s.acquire(ctx)
		if err == nil {
			release()
		}
		outcome <- err
	}()
	await(t, func() bool { return s.waiting.Load() == 1 })
	if _, err := s.acquire(Background(t.Context())); !errors.Is(err, ErrBusy) {
		t.Fatalf("background budget not bounded: %v", err)
	}
	// Saturated background queue must not consume the foreground waiting budget.
	foreground, err := s.acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-outcome; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	foreground()
	foreground()
	bg()
	bg()
	if s.running.Load() != 0 || s.waiting.Load() != 0 {
		t.Fatal("leaked admission")
	}
	recovered, err := s.acquire(Background(t.Context()))
	if err != nil {
		t.Fatal(err)
	}
	recovered()
}
func TestSchedulerQueueTimeoutDoesNotCancelRunningOperation(t *testing.T) {
	s := testScheduler(1, 1)
	s.limits.WaitTimeout = 5 * time.Millisecond
	release, err := s.acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.acquire(t.Context()); !errors.Is(err, ErrBusy) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wrong timeout: %v", err)
	}
	if s.running.Load() != 1 {
		t.Fatal("timeout released another operation")
	}
	release()
}

type fakeBackend struct {
	poolBackend
	tx    pgx.Tx
	rows  pgx.Rows
	row   pgx.Row
	err   error
	calls atomic.Int32
}

func (f *fakeBackend) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	f.calls.Add(1)
	return f.tx, f.err
}
func (f *fakeBackend) Query(context.Context, string, ...any) (pgx.Rows, error) {
	f.calls.Add(1)
	return f.rows, f.err
}
func (f *fakeBackend) QueryRow(context.Context, string, ...any) pgx.Row { f.calls.Add(1); return f.row }

type fakeTx struct {
	pgx.Tx
	commits, rollbacks int
	commitErr          error
	child              pgx.Tx
}

func (f *fakeTx) Begin(context.Context) (pgx.Tx, error) { return f.child, nil }
func (f *fakeTx) Commit(context.Context) error          { f.commits++; return f.commitErr }
func (f *fakeTx) Rollback(context.Context) error        { f.rollbacks++; return nil }
func (f *fakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

type fakeRows struct {
	pgx.Rows
	remaining int
	closed    bool
	scanErr   error
}

func (f *fakeRows) Next() bool {
	if f.remaining > 0 {
		f.remaining--
		return true
	}
	f.closed = true
	return false
}
func (f *fakeRows) Close()            { f.closed = true }
func (f *fakeRows) Scan(...any) error { return f.scanErr }

func TestPoolTransactionRetainsOneSlotAndDoesNotRetryCommit(t *testing.T) {
	child := &fakeTx{}
	underlying := &fakeTx{child: child, commitErr: errors.New("commit acknowledgement lost")}
	backend := &fakeBackend{tx: underlying}
	p := &Pool{backend: backend, scheduler: testScheduler(1, 0)}
	tx, err := p.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Begin(t.Context()); !errors.Is(err, ErrBusy) {
		t.Fatalf("transaction released too early: %v", err)
	}
	if _, err = tx.Exec(t.Context(), "update"); err != nil {
		t.Fatal(err)
	}
	nested, err := tx.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err = nested.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if p.Snapshot().Active != 1 {
		t.Fatal("savepoint released outer slot")
	}
	if err = tx.Commit(t.Context()); err != underlying.commitErr {
		t.Fatalf("lost commit uncertainty: %v", err)
	}
	_ = tx.Rollback(t.Context())
	if underlying.commits != 1 || backend.calls.Load() != 1 || p.Snapshot().Active != 0 {
		t.Fatal("retried or leaked transaction")
	}
}
func TestPoolRowsReleaseOnDrainCloseAndScanFailure(t *testing.T) {
	for _, mode := range []string{"drain", "close", "scan-failure", "query-failure", "row-failure"} {
		t.Run(mode, func(t *testing.T) {
			rows := &fakeRows{remaining: 1}
			backend := &fakeBackend{rows: rows, row: errorRow{pgx.ErrNoRows}}
			p := &Pool{backend: backend, scheduler: testScheduler(1, 0)}
			switch mode {
			case "row-failure":
				if err := p.QueryRow(t.Context(), "select").Scan(); !errors.Is(err, pgx.ErrNoRows) {
					t.Fatal(err)
				}
			case "query-failure":
				backend.err = errors.New("network")
				if _, err := p.Query(t.Context(), "select"); err == nil {
					t.Fatal("missing error")
				}
			default:
				stream, err := p.Query(t.Context(), "select")
				if err != nil {
					t.Fatal(err)
				}
				if p.Snapshot().Active != 1 {
					t.Fatal("released unread rows")
				}
				switch mode {
				case "drain":
					for stream.Next() {
					}
				case "close":
					stream.Close()
				case "scan-failure":
					rows.scanErr = errors.New("bad value")
					_ = stream.Scan()
				}
				stream.Close()
			}
			if p.Snapshot().Active != 0 {
				t.Fatal("leaked result slot")
			}
		})
	}
}
