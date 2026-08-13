package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/observability"
)

func main() {
	resetDevelopmentData := flag.Bool("reset-development-data", false,
		"delete the occccad schema and local ArtifactStore before migrating")
	flag.Parse()
	ctx := context.Background()
	shutdown, err := observability.Initialize(ctx, "occccad-migrate")
	if err != nil {
		slog.Error("initialize observability", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(context.Background()) }()
	configuration := config.Load()
	pool, err := database.Open(ctx, configuration.DatabaseURL)
	if err == nil {
		defer pool.Close()
		if *resetDevelopmentData {
			if os.Getenv("OCCCCAD_ALLOW_DEV_RESET") != "1" {
				err = errors.New("development reset requires OCCCCAD_ALLOW_DEV_RESET=1")
			} else {
				var artifactDirectory string
				artifactDirectory, err = validateArtifactDirectory(configuration.DataDirectory)
				if err == nil {
					err = resetArtifactDirectory(artifactDirectory)
					if err == nil {
						var databaseName string
						databaseName, err = database.ResetDevelopmentSchema(ctx, pool)
						if err == nil {
							slog.Warn("development data cleared", "database", databaseName,
								"schema", "occcad", "artifact_directory", artifactDirectory)
						}
					}
				}
			}
		}
	}
	if err == nil {
		err = database.Migrate(ctx, pool)
	}
	if err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations are up to date")
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
