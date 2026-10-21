package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/occccad/occccad/internal/assistant"
)

func assistantJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid assistant request")
		return false
	}
	return true
}
func assistantPage(r *http.Request) (uint64, uint64, int) {
	after, _ := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	before, _ := strconv.ParseUint(r.URL.Query().Get("before"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return after, before, limit
}
func (server *Server) registerAssistantRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/assistant/settings", func(w http.ResponseWriter, r *http.Request) {
		v, e := server.assistant.Settings(r.Context(), principal(r).ID)
		writeAutomationResult(w, v, e)
	})
	mux.HandleFunc("PUT /api/assistant/settings", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			APIKey      string          `json:"apiKey"`
			Model       *string         `json:"model"`
			Thinking    *string         `json:"thinking"`
			TokenBudget json.RawMessage `json:"tokenBudget"`
		}
		if !assistantJSON(w, r, &input) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		actor := principal(r).ID
		if input.APIKey != "" {
			v, e := server.assistant.SaveKey(ctx, actor, input.APIKey)
			writeAutomationResult(w, v, e)
			return
		}
		settings, e := server.assistant.Settings(ctx, actor)
		if e != nil {
			writeAutomationResult(w, nil, e)
			return
		}
		generation := assistant.Generation{Model: settings.Model, Thinking: settings.Thinking}
		if input.Model != nil {
			generation.Model = *input.Model
		}
		if input.Thinking != nil {
			generation.Thinking = *input.Thinking
		}
		budget := settings.TokenBudget
		if len(input.TokenBudget) > 0 {
			if json.Unmarshal(input.TokenBudget, &budget) != nil {
				writeError(w, 400, "tokenBudget must be a positive integer or null")
				return
			}
		}
		v, e := server.assistant.SavePreferences(ctx, actor, generation, budget)
		writeAutomationResult(w, v, e)
	})
	mux.HandleFunc("GET /api/assistant/models", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		v, e := server.assistant.Models(ctx, principal(r).ID)
		writeAutomationResult(w, map[string]any{"models": v}, e)
	})
	mux.HandleFunc("DELETE /api/assistant/settings", func(w http.ResponseWriter, r *http.Request) {
		e := server.assistant.DeleteKey(r.Context(), principal(r).ID)
		writeAutomationResult(w, map[string]bool{"configured": false}, e)
	})
	const root = "/api/documents/{documentID}/assistant/runs"
	mux.HandleFunc("POST "+root, func(w http.ResponseWriter, r *http.Request) {
		var input assistant.RunInput
		if !assistantJSON(w, r, &input) {
			return
		}
		v, e := server.assistant.CreateRun(r.Context(), principal(r).ID, r.PathValue("documentID"), input)
		if e != nil {
			writeAutomationResult(w, nil, e)
			return
		}
		writeJSON(w, http.StatusAccepted, v)
	})
	mux.HandleFunc("GET "+root, func(w http.ResponseWriter, r *http.Request) {
		_, _, limit := assistantPage(r)
		v, e := server.assistant.ListRuns(r.Context(), principal(r).ID, r.PathValue("documentID"), r.URL.Query().Get("cursor"), limit)
		writeAutomationResult(w, v, e)
	})
	mux.HandleFunc("GET "+root+"/{runID}", func(w http.ResponseWriter, r *http.Request) {
		v, e := server.assistant.GetRun(r.Context(), principal(r).ID, r.PathValue("documentID"), r.PathValue("runID"))
		writeAutomationResult(w, v, e)
	})
	mux.HandleFunc("POST "+root+"/{runID}/messages", func(w http.ResponseWriter, r *http.Request) {
		var input assistant.MessageInput
		if !assistantJSON(w, r, &input) {
			return
		}
		v, t, e := server.assistant.AddMessage(r.Context(), principal(r).ID, r.PathValue("documentID"), r.PathValue("runID"), input)
		if e != nil {
			writeAutomationResult(w, nil, e)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"run": v, "turn": t})
	})
	mux.HandleFunc("GET "+root+"/{runID}/messages", func(w http.ResponseWriter, r *http.Request) {
		a, b, l := assistantPage(r)
		v, e := server.assistant.Messages(r.Context(), principal(r).ID, r.PathValue("documentID"), r.PathValue("runID"), a, b, l)
		writeAutomationResult(w, v, e)
	})
	mux.HandleFunc("GET "+root+"/{runID}/events", func(w http.ResponseWriter, r *http.Request) {
		a, b, l := assistantPage(r)
		v, e := server.assistant.Events(r.Context(), principal(r).ID, r.PathValue("documentID"), r.PathValue("runID"), a, b, l)
		writeAutomationResult(w, v, e)
	})
	mux.HandleFunc("GET "+root+"/{runID}/stages", func(w http.ResponseWriter, r *http.Request) {
		a, b, l := assistantPage(r)
		v, e := server.assistant.Stages(r.Context(), principal(r).ID, r.PathValue("documentID"), r.PathValue("runID"), a, b, l)
		writeAutomationResult(w, v, e)
	})
	mux.HandleFunc("GET "+root+"/{runID}/calls", func(w http.ResponseWriter, r *http.Request) {
		_, _, l := assistantPage(r)
		v, e := server.assistant.Calls(r.Context(), principal(r).ID, r.PathValue("documentID"), r.PathValue("runID"), r.URL.Query().Get("cursor"), l)
		writeAutomationResult(w, v, e)
	})
	mux.HandleFunc("GET "+root+"/{runID}/trace", func(w http.ResponseWriter, r *http.Request) {
		_, _, limit := assistantPage(r)
		v, e := server.assistant.Trace(r.Context(), principal(r).ID, r.PathValue("documentID"), r.PathValue("runID"), r.URL.Query().Get("cursor"), limit)
		writeAutomationResult(w, v, e)
	})
	mux.HandleFunc("GET "+root+"/{runID}/calls/{callID}", func(w http.ResponseWriter, r *http.Request) {
		v, e := server.assistant.CallTrace(r.Context(), principal(r).ID, r.PathValue("documentID"), r.PathValue("runID"), r.PathValue("callID"))
		writeAutomationResult(w, v, e)
	})
	mux.HandleFunc("POST "+root+"/{runID}/control", func(w http.ResponseWriter, r *http.Request) {
		var input assistant.RunControl
		if !assistantJSON(w, r, &input) {
			return
		}
		v, e := server.assistant.ControlRun(r.Context(), principal(r).ID, r.PathValue("documentID"), r.PathValue("runID"), input)
		writeAutomationResult(w, v, e)
	})
}
