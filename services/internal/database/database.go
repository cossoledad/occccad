package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, databaseURL string) (*Pool, error) {
	if strings.HasPrefix(databaseURL, "sqlite:") {
		return openSQLite(ctx, databaseURL)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = "occccad,public"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
	config.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "15000"
	limits, err := schedulingLimits(int(config.MaxConns))
	if err != nil {
		return nil, err
	}
	if limits.Concurrent > int(config.MaxConns) {
		return nil, fmt.Errorf("database concurrency exceeds pool_max_conns")
	}
	config.ConnConfig.Tracer = timingTracer{}
	rawPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	pool := &Pool{backend: &postgresBackend{Pool: rawPool}, scheduler: newScheduler(limits)}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// ResetDevelopmentSchema clears the configured backend development schema. It is
// intentionally separate from Migrate so callers must opt into the destructive
// development workflow before rebuilding the current schema baseline.
func ResetDevelopmentSchema(ctx context.Context, pool *Pool) (string, error) {
	return pool.backend.ResetDevelopmentSchema(ctx)
}
func (backend *postgresBackend) ResetDevelopmentSchema(ctx context.Context) (string, error) {
	connection, err := backend.Pool.Acquire(ctx)
	if err != nil {
		return "", fmt.Errorf("acquire development reset connection: %w", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock(hashtext('occccad.schema_migrations'))`); err != nil {
		return "", fmt.Errorf("acquire development reset lock: %w", err)
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('occccad.schema_migrations'))`)
	}()
	var databaseName string
	if err := connection.QueryRow(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil {
		return "", fmt.Errorf("read development database name: %w", err)
	}
	slog.Warn("clearing development PostgreSQL schema", "database", databaseName, "schema", "occccad")
	if _, err := connection.Exec(ctx, `DROP SCHEMA IF EXISTS occccad CASCADE`); err != nil {
		return "", fmt.Errorf("drop development schema occccad: %w", err)
	}
	return databaseName, nil
}

func Migrate(ctx context.Context, pool *Pool) error { return pool.backend.Migrate(ctx) }
func (backend *postgresBackend) Migrate(ctx context.Context) error {
	migrations, err := readMigrations("postgres_migrations")
	if err != nil {
		return err
	}
	connection, err := backend.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock(hashtext('occccad.schema_migrations'))`); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('occccad.schema_migrations'))`)
	}()
	if _, err := connection.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS occccad`); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if _, err := connection.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS occccad.schema_migrations (
			version text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now(),
			checksum text NOT NULL,
			execution_ms bigint NOT NULL
		)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	rows, err := connection.Query(ctx, `SELECT version FROM occccad.schema_migrations ORDER BY version`)
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}
	applied, err := CollectRows(rows, func(row CollectableRow) (string, error) {
		var version string
		err := row.Scan(&version)
		return version, err
	})
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}
	if err := validateAppliedMigrations(migrations, applied); err != nil {
		return err
	}
	for _, migration := range migrations {
		var storedChecksum *string
		err = connection.QueryRow(ctx,
			`SELECT checksum FROM occccad.schema_migrations WHERE version=$1`, migration.name).
			Scan(&storedChecksum)
		if err == nil {
			if storedChecksum == nil || *storedChecksum != migration.checksum {
				return fmt.Errorf("migration %s checksum changed after it was applied", migration.name)
			}
			continue
		}
		if err != pgx.ErrNoRows {
			return fmt.Errorf("check migration %s: %w", migration.name, err)
		}
		started := time.Now()
		tx, err := connection.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, migration.sql); err == nil {
			_, err = tx.Exec(ctx, `
				INSERT INTO occccad.schema_migrations(version,checksum,execution_ms)
				VALUES($1,$2,$3)`, migration.name, migration.checksum, time.Since(started).Milliseconds())
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", migration.name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", migration.name, err)
		}
	}
	return nil
}

func schedulingLimits(capacity int) (Limits, error) {
	concurrent := capacity
	queued := 64
	background := max(1, concurrent/4)
	backgroundQueued := 16
	wait := 2000
	for name, target := range map[string]*int{"OCCCCAD_DB_CONCURRENCY": &concurrent, "OCCCCAD_DB_QUEUE_CAPACITY": &queued, "OCCCCAD_DB_BACKGROUND_CONCURRENCY": &background, "OCCCCAD_DB_BACKGROUND_QUEUE_CAPACITY": &backgroundQueued, "OCCCCAD_DB_QUEUE_TIMEOUT_MS": &wait} {
		if raw := os.Getenv(name); raw != "" {
			value, parseErr := strconv.Atoi(raw)
			if parseErr != nil {
				return Limits{}, fmt.Errorf("invalid %s", name)
			}
			*target = value
		}
	}
	if os.Getenv("OCCCCAD_DB_BACKGROUND_CONCURRENCY") == "" {
		background = max(1, concurrent/4)
	}
	limits := Limits{Concurrent: concurrent, Queued: queued, BackgroundConcurrent: background, BackgroundQueued: backgroundQueued, WaitTimeout: time.Duration(wait) * time.Millisecond}
	if err := limits.validate(); err != nil {
		return Limits{}, err
	}

	return limits, nil
}

// ValidateDevelopmentReset must run before any ArtifactStore deletion. The
// configured backend must support the destructive development workflow.
func ValidateDevelopmentReset(pool *Pool) error          { return pool.backend.ValidateDevelopmentReset() }
func (*postgresBackend) ValidateDevelopmentReset() error { return nil }
