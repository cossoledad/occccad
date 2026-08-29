package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/occccad/occccad/internal/access"
)

const diagnosticBundleSchema = "occccad.cad-diagnostic-bundle.v1"

type diagnosticBundleRequest struct {
	FailedCommand json.RawMessage `json:"failedCommand"`
	Error         string          `json:"error"`
	Client        json.RawMessage `json:"client,omitempty"`
}

// downloadDiagnosticBundle exports only data already visible to the document
// member plus execution metadata needed to replay a failed command. Credentials,
// cookies, B-Rep bytes and unrelated server logs are deliberately excluded.
func (server *Server) downloadDiagnosticBundle(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleViewer); !ok {
		return
	}
	var input diagnosticBundleRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if len(input.FailedCommand) == 0 || !json.Valid(input.FailedCommand) || len(input.Error) > 16*1024 ||
		(len(input.Client) > 0 && !json.Valid(input.Client)) {
		writeError(writer, http.StatusBadRequest, "invalid diagnostic bundle request")
		return
	}
	documentID := request.PathValue("documentID")
	view, err := server.workspace.GetDocument(request.Context(), documentID, principal(request).ID)
	if err != nil {
		server.writeDocumentResult(writer, request, view, err)
		return
	}
	history, err := server.workspace.ListHistory(request.Context(), documentID)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}

	var workspaceState json.RawMessage
	if err := server.database.QueryRow(request.Context(), `SELECT jsonb_build_object(
		'id',w.id,'name',w.name,'headRevisionId',w.head_revision_id,'headSequence',w.head_sequence,
		'baseRevisionId',w.base_revision_id,'policy',w.policy,'updatedAt',w.updated_at)
		FROM occccad.workspaces w WHERE w.document_id=$1 AND w.name='main'`, documentID).Scan(&workspaceState); err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	rows, err := server.database.Query(request.Context(), `SELECT jsonb_build_object(
		'id',t.id,'sequence',t.sequence,'requestId',t.request_id,'kind',t.kind,'status',t.status,
		'baseRevisionId',t.base_revision_id,'resultRevisionId',t.result_revision_id,
		'createdAt',t.created_at,'committedAt',t.committed_at)
		FROM occccad.domain_transactions t JOIN occccad.workspaces w ON w.id=t.workspace_id
		WHERE w.document_id=$1 ORDER BY t.sequence DESC LIMIT 50`, documentID)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	transactions := []json.RawMessage{}
	for rows.Next() {
		var item json.RawMessage
		if err := rows.Scan(&item); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		transactions = append(transactions, item)
	}
	if err := rows.Err(); err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	var commandLog, evaluationRuns json.RawMessage
	if err := server.database.QueryRow(request.Context(), `SELECT COALESCE(jsonb_agg(item), '[]'::jsonb) FROM (
		SELECT jsonb_build_object('requestId',request_id,'type',command_type,'status',status,
			'error',error_message,'payload',payload,'createdAt',created_at,'completedAt',completed_at) item
		FROM occccad.commands WHERE document_id=$1 ORDER BY created_at DESC LIMIT 50) recent`, documentID).Scan(&commandLog); err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	if err := server.database.QueryRow(request.Context(), `SELECT COALESCE(jsonb_agg(item), '[]'::jsonb) FROM (
		SELECT jsonb_build_object('id',r.id,'revisionId',r.revision_id,'capability',r.capability,
			'evaluatorDigest',r.evaluator_digest,'inputDigest',r.input_digest,'manifest',r.manifest,
			'manifestDigest',r.manifest_digest,'status',r.status,'authoritative',r.authoritative,'createdAt',r.created_at) item
		FROM occccad.evaluation_runs r JOIN occccad.document_versions v ON v.id=r.revision_id
		WHERE v.document_id=$1 ORDER BY r.created_at DESC LIMIT 20) recent`, documentID).Scan(&evaluationRuns); err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}

	bundle := map[string]any{
		"schema":      diagnosticBundleSchema,
		"generatedAt": time.Now().UTC(),
		"failure": map[string]any{
			"message": input.Error, "command": input.FailedCommand,
		},
		"client": input.Client,
		"runtime": map[string]any{
			"goVersion": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH,
		},
		"document":           view,
		"history":            history,
		"workspace":          workspaceState,
		"recentTransactions": transactions,
		"recentCommandLog":   commandLog,
		"evaluationRuns":     evaluationRuns,
		"logCorrelation": map[string]any{
			"requestId":   request.Header.Get("X-Request-ID"),
			"traceparent": request.Header.Get("Traceparent"),
			"note":        "Search structured API, control and geometry-worker logs by the failed command requestId stored in failure.command.",
		},
	}
	encoded, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writer.Header().Set("Content-Type", "application/vnd.occccad.diagnostic+json")
	writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="occccad-diagnostic-%s-%s.json"`,
		documentID, time.Now().UTC().Format("20060102T150405Z")))
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(encoded)
}
