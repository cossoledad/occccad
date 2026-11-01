package api

import (
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
	"net/http"
)

func (server *Server) analyzeSpatial(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	var input workspace.SpatialAnalysisRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := server.workspace.AnalyzeSpatial(r.Context(), r.PathValue("documentID"), input)
	if err != nil {
		writeWorkspaceResult(w, workspace.DocumentView{}, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (server *Server) getSpatialResult(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	result, err := server.workspace.GetSpatialResult(r.Context(), r.PathValue("documentID"), r.URL.Query().Get("revisionId"), r.PathValue("analysisID"))
	if err != nil {
		writeWorkspaceResult(w, workspace.DocumentView{}, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
