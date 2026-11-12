package api

import (
	"net/http"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

func (server *Server) partDeletePlan(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.requireDocument(w, r, access.RoleEditor); !ok {
		return
	}
	var input workspace.PartDeletePlanRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := server.workspace.GetPartDeletePlan(r.Context(), r.PathValue("documentID"), input)
	if err != nil {
		writeTopologySelectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
