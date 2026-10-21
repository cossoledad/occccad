package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEvalProviderSourceRejectsLocalFixtures(t *testing.T) {
	for _, origin := range []string{"", "http://127.0.0.1:19091", "https://127.0.0.1", "https://localhost", "https://fixture.localhost", "https://192.168.1.9", "https://user:secret@api.deepseek.com", "https://api.deepseek.com?token=hidden"} {
		if providerIsLive(origin) {
			t.Fatalf("unattested provider accepted: %q", origin)
		}
	}
	if !providerIsLive("https://api.deepseek.com") {
		t.Fatal("default production provider origin rejected")
	}
	for _, origin := range []string{"http://public.example", "https://user:password@example.com", "https://example.com?key=secret"} {
		if _, err := newClient(origin); err == nil {
			t.Fatal("unsafe credential transport accepted")
		}
	}
}

// This is an HTTP-contract fixture, not live provider or CAD evidence. The
// completed Run's receipt-confirmed target roles must scope the independent
// checker; the Run's own VERIFIED label cannot replace that request.
func TestEvalCompletedPartVerificationUsesReceiptTargetScope(t *testing.T) {
	type check struct {
		ID            string  `json:"id"`
		Kind          string  `json:"kind"`
		BodyID        string  `json:"bodyId"`
		Unit          string  `json:"unit"`
		Width         float64 `json:"width"`
		Height        float64 `json:"height"`
		Length        float64 `json:"length"`
		Radius        float64 `json:"radius"`
		ExpectedCount int     `json:"expectedCount"`
	}
	for _, fillet := range []bool{false, true} {
		caseID := "box"
		if fillet {
			caseID = "box-fillet"
		}
		t.Run(caseID, func(t *testing.T) {
			const document, body, initialRevision, finalRevision, runID = "fixture-document", "fixture-body", "initial-revision", "verified-revision", "fixture-run"
			targets := map[string]string{"SKETCH": "receipt-sketch", "LINEAR_EXTRUDE": "receipt-extrusion", "BODY": body, "ASSEMBLY_CONSTRAINT": "unrelated-receipt-role"}
			expectedIDs := []string{"receipt-sketch", "receipt-extrusion"}
			checks := []map[string]any{
				{"id": "rectangle", "kind": "RECTANGLE_DIMENSIONS", "width": 80, "height": 50, "unit": "mm"},
				{"id": "extrusion", "kind": "EXTRUSION_LENGTH", "length": 12, "unit": "mm"},
			}
			expectedChecks := []check{
				{ID: "rectangle", Kind: "RECTANGLE_DIMENSIONS", BodyID: body, Unit: "mm", Width: 80, Height: 50},
				{ID: "extrusion", Kind: "EXTRUSION_LENGTH", BodyID: body, Unit: "mm", Length: 12},
			}
			if fillet {
				targets["FILLET"] = "receipt-all-edge-fillet"
				expectedIDs = append(expectedIDs, "receipt-all-edge-fillet")
				checks = append(checks, map[string]any{"id": "all-edges", "kind": "ALL_EDGES_FILLET", "radius": 2, "expectedCount": 12, "unit": "mm"})
				expectedChecks = append(expectedChecks, check{ID: "all-edges", Kind: "ALL_EDGES_FILLET", BodyID: body, Unit: "mm", Radius: 2, ExpectedCount: 12})
			}
			checks = append(checks, map[string]any{"id": "solid", "kind": "BODY_GEOMETRY_VALID"})
			expectedChecks = append(expectedChecks, check{ID: "solid", Kind: "BODY_GEOMETRY_VALID", BodyID: body})
			verified := false
			polls := 0
			completed := map[string]any{"id": runID, "documentId": document, "version": 7, "epoch": 2, "goalVersion": 3, "status": "COMPLETED", "latestRevisionId": finalRevision, "targets": targets, "verification": map[string]any{"status": "VERIFIED", "revisionId": finalRevision, "goalVersion": 3}, "modelCalls": 1, "usage": map[string]int{"total_tokens": 250}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				write := func(value any) {
					if err := json.NewEncoder(w).Encode(value); err != nil {
						t.Error(err)
					}
				}
				switch r.Method + " " + r.URL.Path {
				case "POST /api/documents":
					write(map[string]any{"document": map[string]string{"id": document, "versionId": initialRevision}, "part": map[string]string{"activeBodyId": body}})
				case "POST /api/documents/fixture-document/assistant/runs":
					write(map[string]any{"id": runID, "documentId": document, "version": 1, "epoch": 1, "status": "PLANNING"})
				case "GET /api/documents/fixture-document/assistant/runs/fixture-run":
					polls++
					write(completed)
				case "POST /api/documents/fixture-document/automation/verify-part":
					var input struct {
						RevisionID       string   `json:"revisionId"`
						GoalVersion      uint64   `json:"goalVersion"`
						CreatedEntityIDs []string `json:"createdEntityIds"`
						Checks           []check  `json:"checks"`
					}
					decoder := json.NewDecoder(r.Body)
					decoder.DisallowUnknownFields()
					if err := decoder.Decode(&input); err != nil {
						t.Error("invalid independent verification request", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if polls == 0 || input.RevisionID != finalRevision || input.GoalVersion != 3 || !reflect.DeepEqual(input.CreatedEntityIDs, expectedIDs) || !reflect.DeepEqual(input.Checks, expectedChecks) {
						t.Errorf("independent checker lost receipt scope or predefined dimensions/fillet checks: got %+v; expected IDs=%v checks=%+v", input, expectedIDs, expectedChecks)
						w.WriteHeader(http.StatusBadRequest)
						write(map[string]any{"passed": false, "code": "OUTPUT_SCOPE_REQUIRED"})
						return
					}
					verified = true
					write(map[string]any{"passed": true, "revisionId": finalRevision, "checks": input.Checks})
				case "GET /api/documents/fixture-document/assistant/runs/fixture-run/calls":
					write(map[string]any{"items": []map[string]any{{"id": "fixture-call", "status": "APPLIED", "usageState": "REPORTED", "generation": map[string]any{"provider": "DeepSeek", "providerOrigin": "https://api.deepseek.com", "model": "reported-model", "thinking": "off", "version": 9}, "usage": map[string]int{"prompt_tokens": 200, "completion_tokens": 50, "total_tokens": 250, "cacheHitTokens": 0, "reasoningTokens": 0}}}, "hasMore": false})
				case "GET /api/documents/fixture-document/assistant/runs/fixture-run/events":
					write(map[string]any{"items": []any{}, "hasMore": false})
				default:
					t.Errorf("unexpected request (must not modify credentials/settings): %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			client, err := newClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			result := client.evaluate(t.Context(), evalCase{ID: caseID, Setup: "EMPTY_PART", Prompt: "HTTP contract fixture", Checks: checks}, settings{ProviderOrigin: "https://api.deepseek.com", Model: "reported-model", Thinking: "off", Version: 9}, 20000, 2*time.Second)
			if !verified || result.Status != "PASSED" || result.RunID != runID || result.ModelCalls != 1 || len(result.Calls) != 1 {
				t.Fatal("completed Run failed independent verification despite authoritative receipt targets", result)
			}
		})
	}
}

// This test verifies only the evaluation client's HTTP lifecycle. It never
// represents real provider planning, geometry execution or live acceptance.
func TestEvalWaitTimeoutStopsVersionedRunAndPreservesUnknownUsage(t *testing.T) {
	for _, stopSucceeds := range []bool{true, false} {
		t.Run(map[bool]string{true: "stopped", false: "unknown-stop"}[stopSucceeds], func(t *testing.T) {
			stopped := false
			version := uint64(1)
			stopCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				write := func(v any) {
					if err := json.NewEncoder(w).Encode(v); err != nil {
						t.Error(err)
					}
				}
				switch r.Method + " " + r.URL.Path {
				case "POST /api/documents":
					write(map[string]any{"document": map[string]string{"id": "fixture-document", "versionId": "fixture-revision"}, "part": map[string]string{"activeBodyId": "fixture-body"}})
				case "POST /api/documents/fixture-document/assistant/runs":
					var input map[string]any
					if json.NewDecoder(r.Body).Decode(&input) != nil {
						t.Error("invalid create input")
					}
					if input["tokenBudget"] != float64(20000) || input["mode"] != "AUTOMATIC" {
						t.Error("evaluation did not supply explicit independent Run budget")
					}
					if _, ok := input["deadline"]; ok {
						t.Error("evaluation timeout leaked into production Run lifetime")
					}
					write(map[string]any{"id": "fixture-run", "documentId": "fixture-document", "version": version, "epoch": 3, "status": "PLANNING"})
				case "GET /api/documents/fixture-document/assistant/runs/fixture-run":
					version++
					status := "PLANNING"
					if stopped {
						status = "STOPPED"
					}
					write(map[string]any{"id": "fixture-run", "documentId": "fixture-document", "version": version, "epoch": 3, "status": status, "modelCalls": 1, "usage": map[string]int{"unknownAttempts": 1}})
				case "POST /api/documents/fixture-document/assistant/runs/fixture-run/control":
					stopCalls++
					var input map[string]any
					if json.NewDecoder(r.Body).Decode(&input) != nil {
						t.Error("invalid stop input")
					}
					if input["action"] != "stop" || input["expectedVersion"] != float64(version) || input["epoch"] != float64(3) {
						t.Error("stop was not bound to current Run version/epoch")
					}
					if !stopSucceeds {
						w.WriteHeader(500)
						write(map[string]string{"error": "fixture stop failure"})
						return
					}
					stopped = true
					write(map[string]any{"id": "fixture-run", "documentId": "fixture-document", "version": version + 1, "epoch": 4, "status": "STOPPED", "modelCalls": 1, "usage": map[string]int{"unknownAttempts": 1}})
				case "GET /api/documents/fixture-document/assistant/runs/fixture-run/calls":
					write(map[string]any{"items": []map[string]any{{"id": "call", "status": "UNKNOWN", "usageState": "UNKNOWN", "generation": map[string]any{"provider": "DeepSeek", "providerOrigin": "https://api.deepseek.com", "model": "recorded-model", "thinking": "off", "version": 9}, "response": map[string]string{"reasoning_content": "PRIVATE_DO_NOT_EXPORT"}}}, "hasMore": false})
				case "GET /api/documents/fixture-document/assistant/runs/fixture-run/events":
					write(map[string]any{"items": []any{}, "hasMore": false})
				default:
					t.Errorf("unexpected request (must never modify credentials/settings): %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			client, err := newClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			result := client.evaluate(context.Background(), evalCase{ID: "box", Setup: "EMPTY_PART", Prompt: "protocol fixture"}, settings{ProviderOrigin: "https://api.deepseek.com", Model: "initial-model", Thinking: "off", Version: 8}, 20000, 10*time.Millisecond)
			if result.Status != "TIMED_OUT" || result.RunID != "fixture-run" || stopCalls < 1 {
				t.Fatal("timeout did not retain/stop real Run identity", result)
			}
			if stopSucceeds && result.StopStatus != "STOPPED" {
				t.Fatal("successful stop not recorded", result)
			}
			if !stopSucceeds && result.StopStatus != "UNKNOWN" {
				t.Fatal("failed stop falsely reported stopped", result)
			}
			if len(result.Calls) != 1 || result.Calls[0].Generation.Version != 9 || result.Calls[0].Generation.Model != "recorded-model" {
				t.Fatal("call settings snapshot not retained", result)
			}
			raw, _ := json.Marshal(result)
			if strings.Contains(string(raw), "PRIVATE_DO_NOT_EXPORT") || !strings.Contains(string(raw), "unknown for 1 attempts") {
				t.Fatal("private response leaked or unknown usage became zero", string(raw))
			}
		})
	}
}
