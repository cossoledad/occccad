package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
)

func main() {
	ctx := context.Background()
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
