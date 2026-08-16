package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/occccad/occccad/internal/access"
)

func TestRealtimeEnvelopeCarriesVersionAndCorrelation(t *testing.T) {
	sequence := uint64(7)
	envelope := newRealtimeEnvelope("event", "workspace.transaction.committed.v1", "request-1",
		&sequence, map[string]string{"documentId": "document-1"}, nil)
	if envelope.Protocol != realtimeProtocol || envelope.Kind != "event" ||
		envelope.CorrelationID != "request-1" || envelope.Sequence == nil || *envelope.Sequence != sequence {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	var payload map[string]string
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil || payload["documentId"] != "document-1" {
		t.Fatalf("unexpected payload: %s (%v)", envelope.Payload, err)
	}
}

func TestRealtimeHubScopesBroadcastByDocument(t *testing.T) {
	hub := newRealtimeHub()
	first := &realtimeClient{hub: hub, send: make(chan []byte, 1), done: make(chan struct{}),
		subscriptions: map[string]string{}, acknowledged: map[string]uint64{}}
	second := &realtimeClient{hub: hub, send: make(chan []byte, 1), done: make(chan struct{}),
		subscriptions: map[string]string{}, acknowledged: map[string]uint64{}}
	hub.add(first)
	hub.add(second)
	first.subscribe("document-1", "workspace-1")
	second.subscribe("document-2", "workspace-2")
	hub.broadcast("document-1", []byte("event"))
	select {
	case message := <-first.send:
		if string(message) != "event" {
			t.Fatalf("unexpected message %q", message)
		}
	default:
		t.Fatal("subscribed client did not receive the event")
	}
	select {
	case <-second.send:
		t.Fatal("event leaked to a different document subscription")
	default:
	}
	hub.close()
}

func TestRealtimeHubScopesJobEventsByUser(t *testing.T) {
	hub := newRealtimeHub()
	first := &realtimeClient{hub: hub, actor: access.User{ID: "user-1"}, send: make(chan []byte, 1), done: make(chan struct{}),
		subscriptions: map[string]string{}, acknowledged: map[string]uint64{}}
	second := &realtimeClient{hub: hub, actor: access.User{ID: "user-2"}, send: make(chan []byte, 1), done: make(chan struct{}),
		subscriptions: map[string]string{}, acknowledged: map[string]uint64{}}
	hub.add(first)
	hub.add(second)
	hub.broadcastUser("user-1", []byte("job-event"))
	select {
	case message := <-first.send:
		if string(message) != "job-event" {
			t.Fatalf("unexpected message %q", message)
		}
	default:
		t.Fatal("job owner did not receive the event")
	}
	select {
	case <-second.send:
		t.Fatal("job event leaked to a different user")
	default:
	}
	hub.close()
}

func TestRealtimeHubDisconnectsSlowConsumer(t *testing.T) {
	hub := newRealtimeHub()
	client := &realtimeClient{hub: hub, send: make(chan []byte, 1), done: make(chan struct{}),
		subscriptions: map[string]string{}, acknowledged: map[string]uint64{}}
	hub.add(client)
	client.subscribe("document-1", "workspace-1")
	client.send <- []byte("already full")
	hub.broadcast("document-1", []byte("new event"))
	select {
	case <-client.done:
	default:
		t.Fatal("slow consumer was not disconnected")
	}
}

func TestRealtimeAcknowledgementIsSubscriptionScopedAndMonotonic(t *testing.T) {
	sequence := uint64(8)
	client := &realtimeClient{subscriptions: map[string]string{"document-1": "workspace-1"},
		acknowledged: map[string]uint64{}}
	payload, _ := json.Marshal(map[string]string{"documentId": "document-1"})
	client.handleAcknowledgement(realtimeEnvelope{Kind: "ack", Type: "stream.ack.v1",
		Sequence: &sequence, Payload: payload})
	sequence = 3
	client.handleAcknowledgement(realtimeEnvelope{Kind: "ack", Type: "stream.ack.v1",
		Sequence: &sequence, Payload: payload})
	if client.acknowledged["document-1"] != 8 {
		t.Fatalf("acknowledgement moved backwards: %d", client.acknowledged["document-1"])
	}
	sequence = 9
	other, _ := json.Marshal(map[string]string{"documentId": "document-2"})
	client.handleAcknowledgement(realtimeEnvelope{Kind: "ack", Type: "stream.ack.v1",
		Sequence: &sequence, Payload: other})
	if _, exists := client.acknowledged["document-2"]; exists {
		t.Fatal("acknowledgement was accepted without a subscription")
	}
}

func TestRealtimeOriginPolicy(t *testing.T) {
	server := New(nil, nil, nil, nil, nil, nil, nil, false, nil)
	defer server.Close()
	sameOrigin := httptest.NewRequest("GET", "http://cad.local/api/realtime", nil)
	sameOrigin.Host = "cad.local"
	sameOrigin.Header.Set("Origin", "http://cad.local")
	if !server.realtimeOriginAllowed(sameOrigin) {
		t.Fatal("same-origin websocket was rejected")
	}
	crossOrigin := httptest.NewRequest("GET", "http://cad.local/api/realtime", nil)
	crossOrigin.Host = "cad.local"
	crossOrigin.Header.Set("Origin", "https://untrusted.example")
	if server.realtimeOriginAllowed(crossOrigin) {
		t.Fatal("cross-origin websocket was accepted")
	}
	configured := New(nil, nil, nil, nil, nil, nil, nil, false, []string{"https://cad.example"})
	defer configured.Close()
	crossOrigin.Header.Set("Origin", "https://cad.example")
	if !configured.realtimeOriginAllowed(crossOrigin) {
		t.Fatal("configured websocket origin was rejected")
	}
}
