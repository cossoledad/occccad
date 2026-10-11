package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestRealtimeMechanismNoopHasFrozenReceipt(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "application.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	auth := authn.New(db, time.Hour)
	if err = auth.BootstrapAdmin(t.Context(), "application@test.invalid", "test", "test-password"); err != nil {
		t.Fatal(err)
	}
	session, err := auth.Login(t.Context(), "application@test.invalid", "test-password", "test", "local")
	if err != nil {
		t.Fatal(err)
	}
	local, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	objects := artifact.NewService(db, local)
	domain := workspace.NewWithArtifacts(db, nil, objects)
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: session.User.ID, Type: "PRODUCT", Name: "empty application"})
	if err != nil {
		t.Fatal(err)
	}
	mid := uuid.NewString()
	k := workspace.KinematicsDefinitions{Mechanisms: []workspace.Mechanism{{ID: mid, Name: "empty", UnitIDs: []string{}, Joints: []workspace.MechanismJoint{}}}}
	view, err = domain.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: session.User.ID, RequestID: uuid.NewString(), Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &k})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := domain.PrepareMechanismApplication(t.Context(), view.Document.ID, workspace.MechanismApplicationRequest{BaseRevisionID: view.Document.VersionID, MechanismID: mid, Resume: true})
	if err != nil || prepared.InstancePoses == nil || len(prepared.InstancePoses) != 0 {
		t.Fatal("empty Product application must return an empty pose array", err, prepared.InstancePoses)
	}
	server := New(db, nil, domain, access.New(db), auth, objects, jobs.New(db), false, nil)
	defer server.Close()
	host := httptest.NewServer(server.Handler())
	defer host.Close()
	dialer := websocket.Dialer{Subprotocols: []string{realtimeProtocol}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(host.URL, "http")+"/api/realtime", http.Header{"Cookie": {(&http.Cookie{Name: sessionCookieName, Value: session.Token}).String()}, "Origin": {host.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	invoke := func(kind string, payload any) realtimeEnvelope {
		t.Helper()
		request := newRealtimeEnvelope("request", kind, "", nil, payload, nil)
		if err := conn.WriteJSON(request); err != nil {
			t.Fatal(err)
		}
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		for {
			var response realtimeEnvelope
			if err := conn.ReadJSON(&response); err != nil {
				t.Fatal(err)
			}
			if response.CorrelationID == request.ID {
				return response
			}
		}
	}
	if ready := invoke("connection.initialize.v1", map[string]string{"csrfToken": session.CSRFToken}); ready.Type != "connection.ready.v1" {
		t.Fatal(ready)
	}
	command := workspace.CommandRequest{Type: "ACTIVATE_MECHANISM", RequestID: uuid.NewString(), VersionID: view.Document.VersionID, MechanismApplication: &workspace.MechanismApplicationRequest{MechanismID: mid, Resume: true}}
	execute := func() realtimeEnvelope {
		return invoke("workspace.command.execute.v1", map[string]any{"documentId": view.Document.ID, "command": command})
	}
	first := execute()
	if first.Type != "workspace.command.completed.v1" {
		t.Fatalf("no-op was committed but transport failed: %+v", first)
	}
	var receipt struct {
		Sequence  uint64 `json:"sequence"`
		VersionID string `json:"versionId"`
	}
	if json.Unmarshal(first.Payload, &receipt) != nil || receipt.VersionID != view.Document.VersionID {
		t.Fatal("incorrect no-op receipt", string(first.Payload))
	}
	same := execute()
	if string(same.Payload) != string(first.Payload) {
		t.Fatal("retry changed receipt")
	}
	latest, err := domain.GetDocument(t.Context(), view.Document.ID)
	if err != nil || latest.Document.VersionID != view.Document.VersionID {
		t.Fatal("no-op advanced Head", err)
	}
	// An idempotent reply continues to name its original Head after another edit.
	k = latest.Product.Kinematics
	k.Mechanisms[0].Name = "renamed"
	advanced, err := domain.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{ActorID: session.User.ID, RequestID: uuid.NewString(), Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &k})
	if err != nil {
		t.Fatal(err)
	}
	old := execute()
	var repeated struct {
		Sequence  uint64                  `json:"sequence"`
		VersionID string                  `json:"versionId"`
		View      *workspace.DocumentView `json:"view"`
	}
	if old.Type != "workspace.command.completed.v1" || json.Unmarshal(old.Payload, &repeated) != nil || repeated.VersionID != receipt.VersionID || repeated.Sequence != receipt.Sequence || repeated.View != nil {
		t.Fatal("old receipt mislabeled new Head", string(old.Payload))
	}
	if advanced.Document.VersionID == receipt.VersionID {
		t.Fatal("fixture did not advance")
	}
	command.MechanismApplication.Resume = false
	if rejected := execute(); rejected.Error == nil {
		t.Fatal("request key accepted a changed invocation")
	}
}
