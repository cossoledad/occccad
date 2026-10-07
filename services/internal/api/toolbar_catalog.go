package api

import (
	"github.com/occccad/occccad/internal/workbenchconfig"
	"github.com/occccad/occccad/internal/workspace"
	"net/http"
)

func (server *Server) toolbarCatalog(writer http.ResponseWriter, request *http.Request) {
	catalog, err := workbenchconfig.Read()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	if err := workspace.ValidateDocumentCatalog(catalog); err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, struct {
		workbenchconfig.Catalog
		Toolbars []workbenchconfig.Toolbar `json:"toolbars"`
	}{catalog, catalog.Toolbars()})
}
