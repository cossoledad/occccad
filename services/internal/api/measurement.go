package api

import (
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
	"net/http"
)

func (server *Server) measureGeometry(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	var input workspace.MeasurementRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	report, err := server.workspace.Measure(r.Context(), r.PathValue("documentID"), input)
	if err != nil {
		writeTopologySelectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
func (server *Server) analyzeShapeQuality(w http.ResponseWriter, r *http.Request) {
	if _, ok := server.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	var input workspace.ShapeQualityRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	report, err := server.workspace.AnalyzeShapeQuality(r.Context(), r.PathValue("documentID"), input)
	if err != nil {
		writeTopologySelectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
