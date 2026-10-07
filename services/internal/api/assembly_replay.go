package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

func (server *Server) listAssemblyReplays(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	items, err := server.workspace.ListAssemblyReplays(r.Context(), r.PathValue("documentID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (server *Server) downloadAssemblyReplay(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	item, data, err := server.workspace.ReadAssemblyReplay(r.Context(), r.PathValue("documentID"), r.PathValue("replayID"), r.URL.Query().Get("requestId"))
	if err == workspace.ErrNotFound {
		writeError(w, http.StatusNotFound, "该次请求尚无三维求解记录；几何解析前失败不会产生 3dreplay")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var pretty json.RawMessage = data
	encoded, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/vnd.occccad.3dreplay+json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.3dreplay"`, item.ID))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

func (server *Server) downloadAssemblyDiagnostic(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	data, err := server.workspace.ExportAssemblyDiagnosticWithOptions(r.Context(), r.PathValue("documentID"), workspace.AssemblyDiagnosticExportOptions{DetailedNaming: r.URL.Query().Get("naming") == "true"})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/vnd.occccad.3dreplay+json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="assembly-%s.3dreplay"`, r.PathValue("documentID")))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
