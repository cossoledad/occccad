package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/api"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/authn"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/observability"
	"github.com/occccad/occccad/internal/workspace"
)

func main() {
	if err := run(); err != nil {
		slog.Error("occccad server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configuration := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTracing, err := observability.Initialize(ctx, "occccad-server")
	if err != nil {
		return err
	}
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownContext); err != nil {
			slog.Error("flush tracing", "error", err)
		}
	}()

	pool, err := database.Open(ctx, configuration.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		return err
	}
	if configuration.AdminPassword == "" {
		return errors.New("OCCCCAD_ADMIN_PASSWORD must be set")
	}
	authenticationService := authn.New(pool, configuration.SessionTTL)
	if err := authenticationService.BootstrapAdmin(ctx, configuration.AdminEmail,
		configuration.AdminName, configuration.AdminPassword); err != nil {
		return err
	}
	localArtifactStore, err := artifact.NewLocalStore(configuration.DataDirectory)
	if err != nil {
		return err
	}
	artifactService := artifact.NewService(pool, localArtifactStore)
	jobService := jobs.New(pool)

	worker, err := geometry.Open(configuration.WorkerAddress)
	if err != nil {
		return err
	}
	defer func() { _ = worker.Close() }()

	workspaceService := workspace.NewWithArtifacts(pool, worker, artifactService)
	accessService := access.New(pool)
	apiServer := api.New(
		pool, worker, workspaceService, accessService, authenticationService,
		artifactService, jobService, configuration.SecureCookies, configuration.AllowedOrigins)
	defer apiServer.Close()
	httpServer := &http.Server{
		Addr:              configuration.ListenAddress,
		Handler:           observability.HTTPHandler(apiServer.Handler()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownContext)
	}()

	slog.Info("occccad server listening",
		"address", configuration.ListenAddress,
		"worker", configuration.WorkerAddress,
		"artifact_backend", "LOCAL",
		"data_directory", localArtifactStore.Root())
	err = httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
