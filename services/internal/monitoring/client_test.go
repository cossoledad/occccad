package monitoring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientFetchesVersionedSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/control/monitoring/snapshot" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"schema":"occccad.monitoring.snapshot.v1","business":{"counts":{}},"parameters":{}}`))
	}))
	defer server.Close()
	snapshot, err := NewClient(server.URL).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Schema != Schema {
		t.Fatalf("schema = %q", snapshot.Schema)
	}
}

func TestClientRejectsUnknownSchema(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"schema":"future.v2"}`))
	}))
	defer server.Close()
	if _, err := NewClient(server.URL).Fetch(context.Background()); err == nil {
		t.Fatal("expected schema error")
	}
}
