package api

import (
	"fmt"
	"io"
	"net/http"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

func (s *Server) downloadRepresentation(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireDocument(w, r, access.RoleViewer); !ok {
		return
	}
	id := r.PathValue("objectID")
	version := r.URL.Query().Get("versionId")
	var allowed bool
	err := s.database.QueryRow(r.Context(), `WITH RECURSIVE reachable(id) AS (
 SELECT v.id FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id
 WHERE d.id=$1 AND v.id=COALESCE(NULLIF($2,'')::uuid,d.head_version_id)
 UNION
 SELECT p.referenced_version_id FROM occccad.product_instances p JOIN reachable r ON p.product_version_id=r.id
 ) SELECT EXISTS(SELECT 1 FROM reachable r JOIN occccad.document_versions v ON v.id=r.id
 JOIN occccad.geometry_representations a ON a.geometry_key=v.geometry_key WHERE a.object_id=$3)`, r.PathValue("documentID"), version, id).Scan(&allowed)
	if err != nil {
		writeError(w, http.StatusNotFound, "representation unavailable")
		return
	}
	if !allowed {
		// Context variants are derived from accepted bindings, not merely from a base
		// Part revision. Resolve that uncommon path through the existing domain gate.
		view, err := s.workspace.GetDocument(r.Context(), r.PathValue("documentID"))
		if err == nil && (version == "" || view.Document.VersionID == version) {
			contains := func(a workspace.Artifact) bool {
				for _, ref := range a.Representations {
					if ref.ObjectID == id {
						return true
					}
				}
				return false
			}
			allowed = view.Artifact != nil && contains(*view.Artifact)
			for _, a := range view.Artifacts {
				allowed = allowed || contains(a)
			}
		}
	}
	if !allowed {
		writeError(w, http.StatusNotFound, "representation unavailable for this document")
		return
	}
	object, reader, err := s.artifacts.Open(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "representation unavailable")
		return
	}
	defer reader.Close()
	etag := `"` + object.SHA256 + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", object.ContentType)
	w.Header().Set("Content-Length", fmt.Sprint(object.Size))
	filename := "artifact.bin"
	switch object.Kind {
	case "GLB":
		filename = "geometry.mesh.glb"
	case "BREP":
		filename = "shape.brep"
	case "TOPOLOGY_MANIFEST":
		filename = "naming.pb"
	}
	w.Header().Set("Content-Disposition", `inline; filename="`+filename+`"`)
	_, _ = io.Copy(w, reader)
}
