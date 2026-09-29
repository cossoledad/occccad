package api

import (
	"net/http"
	"sort"
	"sync"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

// openDocumentRegistry is process-local live editor state. It is deliberately
// separate from durable document history and can later be replaced by a
// distributed presence service without changing the HTTP contract.
type openDocumentRegistry struct {
	mu      sync.RWMutex
	byScope map[openDocumentScope][]workspace.DocumentSummary
}

type openDocumentScope struct {
	userID      string
	workspaceID string
}

func newOpenDocumentRegistry() *openDocumentRegistry {
	return &openDocumentRegistry{byScope: make(map[openDocumentScope][]workspace.DocumentSummary)}
}

func (registry *openDocumentRegistry) Open(userID, workspaceID string, document workspace.DocumentSummary) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	scope := openDocumentScope{userID: userID, workspaceID: workspaceID}
	current := registry.byScope[scope]
	for index := range current {
		if current[index].ID == document.ID {
			current[index] = document
			registry.byScope[scope] = current
			return
		}
	}
	registry.byScope[scope] = append(current, document)
}

func (registry *openDocumentRegistry) Update(userID string, document workspace.DocumentSummary) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for scope, documents := range registry.byScope {
		if scope.userID != userID {
			continue
		}
		for index := range documents {
			if documents[index].ID == document.ID {
				documents[index] = document
				registry.byScope[scope] = documents
				break
			}
		}
	}
}

func (registry *openDocumentRegistry) Close(userID, workspaceID, documentID string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	scope := openDocumentScope{userID: userID, workspaceID: workspaceID}
	current := registry.byScope[scope]
	next := current[:0]
	for _, candidate := range current {
		if candidate.ID != documentID {
			next = append(next, candidate)
		}
	}
	if len(next) == 0 {
		delete(registry.byScope, scope)
		return
	}
	registry.byScope[scope] = next
}

func (registry *openDocumentRegistry) CloseDocument(userID, documentID string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for scope, current := range registry.byScope {
		if scope.userID != userID {
			continue
		}
		next := current[:0]
		for _, candidate := range current {
			if candidate.ID != documentID {
				next = append(next, candidate)
			}
		}
		if len(next) == 0 {
			delete(registry.byScope, scope)
		} else {
			registry.byScope[scope] = next
		}
	}
}

func (registry *openDocumentRegistry) CloseAll(userID string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for scope := range registry.byScope {
		if scope.userID == userID {
			delete(registry.byScope, scope)
		}
	}
}

func (registry *openDocumentRegistry) List(userID, workspaceID string) []workspace.DocumentSummary {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return append([]workspace.DocumentSummary(nil), registry.byScope[openDocumentScope{userID: userID, workspaceID: workspaceID}]...)
}

func (registry *openDocumentRegistry) sessionCount() int {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	total := 0
	for _, documents := range registry.byScope {
		total += len(documents)
	}
	return total
}

type monitoredOpenDocument struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Sessions int    `json:"sessions"`
}

func (registry *openDocumentRegistry) monitoringDocuments() []monitoredOpenDocument {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	byID := map[string]monitoredOpenDocument{}
	for _, documents := range registry.byScope {
		for _, document := range documents {
			item := byID[document.ID]
			item.ID, item.Name, item.Type, item.Sessions = document.ID, document.Name, document.Type, item.Sessions+1
			byID[document.ID] = item
		}
	}
	result := make([]monitoredOpenDocument, 0, len(byID))
	for _, item := range byID {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (server *Server) listOpenDocuments(writer http.ResponseWriter, request *http.Request) {
	userID := principal(request).ID
	workspaceID := openDocumentWorkspaceID(request)
	documents := server.openDocuments.List(userID, workspaceID)
	visible := make([]workspace.DocumentSummary, 0, len(documents))
	for _, document := range documents {
		role, err := server.access.EffectiveDocumentRole(request.Context(), document.ID, userID)
		if err != nil {
			server.openDocuments.Close(userID, workspaceID, document.ID)
			continue
		}
		document.Permission = string(role)
		visible = append(visible, document)
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"documents": visible,
	})
}

func (server *Server) openDocument(writer http.ResponseWriter, request *http.Request) {
	role, ok := server.requireDocument(writer, request, access.RoleViewer)
	if !ok {
		return
	}
	result, err := server.workspace.GetDocument(request.Context(), request.PathValue("documentID"), principal(request).ID)
	if err == nil {
		result.Document.Permission = string(role)
		server.openDocuments.Open(principal(request).ID, openDocumentWorkspaceID(request), result.Document)
		_ = server.workspace.MarkDocumentOpened(request.Context(), result.Document.ID)
	}
	writeWorkspaceResult(writer, result, err)
}

func (server *Server) closeOpenDocument(writer http.ResponseWriter, request *http.Request) {
	server.openDocuments.Close(principal(request).ID, openDocumentWorkspaceID(request), request.PathValue("documentID"))
	writer.WriteHeader(http.StatusNoContent)
}

func openDocumentWorkspaceID(request *http.Request) string {
	const fallback = "default"
	workspaceID := request.Header.Get("X-OCCCCAD-Workspace-ID")
	if workspaceID == "" || len(workspaceID) > 128 {
		return fallback
	}
	return workspaceID
}
