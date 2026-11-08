package api

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

func (s *Server) featureRepairContext(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	var input workspace.FeatureRepairRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input.ActorID = principal(r).ID
	if input.RootProductDocumentID != "" {
		if _, err := s.access.RequireDocument(r.Context(), input.RootProductDocumentID, principal(r).ID, access.RoleViewer); err != nil {
			writeAccessError(w, err)
			return
		}
	}
	result, err := s.workspace.GetFeatureRepairContext(r.Context(), r.PathValue("documentID"), input)
	if err != nil {
		writeTopologySelectionError(w, err)
		return
	}
	if result.Snapshot != nil {
		for i := range result.Snapshot.Artifacts {
			for role, rep := range result.Snapshot.Artifacts[i].Representations {
				rep.URL = fmt.Sprintf("/api/documents/%s/representations/%s?previewId=%s", url.PathEscape(r.PathValue("documentID")), url.PathEscape(rep.ObjectID), url.QueryEscape(result.DisplayID))
				result.Snapshot.Artifacts[i].Representations[role] = rep
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) discardFeatureRepairContext(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	s.workspace.DiscardPreview(r.PathValue("documentID"), principal(r).ID, r.PathValue("displayID"))
	w.WriteHeader(http.StatusNoContent)
}
