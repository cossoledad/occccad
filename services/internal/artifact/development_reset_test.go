package artifact

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/occccad/occccad/internal/config"
)

func TestS3DevelopmentResetVersionsUploadsAndFailures(t *testing.T) {
	for _, failure := range []string{"", "list", "delete", "multipart"} {
		t.Run(failure, func(t *testing.T) {
			deleted, aborted := false, false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/test-reset/" && r.URL.Path != "/test-reset" && r.URL.Path != "/test-reset/unfinished" {
					t.Errorf("unexpected target %s", r.URL.Path)
				}
				if r.Method == "HEAD" {
					w.WriteHeader(200)
					return
				}
				query := r.URL.Query()
				if query.Has("versions") {
					if failure == "list" {
						w.WriteHeader(403)
						fmt.Fprint(w, "<Error><Code>AccessDenied</Code></Error>")
						return
					}
					body := ""
					if !deleted {
						body = "<Version><Key>part</Key><VersionId>v1</VersionId></Version><Version><Key>part</Key><VersionId>v2</VersionId></Version><DeleteMarker><Key>part</Key><VersionId>v3</VersionId></DeleteMarker>"
					}
					fmt.Fprint(w, "<ListVersionsResult><IsTruncated>false</IsTruncated>"+body+"</ListVersionsResult>")
					return
				}
				if query.Has("delete") {
					var batch struct {
						Objects []struct {
							Key     string `xml:"Key"`
							Version string `xml:"VersionId"`
						} `xml:"Object"`
					}
					if err := xml.NewDecoder(r.Body).Decode(&batch); err != nil {
						t.Error(err)
					}
					if len(batch.Objects) != 3 {
						t.Errorf("expected all versions and delete marker: %+v", batch)
					}
					for _, obj := range batch.Objects {
						if obj.Key != "part" || obj.Version == "" {
							t.Errorf("lost version: %+v", obj)
						}
					}
					if failure == "delete" {
						fmt.Fprint(w, "<DeleteResult><Error><Key>part</Key><VersionId>v1</VersionId><Code>AccessDenied</Code><Message>retained</Message></Error></DeleteResult>")
						return
					}
					deleted = true
					fmt.Fprint(w, "<DeleteResult/>")
					return
				}
				if query.Has("uploads") {
					body := ""
					if !aborted {
						body = "<Upload><Key>unfinished</Key><UploadId>upload-1</UploadId></Upload>"
					}
					fmt.Fprint(w, "<ListMultipartUploadsResult><IsTruncated>false</IsTruncated>"+body+"</ListMultipartUploadsResult>")
					return
				}
				if r.Method == "DELETE" && query.Get("uploadId") == "upload-1" {
					if failure == "multipart" {
						w.WriteHeader(403)
						fmt.Fprint(w, "<Error><Code>AccessDenied</Code></Error>")
						return
					}
					aborted = true
					w.WriteHeader(204)
					return
				}
				t.Errorf("unexpected S3 request %s %s", r.Method, r.URL)
				w.WriteHeader(500)
			}))
			defer server.Close()
			store, err := NewS3Store(t.Context(), S3Config{Endpoint: strings.TrimPrefix(server.URL, "http://"), AccessKey: "test", SecretKey: "testsecret", Bucket: "test-reset", Region: "us-east-1", TemporaryDirectory: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			err = store.ResetDevelopment(t.Context())
			if (err != nil) != (failure != "") {
				t.Fatalf("failure=%q err=%v", failure, err)
			}
			if failure == "" && (!deleted || !aborted) {
				t.Fatal("incomplete cleanup")
			}
		})
	}
}

// Creates and clears only a unique test bucket; never resets the configured bucket.
func TestS3DevelopmentResetIntegration(t *testing.T) {
	if os.Getenv("OCCCCAD_TEST_S3_RESET") != "1" {
		t.Skip("opt-in isolated MinIO bucket test")
	}
	if _, err := config.LoadProjectEnv(); err != nil {
		t.Fatal(err)
	}
	store, _, err := OpenConfigured(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original, ok := store.(*S3Store)
	if !ok {
		t.Fatal("requires configured S3")
	}
	bucket := fmt.Sprintf("occccad-reset-test-%d", time.Now().UnixNano())
	if err := original.client.MakeBucket(t.Context(), bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	isolated := &S3Store{client: original.client, bucket: bucket, temporaryDirectory: t.TempDir()}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := isolated.ResetDevelopment(ctx); err != nil {
			t.Error(err)
		}
		if err := original.client.RemoveBucket(ctx, bucket); err != nil {
			t.Error(err)
		}
	}()
	// Exercise unversioned buckets before enabling versions on the same isolated bucket.
	if _, err := original.client.PutObject(t.Context(), bucket, "plain", strings.NewReader("data"), 4, minio.PutObjectOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := isolated.ResetDevelopment(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := original.client.SetBucketVersioning(t.Context(), bucket, minio.BucketVersioningConfiguration{Status: "Enabled"}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := original.client.PutObject(t.Context(), bucket, "part", strings.NewReader("data"), 4, minio.PutObjectOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := original.client.RemoveObject(t.Context(), bucket, "part", minio.RemoveObjectOptions{}); err != nil {
		t.Fatal(err)
	}
	core := minio.Core{Client: original.client}
	if _, err := core.NewMultipartUpload(t.Context(), bucket, "unfinished", minio.PutObjectOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := isolated.ResetDevelopment(t.Context()); err != nil {
		t.Fatal(err)
	}
	if exists, err := original.client.BucketExists(t.Context(), bucket); err != nil || !exists {
		t.Fatalf("bucket not preserved: %v", err)
	}
}
