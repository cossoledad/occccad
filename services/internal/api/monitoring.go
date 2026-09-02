package api

import (
	"crypto/subtle"
	"net/http"
	"os"
)

func (server *Server) monitoringSnapshot(writer http.ResponseWriter, request *http.Request) {
	expected := os.Getenv("OCCCCAD_MONITORING_TOKEN")
	provided := request.Header.Get("X-Occccad-Monitoring-Token")
	if expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		writeError(writer, http.StatusNotFound, "route not found")
		return
	}
	connections, subscribed := server.realtime.monitoringCounts()
	counts := map[string]int{}
	queries := map[string]string{
		"documents":  `SELECT count(*) FROM occccad.documents WHERE deleted_at IS NULL`,
		"revisions":  `SELECT count(*) FROM occccad.document_versions`,
		"jobsQueued": `SELECT count(*) FROM occccad.jobs WHERE state IN ('QUEUED','RUNNING','RETRY_WAIT')`,
		"artifacts":  `SELECT count(*) FROM occccad.artifact_objects`,
	}
	for name, query := range queries {
		var count int
		if err := server.database.QueryRow(request.Context(), query).Scan(&count); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "monitoring query failed")
			return
		}
		counts[name] = count
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"realtimeConnections": connections, "subscribedDocuments": subscribed,
		"openDocumentSessions": server.openDocuments.sessionCount(), "counts": counts,
		"openDocuments": server.openDocuments.monitoringDocuments(),
	})
}
