package main

import (
	"fmt"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateArtifactDirectoryRejectsUnsafeTargets(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{string(filepath.Separator), workingDirectory, filepath.Dir(workingDirectory)} {
		if _, err := validateArtifactDirectory(target); err == nil {
			t.Fatalf("validateArtifactDirectory(%q) succeeded", target)
		}
	}
}

func TestValidateArtifactDirectoryRejectsSymlink(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "artifacts")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := validateArtifactDirectory(link); err == nil {
		t.Fatal("validateArtifactDirectory accepted a symlink")
	}
}

func TestResetArtifactDirectoryClearsAndRecreatesTarget(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "artifacts")
	if err := os.MkdirAll(filepath.Join(directory, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "nested", "object"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resetArtifactDirectory(directory); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("artifact directory contains %d entries after reset", len(entries))
	}
}

func TestResetConfiguredArtifactsLocalAndInvalidBackend(t *testing.T) {
	for _, backend := range []string{"LOCAL", "UNKNOWN"} {
		t.Run(backend, func(t *testing.T) {
			t.Setenv("OCCCCAD_ARTIFACT_BACKEND", backend)
			directory := t.TempDir()
			path := filepath.Join(directory, "retained-until-backend-valid")
			if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
			err := resetConfiguredArtifacts(t.Context(), directory)
			if backend == "UNKNOWN" {
				if err == nil {
					t.Fatal("unknown backend accepted")
				}
				if _, err := os.Stat(path); err != nil {
					t.Fatal("local data deleted before backend validation", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("local data retained", err)
				}
			}
		})
	}
}

func TestSQLiteDevelopmentReset(t *testing.T) {
	for _, inside := range []bool{false, true} {
		t.Run(fmt.Sprintf("databaseInsideArtifacts=%v", inside), func(t *testing.T) {
			t.Setenv("OCCCCAD_ALLOW_DEV_RESET", "1")
			t.Setenv("OCCCCAD_ARTIFACT_BACKEND", "LOCAL")
			root := t.TempDir()
			directory := filepath.Join(root, "artifacts")
			path := filepath.Join(root, "local.db")
			if inside {
				path = filepath.Join(directory, "database", "local.db")
			}
			cfg := config.Config{DatabaseURL: "sqlite:" + path, DataDirectory: directory}
			ctx := t.Context()
			if err := migrateConfiguredDatabase(ctx, cfg, false); err != nil {
				t.Fatal(err)
			}
			pool, err := database.Open(ctx, cfg.DatabaseURL)
			if err != nil {
				t.Fatal(err)
			}
			_, err = pool.Exec(ctx, `INSERT INTO occccad.account_audit_events(action) VALUES('before reset')`)
			pool.Close()
			if err != nil {
				t.Fatal(err)
			}
			if err = os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(directory, "old-artifact")
			if err = os.WriteFile(marker, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			unrelated := filepath.Join(root, "unrelated.db")
			if err = os.WriteFile(unrelated, []byte("retain"), 0600); err != nil {
				t.Fatal(err)
			}
			// Invalid configuration and missing authorization must preserve both stores.
			t.Setenv("OCCCCAD_ARTIFACT_BACKEND", "UNKNOWN")
			if err = migrateConfiguredDatabase(ctx, cfg, true); err == nil {
				t.Fatal("accepted unknown backend")
			}
			t.Setenv("OCCCCAD_ARTIFACT_BACKEND", "LOCAL")
			t.Setenv("OCCCCAD_ALLOW_DEV_RESET", "0")
			if err = migrateConfiguredDatabase(ctx, cfg, true); err == nil {
				t.Fatal("accepted unauthorized reset")
			}
			if _, err = os.Stat(marker); err != nil {
				t.Fatal(err)
			}
			pool, err = database.Open(ctx, cfg.DatabaseURL)
			if err != nil {
				t.Fatal(err)
			}
			var count int
			err = pool.QueryRow(ctx, `SELECT count(*) FROM occccad.account_audit_events`).Scan(&count)
			pool.Close()
			if err != nil || count != 1 {
				t.Fatalf("preflight deleted data: %d %v", count, err)
			}
			t.Setenv("OCCCCAD_ALLOW_DEV_RESET", "1")
			for range 2 { // Reset remains repeatable after successful reconstruction.
				if err = migrateConfiguredDatabase(ctx, cfg, true); err != nil {
					t.Fatal(err)
				}
			}
			pool, err = database.Open(ctx, cfg.DatabaseURL)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM occccad.account_audit_events`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("old data: %d %v", count, err)
			}
			if err = pool.QueryRow(ctx, `PRAGMA foreign_keys`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("foreign keys: %d %v", count, err)
			}
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil || count == 0 {
				t.Fatalf("migrations: %d %v", count, err)
			}
			if _, err = os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("artifact retained: %v", err)
			}
			if content, err := os.ReadFile(unrelated); err != nil || string(content) != "retain" {
				t.Fatalf("unrelated file changed: %v", err)
			}
			// Database remains writable and its sequence was rebuilt.
			var id int
			if err = pool.QueryRow(ctx, `INSERT INTO occccad.account_audit_events(action) VALUES('after reset') RETURNING id`).Scan(&id); err != nil || id != 1 {
				t.Fatalf("write after reset: %d %v", id, err)
			}
		})
	}
}
