package api

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/authn"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
	"go.opentelemetry.io/otel/trace"
)

type Server struct {
	database       *pgxpool.Pool
	worker         *geometry.Client
	workspace      *workspace.Service
	access         *access.Service
	authn          *authn.Service
	artifacts      *artifact.Service
	jobs           *jobs.Service
	openDocuments  *openDocumentRegistry
	secureCookies  bool
	allowedOrigins map[string]struct{}
	realtime       *realtimeHub
	realtimeCancel context.CancelFunc
}

func New(
	database *pgxpool.Pool,
	worker *geometry.Client,
	workspaceService *workspace.Service,
	accessService *access.Service,
	authenticationService *authn.Service,
	artifactService *artifact.Service,
	jobService *jobs.Service,
	secureCookies bool,
	allowedOrigins []string,
) *Server {
	origins := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins[origin] = struct{}{}
		}
	}
	server := &Server{
		database: database, worker: worker,
		workspace: workspaceService, access: accessService, authn: authenticationService,
		artifacts: artifactService,
		jobs:      jobService, openDocuments: newOpenDocumentRegistry(),
		secureCookies: secureCookies, allowedOrigins: origins, realtime: newRealtimeHub(),
	}
	if database != nil {
		ctx, cancel := context.WithCancel(context.Background())
		server.realtimeCancel = cancel
		go server.dispatchRealtimeOutbox(ctx)
	}
	return server
}

func (server *Server) Close() {
	if server.realtimeCancel != nil {
		server.realtimeCancel()
	}
	server.realtime.close()
}

func principal(request *http.Request) access.User {
	result, _ := access.Principal(request.Context())
	return result
}

func (server *Server) requireDocument(writer http.ResponseWriter, request *http.Request, role access.Role) (access.Role, bool) {
	effective, err := server.access.RequireDocument(request.Context(), request.PathValue("documentID"), principal(request).ID, role)
	if err != nil {
		writeAccessError(writer, err)
		return "", false
	}
	return effective, true
}

func (server *Server) requireFolder(writer http.ResponseWriter, request *http.Request, role access.Role) (access.Role, bool) {
	effective, err := server.access.RequireFolder(request.Context(), request.PathValue("folderID"), principal(request).ID, role)
	if err != nil {
		writeAccessError(writer, err)
		return "", false
	}
	return effective, true
}

func writeAccessError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, access.ErrUnauthorized):
		writeError(writer, http.StatusUnauthorized, err.Error())
	case errors.Is(err, access.ErrForbidden):
		writeError(writer, http.StatusForbidden, "you do not have permission for this resource")
	case errors.Is(err, access.ErrValidation):
		writeError(writer, http.StatusBadRequest, err.Error())
	case errors.Is(err, access.ErrNotFound):
		writeError(writer, http.StatusNotFound, "resource not found")
	default:
		writeError(writer, http.StatusInternalServerError, err.Error())
	}
}

func (server *Server) writeDocumentResult(writer http.ResponseWriter, request *http.Request,
	result workspace.DocumentView, err error) {
	if err == nil && result.Document.ID != "" {
		if role, roleErr := server.access.EffectiveDocumentRole(request.Context(), result.Document.ID, principal(request).ID); roleErr == nil {
			result.Document.Permission = string(role)
		}
		if request.Method != http.MethodGet {
			if jobErr := server.enqueueDocumentPreviews(request.Context(), result, principal(request).ID); jobErr != nil {
				slog.ErrorContext(request.Context(), "enqueue document previews", "error", jobErr)
			}
		}
		server.openDocuments.Update(principal(request).ID, result.Document)
	}
	writeWorkspaceResult(writer, result, err)
}

