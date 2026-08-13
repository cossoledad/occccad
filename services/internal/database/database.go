package database

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = "occccad,public"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
	config.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "15000"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// ResetDevelopmentSchema removes every occccad-owned PostgreSQL object. It is
// intentionally separate from Migrate so callers must opt into the destructive
// development workflow before rebuilding the current schema baseline.
func ResetDevelopmentSchema(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	connection, err := pool.Acquire(ctx)
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
	if _, err := connection.Exec(ctx, `DROP SCHEMA IF EXISTS occccad CASCADE`); err != nil {
		return "", fmt.Errorf("drop development schema occccad: %w", err)
	}
	return databaseName, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	connection, err := pool.Acquire(ctx)
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
			checksum text,
			execution_ms bigint
		)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	if _, err := connection.Exec(ctx, `
		ALTER TABLE occccad.schema_migrations
		ADD COLUMN IF NOT EXISTS checksum text,
		ADD COLUMN IF NOT EXISTS execution_ms bigint`); err != nil {
		return fmt.Errorf("upgrade migration metadata: %w", err)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		sql, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		digest := sha256.Sum256(sql)
		checksum := hex.EncodeToString(digest[:])
		var storedChecksum *string
		err = connection.QueryRow(ctx,
			`SELECT checksum FROM occccad.schema_migrations WHERE version=$1`, entry.Name()).
			Scan(&storedChecksum)
		if err == nil {
			if storedChecksum == nil || *storedChecksum == "" {
				if _, err := connection.Exec(ctx,
					`UPDATE occccad.schema_migrations SET checksum=$1 WHERE version=$2`, checksum, entry.Name()); err != nil {
					return fmt.Errorf("baseline migration %s checksum: %w", entry.Name(), err)
				}
			} else if *storedChecksum != checksum {
				return fmt.Errorf("migration %s checksum changed after it was applied", entry.Name())
			}
			continue
		}
		if err != pgx.ErrNoRows {
			return fmt.Errorf("check migration %s: %w", entry.Name(), err)
		}
		started := time.Now()
		tx, err := connection.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(sql)); err == nil {
			_, err = tx.Exec(ctx, `
				INSERT INTO occccad.schema_migrations(version,checksum,execution_ms)
				VALUES($1,$2,$3)`, entry.Name(), checksum, time.Since(started).Milliseconds())
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}
