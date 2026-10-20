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
			APIKey   string  `json:"apiKey"`
			Model    *string `json:"model"`
			Thinking *string `json:"thinking"`
			Budget   *string `json:"budget"`
		}
		if !decodeAutomationJSON(w, r, &input) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		var value assistant.Settings
		var err error
		if input.APIKey != "" {
			value, err = server.assistant.SaveKey(ctx, principal(r).ID, input.APIKey)
		} else {
			value, err = server.assistant.Settings(ctx, principal(r).ID)
			if err == nil {
				generation := assistant.Generation{Model: value.Model, Thinking: value.Thinking, Budget: value.Budget}
				if input.Model != nil {
					generation.Model = *input.Model
				}
				if input.Thinking != nil {
					generation.Thinking = *input.Thinking
				}
				if input.Budget != nil {
					generation.Budget = *input.Budget
				}
				value, err = server.assistant.SavePreferences(ctx, principal(r).ID, generation)
			}
		}
		writeAutomationResult(w, value, err)
	})
	mux.HandleFunc("GET /api/assistant/models", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		value, err := server.assistant.Models(ctx, principal(r).ID)
		writeAutomationResult(w, map[string]any{"models": value}, err)
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
