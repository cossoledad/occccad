package artifact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func TestS3SpoolFailureDoesNotPublish(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodHead {
			t.Errorf("unexpected publication %s", r.Method)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	store, err := NewS3Store(context.Background(), S3Config{Endpoint: strings.TrimPrefix(server.URL, "http://"), AccessKey: "test", SecretKey: "testsecret", Bucket: "test-bucket", Region: "us-east-1", TemporaryDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	requests = 0
	if _, err := store.Put(context.Background(), KindBREP, "application/octet-stream", brokenReader{}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("lost failure: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Put(ctx, KindBREP, "application/octet-stream", strings.NewReader("test")); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if requests != 0 {
		t.Fatal("failed upload reached S3")
	}
	entries, _ := os.ReadDir(store.temporaryDirectory)
	if len(entries) != 0 {
		t.Fatal("spools leaked")
	}
}

func TestS3MultipartCancellationAbortsOwnSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var aborted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(200)
		case http.MethodPost:
			if r.URL.Query().Has("uploads") {
				w.Header().Set("Content-Type", "application/xml")
				fmt.Fprint(w, `<InitiateMultipartUploadResult><Bucket>test-bucket</Bucket><Key>test</Key><UploadId>own-session</UploadId></InitiateMultipartUploadResult>`)
			} else {
				t.Error("failed upload was completed")
				w.WriteHeader(500)
			}
		case http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			cancel()
			w.WriteHeader(500)
		case http.MethodDelete:
			if r.URL.Query().Get("uploadId") != "own-session" {
				t.Error("aborted another session")
			}
			aborted.Add(1)
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected request %s", r.Method)
		}
	}))
	defer server.Close()
	store, err := NewS3Store(ctx, S3Config{Endpoint: strings.TrimPrefix(server.URL, "http://"), AccessKey: "test", SecretKey: "testsecret", Bucket: "test-bucket", Region: "us-east-1", TemporaryDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Put(ctx, KindExchangeSource, "application/octet-stream", io.LimitReader(rand.New(rand.NewSource(3)), 17<<20))
	if err == nil {
		t.Fatal("cancellation reported success")
	}
	if aborted.Load() != 1 {
		t.Fatalf("abort count: %d", aborted.Load())
	}
	entries, _ := os.ReadDir(store.temporaryDirectory)
	if len(entries) != 0 {
		t.Fatal("canceled upload left spool")
	}
}