func (server *Server) enqueueDocumentPreviews(ctx context.Context, changed workspace.DocumentView, requestedBy string) error {
	rows, err := server.database.Query(ctx, `WITH RECURSIVE affected(document_id,version_id) AS (
		SELECT $1::uuid,$2::uuid
		UNION
		SELECT parent.id,parent.head_version_id
		FROM affected child
		JOIN occccad.documents parent ON parent.document_type='PRODUCT' AND parent.deleted_at IS NULL
		JOIN occccad.document_versions version ON version.id=parent.head_version_id
		CROSS JOIN LATERAL jsonb_array_elements(
			COALESCE(version.model_json->'instances','[]'::jsonb)) instance
		WHERE instance->>'documentId'=child.document_id::text
		  AND COALESCE(instance->>'referenceMode','FOLLOW_HEAD')='FOLLOW_HEAD'
	)
	SELECT document_id::text,version_id::text FROM affected`, changed.Document.ID, changed.Document.VersionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var documentID, versionID string
		if err := rows.Scan(&documentID, &versionID); err != nil {
			return err
		}
		digest := sha256.Sum256([]byte(documentID + ":" + versionID + ":" +
			changed.Document.ID + ":" + changed.Document.VersionID + ":preview-v2"))
		identity := hex.EncodeToString(digest[:])
		if _, err := server.jobs.Enqueue(ctx, jobs.EnqueueRequest{Type: "THUMBNAIL_RENDER",
			DocumentID: documentID, VersionID: &versionID, RequestedBy: requestedBy,
			IdempotencyKey: identity, Payload: map[string]any{
				"previewIdentity": identity, "rendererVersion": "svg-v2"}}); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", server.health)
	mux.HandleFunc("POST /api/auth/login", server.login)
	mux.HandleFunc("POST /api/auth/register", server.register)
	mux.HandleFunc("POST /api/auth/logout", server.logout)
	mux.HandleFunc("POST /api/auth/change-password", server.changePassword)
	mux.HandleFunc("GET /api/session", server.session)
	mux.HandleFunc("GET /api/realtime", server.realtimeConnection)
	mux.HandleFunc("GET /api/admin/users", server.adminListUsers)
	mux.HandleFunc("POST /api/admin/users", server.adminCreateUser)
	mux.HandleFunc("PATCH /api/admin/users/{userID}", server.adminUpdateUser)
	mux.HandleFunc("DELETE /api/admin/users/{userID}", server.adminDisableUser)
	mux.HandleFunc("POST /api/admin/users/{userID}/reset-password", server.adminResetPassword)
	mux.HandleFunc("GET /api/admin/stats", server.adminStats)
	mux.HandleFunc("GET /api/users", server.listUsers)
	mux.HandleFunc("GET /api/teams", server.listTeams)
	mux.HandleFunc("GET /api/teams/{teamID}/members", server.listTeamMembers)
	mux.HandleFunc("GET /api/documents", server.listDocuments)
	mux.HandleFunc("POST /api/documents", server.createDocument)
	mux.HandleFunc("GET /api/open-documents", server.listOpenDocuments)
	mux.HandleFunc("DELETE /api/open-documents/{documentID}", server.closeOpenDocument)
	mux.HandleFunc("GET /api/folders", server.listFolders)
	mux.HandleFunc("POST /api/folders", server.createFolder)
	mux.HandleFunc("PATCH /api/folders/{folderID}", server.updateFolder)
	mux.HandleFunc("DELETE /api/folders/{folderID}", server.deleteFolder)
	mux.HandleFunc("GET /api/folders/{folderID}/breadcrumbs", server.folderBreadcrumbs)
	mux.HandleFunc("GET /api/documents/{documentID}", server.getDocument)
	mux.HandleFunc("GET /api/documents/{documentID}/properties", server.documentProperties)
	mux.HandleFunc("GET /api/documents/{documentID}/topology-properties", server.topologyProperties)
	mux.HandleFunc("GET /api/documents/{documentID}/preview", server.documentPreview)
	mux.HandleFunc("PATCH /api/documents/{documentID}", server.updateDocument)
	mux.HandleFunc("DELETE /api/documents/{documentID}", server.deleteDocument)
	mux.HandleFunc("POST /api/documents/{documentID}/restore", server.restoreDocument)
	mux.HandleFunc("POST /api/documents/{documentID}/move", server.moveDocument)
	mux.HandleFunc("POST /api/documents/{documentID}/copy", server.copyDocument)
	mux.HandleFunc("GET /api/documents/{documentID}/history", server.getDocumentHistory)
	mux.HandleFunc("GET /api/documents/{documentID}/workspaces", server.listDocumentWorkspaces)
	mux.HandleFunc("POST /api/documents/{documentID}/workspaces", server.branchDocumentWorkspace)
	mux.HandleFunc("POST /api/documents/{documentID}/versions", server.createVersion)
	mux.HandleFunc("POST /api/documents/{documentID}/commands", server.applyCommand)
	mux.HandleFunc("POST /api/documents/{documentID}/command-previews", server.previewCommand)
	mux.HandleFunc("GET /api/exchange/capabilities", server.exchangeCapabilities)
	mux.HandleFunc("POST /api/exchange/imports", server.startExchangeImport)
	mux.HandleFunc("POST /api/exchange/exports", server.startExchangeExport)
	mux.HandleFunc("GET /api/jobs", server.listJobs)
	mux.HandleFunc("GET /api/jobs/{jobID}", server.getJob)
	mux.HandleFunc("POST /api/jobs/{jobID}/cancel", server.cancelJob)
	mux.HandleFunc("POST /api/jobs/{jobID}/retry", server.retryJob)
	mux.HandleFunc("GET /api/jobs/{jobID}/download", server.downloadJob)
	mux.HandleFunc("GET /api/documents/{documentID}/shares", server.documentShares)
	mux.HandleFunc("POST /api/documents/{documentID}/shares", server.shareDocument)
	mux.HandleFunc("DELETE /api/documents/{documentID}/shares/{grantID}", server.unshareDocument)
	mux.HandleFunc("GET /api/folders/{folderID}/shares", server.folderShares)
	mux.HandleFunc("POST /api/folders/{folderID}/shares", server.shareFolder)
	mux.HandleFunc("DELETE /api/folders/{folderID}/shares/{grantID}", server.unshareFolder)
	mux.HandleFunc("GET /api/audit", server.listAudit)
	mux.HandleFunc("/api/", func(writer http.ResponseWriter, _ *http.Request) {
		writeError(writer, http.StatusNotFound, "API route not found")
	})
	mux.HandleFunc("GET /{$}", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{
			"service": "occccad-api",
			"message": "frontend is served by an independent web process",
		})
	})
	mux.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
		writeError(writer, http.StatusNotFound, "route not found")
	})
	return server.middleware(mux)
}

