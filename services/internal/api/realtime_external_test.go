package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// TestRealtimeTwoClientCommit is an opt-in end-to-end probe against a running
// occccad-server. It creates and removes its own document.
func TestRealtimeTwoClientCommit(t *testing.T) {
	base := strings.TrimRight(os.Getenv("OCCCCAD_REALTIME_TEST_URL"), "/")
	password := os.Getenv("OCCCCAD_ADMIN_PASSWORD")
	if base == "" || password == "" {
		t.Skip("set OCCCCAD_REALTIME_TEST_URL and OCCCCAD_ADMIN_PASSWORD")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 20 * time.Second}
	loginBody, _ := json.Marshal(map[string]string{
		"email": value("OCCCCAD_ADMIN_EMAIL", "admin@occcad.local"), "password": password,
	})
	response, err := client.Post(base+"/api/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	csrf := ""
	for _, cookie := range response.Cookies() {
		if cookie.Name == "occccad_csrf" {
			csrf = cookie.Value
		}
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status %d", response.StatusCode)
	}
	baseURL, _ := url.Parse(base)
	baseURL.Path = "/"
	if csrf == "" {
		t.Fatal("login did not set CSRF cookie")
	}
	documentID := createRealtimeTestDocument(t, client, base, csrf)
	defer deleteRealtimeTestDocument(t, client, base, csrf, documentID)

	first := dialRealtime(t, jar, baseURL, csrf)
	defer first.Close()
	second := dialRealtime(t, jar, baseURL, csrf)
	defer second.Close()
	subscribeRealtime(t, first, documentID)
	subscribeRealtime(t, second, documentID)

	requestID := uuid.NewString()
	command := map[string]any{"protocol": "occccad.realtime.v1", "id": requestID, "kind": "request",
		"type": "workspace.command.execute.v1", "sentAt": time.Now().UTC().Format(time.RFC3339Nano),
		"payload": map[string]any{"documentId": documentID, "command": map[string]any{
			"requestId": requestID, "type": "CREATE_SKETCH", "plane": "XY",
		}}}
	if err := first.WriteJSON(command); err != nil {
		t.Fatal(err)
	}
	readRealtimeType(t, first, "workspace.command.completed.v1")
	readRealtimeType(t, second, "workspace.transaction.committed.v1")
}

// TestRealtimeExchangeImportNotification verifies the browser-facing async
// contract: uploading returns a Job immediately and terminal state arrives on
// the authenticated user channel without polling the Job endpoint.
func TestRealtimeExchangeImportNotification(t *testing.T) {
	base := strings.TrimRight(os.Getenv("OCCCCAD_REALTIME_TEST_URL"), "/")
	password := os.Getenv("OCCCCAD_ADMIN_PASSWORD")
	stepPath := os.Getenv("OCCCCAD_EXCHANGE_TEST_STEP")
	if base == "" || password == "" || stepPath == "" {
		t.Skip("set OCCCCAD_REALTIME_TEST_URL, OCCCCAD_ADMIN_PASSWORD, and OCCCCAD_EXCHANGE_TEST_STEP")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 20 * time.Second}
	loginBody, _ := json.Marshal(map[string]string{
		"email": value("OCCCCAD_ADMIN_EMAIL", "admin@occcad.local"), "password": password,
	})
	response, err := client.Post(base+"/api/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	csrf := ""
	for _, cookie := range response.Cookies() {
		if cookie.Name == "occccad_csrf" {
			csrf = cookie.Value
		}
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || csrf == "" {
		t.Fatalf("login status %d, csrf present %v", response.StatusCode, csrf != "")
	}
	baseURL, _ := url.Parse(base)
	baseURL.Path = "/"
	connection := dialRealtime(t, jar, baseURL, csrf)
	defer connection.Close()

	source, err := os.Open(stepPath)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	query := url.Values{"format": {"STEP"}, "fileName": {filepath.Base(stepPath)},
		"documentName": {"Realtime import " + uuid.NewString()}}
	request, _ := http.NewRequest(http.MethodPost, base+"/api/exchange/imports?"+query.Encode(), source)
	request.Header.Set("Content-Type", "model/step")
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", uuid.NewString())
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var submitted struct {
		ID string `json:"id"`
	}
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("submit import status %d: %s", response.StatusCode, body)
	}
	if err := json.NewDecoder(response.Body).Decode(&submitted); err != nil || submitted.ID == "" {
		t.Fatalf("decode submitted job: %v", err)
	}

	job := readRealtimeJob(t, connection, submitted.ID)
	if job.State != "SUCCEEDED" || job.DocumentID == "" {
		t.Fatalf("terminal import job = %+v", job)
	}
	defer deleteRealtimeTestDocument(t, client, base, csrf, job.DocumentID)
}

func dialRealtime(t *testing.T, jar http.CookieJar, base *url.URL, csrf string) *websocket.Conn {
	t.Helper()
	target := *base
	if target.Scheme == "https" {
		target.Scheme = "wss"
	} else {
		target.Scheme = "ws"
	}
	target.Path = "/api/realtime"
	dialer := websocket.Dialer{Subprotocols: []string{"occccad.realtime.v1"}, Jar: jar}
	connection, response, err := dialer.Dial(target.String(), http.Header{"Origin": []string{base.String()}})
	if err != nil {
		if response != nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("dial realtime: %v (status %d: %s)", err, response.StatusCode, body)
		}
		t.Fatal(err)
	}
	id := uuid.NewString()
	if err := connection.WriteJSON(map[string]any{"protocol": "occccad.realtime.v1", "id": id,
		"kind": "request", "type": "connection.initialize.v1", "sentAt": time.Now().UTC().Format(time.RFC3339Nano),
		"payload": map[string]string{"csrfToken": csrf}}); err != nil {
		t.Fatal(err)
	}
	readRealtimeType(t, connection, "connection.ready.v1")
	return connection
}

func subscribeRealtime(t *testing.T, connection *websocket.Conn, documentID string) {
	t.Helper()
	if err := connection.WriteJSON(map[string]any{"protocol": "occccad.realtime.v1", "id": uuid.NewString(),
		"kind": "request", "type": "document.subscribe.v1", "sentAt": time.Now().UTC().Format(time.RFC3339Nano),
		"payload": map[string]string{"documentId": documentID}}); err != nil {
		t.Fatal(err)
	}
	readRealtimeType(t, connection, "document.subscribed.v1")
}

func readRealtimeType(t *testing.T, connection *websocket.Conn, expected string) {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(15 * time.Second))
	for {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := connection.ReadJSON(&envelope); err != nil {
			t.Fatalf("read %s: %v", expected, err)
		}
		if envelope.Type == expected {
			return
		}
		if envelope.Type == "request.failed.v1" {
			t.Fatalf("received request failure while waiting for %s", expected)
		}
	}
}

