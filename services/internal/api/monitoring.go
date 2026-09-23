package api

import (
	"crypto/subtle"
	"net/http"
	"os"

	"github.com/occccad/occccad/internal/database"
)

func (server *Server) monitoringSnapshot(writer http.ResponseWriter, request *http.Request) {
	expected := os.Getenv("OCCCCAD_MONITORING_TOKEN")
	provided := request.Header.Get("X-Occccad-Monitoring-Token")
	if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		writeError(writer, http.StatusNotFound, "route not found")
		return
	}
	connections, subscribed := server.realtime.monitoringCounts()
	var documents, revisions, jobsQueued, artifacts int
	if err := server.database.QueryRow(database.Background(request.Context()), `SELECT
  (SELECT count(*) FROM occccad.documents WHERE deleted_at IS NULL),
  (SELECT count(*) FROM occccad.document_versions),
  (SELECT count(*) FROM occccad.jobs WHERE state IN ('QUEUED','RUNNING','RETRY_WAIT')),
  (SELECT count(*) FROM occccad.artifact_objects)`).Scan(&documents, &revisions, &jobsQueued, &artifacts); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "monitoring query failed")
		return
	}
	counts := map[string]int{"documents": documents, "revisions": revisions, "jobsQueued": jobsQueued, "artifacts": artifacts}
	writeJSON(writer, http.StatusOK, map[string]any{
		"realtimeConnections": connections, "subscribedDocuments": subscribed,
		"openDocumentSessions": server.openDocuments.sessionCount(), "counts": counts,
		"openDocuments": server.openDocuments.monitoringDocuments(),
		"database":      server.database.Snapshot(),
	})
}
