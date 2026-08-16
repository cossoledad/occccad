package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

const (
	realtimeProtocol  = "occccad.realtime.v1"
	realtimeQueueSize = 128
	realtimeMaxBytes  = 1 << 20
)

type realtimeEnvelope struct {
	Protocol      string          `json:"protocol"`
	ID            string          `json:"id"`
	Kind          string          `json:"kind"`
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlationId,omitempty"`
	Sequence      *uint64         `json:"sequence,omitempty"`
	SentAt        string          `json:"sentAt"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Error         *realtimeError  `json:"error,omitempty"`
}

type realtimeError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type realtimeClient struct {
	id        string
	actor     access.User
	conn      *websocket.Conn
	hub       *realtimeHub
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once

	mu            sync.RWMutex
	subscriptions map[string]string
	acknowledged  map[string]uint64
}

func (client *realtimeClient) close() {
	client.closeOnce.Do(func() {
		close(client.done)
		if client.conn != nil {
			_ = client.conn.Close()
		}
	})
}

func (client *realtimeClient) subscribe(documentID, workspaceID string) {
	client.mu.Lock()
	client.subscriptions[documentID] = workspaceID
	client.mu.Unlock()
	client.hub.subscribe(documentID, client)
}

func (client *realtimeClient) unsubscribe(documentID string) {
	client.mu.Lock()
	delete(client.subscriptions, documentID)
	client.mu.Unlock()
	client.hub.unsubscribe(documentID, client)
}

type realtimeHub struct {
	mu          sync.RWMutex
	byDocument  map[string]map[*realtimeClient]struct{}
	connections map[*realtimeClient]struct{}
}

func newRealtimeHub() *realtimeHub {
	return &realtimeHub{byDocument: map[string]map[*realtimeClient]struct{}{},
		connections: map[*realtimeClient]struct{}{}}
}

func (hub *realtimeHub) add(client *realtimeClient) {
	hub.mu.Lock()
	hub.connections[client] = struct{}{}
	hub.mu.Unlock()
}

func (hub *realtimeHub) subscribe(documentID string, client *realtimeClient) {
	hub.mu.Lock()
	if hub.byDocument[documentID] == nil {
		hub.byDocument[documentID] = map[*realtimeClient]struct{}{}
	}
	hub.byDocument[documentID][client] = struct{}{}
	hub.mu.Unlock()
}

func (hub *realtimeHub) unsubscribe(documentID string, client *realtimeClient) {
	hub.mu.Lock()
	delete(hub.byDocument[documentID], client)
	if len(hub.byDocument[documentID]) == 0 {
		delete(hub.byDocument, documentID)
	}
	hub.mu.Unlock()
}

func (hub *realtimeHub) remove(client *realtimeClient) {
	hub.mu.Lock()
	delete(hub.connections, client)
	for documentID, clients := range hub.byDocument {
		delete(clients, client)
		if len(clients) == 0 {
			delete(hub.byDocument, documentID)
		}
	}
	hub.mu.Unlock()
	client.close()
}

func (hub *realtimeHub) broadcast(documentID string, message []byte) {
	hub.mu.RLock()
	clients := make([]*realtimeClient, 0, len(hub.byDocument[documentID]))
	for client := range hub.byDocument[documentID] {
		clients = append(clients, client)
	}
	hub.mu.RUnlock()
	for _, client := range clients {
		select {
		case client.send <- message:
		default:
			// A bounded queue is the backpressure boundary. Disconnecting forces
			// the browser to reconnect and obtain an authoritative snapshot.
			hub.remove(client)
		}
	}
}

func (hub *realtimeHub) broadcastUser(userID string, message []byte) int {
	hub.mu.RLock()
	clients := make([]*realtimeClient, 0)
	for client := range hub.connections {
		if client.actor.ID == userID {
			clients = append(clients, client)
		}
	}
	hub.mu.RUnlock()
	delivered := 0
	for _, client := range clients {
		select {
		case client.send <- message:
			delivered++
		default:
			hub.remove(client)
		}
	}
	return delivered
}

func (hub *realtimeHub) close() {
	hub.mu.RLock()
	clients := make([]*realtimeClient, 0, len(hub.connections))
	for client := range hub.connections {
		clients = append(clients, client)
	}
	hub.mu.RUnlock()
	for _, client := range clients {
		hub.remove(client)
	}
}

func (server *Server) realtimeConnection(writer http.ResponseWriter, request *http.Request) {
	if !strings.Contains(request.Header.Get("Sec-WebSocket-Protocol"), realtimeProtocol) {
		writeError(writer, http.StatusBadRequest, "Sec-WebSocket-Protocol "+realtimeProtocol+" is required")
		return
	}
	upgrader := websocket.Upgrader{
		Subprotocols: []string{realtimeProtocol},
		CheckOrigin:  server.realtimeOriginAllowed,
	}
	connection, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		return
	}
	client := &realtimeClient{id: uuid.NewString(), actor: principal(request), conn: connection,
		hub: server.realtime, send: make(chan []byte, realtimeQueueSize), done: make(chan struct{}),
		subscriptions: map[string]string{}, acknowledged: map[string]uint64{}}
	server.realtime.add(client)
	defer server.realtime.remove(client)
	go client.writeLoop()
	client.readLoop(server, request)
}

func (server *Server) realtimeOriginAllowed(request *http.Request) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	if len(server.allowedOrigins) > 0 {
		_, allowed := server.allowedOrigins[origin]
		return allowed
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(parsed.Host, request.Host) {
		return true
	}
	forwardedHost := strings.TrimSpace(strings.Split(request.Header.Get("X-Forwarded-Host"), ",")[0])
	return forwardedHost != "" && strings.EqualFold(parsed.Host, forwardedHost)
}

func (client *realtimeClient) readLoop(server *Server, request *http.Request) {
	client.conn.SetReadLimit(realtimeMaxBytes)
	_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.conn.SetPongHandler(func(string) error {
		return client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	if !client.initialize(server, request) {
		return
	}
	for {
		var envelope realtimeEnvelope
		if err := client.conn.ReadJSON(&envelope); err != nil {
			return
		}
		if envelope.Protocol != realtimeProtocol || envelope.ID == "" ||
			(envelope.Kind != "request" && envelope.Kind != "ack") {
			client.sendError(envelope.ID, "INVALID_ENVELOPE", "invalid realtime request envelope", false)
			continue
		}
		if envelope.Kind == "ack" {
			client.handleAcknowledgement(envelope)
			continue
		}
		client.handleRequest(server, request.Context(), envelope)
	}
}

func (client *realtimeClient) handleAcknowledgement(envelope realtimeEnvelope) {
	if envelope.Type != "stream.ack.v1" || envelope.Sequence == nil {
		return
	}
	var payload struct {
		DocumentID string `json:"documentId"`
	}
	if json.Unmarshal(envelope.Payload, &payload) != nil || payload.DocumentID == "" {
		return
	}
	client.mu.Lock()
	if _, subscribed := client.subscriptions[payload.DocumentID]; subscribed &&
		*envelope.Sequence > client.acknowledged[payload.DocumentID] {
		client.acknowledged[payload.DocumentID] = *envelope.Sequence
	}
	client.mu.Unlock()
}

func (client *realtimeClient) initialize(server *Server, request *http.Request) bool {
	_ = client.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var envelope realtimeEnvelope
	if err := client.conn.ReadJSON(&envelope); err != nil {
		return false
	}
	var payload struct {
		CSRFToken string `json:"csrfToken"`
	}
	if envelope.Protocol != realtimeProtocol || envelope.Kind != "request" ||
		envelope.Type != "connection.initialize.v1" || json.Unmarshal(envelope.Payload, &payload) != nil {
		client.sendError(envelope.ID, "INITIALIZATION_REQUIRED", "connection.initialize.v1 is required", false)
		return false
	}
	session, err := request.Cookie(sessionCookieName)
	if err != nil || server.authn.ValidateCSRF(request.Context(), session.Value, payload.CSRFToken) != nil {
		client.sendError(envelope.ID, "INVALID_CSRF", "invalid CSRF token", false)
		return false
	}
	_ = client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.sendResponse(envelope.ID, "connection.ready.v1", map[string]any{
		"connectionId": client.id, "heartbeatIntervalMs": 25000, "maxMessageBytes": realtimeMaxBytes,
	})
	return true
}

func (client *realtimeClient) handleRequest(server *Server, ctx context.Context, envelope realtimeEnvelope) {
	switch envelope.Type {
	case "document.subscribe.v1":
		server.handleRealtimeSubscribe(ctx, client, envelope)
	case "document.unsubscribe.v1":
		var payload struct {
			DocumentID string `json:"documentId"`
		}
		if json.Unmarshal(envelope.Payload, &payload) != nil || payload.DocumentID == "" {
			client.sendError(envelope.ID, "INVALID_PAYLOAD", "documentId is required", false)
			return
		}
		client.unsubscribe(payload.DocumentID)
		client.sendResponse(envelope.ID, "document.unsubscribed.v1", map[string]string{"documentId": payload.DocumentID})
	case "workspace.command.execute.v1":
		server.handleRealtimeCommand(ctx, client, envelope)
	default:
		client.sendError(envelope.ID, "UNSUPPORTED_MESSAGE", "unsupported realtime message type", false)
	}
}

func (server *Server) handleRealtimeSubscribe(ctx context.Context, client *realtimeClient, envelope realtimeEnvelope) {
	var payload struct {
		DocumentID string `json:"documentId"`
	}
	if json.Unmarshal(envelope.Payload, &payload) != nil || payload.DocumentID == "" {
		client.sendError(envelope.ID, "INVALID_PAYLOAD", "documentId is required", false)
		return
	}
	role, err := server.access.RequireDocument(ctx, payload.DocumentID, client.actor.ID, access.RoleViewer)
	if err != nil {
		client.sendDomainError(envelope.ID, err)
		return
	}
	var workspaceID string
	var sequence uint64
	if err := server.database.QueryRow(ctx, `SELECT id::text,head_sequence FROM occccad.workspaces
		WHERE document_id=$1 AND name='main'`, payload.DocumentID).Scan(&workspaceID, &sequence); err != nil {
		client.sendDomainError(envelope.ID, err)
		return
	}
	client.subscribe(payload.DocumentID, workspaceID)
	view, err := server.workspace.GetDocument(ctx, payload.DocumentID, client.actor.ID)
	if err != nil {
		client.unsubscribe(payload.DocumentID)
		client.sendDomainError(envelope.ID, err)
		return
	}
	view.Document.Permission = string(role)
	// Read the sequence after the view. Because registration happens first,
	// any commit racing with this snapshot is also delivered as an event and
	// the client can invalidate the snapshot without a lost-update window.
	_ = server.database.QueryRow(ctx, `SELECT head_sequence FROM occccad.workspaces WHERE id=$1`,
		workspaceID).Scan(&sequence)
	client.sendResponse(envelope.ID, "document.subscribed.v1", map[string]any{
		"documentId": payload.DocumentID, "workspaceId": workspaceID, "sequence": sequence, "view": view,
	})
}

func (server *Server) handleRealtimeCommand(ctx context.Context, client *realtimeClient, envelope realtimeEnvelope) {
	var payload struct {
		DocumentID string                   `json:"documentId"`
		Command    workspace.CommandRequest `json:"command"`
	}
	if json.Unmarshal(envelope.Payload, &payload) != nil || payload.DocumentID == "" || payload.Command.Type == "" {
		client.sendError(envelope.ID, "INVALID_PAYLOAD", "documentId and command.type are required", false)
		return
	}
	if _, err := server.access.RequireDocument(ctx, payload.DocumentID, client.actor.ID, access.RoleEditor); err != nil {
		client.sendDomainError(envelope.ID, err)
		return
	}
	payload.Command.ActorID = client.actor.ID
	if payload.Command.RequestID == "" {
		payload.Command.RequestID = envelope.ID
	}
	if strings.EqualFold(payload.Command.Type, "INSERT_INSTANCE") && payload.Command.ReferencedDocumentID != "" {
		if _, err := server.access.RequireDocument(ctx, payload.Command.ReferencedDocumentID,
			client.actor.ID, access.RoleViewer); err != nil {
			client.sendDomainError(envelope.ID, err)
			return
		}
	}
	view, err := server.workspace.ApplyCommand(ctx, payload.DocumentID, payload.Command)
	if err != nil {
		client.sendDomainError(envelope.ID, err)
		return
	}
	role, _ := server.access.EffectiveDocumentRole(ctx, payload.DocumentID, client.actor.ID)
	view.Document.Permission = string(role)
	server.openDocuments.Update(client.actor.ID, view.Document)
	if err := server.enqueueDocumentPreviews(ctx, view, client.actor.ID); err != nil {
		slog.ErrorContext(ctx, "enqueue realtime document previews", "error", err)
	}
	client.sendResponse(envelope.ID, "workspace.command.completed.v1", map[string]any{
		"documentId": payload.DocumentID, "requestId": payload.Command.RequestID, "view": view,
	})
}

func (client *realtimeClient) writeLoop() {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case message := <-client.send:
			_ = client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				client.close()
				return
			}
		case <-ticker.C:
			_ = client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
				client.close()
				return
			}
		case <-client.done:
			return
		}
	}
}

func (client *realtimeClient) sendResponse(correlationID, messageType string, payload any) {
	client.enqueue(newRealtimeEnvelope("response", messageType, correlationID, nil, payload, nil))
}

func (client *realtimeClient) sendError(correlationID, code, message string, retryable bool) {
	client.enqueue(newRealtimeEnvelope("error", "request.failed.v1", correlationID, nil, nil,
		&realtimeError{Code: code, Message: message, Retryable: retryable}))
}

func (client *realtimeClient) sendDomainError(correlationID string, err error) {
	code, retryable := "INTERNAL", true
	switch {
	case strings.Contains(err.Error(), "CONFLICT"):
		code, retryable = "CHANGESET_CONFLICT", false
	case errors.Is(err, access.ErrForbidden):
		code, retryable = "FORBIDDEN", false
	case errors.Is(err, access.ErrNotFound), errors.Is(err, workspace.ErrNotFound):
		code, retryable = "NOT_FOUND", false
	case errors.Is(err, access.ErrValidation), errors.Is(err, workspace.ErrValidation):
		code, retryable = "VALIDATION_FAILED", false
	}
	client.sendError(correlationID, code, err.Error(), retryable)
}

func (client *realtimeClient) enqueue(envelope realtimeEnvelope) {
	message, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	select {
	case client.send <- message:
	case <-client.done:
	default:
		client.hub.remove(client)
	}
}

func newRealtimeEnvelope(kind, messageType, correlationID string, sequence *uint64, payload any,
	failure *realtimeError) realtimeEnvelope {
	var encoded json.RawMessage
	if payload != nil {
		encoded, _ = json.Marshal(payload)
	}
	return realtimeEnvelope{Protocol: realtimeProtocol, ID: uuid.NewString(), Kind: kind, Type: messageType,
		CorrelationID: correlationID, Sequence: sequence, SentAt: time.Now().UTC().Format(time.RFC3339Nano),
		Payload: encoded, Error: failure}
}

func (server *Server) dispatchRealtimeOutbox(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := server.publishRealtimeBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.ErrorContext(ctx, "publish realtime outbox", "error", err)
			}
			if err := server.publishJobRealtimeBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.ErrorContext(ctx, "publish job realtime outbox", "error", err)
			}
		}
	}
}

func (server *Server) publishJobRealtimeBatch(ctx context.Context) error {
	rows, err := server.database.Query(ctx, `SELECT e.id::text,e.aggregate_id::text,j.requested_by_user_id::text
		FROM occccad.outbox_events e JOIN occccad.jobs j ON j.id=e.aggregate_id
		WHERE e.aggregate_type='JOB' AND e.published_at IS NULL
		ORDER BY e.created_at,e.id LIMIT 100`)
	if err != nil {
		return err
	}
	type event struct{ id, jobID, userID string }
	events := []event{}
	for rows.Next() {
		var item event
		if err := rows.Scan(&item.id, &item.jobID, &item.userID); err != nil {
			rows.Close()
			return err
		}
		events = append(events, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range events {
		job, err := server.jobs.Get(ctx, item.jobID)
		if err != nil {
			return err
		}
		envelope := newRealtimeEnvelope("event", "job.state.changed.v1", "", nil,
			map[string]any{"job": job}, nil)
		encoded, _ := json.Marshal(envelope)
		if server.realtime.broadcastUser(item.userID, encoded) == 0 {
			// Job completion is a user notification, not a recoverable document
			// snapshot hint. Keep it durable until at least one session accepts it.
			continue
		}
		if _, err := server.database.Exec(ctx, `UPDATE occccad.outbox_events SET published_at=now()
			WHERE id=$1 AND published_at IS NULL`, item.id); err != nil {
			return err
		}
	}
	return nil
}

func (server *Server) publishRealtimeBatch(ctx context.Context) error {
	rows, err := server.database.Query(ctx, `SELECT e.id::text,w.document_id::text,w.id::text,
		e.event_type,e.schema_version,e.payload,COALESCE((e.payload->>'sequence')::bigint,0)
		FROM occccad.outbox_events e JOIN occccad.workspaces w ON w.id=e.aggregate_id
		WHERE e.aggregate_type='WORKSPACE' AND e.published_at IS NULL
		ORDER BY e.created_at,e.id LIMIT 100`)
	if err != nil {
		return err
	}
	type event struct {
		id, documentID, workspaceID, eventType string
		schemaVersion                          int
		payload                                json.RawMessage
		sequence                               uint64
	}
	events := []event{}
	for rows.Next() {
		var item event
		if err := rows.Scan(&item.id, &item.documentID, &item.workspaceID, &item.eventType,
			&item.schemaVersion, &item.payload, &item.sequence); err != nil {
			rows.Close()
			return err
		}
		events = append(events, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range events {
		messageType := item.eventType
		if !strings.HasSuffix(messageType, ".v1") {
			messageType = fmt.Sprintf("%s.v%d", messageType, item.schemaVersion)
		}
		envelope := newRealtimeEnvelope("event", messageType, "", &item.sequence, map[string]any{
			"eventId": item.id, "documentId": item.documentID, "workspaceId": item.workspaceID,
			"data": item.payload,
		}, nil)
		encoded, _ := json.Marshal(envelope)
		server.realtime.broadcast(item.documentID, encoded)
		if _, err := server.database.Exec(ctx, `UPDATE occccad.outbox_events SET published_at=now()
			WHERE id=$1 AND published_at IS NULL`, item.id); err != nil {
			return err
		}
	}
	return nil
}