type realtimeJob struct {
	ID         string `json:"id"`
	State      string `json:"state"`
	DocumentID string `json:"documentId"`
}

func readRealtimeJob(t *testing.T, connection *websocket.Conn, jobID string) realtimeJob {
	t.Helper()
	_ = connection.SetReadDeadline(time.Now().Add(30 * time.Second))
	for {
		var envelope struct {
			Type    string `json:"type"`
			Payload struct {
				Job realtimeJob `json:"job"`
			} `json:"payload"`
		}
		if err := connection.ReadJSON(&envelope); err != nil {
			t.Fatalf("read terminal job %s: %v", jobID, err)
		}
		if envelope.Type == "job.state.changed.v1" && envelope.Payload.Job.ID == jobID {
			return envelope.Payload.Job
		}
	}
}

func createRealtimeTestDocument(t *testing.T, client *http.Client, base, csrf string) string {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"requestId": uuid.NewString(), "type": "PART",
		"name": "Realtime integration " + uuid.NewString()})
	request, _ := http.NewRequest(http.MethodPost, base+"/api/documents", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result struct {
		Document struct {
			ID string `json:"id"`
		} `json:"document"`
	}
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		t.Fatalf("create document status %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil || result.Document.ID == "" {
		t.Fatalf("decode document: %v", err)
	}
	return result.Document.ID
}

func deleteRealtimeTestDocument(t *testing.T, client *http.Client, base, csrf, documentID string) {
	t.Helper()
	request, _ := http.NewRequest(http.MethodDelete, base+"/api/documents/"+documentID, nil)
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", uuid.NewString())
	response, err := client.Do(request)
	if err == nil {
		response.Body.Close()
	}
}

func value(name, fallback string) string {
	if result := os.Getenv(name); result != "" {
		return result
	}
	return fallback
}
