package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/control"
	"google.golang.org/grpc"
)

type application struct {
	ctx                             context.Context
	root, servicesDirectory         string
	serverBinary, jobsBinary        string
	routerAddress, managedAPITarget string
	pool                            *control.GeometryPool
	mu                              sync.Mutex
	apiTarget                       string
	apiProcess, jobsProcess         *control.ManagedProcess
	jobsPaused                      bool
}

func main() {
	if err := run(); err != nil {
		slog.Error("occccad control stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	envFile, err := config.LoadProjectEnv()
	if err != nil {
		return err
	}
	root, err := projectRoot(envFile)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app := &application{
		ctx: ctx, root: root, servicesDirectory: filepath.Join(root, "services"),
		serverBinary:     value("OCCCCAD_SERVER_BIN", filepath.Join(root, "build", "services", "occccad-server")),
		jobsBinary:       value("OCCCCAD_JOBS_BIN", filepath.Join(root, "build", "services", "occccad-jobs")),
		routerAddress:    value("OCCCCAD_GEOMETRY_ROUTER_LISTEN", "127.0.0.1:51001"),
		managedAPITarget: value("OCCCCAD_API_INTERNAL_LISTEN", "127.0.0.1:18080"),
	}
	app.apiTarget = app.managedAPITarget
	workerBinary := value("OCCCCAD_GEOMETRY_WORKER_BIN", filepath.Join(root, "build", "cmake",
		strings.ToLower(value("OCCCCAD_BUILD_TYPE", "Debug")), "workers", "geometry", "occccad_geometry_worker"))
	app.pool = control.NewGeometryPool(ctx, control.GeometryPoolConfig{
		WorkerBinary: workerBinary, WorkerHost: "127.0.0.1",
		FirstWorkerPort:  integer("OCCCCAD_GEOMETRY_WORKER_FIRST_PORT", 51100),
		MinimumWorkers:   integer("OCCCCAD_GEOMETRY_WORKER_MIN", 1),
		MaximumWorkers:   integer("OCCCCAD_GEOMETRY_WORKER_MAX", 8),
		GeometryCapacity: integer("OCCCCAD_GEOMETRY_PER_WORKER", 2),
		IdleTimeout:      duration("OCCCCAD_GEOMETRY_WORKER_IDLE", 5*time.Minute),
	})
	if err := app.pool.Start(); err != nil {
		return err
	}
	defer app.pool.Close()

	routerListener, err := net.Listen("tcp", app.routerAddress)
	if err != nil {
		return fmt.Errorf("listen geometry router: %w", err)
	}
	grpcServer := grpc.NewServer()
	workerv1.RegisterGeometryWorkerServer(grpcServer, app.pool)
	go func() {
		if err := grpcServer.Serve(routerListener); err != nil {
			slog.Error("geometry router stopped", "error", err)
			stop()
		}
	}()
	defer grpcServer.GracefulStop()

	if err := app.startManagedServices(); err != nil {
		return err
	}
	defer app.stopManagedServices()

	management := &http.Server{Addr: value("OCCCCAD_CONTROL_LISTEN", "127.0.0.1:19090"),
		Handler: app.managementHandler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := management.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("control API stopped", "error", err)
			stop()
		}
	}()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = management.Shutdown(shutdown)
	}()

	proxy := &http.Server{
		Addr:    value("OCCCCAD_APP_LISTEN", value("OCCCCAD_SERVER_LISTEN", "0.0.0.0:8080")),
		Handler: app.proxy(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 2 * time.Minute, WriteTimeout: 2 * time.Minute, IdleTimeout: 60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = proxy.Shutdown(shutdown)
	}()
	slog.Info("occccad application ready", "application", proxy.Addr, "control", management.Addr,
		"geometry_router", app.routerAddress, "api_target", app.managedAPITarget)
	err = proxy.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (app *application) startManagedServices() error {
	if err := app.startAPI(); err != nil {
		return err
	}
	if err := waitTCP(app.ctx, app.managedAPITarget, 20*time.Second); err != nil {
		return err
	}
	return app.startJobs()
}

func (app *application) startAPI() error {
	api, err := control.StartManagedProcess(app.ctx, "api", app.serverBinary, app.servicesDirectory, nil,
		withEnvironment("OCCCCAD_SERVER_LISTEN", app.managedAPITarget,
			"OCCCCAD_GEOMETRY_WORKER_ADDRESS", app.routerAddress))
	if err != nil {
		return err
	}
	app.mu.Lock()
	app.apiProcess = api
	app.mu.Unlock()
	go app.monitorAPI(api)
	return nil
}

func (app *application) startJobs() error {
	jobs, err := control.StartManagedProcess(app.ctx, "jobs", app.jobsBinary, app.servicesDirectory, nil,
		withEnvironment("OCCCCAD_GEOMETRY_WORKER_ADDRESS", app.routerAddress))
	if err != nil {
		return err
	}
	app.mu.Lock()
	app.jobsProcess = jobs
	app.mu.Unlock()
	go app.monitorJobs(jobs)
	return nil
}

func (app *application) monitorAPI(process *control.ManagedProcess) {
	<-process.Done()
	if app.ctx.Err() != nil {
		return
	}
	app.mu.Lock()
	current := app.apiProcess == process
	if current {
		app.apiProcess = nil
	}
	app.mu.Unlock()
	if !current {
		return
	}
	slog.Error("managed API exited; restarting", "error", process.Err())
	select {
	case <-app.ctx.Done():
		return
	case <-time.After(time.Second):
	}
	if err := app.startAPI(); err != nil {
		slog.Error("restart managed API", "error", err)
	}
}

func (app *application) monitorJobs(process *control.ManagedProcess) {
	<-process.Done()
	if app.ctx.Err() != nil {
		return
	}
	app.mu.Lock()
	current, paused := app.jobsProcess == process, app.jobsPaused
	if current {
		app.jobsProcess = nil
	}
	app.mu.Unlock()
	if !current || paused {
		return
	}
	slog.Error("managed Jobs exited; restarting", "error", process.Err())
	select {
	case <-app.ctx.Done():
		return
	case <-time.After(time.Second):
	}
	if err := app.startJobs(); err != nil {
		slog.Error("restart managed Jobs", "error", err)
	}
}

func (app *application) stopManagedServices() {
	app.mu.Lock()
	jobs, api := app.jobsProcess, app.apiProcess
	app.jobsProcess, app.apiProcess = nil, nil
	app.mu.Unlock()
	if jobs != nil {
		_ = jobs.Stop(5 * time.Second)
	}
	if api != nil {
		_ = api.Stop(5 * time.Second)
	}
}

func (app *application) proxy() http.Handler {
	return &httputil.ReverseProxy{
		Director: func(request *http.Request) {
			app.mu.Lock()
			target := app.apiTarget
			app.mu.Unlock()
			request.URL.Scheme = "http"
			request.URL.Host = target
			// Preserve the browser-facing Host header. The API compares it with
			// Origin during WebSocket Upgrade; replacing it with the internal
			// target would reject a legitimate same-origin connection.
		},
		ErrorHandler: func(writer http.ResponseWriter, _ *http.Request, err error) {
			writeJSON(writer, http.StatusBadGateway, map[string]any{
				"error": "selected API service is unavailable", "detail": err.Error()})
		},
	}
}

func (app *application) managementHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /control/status", func(writer http.ResponseWriter, _ *http.Request) {
		app.mu.Lock()
		target, paused, apiProcess, jobsProcess := app.apiTarget, app.jobsPaused, app.apiProcess, app.jobsProcess
		app.mu.Unlock()
		writeJSON(writer, http.StatusOK, map[string]any{"apiTarget": target,
			"managedApiTarget": app.managedAPITarget, "jobsPausedForDebug": paused,
			"services": map[string]any{"api": processStatus(apiProcess), "jobs": processStatus(jobsProcess)},
			"geometry": app.pool.Status()})
	})
	mux.HandleFunc("POST /control/debug/api", func(writer http.ResponseWriter, request *http.Request) {
		target, ok := decodeTarget(writer, request)
		if !ok {
			return
		}
		app.mu.Lock()
		app.apiTarget = target
		app.mu.Unlock()
		slog.Info("API debug override enabled", "target", target)
		writeJSON(writer, http.StatusOK, map[string]string{"target": target})
	})
	mux.HandleFunc("DELETE /control/debug/api", func(writer http.ResponseWriter, _ *http.Request) {
		app.mu.Lock()
		app.apiTarget = app.managedAPITarget
		app.mu.Unlock()
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /control/debug/geometry", func(writer http.ResponseWriter, request *http.Request) {
		target, ok := decodeTarget(writer, request)
		if !ok {
			return
		}
		if err := app.pool.SetDebugAddress(target); err != nil {
			writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(writer, http.StatusOK, map[string]string{"target": target})
	})
	mux.HandleFunc("DELETE /control/debug/geometry", func(writer http.ResponseWriter, _ *http.Request) {
		_ = app.pool.SetDebugAddress("")
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /control/debug/jobs", func(writer http.ResponseWriter, _ *http.Request) {
		app.mu.Lock()
		if app.jobsPaused {
			app.mu.Unlock()
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		process := app.jobsProcess
		app.jobsProcess, app.jobsPaused = nil, true
		app.mu.Unlock()
		if process != nil {
			_ = process.Stop(5 * time.Second)
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /control/debug/jobs", func(writer http.ResponseWriter, _ *http.Request) {
		app.mu.Lock()
		if !app.jobsPaused {
			app.mu.Unlock()
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		app.jobsPaused = false
		app.mu.Unlock()
		if err := app.startJobs(); err != nil {
			writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func processStatus(process *control.ManagedProcess) map[string]any {
	if process == nil {
		return map[string]any{"running": false, "pid": 0}
	}
	return map[string]any{"running": process.Running(), "pid": process.PID()}
}

func decodeTarget(writer http.ResponseWriter, request *http.Request) (string, bool) {
	var input struct {
		Target string `json:"target"`
	}
	if json.NewDecoder(request.Body).Decode(&input) != nil || strings.TrimSpace(input.Target) == "" {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "target is required"})
		return "", false
	}
	if _, _, err := net.SplitHostPort(input.Target); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "target must be host:port"})
		return "", false
	}
	return input.Target, true
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func withEnvironment(values ...string) []string {
	overrides := map[string]string{}
	for index := 0; index < len(values); index += 2 {
		overrides[values[index]] = values[index+1]
	}
	result := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[key]; !replaced {
			result = append(result, entry)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func waitTCP(ctx context.Context, address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		connection, err := net.DialTimeout("tcp", address, 300*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service %s did not become ready: %w", address, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func projectRoot(envFile string) (string, error) {
	if envFile != "" {
		return filepath.Dir(envFile), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if filepath.Base(cwd) == "services" {
		return filepath.Dir(cwd), nil
	}
	return cwd, nil
}

func value(name, fallback string) string {
	if result := strings.TrimSpace(os.Getenv(name)); result != "" {
		return result
	}
	return fallback
}
func integer(name string, fallback int) int {
	result, err := strconv.Atoi(value(name, ""))
	if err != nil || result < 1 {
		return fallback
	}
	return result
}
func duration(name string, fallback time.Duration) time.Duration {
	result, err := time.ParseDuration(value(name, ""))
	if err != nil || result <= 0 {
		return fallback
	}
	return result
}
