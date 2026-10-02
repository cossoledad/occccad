package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

// These bounded operations must not block the WebSocket reader or cancellation.
// The Session itself admits one in-flight solve; the client merges one latest
// unsent target instead of repeatedly cancelling ongoing numerical work.
func (client *realtimeClient) startAssemblyOperation(server *Server, ctx context.Context, envelope realtimeEnvelope) {
	var identity struct {
		DocumentID string `json:"documentId"`
		AnalysisID string `json:"analysisId"`
	}
	if json.Unmarshal(envelope.Payload, &identity) != nil || identity.DocumentID == "" {
		client.sendError(envelope.ID, "INVALID_PAYLOAD", "documentId required", false)
		return
	}
	client.mu.Lock()
	if client.previewSlots == nil {
		client.previewSlots = make(chan struct{}, 4)
	}
	slots := client.previewSlots
	if client.assemblySessions == nil {
		client.assemblySessions = map[string]string{}
	}
	client.mu.Unlock()
	select {
	case slots <- struct{}{}:
	default:
		client.sendError(envelope.ID, "REALTIME_BUSY", "bounded assembly work slots are full", true)
		return
	}
	work, cancel := context.WithTimeout(ctx, 15*time.Second)
	analysisKey := ""
	if envelope.Type == "assembly.conflict.analyze.v1" {
		if identity.AnalysisID == "" || len(identity.AnalysisID) > 128 {
			cancel()
			<-slots
			client.sendError(envelope.ID, "INVALID_PAYLOAD", "analysisId required", false)
			return
		}
		analysisKey = identity.DocumentID + "/" + identity.AnalysisID
		client.mu.Lock()
		if client.assemblyAnalyses == nil {
			client.assemblyAnalyses = map[string]context.CancelFunc{}
		}
		if _, exists := client.assemblyAnalyses[analysisKey]; exists {
			client.mu.Unlock()
			cancel()
			<-slots
			client.sendError(envelope.ID, "ASSEMBLY_ANALYSIS_BUSY", "analysisId already in flight", false)
			return
		}
		client.assemblyAnalyses[analysisKey] = cancel
		client.mu.Unlock()
	}
	go func() {
		defer func() {
			<-slots
			if analysisKey != "" {
				client.mu.Lock()
				delete(client.assemblyAnalyses, analysisKey)
				client.mu.Unlock()
			}
		}()
		defer cancel()
		role := access.RoleEditor
		if envelope.Type == "assembly.conflict.analyze.v1" {
			role = access.RoleViewer
		}
		_, err := server.access.RequireDocument(work, identity.DocumentID, client.actor.ID, role)
		var response any
		if err == nil {
			switch envelope.Type {
			case "assembly.interaction.begin.v1":
				var input workspace.AssemblyInteractionBegin
				err = json.Unmarshal(envelope.Payload, &input)
				if err == nil && input.EditContext != nil {
					_, err = server.access.RequireDocument(work, input.EditContext.RootDocumentID, client.actor.ID, access.RoleViewer)
				}
				if err == nil {
					var opened workspace.AssemblyInteractionOpened
					opened, err = server.workspace.BeginAssemblyInteraction(work, identity.DocumentID, client.actor.ID, input)
					response = opened
					if err == nil {
						client.mu.Lock()
						if len(client.assemblySessions) >= 32 {
							client.mu.Unlock()
							server.workspace.CancelAssemblyInteraction(identity.DocumentID, client.actor.ID, opened.SessionID)
							err = fmt.Errorf("connection Session limit reached")
						} else {
							client.assemblySessions[opened.SessionID] = identity.DocumentID
							client.mu.Unlock()
						}
					}
				}
			case "assembly.interaction.update.v1":
				var input workspace.AssemblyInteractionUpdate
				err = json.Unmarshal(envelope.Payload, &input)
				if err == nil {
					client.mu.RLock()
					bound := client.assemblySessions[input.SessionID] == identity.DocumentID
					client.mu.RUnlock()
					if !bound {
						err = fmt.Errorf("Session is not bound to this connection")
					} else {
						response, err = server.workspace.UpdateAssemblyInteraction(work, identity.DocumentID, client.actor.ID, input)
					}
				}
			case "assembly.conflict.analyze.v1":
				var input workspace.AssemblyConflictRequest
				err = json.Unmarshal(envelope.Payload, &input)
				if err == nil {
					response, err = server.workspace.AnalyzeAssemblyConflicts(work, identity.DocumentID, client.actor.ID, input)
				}
			}
		}
		if err != nil {
			client.sendError(envelope.ID, "ASSEMBLY_OPERATION_FAILED", err.Error(), false)
			return
		}
		select {
		case <-client.done:
			if opened, ok := response.(workspace.AssemblyInteractionOpened); ok {
				server.workspace.CancelAssemblyInteraction(identity.DocumentID, client.actor.ID, opened.SessionID)
			}
			return
		default:
		}
		client.sendResponse(envelope.ID, envelope.Type, response)
	}()
}

func (client *realtimeClient) cancelAssemblyAnalysis(envelope realtimeEnvelope) {
	var input struct {
		DocumentID string `json:"documentId"`
		AnalysisID string `json:"analysisId"`
	}
	if json.Unmarshal(envelope.Payload, &input) != nil || input.DocumentID == "" || input.AnalysisID == "" {
		client.sendError(envelope.ID, "INVALID_PAYLOAD", "documentId and analysisId required", false)
		return
	}
	client.mu.Lock()
	cancel := client.assemblyAnalyses[input.DocumentID+"/"+input.AnalysisID]
	client.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	client.sendResponse(envelope.ID, envelope.Type, map[string]any{"cancelled": cancel != nil})
}
func (client *realtimeClient) cancelAssemblyOperation(server *Server, envelope realtimeEnvelope) {
	var input struct {
		DocumentID string `json:"documentId"`
		SessionID  string `json:"sessionId"`
	}
	if json.Unmarshal(envelope.Payload, &input) != nil || input.DocumentID == "" || input.SessionID == "" {
		client.sendError(envelope.ID, "INVALID_PAYLOAD", "documentId and sessionId required", false)
		return
	}
	client.mu.Lock()
	bound := client.assemblySessions[input.SessionID] == input.DocumentID
	if bound {
		delete(client.assemblySessions, input.SessionID)
	}
	client.mu.Unlock()
	if bound {
		server.workspace.CancelAssemblyInteraction(input.DocumentID, client.actor.ID, input.SessionID)
	}
	client.sendResponse(envelope.ID, envelope.Type, map[string]any{"cancelled": bound})
}
