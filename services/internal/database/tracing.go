package database

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	perf "github.com/occccad/occccad/internal/performance"
)

// pgx tracing covers statements inside transactions too, without logging SQL,
// arguments or credentials. Batch duration includes draining its results.
type timingTracer struct{}
type queryTimingKey struct{}
type batchTimingKey struct{}

func (timingTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryTimingKey{}, perf.Start(ctx, "db-query"))
}
func (timingTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if finish, ok := ctx.Value(queryTimingKey{}).(func()); ok {
		finish()
	}
}
func (timingTracer) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	return context.WithValue(ctx, batchTimingKey{}, perf.Start(ctx, "db-batch"))
}
func (timingTracer) TraceBatchQuery(context.Context, *pgx.Conn, pgx.TraceBatchQueryData) {}
func (timingTracer) TraceBatchEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchEndData) {
	if finish, ok := ctx.Value(batchTimingKey{}).(func()); ok {
		finish()
	}
}

type acquireTimingKey struct{}

func (timingTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return context.WithValue(ctx, acquireTimingKey{}, perf.Start(ctx, "db-pool-wait"))
}
func (timingTracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	if finish, ok := ctx.Value(acquireTimingKey{}).(func()); ok {
		finish()
	}
}
