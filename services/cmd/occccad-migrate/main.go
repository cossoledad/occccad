package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/observability"
)

func main() {
	ctx := context.Background()
	shutdown, err := observability.Initialize(ctx, "occccad-migrate")
	if err != nil {
		slog.Error("initialize observability", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(context.Background()) }()
	pool, err := database.Open(ctx, config.Load().DatabaseURL)
	if err == nil {
		defer pool.Close()
		err = database.Migrate(ctx, pool)
	}
	if err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations are up to date")
}
