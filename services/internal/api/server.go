package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/occccad/occccad/internal/demo"
	"github.com/occccad/occccad/internal/geometry"
)

type Server struct {
	database     *pgxpool.Pool
	worker       *geometry.Client
	demo         *demo.Service
	webDirectory string
}

func New(
	database *pgxpool.Pool,
	worker *geometry.Client,
	demoService *demo.Service,
	webDirectory string,
) *Server {
	return &Server{
		database: database, worker: worker, demo: demoService, webDirectory: webDirectory,
	}
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", server.health)
	mux.HandleFunc("POST /api/demo/seed", server.seed)
	mux.HandleFunc("GET /api/demo", server.seed)
	mux.HandleFunc("/api/", func(writer http.ResponseWriter, _ *http.Request) {
		writeError(writer, http.StatusNotFound, "API route not found")
	})
	mux.Handle("/", server.staticHandler())
	return server.middleware(mux)
}

func (server *Server) health(writer http.ResponseWriter, request *http.Request) {
	if err := server.database.Ping(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database unavailable: "+err.Error())
		return
	}
	worker, err := server.worker.Ping(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "geometry worker unavailable: "+err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"status": "ok", "database": "ok", "workerId": worker.GetWorkerId(),
		"occtVersion":           worker.GetOcctVersion(),
		"residentGeometryCount": worker.GetResidentGeometryCount(),
	})
}

func (server *Server) seed(writer http.ResponseWriter, request *http.Request) {
	result, err := server.demo.Seed(request.Context())
	if err != nil {
		slog.Error("Demo 01 seed failed", "error", err)
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(writer, request)
	})
}

func (server *Server) staticHandler() http.Handler {
	root := filepath.Clean(server.webDirectory)
	fileServer := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path := filepath.Join(root, filepath.Clean(request.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(writer, request)
			return
		}
		if strings.Contains(request.URL.Path, ".") {
			http.NotFound(writer, request)
			return
		}
		http.ServeFile(writer, request, filepath.Join(root, "index.html"))
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		slog.Error("encode HTTP response", "error", err)
	}
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}
