package api

import (
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
	"net/http"
)

func (server *Server) mechanismSessionAccess(w http.ResponseWriter, r *http.Request) bool {
	_, err := server.access.RequireDocument(r.Context(), r.PathValue("documentID"), principal(r).ID, access.RoleViewer)
	if err != nil {
		writeAccessError(w, err)
		return false
	}
	return true
}
func (server *Server) beginMechanismSession(w http.ResponseWriter, r *http.Request) {
	if !server.mechanismSessionAccess(w, r) {
		return
	}
	var req workspace.MechanismSessionBegin
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := server.workspace.BeginMechanismSession(r.Context(), r.PathValue("documentID"), principal(r).ID, req)
	if err != nil {
		writeWorkspaceResult(w, workspace.DocumentView{}, err)
		return
	}
	writeJSON(w, 200, result)
}
func (server *Server) updateMechanismSession(w http.ResponseWriter, r *http.Request) {
	if !server.mechanismSessionAccess(w, r) {
		return
	}
	var req workspace.MechanismSessionUpdate
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := server.workspace.UpdateMechanismSession(r.Context(), r.PathValue("documentID"), principal(r).ID, r.PathValue("sessionID"), req)
	if err != nil {
		writeWorkspaceResult(w, workspace.DocumentView{}, err)
		return
	}
	writeJSON(w, 200, result)
}
func (server *Server) recordMechanismKey(w http.ResponseWriter, r *http.Request) {
	if !server.mechanismSessionAccess(w, r) {
		return
	}
	var req struct {
		Sequence uint64                   `json:"sequence"`
		Key      workspace.MotionKeyframe `json:"key"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := server.workspace.RecordMechanismKey(r.Context(), r.PathValue("documentID"), principal(r).ID, r.PathValue("sessionID"), req.Sequence, req.Key)
	if err != nil {
		writeWorkspaceResult(w, workspace.DocumentView{}, err)
		return
	}
	writeJSON(w, 200, result)
}
func (server *Server) finalizeMechanismSession(w http.ResponseWriter, r *http.Request) {
	if _, err := server.access.RequireDocument(r.Context(), r.PathValue("documentID"), principal(r).ID, access.RoleEditor); err != nil {
		writeAccessError(w, err)
		return
	}
	var req workspace.MechanismSessionFinalize
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := server.workspace.FinalizeMechanismSession(r.Context(), r.PathValue("documentID"), principal(r).ID, r.PathValue("sessionID"), req)
	if err != nil {
		writeWorkspaceResult(w, workspace.DocumentView{}, err)
		return
	}
	writeJSON(w, 200, result)
}
func (server *Server) cancelMechanismSession(w http.ResponseWriter, r *http.Request) {
	if !server.mechanismSessionAccess(w, r) {
		return
	}
	server.workspace.CancelMechanismSession(r.PathValue("documentID"), principal(r).ID, r.PathValue("sessionID"))
	w.WriteHeader(http.StatusNoContent)
}
func (server *Server) touchMechanismSession(w http.ResponseWriter, r *http.Request) {
	if !server.mechanismSessionAccess(w, r) {
		return
	}
	if err := server.workspace.TouchMechanismSession(r.Context(), r.PathValue("documentID"), principal(r).ID, r.PathValue("sessionID")); err != nil {
		writeWorkspaceResult(w, workspace.DocumentView{}, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (server *Server) previewMechanismRecording(w http.ResponseWriter, r *http.Request) {
	if !server.mechanismSessionAccess(w, r) {
		return
	}
	var req workspace.MotionStudy
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := server.workspace.PreviewMechanismRecording(r.Context(), r.PathValue("documentID"), principal(r).ID, r.PathValue("sessionID"), req)
	if err != nil {
		writeWorkspaceResult(w, workspace.DocumentView{}, err)
		return
	}
	writeJSON(w, 200, result)
}
