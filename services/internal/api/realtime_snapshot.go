package api

import (
	"context"
	"net/http"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

type realtimeSnapshot struct {
	DocumentID  string                  `json:"documentId"`
	WorkspaceID string                  `json:"workspaceId"`
	VersionID   string                  `json:"versionId"`
	Sequence    uint64                  `json:"sequence"`
	View        *workspace.DocumentView `json:"view,omitempty"`
}

func (server *Server) realtimeHead(ctx context.Context, documentID string) (realtimeSnapshot, error) {
	result := realtimeSnapshot{DocumentID: documentID}
	err := server.database.QueryRow(ctx, `SELECT w.id::text,w.head_revision_id::text,w.head_sequence FROM occccad.workspaces w JOIN occccad.documents d ON d.id=w.document_id WHERE w.document_id=$1 AND w.name='main' AND d.deleted_at IS NULL`, documentID).Scan(&result.WorkspaceID, &result.VersionID, &result.Sequence)
	return result, err
}

// The HTTP snapshot is a resource query, not a second command channel. Large
// business trees never need a larger realtime frame. A sequence must describe
// exactly the same head as the returned view, not a later concurrent commit.
func (server *Server) realtimeSnapshot(writer http.ResponseWriter, request *http.Request) {
	role, ok := server.requireDocument(writer, request, access.RoleViewer)
	if !ok {
		return
	}
	for attempt := 0; attempt < 3; attempt++ {
		before, err := server.realtimeHead(request.Context(), request.PathValue("documentID"))
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, "snapshot head unavailable")
			return
		}
		view, err := server.workspace.GetDocument(request.Context(), before.DocumentID, principal(request).ID)
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, "snapshot unavailable")
			return
		}
		after, err := server.realtimeHead(request.Context(), before.DocumentID)
		if err == nil && before.Sequence == after.Sequence && before.VersionID == after.VersionID && view.Document.VersionID == after.VersionID {
			view.Document.Permission = string(role)
			after.View = &view
			server.openDocuments.Update(principal(request).ID, view.Document)
			writer.Header().Set("Cache-Control", "no-store")
			writeJSON(writer, http.StatusOK, after)
			return
		}
	}
	writer.Header().Set("Retry-After", "1")
	writeError(writer, http.StatusConflict, "snapshot changed while loading; retry")
}