func (server *Server) session(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"user": principal(request), "authenticationMode": "password-session"})
}

func (server *Server) listUsers(writer http.ResponseWriter, request *http.Request) {
	users, err := server.access.ListUsers(request.Context(), request.URL.Query().Get("q"))
	if err != nil {
		writeAccessError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"users": users})
}

func (server *Server) listTeams(writer http.ResponseWriter, request *http.Request) {
	teams, err := server.access.ListTeams(request.Context(), principal(request).ID)
	if err != nil {
		writeAccessError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"teams": teams})
}

func (server *Server) listTeamMembers(writer http.ResponseWriter, request *http.Request) {
	members, err := server.access.ListTeamMembers(request.Context(), request.PathValue("teamID"), principal(request).ID)
	if err != nil {
		writeAccessError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"members": members})
}

func (server *Server) documentShares(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleOwner); !ok {
		return
	}
	server.listShares(writer, request, "DOCUMENT", request.PathValue("documentID"))
}

func (server *Server) shareDocument(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleOwner); !ok {
		return
	}
	server.upsertShare(writer, request, "DOCUMENT", request.PathValue("documentID"))
}

func (server *Server) unshareDocument(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleOwner); !ok {
		return
	}
	server.deleteShare(writer, request, "DOCUMENT", request.PathValue("documentID"))
}

func (server *Server) folderShares(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireFolder(writer, request, access.RoleOwner); !ok {
		return
	}
	server.listShares(writer, request, "FOLDER", request.PathValue("folderID"))
}

func (server *Server) shareFolder(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireFolder(writer, request, access.RoleOwner); !ok {
		return
	}
	server.upsertShare(writer, request, "FOLDER", request.PathValue("folderID"))
}

func (server *Server) unshareFolder(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireFolder(writer, request, access.RoleOwner); !ok {
		return
	}
	server.deleteShare(writer, request, "FOLDER", request.PathValue("folderID"))
}

func (server *Server) listShares(writer http.ResponseWriter, request *http.Request, resourceType, resourceID string) {
	grants, err := server.access.ListGrants(request.Context(), resourceType, resourceID)
	if err != nil {
		writeAccessError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"grants": grants})
}

func (server *Server) upsertShare(writer http.ResponseWriter, request *http.Request, resourceType, resourceID string) {
	var input access.ShareRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	grant, err := server.access.UpsertGrant(request.Context(), resourceType, resourceID, principal(request).ID, input)
	if err != nil {
		writeAccessError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, grant)
}

