package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/observability"
)

func main() {
	resetDevelopmentData := flag.Bool("reset-development-data", false,
		"clear the configured development database, local staging and ArtifactStore before migrating")
	flag.Parse()
	ctx := context.Background()
	shutdown, err := observability.Initialize(ctx, "occccad-migrate")
	if err != nil {
		slog.Error("initialize observability", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(context.Background()) }()
	if _, err := config.LoadProjectEnv(); err != nil {
		slog.Error("load environment", "error", err)
		os.Exit(1)
	}
	configuration := config.Load()
	err = migrateConfiguredDatabase(ctx, configuration, *resetDevelopmentData)
	if err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations are up to date")
}

// Close before deleting local storage: the SQLite database can live inside it.
func migrateConfiguredDatabase(ctx context.Context, configuration config.Config, reset bool) error {
	if reset && os.Getenv("OCCCCAD_ALLOW_DEV_RESET") != "1" {
		return errors.New("development reset requires OCCCCAD_ALLOW_DEV_RESET=1")
	}
	pool, err := database.Open(ctx, configuration.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { pool.Close() }()
	if reset {
		if err = database.ValidateDevelopmentReset(pool); err != nil {
			return err
		}
		directory, err := validateArtifactDirectory(configuration.DataDirectory)
		if err != nil {
			return err
		}
		// Resolve both targets before deleting either one.
		store, _, err := artifact.OpenConfigured(ctx, directory)
		if err != nil {
			return err
		}
		if store.Backend() != "LOCAL" {
			if _, ok := store.(artifact.DevelopmentResetter); !ok {
				return fmt.Errorf("artifact backend %s does not support development reset", store.Backend())
			}
		}
		if err = resetRemoteArtifacts(ctx, directory, store); err != nil {
			return err
		}
		target, err := database.ResetDevelopmentSchema(ctx, pool)
		if err != nil {
			return err
		}
		pool.Close()
		if err = resetArtifactDirectory(directory); err != nil {
			return err
		}
		reopened, err := database.Open(ctx, configuration.DatabaseURL)
		if err != nil {
			return err
		}
		pool = reopened
		slog.Warn("development data cleared", "database", target, "artifact_directory", directory)
	}
	return database.Migrate(ctx, pool)
}

func validateArtifactDirectory(configured string) (string, error) {
	if configured == "" {
		return "", errors.New("OCCCCAD_DATA_DIR must not be empty during development reset")
	}
	absolute, err := filepath.Abs(configured)
	if err != nil {
		return "", fmt.Errorf("resolve OCCCCAD_DATA_DIR: %w", err)
	}
	absolute = filepath.Clean(absolute)
	root := filepath.Clean(string(filepath.Separator))
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("read working directory: %w", err)
	}
	workingDirectory, err = filepath.Abs(workingDirectory)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	projectRoot := filepath.Dir(workingDirectory)
	if absolute == root || absolute == filepath.Clean(workingDirectory) || absolute == filepath.Clean(projectRoot) {
		return "", fmt.Errorf("refusing unsafe OCCCCAD_DATA_DIR %q", absolute)
	}
	if info, statErr := os.Lstat(absolute); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing symlink OCCCCAD_DATA_DIR %q", absolute)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", fmt.Errorf("inspect OCCCCAD_DATA_DIR: %w", statErr)
	}
	return absolute, nil
}

func resetArtifactDirectory(directory string) error {
	if err := os.RemoveAll(directory); err != nil {
		return fmt.Errorf("clear local ArtifactStore %q: %w", directory, err)
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return fmt.Errorf("recreate local ArtifactStore %q: %w", directory, err)
	}
	return nil
}

func resetConfiguredArtifacts(ctx context.Context, directory string) error {
	store, _, err := artifact.OpenConfigured(ctx, directory)
	if err != nil {
		return err
	}
	if err := resetRemoteArtifacts(ctx, directory, store); err != nil {
		return err
	}
	return resetArtifactDirectory(directory)
}

func resetRemoteArtifacts(ctx context.Context, directory string, store artifact.Store) error {
	slog.Warn("development reset targets", "local_directory", directory, "artifact_backend", store.Backend())
	if store.Backend() != "LOCAL" {
		resetter, ok := store.(artifact.DevelopmentResetter)
		if !ok {
			return fmt.Errorf("artifact backend %s does not support development reset", store.Backend())
		}
		slog.Warn("clearing development object storage", "target", resetter.DevelopmentResetTarget())
		if err := resetter.ResetDevelopment(ctx); err != nil {
			return err
		}
	}
	return nil
}
