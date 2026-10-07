package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

func migrateSQLite(ctx context.Context, p *sqliteBackend) error {
	migrations, err := readMigrations("sqlite_migrations")
	if err != nil {
		return err
	}
	// BEGIN IMMEDIATE serializes migration runners across API/Jobs processes.
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return sqliteError(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version TEXT PRIMARY KEY,applied_at TEXT NOT NULL DEFAULT (now()),checksum TEXT NOT NULL,execution_ms INTEGER NOT NULL)`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return err
	}
	var applied []string
	for rows.Next() {
		var version string
		if err = rows.Scan(&version); err != nil {
			rows.Close()
			return err
		}
		applied = append(applied, version)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if err = validateAppliedMigrations(migrations, applied); err != nil {
		return err
	}
	for _, migration := range migrations {
		var existing string
		err = tx.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version=?`, migration.name).Scan(&existing)
		if err == nil {
			if existing != migration.checksum {
				return fmt.Errorf("SQLite migration %s checksum changed after application", migration.name)
			}
			continue
		}
		if !errors.Is(err, ErrNoRows) {
			return err
		}
		start := time.Now()
		if _, err = tx.ExecContext(ctx, migration.sql); err != nil {
			return fmt.Errorf("apply SQLite migration %s: %w", migration.name, err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,checksum,execution_ms) VALUES(?,?,?)`, migration.name, migration.checksum, time.Since(start).Milliseconds()); err != nil {
			return err
		}
	}
	return sqliteError(tx.Commit())
}

func (p *sqliteBackend) Migrate(ctx context.Context) error { return migrateSQLite(ctx, p) }
func (*sqliteBackend) ValidateDevelopmentReset() error     { return nil }

// ResetDevelopmentSchema clears the configured dedicated Local Mode database in
// place. Keep foreign_keys off only on this connection, outside the transaction.
func (p *sqliteBackend) ResetDevelopmentSchema(ctx context.Context) (string, error) {
	slog.Warn("clearing development SQLite database", "database_file", p.path)
	conn, err := p.db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return "", err
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "PRAGMA foreign_keys=ON") }()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT type, name FROM sqlite_schema WHERE type IN ('table','view') AND name NOT GLOB 'sqlite_*' ORDER BY type DESC`)
	if err != nil {
		return "", err
	}
	var statements []string
	for rows.Next() {
		var kind, name string
		if err = rows.Scan(&kind, &name); err != nil {
			rows.Close()
			return "", err
		}
		statements = append(statements, "DROP "+kind+` "`+strings.ReplaceAll(name, `"`, `""`)+`"`)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", err
	}
	for _, statement := range statements {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return p.path, nil
}
