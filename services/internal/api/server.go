package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/occccad/occccad/internal/demo"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
	"go.opentelemetry.io/otel/trace"
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
	mux.HandleFunc("GET /api/documents/{documentID}/history", server.getDocumentHistory)
	mux.HandleFunc("POST /api/documents/{documentID}/versions", server.createVersion)
	mux.HandleFunc("POST /api/documents/{documentID}/commands", server.applyCommand)
	mux.HandleFunc("POST /api/documents/{documentID}/import-step", server.importStep)
	mux.HandleFunc("GET /api/documents/{documentID}/export-step", server.exportStep)
	mux.HandleFunc("/api/", func(writer http.ResponseWriter, _ *http.Request) {
		writeError(writer, http.StatusNotFound, "API route not found")
	})
	mux.Handle("/", server.staticHandler())
	return server.middleware(mux)
}

func (server *Server) createVersion(writer http.ResponseWriter, request *http.Request) {
	var input workspace.CreateVersionRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	history, err := server.workspace.CreateVersion(
		request.Context(), request.PathValue("documentID"), input)
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"workspace": "Main", "history": history})
}

func (server *Server) getDocumentHistory(writer http.ResponseWriter, request *http.Request) {
	history, err := server.workspace.ListHistory(request.Context(), request.PathValue("documentID"))
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"workspace": "Main", "history": history})
}

func (server *Server) importStep(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 64<<20)
	if err := request.ParseMultipartForm(8 << 20); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid STEP upload: "+err.Error())
		return
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		writeError(writer, http.StatusBadRequest, "STEP file is required")
		return
	}
	defer file.Close()
	extension := strings.ToLower(filepath.Ext(header.Filename))
	if extension != ".step" && extension != ".stp" {
		writeError(writer, http.StatusBadRequest, "file extension must be .step or .stp")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, (64<<20)+1))
	if err != nil || len(data) > 64<<20 {
		writeError(writer, http.StatusBadRequest, "STEP file exceeds the 64 MiB limit")
		return
	}
	requestID := request.FormValue("requestId")
	if requestID == "" {
		requestID = request.Header.Get("X-Request-ID")
	}
	result, err := server.workspace.ImportStep(
		request.Context(), request.PathValue("documentID"), requestID,
		filepath.Base(header.Filename), data)
	writeWorkspaceResult(writer, result, err)
}

func (server *Server) exportStep(writer http.ResponseWriter, request *http.Request) {
	data, fileName, err := server.workspace.ExportStep(
		request.Context(), request.PathValue("documentID"), request.Header.Get("X-Request-ID"))
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	writer.Header().Set("Content-Type", "application/step")
	writer.Header().Set("Content-Disposition", mime.FormatMediaType(
		"attachment", map[string]string{"filename": fileName}))
	writer.Header().Set("Content-Length", fmt.Sprint(len(data)))
	writer.WriteHeader(http.StatusOK)
	if _, err := writer.Write(data); err != nil {
		slog.Error("write STEP response", "error", err)
	}
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
		started := time.Now()
		requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if requestID == "" {
			buffer := make([]byte, 12)
			if _, err := rand.Read(buffer); err == nil {
				requestID = hex.EncodeToString(buffer)
			}
		}
		writer.Header().Set("X-Request-ID", requestID)
		spanContext := trace.SpanContextFromContext(request.Context())
		if spanContext.IsValid() {
			writer.Header().Set("Trace-ID", spanContext.TraceID().String())
		}
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID, Traceparent, Tracestate")
		writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		slog.InfoContext(request.Context(), "http request",
			"request_id", requestID,
			"trace_id", spanContext.TraceID().String(),
			"method", request.Method,
			"route", request.Pattern,
			"path", request.URL.Path,
			"status", recorder.status,
			"bytes", recorder.bytes,
			"duration_ms", time.Since(started).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(data []byte) (int, error) {
	written, err := recorder.ResponseWriter.Write(data)
	recorder.bytes += written
	return written, err
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
