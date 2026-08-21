package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/exchange"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/thumbnail"
	"github.com/occccad/occccad/internal/workspace"
)

const maxExchangeUploadBytes int64 = 128 << 20

func exchangeFormat(value string) (string, string, string, bool) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "STEP", "STP":
		return "STEP", ".step", "application/step", true
	case "BREP", "BRP":
		return "BREP", ".brep", "application/vnd.opencascade.brep", true
	default:
		return "", "", "", false
	}
}

func (server *Server) exchangeCapabilities(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"formats": []string{"STEP", "BREP"}, "documentTypes": []string{"PART", "PRODUCT"},
		"maxUploadBytes": maxExchangeUploadBytes,
	})
}

// startExchangeImport streams the raw request body into ArtifactStore. It does
// not parse multipart data or retain the model in an API-process byte slice.
func (server *Server) startExchangeImport(writer http.ResponseWriter, request *http.Request) {
	format, _, contentType, ok := exchangeFormat(request.URL.Query().Get("format"))
	if !ok {
		writeError(writer, http.StatusBadRequest, "format must be STEP or BREP")
		return
	}
	if request.ContentLength == 0 {
		writeError(writer, http.StatusBadRequest, "exchange file is empty")
		return
	}
	if request.ContentLength > maxExchangeUploadBytes {
		writeError(writer, http.StatusRequestEntityTooLarge, "exchange file exceeds the 128 MiB limit")
		return
	}
	fileName := exchange.ImportedDocumentName(request.URL.Query().Get("fileName"))
	if fileName == "." || fileName == "" {
		writeError(writer, http.StatusBadRequest, "fileName is required")
		return
	}
	folderID := strings.TrimSpace(request.URL.Query().Get("folderId"))
	if folderID != "" {
		if _, err := server.access.RequireFolder(request.Context(), folderID, principal(request).ID, access.RoleEditor); err != nil {
			writeAccessError(writer, err)
			return
		}
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxExchangeUploadBytes)
	stored, err := server.artifacts.Put(request.Context(), artifact.KindExchangeSource, contentType, request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(writer, http.StatusRequestEntityTooLarge, "exchange file exceeds the 128 MiB limit")
			return
		}
		writeError(writer, http.StatusInternalServerError, "store exchange source: "+err.Error())
		return
	}
	if stored.Size == 0 {
		writeError(writer, http.StatusBadRequest, "exchange file is empty")
		return
	}
	requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
	job, err := server.jobs.Enqueue(request.Context(), jobs.EnqueueRequest{Type: "EXCHANGE_IMPORT",
		RequestedBy: principal(request).ID, InputObjectID: stored.ID, IdempotencyKey: requestID,
		UserVisible: true,
		Payload: map[string]any{"fileName": fileName, "folderId": folderID, "format": format,
			"requestId": requestID}})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusAccepted, job)
}

func (server *Server) startExchangeExport(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		DocumentID string `json:"documentId"`
		Format     string `json:"format"`
	}
	if !decodeJSON(writer, request, &input) {
		return
	}
	format, extension, _, ok := exchangeFormat(input.Format)
	if !ok {
		writeError(writer, http.StatusBadRequest, "format must be STEP or BREP")
		return
	}
	if _, err := server.access.RequireDocument(request.Context(), input.DocumentID, principal(request).ID, access.RoleViewer); err != nil {
		writeAccessError(writer, err)
		return
	}
	view, err := server.workspace.GetDocument(request.Context(), input.DocumentID)
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	idempotencyKey := strings.TrimSpace(request.Header.Get("X-Request-ID"))
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("export-%s-%s-%s", view.Document.ID, view.Document.VersionID, format)
	}
	job, err := server.jobs.Enqueue(request.Context(), jobs.EnqueueRequest{Type: "EXCHANGE_EXPORT",
		DocumentID: view.Document.ID, VersionID: &view.Document.VersionID,
		RequestedBy: principal(request).ID, IdempotencyKey: idempotencyKey,
		UserVisible: true,
		Payload:     map[string]any{"fileName": view.Document.Name + extension, "format": format, "requestId": idempotencyKey}})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusAccepted, job)
}

func (server *Server) listJobs(writer http.ResponseWriter, request *http.Request) {
	items, err := server.jobs.ListForUser(request.Context(), principal(request).ID, 100)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"jobs": items})
}

func (server *Server) getJob(writer http.ResponseWriter, request *http.Request) {
	job, ok := server.authorizedJob(writer, request)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, job)
}