func (server *Server) deleteShare(writer http.ResponseWriter, request *http.Request, resourceType, resourceID string) {
	if err := server.access.DeleteGrant(request.Context(), resourceType, resourceID, request.PathValue("grantID")); err != nil {
		writeAccessError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) listAudit(writer http.ResponseWriter, request *http.Request) {
	documentID := strings.TrimSpace(request.URL.Query().Get("documentId"))
	if documentID == "" {
		writeError(writer, http.StatusBadRequest, "documentId is required")
		return
	}
	if _, err := server.access.RequireDocument(request.Context(), documentID, principal(request).ID, access.RoleViewer); err != nil {
		writeAccessError(writer, err)
		return
	}
	limit, err := queryInteger(request, "limit", 50)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	events, err := server.access.ListAudit(request.Context(), "DOCUMENT", documentID, limit)
	if err != nil {
		writeAccessError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"events": events})
}

func (server *Server) createVersion(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleEditor); !ok {
		return
	}
	var input workspace.CreateVersionRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	history, err := server.workspace.CreateVersion(
		request.Context(), request.PathValue("documentID"), input)
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"workspace": "Main", "history": history})
}

func (server *Server) getDocumentHistory(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleViewer); !ok {
		return
	}
	history, err := server.workspace.ListHistory(request.Context(), request.PathValue("documentID"))
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"workspace": "Main", "history": history})
}

func (server *Server) listDocumentWorkspaces(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleViewer); !ok {
		return
	}
	items, err := server.workspace.ListWorkspaces(request.Context(), request.PathValue("documentID"))
	if err != nil {
		server.writeDocumentResult(writer, request, workspace.DocumentView{}, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"workspaces": items})
}

func (server *Server) branchDocumentWorkspace(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleEditor); !ok {
		return
	}
	var input workspace.CreateWorkspaceRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	item, err := server.workspace.BranchWorkspace(request.Context(), request.PathValue("documentID"), input)
	if err != nil {
		server.writeDocumentResult(writer, request, workspace.DocumentView{}, err)
		return
	}
	writeJSON(writer, http.StatusCreated, item)
}

func (server *Server) listDocuments(writer http.ResponseWriter, request *http.Request) {
	limit, err := queryInteger(request, "limit", 50)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := queryInteger(request, "offset", 0)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	documents, err := server.workspace.ListDocuments(request.Context(), workspace.DocumentListOptions{
		Scope: request.URL.Query().Get("scope"), Query: request.URL.Query().Get("q"),
		Type: request.URL.Query().Get("type"), FolderID: request.URL.Query().Get("folderId"),
		Sort: request.URL.Query().Get("sort"), Recent: request.URL.Query().Get("recent") == "true",
		AllFolders: request.URL.Query().Get("allFolders") == "true",
		Shared:     request.URL.Query().Get("shared") == "true",
		Limit:      limit, Offset: offset, ActorID: principal(request).ID,
	})
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	writeJSON(writer, http.StatusOK, documents)
}

func queryInteger(request *http.Request, name string, fallback int) (int, error) {
	value := request.URL.Query().Get(name)
	if value == "" {
		return fallback, nil
	}
	result, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return result, nil
}

func (server *Server) listFolders(writer http.ResponseWriter, request *http.Request) {
	folders, err := server.workspace.ListFolders(request.Context(), request.URL.Query().Get("parentId"),
		principal(request).ID, request.URL.Query().Get("shared") == "true")
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"folders": folders})
}

func (server *Server) folderBreadcrumbs(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireFolder(writer, request, access.RoleViewer); !ok {
		return
	}
	folders, err := server.workspace.FolderBreadcrumbs(request.Context(), request.PathValue("folderID"), principal(request).ID)
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"folders": folders})
}

func (server *Server) createFolder(writer http.ResponseWriter, request *http.Request) {
	var input workspace.CreateFolderRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.ParentID != nil && *input.ParentID != "" {
		if _, err := server.access.RequireFolder(request.Context(), *input.ParentID, principal(request).ID, access.RoleEditor); err != nil {
			writeAccessError(writer, err)
			return
		}
	}
	input.ActorID = principal(request).ID
	result, err := server.workspace.CreateFolder(request.Context(), input)
	writeFolderResult(writer, result, err)
}

func (server *Server) updateFolder(writer http.ResponseWriter, request *http.Request) {
	role, ok := server.requireFolder(writer, request, access.RoleEditor)
	if !ok {
		return
	}
	var input workspace.UpdateFolderRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	result, err := server.workspace.UpdateFolder(request.Context(), request.PathValue("folderID"), input)
	result.Permission = string(role)
	writeFolderResult(writer, result, err)
}

