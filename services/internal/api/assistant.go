package api

import (
	"context"
	"net/http"
	"time"

	"github.com/occccad/occccad/internal/assistant"
)

func (server *Server) registerAssistantRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/assistant/settings", func(w http.ResponseWriter, r *http.Request) {
		value, err := server.assistant.Settings(r.Context(), principal(r).ID)
		writeAutomationResult(w, value, err)
	})
	mux.HandleFunc("PUT /api/assistant/settings", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			APIKey string `json:"apiKey"`
		}
		if !decodeAutomationJSON(w, r, &input) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		value, err := server.assistant.SaveKey(ctx, principal(r).ID, input.APIKey)
		writeAutomationResult(w, value, err)
	})
	mux.HandleFunc("DELETE /api/assistant/settings", func(w http.ResponseWriter, r *http.Request) {
		err := server.assistant.DeleteKey(r.Context(), principal(r).ID)
		writeAutomationResult(w, map[string]bool{"configured": false}, err)
	})
	mux.HandleFunc("GET /api/documents/{documentID}/assistant/turns", func(w http.ResponseWriter, r *http.Request) {
		value, err := server.assistant.Conversation(r.Context(), principal(r).ID, r.PathValue("documentID"))
		writeAutomationResult(w, value, err)
	})
	mux.HandleFunc("POST /api/documents/{documentID}/assistant/turns", func(w http.ResponseWriter, r *http.Request) {
		var input assistant.TurnInput
		if !decodeAutomationJSON(w, r, &input) {
			return
		}
		value, err := server.assistant.Run(r.Context(), principal(r).ID, r.PathValue("documentID"), input)
		writeAutomationResult(w, value, err)
	})
	mux.HandleFunc("POST /api/documents/{documentID}/assistant/turns/{turnID}/cancel", func(w http.ResponseWriter, r *http.Request) {
		value, err := server.assistant.Cancel(r.Context(), principal(r).ID, r.PathValue("documentID"), r.PathValue("turnID"))
		writeAutomationResult(w, value, err)
	})
}
