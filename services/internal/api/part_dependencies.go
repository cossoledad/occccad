package api

import (
	"net/http"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

func (server *Server) partDependencies(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	var input workspace.PartDependencyRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.RootProductDocumentID != "" {
		if _, err := server.access.RequireDocument(r.Context(), input.RootProductDocumentID, principal(r).ID, access.RoleViewer); err != nil {
			writeAccessError(w, err)
			return
		}
	}
	result, err := server.workspace.GetPartDependencies(r.Context(), r.PathValue("documentID"), input)
	if err != nil {
		writeTopologySelectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
