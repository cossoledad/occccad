package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

const realtimePreviewTimeout = 15 * time.Second

type realtimePreview struct {
	sequence  uint64
	requestID string
	previewID string
	cancel    context.CancelFunc
	discard   func(string)
	expires   time.Time
}

type previewRequest struct {
	DocumentID    string                   `json:"documentId"`
	InteractionID string                   `json:"interactionId"`
	Sequence      uint64                   `json:"previewSequence"`
	Command       workspace.CommandRequest `json:"command"`
}

// Called by the reader, never by the serial command executor: cancel must remain
// responsive while geometry is being evaluated. Slots bound even canceled work
// until its evaluator has actually returned.
func (client *realtimeClient) startPreview(server *Server, ctx context.Context, envelope realtimeEnvelope) {
	var input previewRequest
	if json.Unmarshal(envelope.Payload, &input) != nil || input.DocumentID == "" || input.InteractionID == "" || len(input.InteractionID) > 128 || input.Sequence == 0 || input.Command.Type == "" || input.Command.RequestID == "" {
		client.sendError(envelope.ID, "INVALID_PAYLOAD", "documentId, interactionId, previewSequence and command requestId/type are required", false)
		return
	}
	input.Command.ActorID = client.actor.ID
	input.Command.InteractionID = input.InteractionID
	input.Command.PreviewSequence = input.Sequence
	key := input.DocumentID + "/" + input.InteractionID
	client.mu.Lock()
	if client.previews == nil {
		client.previews = map[string]*realtimePreview{}
	}
	if client.previewSlots == nil {
		client.previewSlots = make(chan struct{}, 4)
	}
	for k, p := range client.previews {
		if time.Now().After(p.expires) {
			p.cancel()
			p.discard(p.previewID)
			delete(client.previews, k)
		}
	}
	old := client.previews[key]
	if old != nil && input.Sequence <= old.sequence {
		client.mu.Unlock()
		client.sendError(envelope.ID, "STALE_PREVIEW", "preview sequence is not newer", false)
		return
	}
	if old == nil && len(client.previews) >= 64 {
		client.mu.Unlock()
		client.sendError(envelope.ID, "PREVIEW_BUSY", "too many interaction sessions", true)
		return
	}
	if old != nil {
		old.cancel()
		old.discard(old.previewID)
		if old.requestID != "" {
			client.sendResponse(old.requestID, "workspace.preview.canceled.v1", map[string]any{"interactionId": input.InteractionID, "previewSequence": old.sequence})
		}
	}
	// Preserve the high-water mark even when admission fails.
	work, cancel := context.WithTimeout(ctx, realtimePreviewTimeout)
	p := &realtimePreview{sequence: input.Sequence, requestID: envelope.ID, cancel: cancel, expires: time.Now().Add(time.Minute), discard: func(id string) { server.workspace.DiscardPreview(input.DocumentID, client.actor.ID, id) }}
	client.previews[key] = p
	select {
	case client.previewSlots <- struct{}{}:
	default:
		cancel()
		p.requestID = ""
		client.mu.Unlock()
		client.sendError(envelope.ID, "PREVIEW_BUSY", "preview evaluator capacity exhausted", true)
		return
	}
	client.mu.Unlock()
	go func() {
		work, finishTiming := realtimeOperationTiming(work, envelope)
		defer finishTiming()
		defer func() { cancel(); <-client.previewSlots }()
		var result workspace.CommandPreview
		_, err := server.access.RequireDocument(work, input.DocumentID, client.actor.ID, access.RoleEditor)
		if err == nil {
			err = server.requireCommandReferences(work, client.actor.ID, input.DocumentID, input.Command)
		}
		if err == nil {
			result, err = server.workspace.PreviewCommand(work, input.DocumentID, input.Command)
		}
		if err == nil {
			head, headErr := server.realtimeHead(work, input.DocumentID)
			if headErr != nil {
				err = headErr
			} else if head.VersionID != result.BaseVersionID || head.Sequence != result.BaseSequence {
				err = fmt.Errorf("%w: STALE_PREVIEW_BASE", workspace.ErrValidation)
			}
		}
		client.mu.Lock()
		defer client.mu.Unlock()
		if client.previews[key] != p || errors.Is(work.Err(), context.Canceled) {
			p.discard(result.PreviewID)
			return
		}
		p.requestID = ""
		if err == nil {
			err = work.Err()
		}
		if err != nil {
			p.discard(result.PreviewID)
			failure := realtimeDomainError(err)
			client.enqueue(newRealtimeEnvelope("error", "workspace.preview.failed.v1", envelope.ID, nil, nil, &failure))
			return
		}
		p.previewID = result.PreviewID
		// URLs are session/actor-scoped grants checked against the candidate cache.
		// Never mutate the cached descriptor's map or expose inline mesh arrays.
		if result.Artifact != nil {
			a := *result.Artifact
			a.RepresentationKind = "TRANSIENT_PREVIEW"
			a.Representations = map[string]workspace.Representation{}
			for role, ref := range result.Artifact.Representations {
				ref.URL = "/api/documents/" + url.PathEscape(input.DocumentID) + "/representations/" + url.PathEscape(ref.ObjectID) + "?previewId=" + url.QueryEscape(result.PreviewID)
				a.Representations[role] = ref
			}
			result.Artifact = &a
		}
		client.sendResponse(envelope.ID, "workspace.preview.ready.v1", map[string]any{"documentId": input.DocumentID, "interactionId": input.InteractionID, "previewSequence": input.Sequence, "preview": result})
	}()
}

func (client *realtimeClient) cancelPreview(envelope realtimeEnvelope) {
	var input previewRequest
	if json.Unmarshal(envelope.Payload, &input) != nil || input.DocumentID == "" || input.InteractionID == "" || input.Sequence == 0 {
		client.sendError(envelope.ID, "INVALID_PAYLOAD", "preview identity required", false)
		return
	}
	client.mu.Lock()
	if p := client.previews[input.DocumentID+"/"+input.InteractionID]; p != nil && input.Sequence >= p.sequence {
		p.cancel()
		p.discard(p.previewID)
		p.previewID = ""
		if p.requestID != "" {
			client.sendResponse(p.requestID, "workspace.preview.canceled.v1", map[string]any{"interactionId": input.InteractionID, "previewSequence": p.sequence})
			p.requestID = ""
		}
		p.sequence = input.Sequence
	}
	client.mu.Unlock()
	client.sendResponse(envelope.ID, "workspace.preview.canceled.v1", map[string]any{"documentId": input.DocumentID, "interactionId": input.InteractionID, "previewSequence": input.Sequence})
}
