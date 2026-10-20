package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

func (server *Server) registerAutomationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/automation/operations", func(w http.ResponseWriter, r *http.Request) {
		items, err := server.automation.Catalog.Search(r.URL.Query().Get("q"), r.URL.Query().Get("documentType"), 20)
		if err != nil {
			writeAutomationResult(w, nil, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"operations": items, "catalogDigest": server.automation.Catalog.Digest})
	})
	mux.HandleFunc("GET /api/automation/operations/{operationID}", func(w http.ResponseWriter, r *http.Request) {
		item, err := server.automation.Catalog.Describe(r.PathValue("operationID"))
		writeAutomationResult(w, item, err)
	})
	mux.HandleFunc("GET /api/documents/{documentID}/automation/context", func(w http.ResponseWriter, r *http.Request) {
		value, err := server.automation.Context(r.Context(), principal(r).ID, r.PathValue("documentID"))
		writeAutomationResult(w, value, err)
	})
	mux.HandleFunc("POST /api/documents/{documentID}/automation/query", func(w http.ResponseWriter, r *http.Request) {
		var input automation.Query
		if !decodeAutomationJSON(w, r, &input) {
			return
		}
		value, err := server.automation.Query(r.Context(), principal(r).ID, r.PathValue("documentID"), input)
		writeAutomationResult(w, value, err)
	})
	mux.HandleFunc("POST /api/documents/{documentID}/automation/tasks", func(w http.ResponseWriter, r *http.Request) {
		var input automation.CreateTask
		if !decodeAutomationJSON(w, r, &input) {
			return
		}
		value, err := server.automation.CreateTask(r.Context(), principal(r).ID, r.PathValue("documentID"), input)
		writeAutomationResult(w, value, err)
	})
	mux.HandleFunc("GET /api/documents/{documentID}/automation/tasks", func(w http.ResponseWriter, r *http.Request) {
		value, err := server.automation.ListTasks(r.Context(), principal(r).ID, r.PathValue("documentID"))
		for i := range value {
			decorateAutomationTask(&value[i])
		}
		writeAutomationResult(w, map[string]any{"tasks": value}, err)
	})
	mux.HandleFunc("GET /api/documents/{documentID}/automation/tasks/{taskID}", func(w http.ResponseWriter, r *http.Request) {
		value, err := server.automation.GetTask(r.Context(), principal(r).ID, r.PathValue("documentID"), r.PathValue("taskID"))
		decorateAutomationTask(&value)
		writeAutomationResult(w, value, err)
	})
}

func decorateAutomationTask(task *automation.Task) {
	for i := range task.Steps {
		if task.Steps[i].Preview != nil {
			automationPreviewURLs(task.DocumentID, task.Steps[i].Preview)
		}
	}
}

func decodeAutomationJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeError(w, http.StatusBadRequest, "invalid automation request")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeError(w, http.StatusBadRequest, "trailing JSON is not allowed")
		return false
	}
	return true
}
func writeAutomationResult(w http.ResponseWriter, value any, err error) {
	w.Header().Set("Cache-Control", "no-store")
	if err == nil {
		writeJSON(w, http.StatusOK, value)
		return
	}
	if writeDatabaseBusy(w, err) {
		return
	}
	status := http.StatusBadRequest
	code := "VALIDATION_FAILED"
	retryable := false
	var f *automation.Fault
	switch {
	case errors.As(err, &f):
		code = f.Code
		retryable = f.Retryable
		if code == "TASK_NOT_FOUND" || code == "OPERATION_UNSUPPORTED" {
			status = http.StatusNotFound
		}
		if code == "TASK_VERSION_CONFLICT" || code == "WORKSPACE_HEAD_CONFLICT" {
			status = http.StatusConflict
		}
		if code == "AUTOMATION_BUSY" {
			status = http.StatusTooManyRequests
		}
	case errors.Is(err, access.ErrForbidden):
		status = http.StatusForbidden
		code = "FORBIDDEN"
	case errors.Is(err, access.ErrUnauthorized):
		status = http.StatusUnauthorized
		code = "UNAUTHORIZED"
	case errors.Is(err, access.ErrNotFound) || errors.Is(err, workspace.ErrNotFound):
		status = http.StatusNotFound
		code = "NOT_FOUND"
	case errors.Is(err, workspace.ErrValidation):
		status = http.StatusBadRequest
	default:
		status = http.StatusInternalServerError
		code = "EXECUTION_FAILURE"
	}
	body := map[string]any{"error": err.Error(), "code": code, "retryable": retryable}
	if f != nil {
		if f.Phase != "" {
			body["phase"] = f.Phase
		}
		if f.DiagnosticID != "" {
			body["diagnosticId"] = f.DiagnosticID
		}
	}
	writeJSON(w, status, body)
}

type automationRequest struct {
	DocumentID      string                   `json:"documentId"`
	Method          string                   `json:"method"`
	Action          automation.ActionRequest `json:"action,omitempty"`
	TaskID          string                   `json:"taskId,omitempty"`
	ExpectedVersion uint64                   `json:"expectedVersion,omitempty"`
	Control         automation.Control       `json:"control,omitempty"`
}

func (server *Server) handleAutomationRequest(ctx context.Context, client *realtimeClient, envelope realtimeEnvelope) {
	decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
	decoder.DisallowUnknownFields()
	var input automationRequest
	if len(envelope.Payload) > 64<<10 || decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.DocumentID == "" {
		client.sendError(envelope.ID, "INVALID_PAYLOAD", "invalid bounded automation request", false)
		return
	}
	var result any
	var err error
	switch input.Method {
	case "action.prepare":
		var preview workspace.CommandPreview
		preview, err = server.automation.Prepare(ctx, client.actor.ID, input.DocumentID, input.Action)
		if err == nil {
			automationPreviewURLs(input.DocumentID, &preview)
		}
		result = preview
	case "action.commit":
		result, err = server.automation.Commit(ctx, client.actor.ID, input.DocumentID, input.Action)
	case "task.advance":
		var task automation.Task
		task, err = server.automation.Advance(ctx, client.actor.ID, input.DocumentID, input.TaskID, input.ExpectedVersion)
		if err == nil {
			for i := range task.Steps {
				if task.Steps[i].Preview != nil {
					automationPreviewURLs(input.DocumentID, task.Steps[i].Preview)
				}
			}
		}
		result = task
	case "task.control":
		result, err = server.automation.Control(ctx, client.actor.ID, input.DocumentID, input.TaskID, input.Control)
	default:
		client.sendError(envelope.ID, "UNSUPPORTED_METHOD", "unsupported automation method", false)
		return
	}
	if err != nil {
		var f *automation.Fault
		if errors.As(err, &f) {
			client.enqueue(newRealtimeEnvelope("error", "request.failed.v1", envelope.ID, nil, nil, &realtimeError{Code: f.Code, Message: f.Message, Retryable: f.Retryable, Phase: f.Phase, DiagnosticID: f.DiagnosticID}))
		} else {
			client.sendDomainError(envelope.ID, err)
		}
		return
	}
	client.sendResponse(envelope.ID, "automation.completed.v1", result)
}
func automationPreviewURLs(document string, preview *workspace.CommandPreview) {
	if preview.Artifact == nil {
		return
	}
	for role, ref := range preview.Artifact.Representations {
		if ref.ObjectID != "" {
			ref.URL = "/api/documents/" + url.PathEscape(document) + "/representations/" + url.PathEscape(ref.ObjectID) + "?previewId=" + url.QueryEscape(preview.PreviewID)
			preview.Artifact.Representations[role] = ref
		}
	}
}
