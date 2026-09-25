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
	"github.com/occccad/occccad/internal/database"
	perf "github.com/occccad/occccad/internal/performance"
	"github.com/occccad/occccad/internal/workspace"
)

const (
	realtimeProtocol        = "occccad.realtime.v1"
	realtimeQueueSize       = 128
	realtimeMaxBytes        = 1 << 20
	realtimeInlineViewBytes = 64 << 10
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
	Phase     string `json:"phase,omitempty"`
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
	previews      map[string]*realtimePreview
	previewSlots  chan struct{}
}

func (client *realtimeClient) close() {
	client.closeOnce.Do(func() {
		close(client.done)
		go func() {
			client.mu.Lock()
			defer client.mu.Unlock()
			for _, p := range client.previews {
				p.cancel()
				p.discard(p.previewID)
			}
			client.previews = nil
		}()
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
	for key, p := range client.previews {
		if strings.HasPrefix(key, documentID+"/") {
			p.cancel()
			p.discard(p.previewID)
			if p.requestID != "" {
				client.sendResponse(p.requestID, "workspace.preview.canceled.v1", map[string]any{"documentId": documentID, "previewSequence": p.sequence})
			}
			delete(client.previews, key)
		}
	}
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
		if len(message) > realtimeMaxBytes {
			hub.remove(client)
			continue
		}
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
		if len(message) > realtimeMaxBytes {
			hub.remove(client)
			continue
		}
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

func (hub *realtimeHub) monitoringCounts() (connections, documents int) {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.connections), len(hub.byDocument)
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
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	type queuedRequest struct {
		envelope realtimeEnvelope
		deadline time.Time
	}
	queue := make(chan queuedRequest, 16)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case item := <-queue:
				operation, finish := context.WithDeadline(ctx, item.deadline)
				if operation.Err() != nil {
					client.sendDomainError(item.envelope.ID, operation.Err())
					finish()
					continue
				}
				client.handleRequest(server, operation, item.envelope)
				finish()
			}
		}
	}()
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
		switch envelope.Type {
		case "workspace.preview.request.v1":
			client.startPreview(server, ctx, envelope)
		case "workspace.preview.cancel.v1":
			client.cancelPreview(envelope)
		default:
			select {
			case queue <- queuedRequest{envelope: envelope, deadline: time.Now().Add(2 * time.Minute)}:
			default:
				client.sendError(envelope.ID, "REALTIME_BUSY", "realtime command queue is full", true)
			}
		}
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
	_, err := server.access.RequireDocument(ctx, payload.DocumentID, client.actor.ID, access.RoleViewer)
	if err != nil {
		client.sendDomainError(envelope.ID, err)
		return
	}
	client.subscribe(payload.DocumentID, "")
	head, err := server.realtimeHead(ctx, payload.DocumentID)
	if err != nil {
		client.unsubscribe(payload.DocumentID)
		client.sendDomainError(envelope.ID, err)
		return
	}
	client.subscribe(payload.DocumentID, head.WorkspaceID)
	client.sendResponse(envelope.ID, "document.subscribed.v1", head)
}

func (server *Server) handleRealtimeCommand(ctx context.Context, client *realtimeClient, envelope realtimeEnvelope) {
	ctx, finishTiming := realtimeOperationTiming(ctx, envelope)
	defer finishTiming()
	var payload struct {
		DocumentID     string                   `json:"documentId"`
		InlineSnapshot *bool                    `json:"inlineSnapshot,omitempty"`
		Command        workspace.CommandRequest `json:"command"`
	}
	if json.Unmarshal(envelope.Payload, &payload) != nil || payload.DocumentID == "" || payload.Command.Type == "" || payload.Command.RequestID == "" {
		client.sendError(envelope.ID, "INVALID_PAYLOAD", "documentId and command requestId/type are required", false)
		return
	}
	role, err := server.access.RequireDocument(ctx, payload.DocumentID, client.actor.ID, access.RoleEditor)
	if err != nil {
		client.sendDomainError(envelope.ID, err)
		return
	}
	payload.Command.ActorID = client.actor.ID
	if err := server.requireCommandReferences(ctx, client.actor.ID, payload.DocumentID, payload.Command); err != nil {
		client.sendDomainError(envelope.ID, err)
		return
	}
	finishExecute := perf.Start(ctx, "command-execute")
	executeErr := server.workspace.ExecuteCommand(ctx, payload.DocumentID, payload.Command)
	finishExecute()
	if err := executeErr; err != nil {
		client.sendDomainError(envelope.ID, err)
		return
	}
	var sequence uint64
	var committedRevision string
	if err := server.database.QueryRow(ctx, `SELECT t.sequence,t.result_revision_id::text FROM occccad.domain_transactions t JOIN occccad.workspaces w ON w.id=t.workspace_id WHERE w.document_id=$1 AND t.request_id=$2 AND t.status='COMMITTED'`, payload.DocumentID, payload.Command.RequestID).Scan(&sequence, &committedRevision); err != nil {
		client.sendDomainError(envelope.ID, err)
		return
	}
	finishSnapshot := perf.Start(ctx, "command-snapshot")
	completion := map[string]any{
		"documentId": payload.DocumentID, "requestId": payload.Command.RequestID, "versionId": committedRevision, "sequence": sequence,
	}
	// Small business snapshots avoid a second authenticated HTTP roundtrip.
	// A receipt can be old (idempotent retry): never label a newer view with it.
	if payload.InlineSnapshot == nil || *payload.InlineSnapshot {
		if view, err := server.workspace.GetDocument(ctx, payload.DocumentID, client.actor.ID); err == nil && view.Document.VersionID == committedRevision {
			view.Document.Permission = string(role)
			if encoded := realtimeInlineView(view); encoded != nil {
				if head, err := server.realtimeHead(ctx, payload.DocumentID); err == nil && head.Sequence == sequence && head.VersionID == committedRevision {
					completion["view"] = json.RawMessage(encoded)
					server.openDocuments.Update(client.actor.ID, view.Document)
				}
			}
		}
	}
	finishSnapshot()
	// The transaction and receipt are authoritative already. Thumbnail scheduling
	// remains in this bounded executor, but is not on the response critical path.
	client.sendResponse(envelope.ID, "workspace.command.completed.v1", completion)
	finishTiming()
	if err := server.enqueueDocumentPreviews(ctx, workspace.DocumentView{Document: workspace.DocumentSummary{ID: payload.DocumentID, VersionID: committedRevision}}, client.actor.ID); err != nil {
		slog.ErrorContext(ctx, "enqueue realtime document previews", "error", err)
	}
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
	failure := realtimeDomainError(err)
	client.enqueue(newRealtimeEnvelope("error", "request.failed.v1", correlationID, nil, nil, &failure))
}

