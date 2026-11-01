package api

import (
	"errors"
	"net/http"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

func (s *Server) annotationLayouts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	result, err := s.workspace.AnnotationLayouts(r.Context(), r.PathValue("documentID"))
	if err != nil {
		writeTopologySelectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) saveAnnotationLayout(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDocument(w, r, access.RoleEditor); !ok {
		return
	}
	var input workspace.AnnotationLayout
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := s.workspace.SaveAnnotationLayout(r.Context(), r.PathValue("documentID"), r.PathValue("measurementID"), input)
	if errors.Is(err, workspace.ErrPresentationConflict) {
		writeError(w, http.StatusConflict, "annotation layout changed; reload its current position")
		return
	}
	if err != nil {
		writeTopologySelectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
