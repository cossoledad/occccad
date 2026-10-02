package api

import (
	"encoding/json"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/assemblycontract"
	"github.com/occccad/occccad/internal/workspace"
	"net/http"
)

// Returns semantic availability, independent of execution reports. Exact support
// resolution and server command validation remain authoritative at submission.
func (server *Server) assemblyCapabilities(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(assemblycontract.Read()); err != nil {
		return
	}
}

func (server *Server) inspectAssemblySupports(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleViewer); !ok {
		return
	}
	var input struct {
		References []workspace.AssemblyGeometryRef `json:"references"`
	}
	if !decodeJSON(writer, request, &input) {
		return
	}
	result, err := server.workspace.InspectAssemblySupports(request.Context(), request.PathValue("documentID"), input.References)
	if err != nil {
		writeTopologySelectionError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) assemblyEngineeringEvidence(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleViewer); !ok {
		return
	}
	revision := request.URL.Query().Get("revisionId")
	if revision == "" {
		writeError(writer, http.StatusBadRequest, "revisionId is required")
		return
	}
	result, err := server.workspace.GetAssemblyEngineeringEvidence(request.Context(), request.PathValue("documentID"), revision)
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}
