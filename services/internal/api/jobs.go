package api

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/workspace"
)

func (server *Server) startExportStep(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleViewer); !ok {
		return
	}
	view, err := server.workspace.GetDocument(request.Context(), request.PathValue("documentID"))
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	idempotencyKey := strings.TrimSpace(request.Header.Get("X-Request-ID"))
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("export-%s-%s", view.Document.ID, view.Document.VersionID)
	}
	job, err := server.jobs.Enqueue(request.Context(), jobs.EnqueueRequest{Type: "STEP_EXPORT",
		DocumentID: view.Document.ID, VersionID: &view.Document.VersionID,
		RequestedBy: principal(request).ID, IdempotencyKey: idempotencyKey,
		Payload: map[string]any{"fileName": view.Document.Name + ".step", "requestId": idempotencyKey}})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusAccepted, job)
}

func (server *Server) listJobs(writer http.ResponseWriter, request *http.Request) {
	items, err := server.jobs.ListForUser(request.Context(), principal(request).ID, 30)
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
	writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment",
		map[string]string{"filename": "occccad-export.step"}))
	if _, err := io.Copy(writer, reader); err != nil {
		return
	}
}

func (server *Server) documentPreview(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleViewer); !ok {
		return
	}
	var objectID string
	err := server.database.QueryRow(request.Context(), `SELECT p.object_id::text
		FROM occccad.document_previews p
		JOIN occccad.documents d ON d.id=p.document_id AND d.head_version_id=p.version_id
		WHERE p.document_id=$1 AND p.state='READY'
		ORDER BY p.updated_at DESC LIMIT 1`, request.PathValue("documentID")).Scan(&objectID)
	if err != nil {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	object, reader, err := server.artifacts.Open(request.Context(), objectID)
	if err != nil {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	defer reader.Close()
	writer.Header().Set("Content-Type", object.ContentType)
	// A FOLLOW_HEAD Product can change when a referenced document changes even
	// though the Product's own version ID remains stable.
	writer.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(writer, reader)
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
	if actor.PlatformRole == "ADMIN" || job.RequestedBy == actor.ID {
		return job, true
	}
	if job.DocumentID != nil {
		if _, err := server.access.RequireDocument(request.Context(), *job.DocumentID, actor.ID, access.RoleViewer); err == nil {
			return job, true
		}
	}
	writeError(writer, http.StatusForbidden, "job access denied")
	return jobs.Job{}, false
}
