package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRootDescribesAPIInsteadOfServingFrontend(t *testing.T) {
	response := httptest.NewRecorder()
	New(nil, nil, nil, nil, nil, nil, nil, false, nil).Handler().ServeHTTP(
		response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"service":"occccad-api"`) {
		t.Fatalf("unexpected root response: status=%d body=%s", response.Code, response.Body.String())
	}

	missing := httptest.NewRecorder()
	New(nil, nil, nil, nil, nil, nil, nil, false, nil).Handler().ServeHTTP(
		missing, httptest.NewRequest(http.MethodGet, "/index.html", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("frontend files must not be served by the API: status=%d", missing.Code)
	}
}

func TestConfiguredCORSOrigin(t *testing.T) {
	handler := New(nil, nil, nil, nil, nil, nil, nil, false, []string{"http://localhost:5173"}).Handler()
	request := httptest.NewRequest(http.MethodOptions, "/api/documents", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" ||
		response.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("unexpected CORS response: status=%d headers=%v", response.Code, response.Header())
	}

	request = httptest.NewRequest(http.MethodOptions, "/api/documents", nil)
	request.Header.Set("Origin", "https://untrusted.example")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unexpected disallowed origin response: status=%d", response.Code)
	}
}
