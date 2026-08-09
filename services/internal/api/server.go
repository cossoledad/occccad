package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/occccad/occccad/internal/demo"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

type Server struct {
	database     *pgxpool.Pool
	worker       *geometry.Client
	demo         *demo.Service
	workspace    *workspace.Service
	webDirectory string
}

func New(
	database *pgxpool.Pool,
	worker *geometry.Client,
	demoService *demo.Service,
	workspaceService *workspace.Service,
	webDirectory string,
) *Server {
	return &Server{
		database: database, worker: worker, demo: demoService,
		workspace: workspaceService, webDirectory: webDirectory,
	}
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", server.health)
	mux.HandleFunc("POST /api/demo/seed", server.seed)
	mux.HandleFunc("GET /api/demo", server.seed)
	mux.HandleFunc("GET /api/documents", server.listDocuments)
	mux.HandleFunc("POST /api/documents", server.createDocument)
	mux.HandleFunc("GET /api/documents/{documentID}", server.getDocument)
	mux.HandleFunc("POST /api/documents/{documentID}/commands", server.applyCommand)
	mux.HandleFunc("/api/", func(writer http.ResponseWriter, _ *http.Request) {
		writeError(writer, http.StatusNotFound, "API route not found")
	})
	mux.Handle("/", server.staticHandler())
	return server.middleware(mux)
}

func (server *Server) listDocuments(writer http.ResponseWriter, request *http.Request) {
	documents, err := server.workspace.ListDocuments(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"documents": documents})
}

func (server *Server) createDocument(writer http.ResponseWriter, request *http.Request) {
	var input workspace.CreateDocumentRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	result, err := server.workspace.CreateDocument(request.Context(), input)
	writeWorkspaceResult(writer, result, err)
}

func (server *Server) getDocument(writer http.ResponseWriter, request *http.Request) {
	result, err := server.workspace.GetDocument(request.Context(), request.PathValue("documentID"))
	writeWorkspaceResult(writer, result, err)
}

func (server *Server) applyCommand(writer http.ResponseWriter, request *http.Request) {
	var input workspace.CommandRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	result, err := server.workspace.ApplyCommand(
		request.Context(), request.PathValue("documentID"), input)
	writeWorkspaceResult(writer, result, err)
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, value any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid JSON request: "+err.Error())
		return false
	}
	return true
}

func writeWorkspaceResult(writer http.ResponseWriter, result workspace.DocumentView, err error) {
	if err == nil {
		writeJSON(writer, http.StatusOK, result)
		return
	}
	if errors.Is(err, workspace.ErrNotFound) {
		writeError(writer, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, workspace.ErrValidation) {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	slog.Error("workspace request failed", "error", err)
	writeError(writer, http.StatusInternalServerError, err.Error())
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
