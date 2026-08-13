package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestApplicationProxyPreservesBrowserFacingHost(t *testing.T) {
	requestSeen := make(chan struct {
		host, origin string
	}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestSeen <- struct {
			host, origin string
		}{request.Host, request.Header.Get("Origin")}
		_ = json.NewEncoder(writer).Encode(map[string]bool{"ok": true})
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	app := &application{apiTarget: target.Host}
	request := httptest.NewRequest(http.MethodGet, "http://localhost:5173/api/realtime", nil)
	request.Host = "localhost:5173"
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()
	app.proxy().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected proxy status %d", response.Code)
	}
	seen := <-requestSeen
	if seen.host != "localhost:5173" || seen.origin != "http://localhost:5173" {
		t.Fatalf("proxy changed same-origin identity: host=%q origin=%q", seen.host, seen.origin)
	}
}