func (server *Server) deleteFolder(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireFolder(writer, request, access.RoleEditor); !ok {
		return
	}
	if err := server.workspace.DeleteFolder(request.Context(), request.PathValue("folderID")); err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func writeFolderResult(writer http.ResponseWriter, result workspace.FolderSummary, err error) {
	if err == nil {
		writeJSON(writer, http.StatusOK, result)
		return
	}
	writeWorkspaceResult(writer, workspace.DocumentView{}, err)
}

func (server *Server) updateDocument(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleEditor); !ok {
		return
	}
	var input workspace.UpdateDocumentRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	result, err := server.workspace.UpdateDocument(
		request.Context(), request.PathValue("documentID"), input)
	server.writeDocumentResult(writer, request, result, err)
}

func (server *Server) deleteDocument(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleEditor); !ok {
		return
	}
	err := server.workspace.DeleteDocument(request.Context(), request.PathValue("documentID"),
		request.Header.Get("X-Request-ID"))
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	server.openDocuments.Close(principal(request).ID, request.PathValue("documentID"))
	writer.WriteHeader(http.StatusNoContent)
}

func (server *Server) restoreDocument(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleEditor); !ok {
		return
	}
	result, err := server.workspace.RestoreDocument(request.Context(),
		request.PathValue("documentID"), request.Header.Get("X-Request-ID"))
	server.writeDocumentResult(writer, request, result, err)
}

func (server *Server) moveDocument(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleEditor); !ok {
		return
	}
	var input workspace.MoveDocumentRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.FolderID != nil && *input.FolderID != "" {
		if _, err := server.access.RequireFolder(request.Context(), *input.FolderID, principal(request).ID, access.RoleEditor); err != nil {
			writeAccessError(writer, err)
			return
		}
	}
	result, err := server.workspace.MoveDocument(request.Context(), request.PathValue("documentID"),
		request.Header.Get("X-Request-ID"), input)
	server.writeDocumentResult(writer, request, result, err)
}

func (server *Server) copyDocument(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleViewer); !ok {
		return
	}
	var input workspace.CopyDocumentRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.FolderID == nil {
		var sourceFolderID *string
		if err := server.database.QueryRow(request.Context(),
			`SELECT folder_id::text FROM occccad.documents WHERE id=$1`, request.PathValue("documentID")).Scan(&sourceFolderID); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		input.FolderID = sourceFolderID
	}
	if input.FolderID != nil && *input.FolderID != "" {
		if _, err := server.access.RequireFolder(request.Context(), *input.FolderID, principal(request).ID, access.RoleEditor); err != nil {
			writeAccessError(writer, err)
			return
		}
	}
	input.ActorID = principal(request).ID
	result, err := server.workspace.CopyDocument(request.Context(), request.PathValue("documentID"), input)
	server.writeDocumentResult(writer, request, result, err)
}

func (server *Server) createDocument(writer http.ResponseWriter, request *http.Request) {
	var input workspace.CreateDocumentRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.FolderID != nil && *input.FolderID != "" {
		if _, err := server.access.RequireFolder(request.Context(), *input.FolderID, principal(request).ID, access.RoleEditor); err != nil {
			writeAccessError(writer, err)
			return
		}
	}
	input.ActorID = principal(request).ID
	result, err := server.workspace.CreateDocument(request.Context(), input)
	if err == nil {
		result.Document.Permission = string(access.RoleOwner)
		server.openDocuments.Open(principal(request).ID, result.Document)
	}
	server.writeDocumentResult(writer, request, result, err)
}

func (server *Server) getDocument(writer http.ResponseWriter, request *http.Request) {
	role, ok := server.requireDocument(writer, request, access.RoleViewer)
	if !ok {
		return
	}
	result, err := server.workspace.GetDocument(request.Context(), request.PathValue("documentID"), principal(request).ID)
	if err == nil {
		result.Document.Permission = string(role)
		server.openDocuments.Open(principal(request).ID, result.Document)
	}
	server.writeDocumentResult(writer, request, result, err)
}

