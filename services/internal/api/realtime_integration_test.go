package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/authn"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/workspace"
)

func TestRealtimeEditingRecoveryAndPreviewDatabase(t *testing.T) {
	databaseURL := os.Getenv("OCCCCAD_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("requires isolated test database")
	}
	db, err := database.Open(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	authentication := authn.New(db, time.Hour)
	if err = authentication.BootstrapAdmin(t.Context(), "realtime@test.invalid", "Realtime test", "realtime-test-password"); err != nil {
		t.Fatal(err)
	}
	session, err := authentication.Login(t.Context(), "realtime@test.invalid", "realtime-test-password", "test", "local")
	if err != nil {
		t.Fatal(err)
	}
	local, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	objects := artifact.NewService(db, local)
	domain := workspace.NewWithArtifacts(db, nil, objects)
	part, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: session.User.ID, Type: "PART", Name: "Realtime " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	server := New(db, nil, domain, access.New(db), authentication, objects, jobs.New(db), false, nil)
	defer server.Close()
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	cookie := (&http.Cookie{Name: sessionCookieName, Value: session.Token}).String()
	receive := func(conn *websocket.Conn, id string) realtimeEnvelope {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		for {
			var response realtimeEnvelope
			if err := conn.ReadJSON(&response); err != nil {
				t.Fatal(err)
			}
			if response.CorrelationID == id {
				return response
			}
		}
	}
	send := func(conn *websocket.Conn, kind string, payload any) string {
		t.Helper()
		e := newRealtimeEnvelope("request", kind, "", nil, payload, nil)
		if err := conn.WriteJSON(e); err != nil {
			t.Fatal(err)
		}
		return e.ID
	}
	connect := func(base string) *websocket.Conn {
		t.Helper()
		dialer := websocket.Dialer{Subprotocols: []string{realtimeProtocol}, HandshakeTimeout: 5 * time.Second}
		conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(base, "http")+"/api/realtime", http.Header{"Cookie": {cookie}, "Origin": {base}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		id := send(conn, "connection.initialize.v1", map[string]string{"csrfToken": session.CSRFToken})
		if result := receive(conn, id); result.Type != "connection.ready.v1" {
			t.Fatalf("initialize: %+v", result)
		}
		return conn
	}
	conn := connect(httpServer.URL)
	id := send(conn, "document.subscribe.v1", map[string]string{"documentId": part.Document.ID})
	subscription := receive(conn, id)
	if subscription.Error != nil || strings.Contains(string(subscription.Payload), `"view"`) {
		t.Fatalf("subscription must be metadata only: %+v", subscription)
	}
	command := workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", RequestID: uuid.NewString(), Name: "preview plane", Origin: [3]float64{0, 0, 4}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}}
	previewPayload := previewRequest{DocumentID: part.Document.ID, InteractionID: "gesture", Sequence: 1, Command: command}
	id = send(conn, "workspace.preview.request.v1", previewPayload)
	response := receive(conn, id)
	if response.Type != "workspace.preview.ready.v1" {
		t.Fatalf("preview: %+v %s", response, string(response.Payload))
	}
	var ready struct {
		Preview workspace.CommandPreview `json:"preview"`
	}
	if err = json.Unmarshal(response.Payload, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Preview.Artifact == nil || strings.Contains(string(response.Payload), `"previewMesh"`) || len(response.Payload) > 16<<10 {
		t.Fatal("preview leaked display payload")
	}
	ref := ready.Preview.Artifact.Representations["VISUAL"]
	download := func(url string) int {
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, httpServer.URL+url, nil)
		request.Header.Set("Cookie", cookie)
		result, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, result.Body)
		result.Body.Close()
		return result.StatusCode
	}
	if status := download(ref.URL); status != 200 {
		t.Fatalf("preview artifact: %d", status)
	}
	command.PreviewID = ready.Preview.PreviewID
	id = send(conn, "workspace.command.execute.v1", map[string]any{"documentId": part.Document.ID, "command": command})
	completed := receive(conn, id)
	if completed.Type != "workspace.command.completed.v1" {
		t.Fatalf("commit: %+v", completed)
	}
	if !strings.Contains(string(completed.Payload), `"view"`) {
		t.Fatal("small command did not include inline business snapshot")
	}
	// Retry after a connection break, with exactly the same request ID and payload.
	conn.Close()
	conn = connect(httpServer.URL)
	id = send(conn, "workspace.command.execute.v1", map[string]any{"documentId": part.Document.ID, "command": command})
	repeated := receive(conn, id)
	if repeated.Type != "workspace.command.completed.v1" || string(repeated.Payload) != string(completed.Payload) {
		t.Fatalf("retry changed receipt: %s / %s", completed.Payload, repeated.Payload)
	}
	// Large-document clients can avoid building an inline projection entirely.
	id = send(conn, "workspace.command.execute.v1", map[string]any{"documentId": part.Document.ID, "command": command, "inlineSnapshot": false})
	metadata := receive(conn, id)
	if metadata.Error != nil || strings.Contains(string(metadata.Payload), `"view"`) {
		t.Fatal("metadata-only command receipt violated", string(metadata.Payload))
	}
	var count int
	if err = db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.domain_transactions WHERE request_id=$1`, command.RequestID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate transaction %d: %v", count, err)
	}
	if status := download(ref.URL); status != 404 {
		t.Fatalf("consumed candidate still grants access: %d", status)
	}
	if status := download("/api/documents/" + part.Document.ID + "/realtime-snapshot"); status != 200 {
		t.Fatalf("snapshot: %d", status)
	}
	// Hold the prepare query while the reader handles supersede/cancel. No worker
	// hook or sleep is needed to prove cancellation is independent of computation.
	lock, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(context.Background())
	if _, err = lock.Exec(t.Context(), `LOCK TABLE occccad.workspaces IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	previewPayload.Sequence = 2
	previewPayload.Command.RequestID = uuid.NewString()
	previewPayload.Command.PreviewID = ""
	first := send(conn, "workspace.preview.request.v1", previewPayload)
	previewPayload.Sequence = 3
	second := send(conn, "workspace.preview.request.v1", previewPayload)
	if result := receive(conn, first); result.Type != "workspace.preview.canceled.v1" {
		t.Fatalf("supersede: %+v", result)
	}
	cancel := send(conn, "workspace.preview.cancel.v1", previewPayload)
	if result := receive(conn, second); result.Type != "workspace.preview.canceled.v1" {
		t.Fatalf("cancel work: %+v", result)
	}
	if result := receive(conn, cancel); result.Type != "workspace.preview.canceled.v1" {
		t.Fatalf("cancel ack: %+v", result)
	}
	if err = lock.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	id = send(conn, "workspace.preview.request.v1", previewPayload)
	if result := receive(conn, id); result.Error == nil || result.Error.Code != "STALE_PREVIEW" {
		t.Fatalf("old sequence accepted: %+v", result)
	}
	previewPayload.Sequence = 4
	id = send(conn, "workspace.preview.request.v1", previewPayload)
	if result := receive(conn, id); result.Type != "workspace.preview.ready.v1" {
		t.Fatalf("new preview did not recover: %+v", result)
	}

	// A committed command whose acknowledgement was never read must not execute
	// again after reconnect. Poll durable state, then drop the socket unread.
	lost := command
	lost.RequestID = uuid.NewString()
	lost.PreviewID = ""
	lost.Name = "lost acknowledgement"
	send(conn, "workspace.command.execute.v1", map[string]any{"documentId": part.Document.ID, "command": lost})
	for deadline := time.Now().Add(5 * time.Second); ; {
		var committed bool
		if err := db.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM occccad.domain_transactions WHERE request_id=$1 AND status='COMMITTED')`, lost.RequestID).Scan(&committed); err != nil {
			t.Fatal(err)
		}
		if committed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("command did not commit")
		}
		time.Sleep(5 * time.Millisecond)
	}
	conn.Close()
	conn = connect(httpServer.URL)
	id = send(conn, "workspace.command.execute.v1", map[string]any{"documentId": part.Document.ID, "command": lost})
	if result := receive(conn, id); result.Type != "workspace.command.completed.v1" {
		t.Fatalf("lost ack recovery: %+v", result)
	}
	if err = db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.domain_transactions WHERE request_id=$1`, lost.RequestID).Scan(&count); err != nil || count != 1 {
		t.Fatal("lost ack duplicated command")
	}
	lost.Name = "different intent"
	id = send(conn, "workspace.command.execute.v1", map[string]any{"documentId": part.Document.ID, "command": lost})
	if result := receive(conn, id); result.Error == nil || !strings.Contains(result.Error.Message, "IDEMPOTENCY_KEY_REUSED") {
		t.Fatalf("reused request changed intent: %+v", result)
	}

	// Two connections retry the same intent concurrently; both receive the same
	// durable receipt, including when one crosses the other's commit boundary.
	peer := connect(httpServer.URL)
	concurrent := command
	concurrent.RequestID = uuid.NewString()
	concurrent.PreviewID = ""
	concurrent.Name = "concurrent request"
	left := send(conn, "workspace.command.execute.v1", map[string]any{"documentId": part.Document.ID, "command": concurrent})
	right := send(peer, "workspace.command.execute.v1", map[string]any{"documentId": part.Document.ID, "command": concurrent})
	firstReceipt, secondReceipt := receive(conn, left), receive(peer, right)
	if firstReceipt.Error != nil || secondReceipt.Error != nil || string(firstReceipt.Payload) != string(secondReceipt.Payload) {
		t.Fatalf("concurrent retry: %+v / %+v", firstReceipt, secondReceipt)
	}
	undo := workspace.CommandRequest{Type: "UNDO", RequestID: uuid.NewString()}
	id = send(conn, "workspace.command.execute.v1", map[string]any{"documentId": part.Document.ID, "command": undo})
	undone := receive(conn, id)
	id = send(conn, "workspace.command.execute.v1", map[string]any{"documentId": part.Document.ID, "command": undo})
	undoRetry := receive(conn, id)
	if undone.Error != nil || undoRetry.Error != nil || string(undone.Payload) != string(undoRetry.Payload) {
		t.Fatalf("Undo retry changed outcome: %+v / %+v", undone, undoRetry)
	}

	// Progress and its outbox hint commit together; geometry/import result payloads
	// are not copied into realtime Job events.
	jobID := uuid.NewString()
	if _, err = db.Exec(t.Context(), `INSERT INTO occccad.jobs(id,job_type,requested_by_user_id,idempotency_key,state,lease_owner,lease_expires_at,attempt_count,payload) VALUES($1::uuid,'EXCHANGE_IMPORT',$2,$1::text,'RUNNING','rt-test',now()+interval '1 hour',1,$3)`, jobID, session.User.ID, []byte(`{"large":"`+strings.Repeat("x", 32768)+`"}`)); err != nil {
		t.Fatal(err)
	}
	if err = server.jobs.UpdateProgressDetail(t.Context(), jobID, "rt-test", 26, &jobs.ProgressDetail{Phase: "EVALUATING", Completed: 1, Total: 3}); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var event realtimeEnvelope
		if err = conn.ReadJSON(&event); err != nil {
			t.Fatal(err)
		}
		if event.Type != "job.state.changed.v1" {
			continue
		}
		var payload struct {
			Job jobs.Job `json:"job"`
		}
		if err = json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Job.ID != jobID {
			continue
		}
		if payload.Job.Progress != 26 || len(event.Payload) > 4096 || string(payload.Job.Payload) != "null" {
			t.Fatalf("unexpected progress event: %s", event.Payload)
		}
		break
	}

	// Exercise actual Vite ws Upgrade (not a browser) when explicitly requested.
	if os.Getenv("OCCCCAD_TEST_VITE_REALTIME") == "1" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		directory, err := filepath.Abs("../../../web/apps/cad")
		if err != nil {
			t.Fatal(err)
		}
		process := exec.Command("node", "node_modules/vite/bin/vite.js", "--mode", "development", "--host", "127.0.0.1", "--port", fmt.Sprint(port))
		process.Dir = directory
		process.Env = append(os.Environ(), "VITE_API_PROXY_TARGET="+httpServer.URL)
		process.Stdout = io.Discard
		process.Stderr = io.Discard
		if err = process.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { process.Process.Kill(); process.Wait() }()
		base := fmt.Sprintf("http://127.0.0.1:%d", port)
		for deadline := time.Now().Add(15 * time.Second); ; {
			response, err := http.Get(base + "/api/session")
			if err == nil {
				response.Body.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("Vite did not start")
			}
			time.Sleep(50 * time.Millisecond)
		}
		proxy := connect(base)
		id = send(proxy, "document.subscribe.v1", map[string]string{"documentId": part.Document.ID})
		if result := receive(proxy, id); result.Type != "document.subscribed.v1" {
			t.Fatalf("Vite subscribe: %+v", result)
		}
	}
}
