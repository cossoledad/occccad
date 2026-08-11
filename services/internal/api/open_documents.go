package api

import (
	"net/http"
	"sync"

	"github.com/occccad/occccad/internal/workspace"
)

// openDocumentRegistry is process-local live editor state. It is deliberately
// separate from durable document history and can later be replaced by a
// distributed presence service without changing the HTTP contract.
type openDocumentRegistry struct {
	mu     sync.RWMutex
	byUser map[string][]workspace.DocumentSummary
}

func newOpenDocumentRegistry() *openDocumentRegistry {
	return &openDocumentRegistry{byUser: make(map[string][]workspace.DocumentSummary)}
}

func (registry *openDocumentRegistry) Open(userID string, document workspace.DocumentSummary) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	current := registry.byUser[userID]
	next := make([]workspace.DocumentSummary, 0, len(current)+1)
	next = append(next, document)
	for _, candidate := range current {
		if candidate.ID != document.ID {
			next = append(next, candidate)
		}
	}
	registry.byUser[userID] = next
}

func (registry *openDocumentRegistry) Update(userID string, document workspace.DocumentSummary) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for index := range registry.byUser[userID] {
		if registry.byUser[userID][index].ID == document.ID {
			registry.byUser[userID][index] = document
			return
		}
	}
}

func (registry *openDocumentRegistry) Close(userID, documentID string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	current := registry.byUser[userID]
	next := current[:0]
	for _, candidate := range current {
		if candidate.ID != documentID {
			next = append(next, candidate)
		}
	}
	if len(next) == 0 {
		delete(registry.byUser, userID)
		return
	}
	registry.byUser[userID] = next
}

func (registry *openDocumentRegistry) CloseAll(userID string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	delete(registry.byUser, userID)
}

func (registry *openDocumentRegistry) List(userID string) []workspace.DocumentSummary {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return append([]workspace.DocumentSummary(nil), registry.byUser[userID]...)
}

func (server *Server) listOpenDocuments(writer http.ResponseWriter, request *http.Request) {
	userID := principal(request).ID
	documents := server.openDocuments.List(userID)
	visible := make([]workspace.DocumentSummary, 0, len(documents))
	for _, document := range documents {
		role, err := server.access.EffectiveDocumentRole(request.Context(), document.ID, userID)
		if err != nil {
			server.openDocuments.Close(userID, document.ID)
			continue
		}
		document.Permission = string(role)
		visible = append(visible, document)
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"documents": visible,
	})
}

func (server *Server) closeOpenDocument(writer http.ResponseWriter, request *http.Request) {
	server.openDocuments.Close(principal(request).ID, request.PathValue("documentID"))
	writer.WriteHeader(http.StatusNoContent)
}