func (server *Server) documentProperties(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleViewer); !ok {
		return
	}
	view, err := server.workspace.GetDocument(request.Context(), request.PathValue("documentID"))
	if err != nil {
		writeWorkspaceResult(writer, workspace.DocumentView{}, err)
		return
	}
	artifacts := make([]workspace.Artifact, 0, len(view.Artifacts)+1)
	if view.Artifact != nil {
		artifacts = append(artifacts, *view.Artifact)
	}
	for _, item := range view.Artifacts {
		artifacts = append(artifacts, item)
	}
	triangles, vertices, solids, glbBytes, brepBytes := 0, 0, 0, 0, 0
	for _, item := range artifacts {
		triangles += len(item.Mesh.Triangles)
		vertices += len(item.Mesh.Vertices)
		if value, ok := item.Topology["solids"].(float64); ok {
			solids += int(value)
		}
		glbBytes += item.GLBBytes
		brepBytes += item.BRepBytes
	}
	worker := map[string]any{"available": false}
	if server.worker != nil {
		if ping, pingErr := server.worker.Ping(request.Context()); pingErr == nil {
			worker = map[string]any{"available": true, "workerId": ping.GetWorkerId(),
				"occtVersion": ping.GetOcctVersion(), "residentGeometryCount": ping.GetResidentGeometryCount()}
		} else {
			worker["error"] = pingErr.Error()
		}
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"documentId": view.Document.ID, "versionId": view.Document.VersionID,
		"documentType": view.Document.Type, "units": "mm", "artifacts": artifacts,
		"aggregate": map[string]any{"artifactCount": len(artifacts), "triangleCount": triangles,
			"vertexCount": vertices, "solidCount": solids, "glbBytes": glbBytes, "brepBytes": brepBytes,
			"resolvedInstanceCount": len(view.ResolvedInstances)},
		"worker": worker,
	})
}

