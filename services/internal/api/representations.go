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
	if previewID := r.URL.Query().Get("previewId"); previewID != "" {
		allowed = s.workspace.PreviewVisualAllowed(r.PathValue("documentID"), principal(r).ID, previewID, id)
		if !allowed {
			writeError(w, http.StatusNotFound, "preview expired or representation unavailable")
			return
		}
	}
	if featureID := r.URL.Query().Get("featureId"); featureID != "" && !allowed {
		input, inputErr := s.workspace.GetFeatureInput(r.Context(), r.PathValue("documentID"), workspace.FeatureInputRequest{VersionID: version, FeatureID: featureID})
		if inputErr != nil {
			writeError(w, http.StatusNotFound, "feature input unavailable")
			return
		}
		for _, artifact := range append(input.Artifacts, input.Artifact) {
			if bodyID := r.URL.Query().Get("bodyId"); bodyID != "" && artifact.BodyID != bodyID {
				continue
			}
			for _, representation := range artifact.Representations {
				if representation.ObjectID == id {
					allowed = true
				}
			}
		}
		if !allowed {
			writeError(w, http.StatusNotFound, "representation outside feature input")
			return
		}
	}
	var err error
	if !allowed {
		err = s.database.QueryRow(r.Context(), `WITH RECURSIVE reachable(id) AS (
 SELECT v.id FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id
 WHERE d.id=$1 AND v.id=COALESCE(NULLIF($2,'')::uuid,d.head_version_id)
 UNION
 SELECT p.referenced_version_id FROM occccad.product_instances p JOIN reachable r ON p.product_version_id=r.id
 ) SELECT EXISTS(SELECT 1 FROM reachable r JOIN occccad.document_versions v ON v.id=r.id
 CROSS JOIN LATERAL jsonb_array_elements(v.model_json->'bodies') body
 JOIN occccad.geometry_representations a ON a.geometry_key=body->>'geometryKey' WHERE a.object_id=$3
 AND ($4='' OR body->>'id'=$4))`, r.PathValue("documentID"), version, id, r.URL.Query().Get("bodyId")).Scan(&allowed)
	}
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
				if bodyID := r.URL.Query().Get("bodyId"); bodyID != "" && a.BodyID != bodyID {
					return false
				}
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
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", disposition+`; filename="`+filename+`"`)
	_, _ = io.Copy(w, reader)
}
