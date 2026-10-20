package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/authn"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAutomationHTTPQueriesAndRealtimeCommitBoundary(t *testing.T) {
	db := testsupport.OpenDevelopmentDatabase(t)
	configuration := testsupport.Environment(t)
	authentication := authn.New(db, time.Hour)
	if err := authentication.BootstrapAdmin(t.Context(), configuration.AdminEmail, configuration.AdminName, configuration.AdminPassword); err != nil {
		t.Fatal(err)
	}
	session, err := authentication.Login(t.Context(), configuration.AdminEmail, configuration.AdminPassword, "automation-test", "local")
	if err != nil {
		t.Fatal(err)
	}
	worker, closeWorker, err := geometry.OpenDevelopmentWorker(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer closeWorker()
	store, staging, err := artifact.OpenConfigured(t.Context(), configuration.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, worker, artifact.NewService(db, store, staging))
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: session.User.ID, Type: "PART", Name: "Automation HTTP " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	server := New(db, nil, domain, access.New(db), authentication, nil, jobs.New(db), false, nil)
	defer server.Close()
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	cookie := (&http.Cookie{Name: sessionCookieName, Value: session.Token}).String()
	get := func(path string) *http.Response {
		t.Helper()
		r, _ := http.NewRequest(http.MethodGet, httpServer.URL+path, nil)
		r.Header.Set("Cookie", cookie)
		response, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		return response
	}
	response := get("/api/automation/operations?q=extrude&documentType=PART")
	var operations struct {
		Operations []automation.OperationSummary `json:"operations"`
	}
	err = json.NewDecoder(response.Body).Decode(&operations)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || len(operations.Operations) != 1 {
		t.Fatal("operation discovery", response.StatusCode, err)
	}
	response = get("/api/documents/" + view.Document.ID + "/automation/context")
	var current automation.Context
	err = json.NewDecoder(response.Body).Decode(&current)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || current.Snapshot.RevisionID != view.Document.VersionID {
		t.Fatal("context", response.StatusCode, err)
	}
	dialer := websocket.Dialer{Subprotocols: []string{realtimeProtocol}}
	connection, _, err := dialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/api/realtime", http.Header{"Cookie": {cookie}, "Origin": {httpServer.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	send := func(kind string, payload any) realtimeEnvelope {
		t.Helper()
		request := newRealtimeEnvelope("request", kind, "", nil, payload, nil)
		if e := connection.WriteJSON(request); e != nil {
			t.Fatal(e)
		}
		connection.SetReadDeadline(time.Now().Add(10 * time.Second))
		for {
			var reply realtimeEnvelope
			if e := connection.ReadJSON(&reply); e != nil {
				t.Fatal(e)
			}
			if reply.CorrelationID == request.ID {
				return reply
			}
		}
	}
	if reply := send("connection.initialize.v1", map[string]string{"csrfToken": session.CSRFToken}); reply.Type != "connection.ready.v1" {
		t.Fatalf("authentication: %+v", reply)
	}
	action := automation.ActionRequest{OperationID: "sketch.create", Version: 1, Arguments: json.RawMessage(`{"bodyId":"` + view.Part.ActiveBodyID + `","plane":"XY"}`), ExpectedHead: current.Snapshot.CommandBaseline, RequestID: uuid.NewString()}
	reply := send("automation.request.v1", automationRequest{DocumentID: view.Document.ID, Method: "action.prepare", Action: action})
	if reply.Error != nil {
		t.Fatalf("prepare: %+v", reply)
	}
	var preview workspace.CommandPreview
	if err = json.Unmarshal(reply.Payload, &preview); err != nil || preview.PreviewID == "" {
		t.Fatal("preview", err)
	}
	afterPreview, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil || afterPreview.RevisionID != view.Document.VersionID {
		t.Fatal("preview changed Head", err)
	}
	action.PreviewID = preview.PreviewID
	var receipt automation.ActionResult
	for range 2 {
		reply = send("automation.request.v1", automationRequest{DocumentID: view.Document.ID, Method: "action.commit", Action: action})
		if reply.Error != nil || json.Unmarshal(reply.Payload, &receipt) != nil || receipt.Output == nil {
			t.Fatalf("idempotent commit: %+v", reply)
		}
	}
	var count int
	if err = db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.domain_transactions WHERE request_id=$1`, action.RequestID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate history", count, err)
	}
	// A stale read must fail before adaptation, even without a preview token.
	if err = domain.ExecuteCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: session.User.ID, RequestID: uuid.NewString(), ExpectedHead: &current.Snapshot.CommandBaseline, Type: "CREATE_SKETCH", Plane: "XZ"}); err == nil || !strings.Contains(err.Error(), "WORKSPACE_HEAD_CONFLICT") {
		t.Fatal("stale caller baseline was accepted", err)
	}
	response = get("/api/documents/" + view.Document.ID + "/automation/tasks/" + uuid.NewString())
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatal("missing task did not return 404", response.StatusCode)
	}
}