func (server *Server) topologyProperties(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleViewer); !ok {
		return
	}
	localID, err := strconv.ParseUint(request.URL.Query().Get("localId"), 10, 64)
	if err != nil || localID == 0 {
		writeError(writer, http.StatusBadRequest, "localId must be a positive integer")
		return
	}
	result, err := server.workspace.GetTopologyElementProperties(request.Context(), request.PathValue("documentID"),
		request.URL.Query().Get("geometryKey"), request.URL.Query().Get("kind"), localID)
	if err != nil {
		if errors.Is(err, workspace.ErrNotFound) {
			writeError(writer, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, workspace.ErrValidation) {
			writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (server *Server) applyCommand(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleEditor); !ok {
		return
	}
	var input workspace.CommandRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	input.ActorID = principal(request).ID
	if strings.EqualFold(input.Type, "INSERT_INSTANCE") && input.ReferencedDocumentID != "" {
		if _, err := server.access.RequireDocument(request.Context(), input.ReferencedDocumentID,
			principal(request).ID, access.RoleViewer); err != nil {
			writeAccessError(writer, err)
			return
		}
	}
	result, err := server.workspace.ApplyCommand(
		request.Context(), request.PathValue("documentID"), input)
	server.writeDocumentResult(writer, request, result, err)
}

func (server *Server) previewCommand(writer http.ResponseWriter, request *http.Request) {
	if _, ok := server.requireDocument(writer, request, access.RoleEditor); !ok {
		return
	}
	var input workspace.CommandRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	input.ActorID = principal(request).ID
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	result, err := server.workspace.PreviewCommand(ctx, request.PathValue("documentID"), input)
	if err == nil {
		writeJSON(writer, http.StatusOK, result)
		return
	}
	if errors.Is(err, workspace.ErrNotFound) {
		writeError(writer, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, workspace.ErrValidation) || errors.Is(err, modelcore.ErrInvalidCommand) || errors.Is(err, modelcore.ErrUnsupportedCommand) {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		writeError(writer, http.StatusGatewayTimeout, "command preview timed out")
		return
	}
	writeError(writer, http.StatusInternalServerError, err.Error())
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, value any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid JSON request: "+err.Error())
		return false
	}
	return true
}

func writeWorkspaceResult(writer http.ResponseWriter, result workspace.DocumentView, err error) {
	if err == nil {
		writeJSON(writer, http.StatusOK, result)
		return
	}
	if errors.Is(err, workspace.ErrNotFound) {
		writeError(writer, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, workspace.ErrValidation) {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	slog.Error("workspace request failed", "error", err)
	writeError(writer, http.StatusInternalServerError, err.Error())
}

func (server *Server) health(writer http.ResponseWriter, request *http.Request) {
	if err := server.database.Ping(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database unavailable: "+err.Error())
		return
	}
	worker, err := server.worker.Ping(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "geometry worker unavailable: "+err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"status": "ok", "database": "ok", "workerId": worker.GetWorkerId(),
		"occtVersion":           worker.GetOcctVersion(),
		"residentGeometryCount": worker.GetResidentGeometryCount(),
	})
}

func (server *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if requestID == "" {
			buffer := make([]byte, 12)
			if _, err := rand.Read(buffer); err == nil {
				requestID = hex.EncodeToString(buffer)
			}
		}
		request.Header.Set("X-Request-ID", requestID)
		writer.Header().Set("X-Request-ID", requestID)
		spanContext := trace.SpanContextFromContext(request.Context())
		if spanContext.IsValid() {
			writer.Header().Set("Trace-ID", spanContext.TraceID().String())
		}
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID, X-CSRF-Token, Traceparent, Tracestate")
		writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		if origin := strings.TrimSpace(request.Header.Get("Origin")); origin != "" && len(server.allowedOrigins) > 0 {
			if _, allowed := server.allowedOrigins[origin]; !allowed {
				writeError(writer, http.StatusForbidden, "origin not allowed")
				return
			}
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Access-Control-Allow-Credentials", "true")
			writer.Header().Add("Vary", "Origin")
		}
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(request.URL.Path, "/api/") && !publicAPI(request.URL.Path) {
			cookie, err := request.Cookie(sessionCookieName)
			if err != nil {
				writeError(writer, http.StatusUnauthorized, "authentication required")
				return
			}
			resolved, err := server.authn.Authenticate(request.Context(), cookie.Value)
			if err != nil {
				server.clearSessionCookies(writer)
				writeError(writer, http.StatusUnauthorized, "authentication required")
				return
			}
			request = request.WithContext(access.WithPrincipal(request.Context(), resolved.Principal()))
			if request.Method != http.MethodGet && request.Method != http.MethodHead {
				csrfCookie, cookieErr := request.Cookie(csrfCookieName)
				csrfHeader := request.Header.Get("X-CSRF-Token")
				if cookieErr != nil || csrfHeader == "" || csrfCookie.Value != csrfHeader ||
					server.authn.ValidateCSRF(request.Context(), cookie.Value, csrfHeader) != nil {
					writeError(writer, http.StatusForbidden, "invalid CSRF token")
					return
				}
			}
		}
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		if recorder.status < http.StatusBadRequest && request.Method != http.MethodGet &&
			strings.HasPrefix(request.URL.Path, "/api/") && request.URL.Path != "/api/health" {
			if actor, ok := access.Principal(request.Context()); ok {
				resourceType, resourceID := auditResource(request)
				if err := server.access.RecordAudit(request.Context(), actor.ID, request.Method+" "+request.Pattern,
					resourceType, resourceID, requestID, spanContext.TraceID().String(),
					map[string]any{"path": request.URL.Path, "status": recorder.status}); err != nil {
					slog.ErrorContext(request.Context(), "record access audit", "error", err)
				}
			}
		}
		slog.InfoContext(request.Context(), "http request",
			"request_id", requestID,
			"trace_id", spanContext.TraceID().String(),
			"method", request.Method,
			"route", request.Pattern,
			"path", request.URL.Path,
			"status", recorder.status,
			"bytes", recorder.bytes,
			"duration_ms", time.Since(started).Milliseconds())
	})
}

func auditResource(request *http.Request) (string, *string) {
	if value := request.PathValue("documentID"); value != "" {
		return "DOCUMENT", &value
	}
	if value := request.PathValue("folderID"); value != "" {
		return "FOLDER", &value
	}
	if value := request.PathValue("teamID"); value != "" {
		return "TEAM", &value
	}
	return "", nil
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (recorder *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(recorder.ResponseWriter).Hijack()
}

func (recorder *statusRecorder) Flush() {
	_ = http.NewResponseController(recorder.ResponseWriter).Flush()
}

func (recorder *statusRecorder) Unwrap() http.ResponseWriter { return recorder.ResponseWriter }

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(data []byte) (int, error) {
	written, err := recorder.ResponseWriter.Write(data)
	recorder.bytes += written
	return written, err
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		slog.Error("encode HTTP response", "error", err)
	}
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}