func realtimeDomainError(err error) realtimeError {
	code, retryable := "INTERNAL", true
	var domainFailure interface {
		Code() string
		Retryable() bool
	}
	switch {
	case errors.Is(err, context.Canceled):
		code, retryable = "CANCELED", false
	case errors.Is(err, context.DeadlineExceeded):
		code, retryable = "TIMEOUT", true
	case errors.Is(err, database.ErrBusy):
		code, retryable = "DATABASE_BUSY", true
	case errors.As(err, &domainFailure):
		code, retryable = domainFailure.Code(), domainFailure.Retryable()
	case strings.Contains(err.Error(), "CONFLICT"):
		code, retryable = "CHANGESET_CONFLICT", false
	case errors.Is(err, access.ErrForbidden):
		code, retryable = "FORBIDDEN", false
	case errors.Is(err, access.ErrNotFound), errors.Is(err, workspace.ErrNotFound):
		code, retryable = "NOT_FOUND", false
	case errors.Is(err, access.ErrValidation), errors.Is(err, workspace.ErrValidation):
		code, retryable = "VALIDATION_FAILED", false
	}
	failure := realtimeError{Code: code, Message: err.Error(), Retryable: retryable}
	var phased interface{ Phase() string }
	if errors.As(err, &phased) {
		failure.Phase = phased.Phase()
	}
	return failure
}

func (client *realtimeClient) enqueue(envelope realtimeEnvelope) {
	message, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	if len(message) > realtimeMaxBytes {
		message, _ = json.Marshal(newRealtimeEnvelope("error", "request.failed.v1", envelope.CorrelationID, nil, nil, &realtimeError{Code: "MESSAGE_TOO_LARGE", Message: "realtime response exceeds control-plane limit", Retryable: false}))
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
	ctx = database.Background(ctx)
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
	rows, err := server.database.Query(ctx, `SELECT array_agg(e.id::text),e.aggregate_id::text,j.requested_by_user_id::text
		FROM occccad.outbox_events e JOIN occccad.jobs j ON j.id=e.aggregate_id
		WHERE e.aggregate_type='JOB' AND e.published_at IS NULL
		GROUP BY e.aggregate_id,j.requested_by_user_id ORDER BY min(e.created_at) LIMIT 100`)
	if err != nil {
		return err
	}
	type event struct {
		ids           []string
		jobID, userID string
	}
	events := []event{}
	for rows.Next() {
		var item event
		if err := rows.Scan(&item.ids, &item.jobID, &item.userID); err != nil {
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
		job.Payload = nil // Large import result lists are fetched through the Jobs resource.
		envelope := newRealtimeEnvelope("event", "job.state.changed.v1", "", nil,
			map[string]any{"job": job}, nil)
		encoded, _ := json.Marshal(envelope)
		if server.realtime.broadcastUser(item.userID, encoded) == 0 {
			// Job completion is a user notification, not a recoverable document
			// snapshot hint. Keep it durable until at least one session accepts it.
			continue
		}
		if _, err := server.database.Exec(ctx, `UPDATE occccad.outbox_events SET published_at=now()
			WHERE id::text=ANY($1) AND published_at IS NULL`, item.ids); err != nil {
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

// Each operation gets its own recorder, rather than accumulating phases for
// the lifetime of the upgraded connection. Log only slow control operations.
func realtimeOperationTiming(ctx context.Context, envelope realtimeEnvelope) (context.Context, func()) {
	ctx, recorder := perf.WithRecorder(ctx)
	started := time.Now()
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			if elapsed := time.Since(started); elapsed >= 250*time.Millisecond {
				slog.InfoContext(ctx, "slow realtime operation", "type", envelope.Type, "correlation_id", envelope.ID, "duration_ms", elapsed.Milliseconds(), "phases_ms", recorder.SnapshotMilliseconds())
			}
		})
	}
}

func realtimeInlineView(view workspace.DocumentView) json.RawMessage {
	encoded, err := json.Marshal(view)
	if err != nil || len(encoded) > realtimeInlineViewBytes {
		return nil
	}
	return encoded
}