func (server *Server) cancelJob(writer http.ResponseWriter, request *http.Request) {
	job, ok := server.ownedJob(writer, request)
	if !ok {
		return
	}
	actor := principal(request)
	updated, err := server.jobs.RequestCancel(request.Context(), job.ID, actor.ID, actor.PlatformRole == "ADMIN")
	if errors.Is(err, jobs.ErrNotFound) {
		writeError(writer, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, jobs.ErrNotCancelable) {
		writeError(writer, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, updated)
}

func (server *Server) retryJob(writer http.ResponseWriter, request *http.Request) {
	job, ok := server.ownedJob(writer, request)
	if !ok {
		return
	}
	actor := principal(request)
	updated, err := server.jobs.Retry(request.Context(), job.ID, actor.ID, actor.PlatformRole == "ADMIN")
	if errors.Is(err, jobs.ErrNotFound) {
		writeError(writer, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, jobs.ErrNotRetryable) {
		writeError(writer, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, updated)
}

func (server *Server) downloadJob(writer http.ResponseWriter, request *http.Request) {
	job, ok := server.authorizedJob(writer, request)
	if !ok {
		return
	}
	if job.State != "SUCCEEDED" || job.ResultObjectID == nil {
		writeError(writer, http.StatusConflict, "job result is not ready")
		return
	}
	object, reader, err := server.artifacts.Open(request.Context(), *job.ResultObjectID)
	if err != nil {
		writeError(writer, http.StatusNotFound, "job result is unavailable")
		return
	}
	defer reader.Close()
	writer.Header().Set("Content-Type", object.ContentType)
	writer.Header().Set("Content-Length", fmt.Sprint(object.Size))
	fileName := "occccad-export.bin"
	var payload struct {
		FileName string `json:"fileName"`
	}
	if json.Unmarshal(job.Payload, &payload) == nil && strings.TrimSpace(payload.FileName) != "" {
		fileName = filepath.Base(payload.FileName)
	}
	writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
	if _, err := io.Copy(writer, reader); err != nil {
		return
	}
}

func (server *Server) documentPreview(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleViewer); !ok {
		return
	}
	documentType := "PART"
	if err := server.database.QueryRow(request.Context(), `SELECT document_type
		FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, request.PathValue("documentID")).Scan(&documentType); err != nil {
		writeThumbnail(writer, thumbnail.DefaultForType(documentType), "default")
		return
	}
	var objectID string
	err := server.database.QueryRow(request.Context(), `SELECT p.object_id::text
		FROM occccad.document_previews p
		JOIN occccad.documents d ON d.id=p.document_id AND d.head_version_id=p.version_id
		WHERE p.document_id=$1 AND p.state='READY' AND p.renderer_version=$2
		ORDER BY p.updated_at DESC LIMIT 1`, request.PathValue("documentID"), thumbnail.RendererVersion).Scan(&objectID)
	if err != nil {
		writeThumbnail(writer, thumbnail.DefaultForType(documentType), "default")
		return
	}
	object, reader, err := server.artifacts.Open(request.Context(), objectID)
	if err != nil {
		writeThumbnail(writer, thumbnail.DefaultForType(documentType), "default")
		return
	}
	defer reader.Close()
	writer.Header().Set("Content-Type", object.ContentType)
	// A FOLLOW_HEAD Product can change when a referenced document changes even
	// though the Product's own version ID remains stable.
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-OCCCCAD-Thumbnail-State", "ready")
	_, _ = io.Copy(writer, reader)
}

func writeThumbnail(writer http.ResponseWriter, payload []byte, state string) {
	writer.Header().Set("Content-Type", "image/svg+xml")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-OCCCCAD-Thumbnail-State", state)
	writer.Header().Set("Content-Length", fmt.Sprint(len(payload)))
	_, _ = writer.Write(payload)
}

func (server *Server) authorizedJob(writer http.ResponseWriter, request *http.Request) (jobs.Job, bool) {
	job, err := server.jobs.Get(request.Context(), request.PathValue("jobID"))
	if errors.Is(err, jobs.ErrNotFound) {
		writeError(writer, http.StatusNotFound, err.Error())
		return jobs.Job{}, false
	}
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return jobs.Job{}, false
	}
	actor := principal(request)
	if actor.PlatformRole == "ADMIN" {
		return job, true
	}
	if job.DocumentID != nil {
		if _, err := server.access.RequireDocument(request.Context(), *job.DocumentID, actor.ID, access.RoleViewer); err == nil {
			return job, true
		}
	} else if job.RequestedBy == actor.ID {
		return job, true
	}
	writeError(writer, http.StatusForbidden, "job access denied")
	return jobs.Job{}, false
}

func (server *Server) ownedJob(writer http.ResponseWriter, request *http.Request) (jobs.Job, bool) {
	job, err := server.jobs.Get(request.Context(), request.PathValue("jobID"))
	if errors.Is(err, jobs.ErrNotFound) {
		writeError(writer, http.StatusNotFound, err.Error())
		return jobs.Job{}, false
	}
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return jobs.Job{}, false
	}
	actor := principal(request)
	if actor.PlatformRole == "ADMIN" || job.RequestedBy == actor.ID {
		return job, true
	}
	writeError(writer, http.StatusForbidden, "job action denied")
	return jobs.Job{}, false
}
