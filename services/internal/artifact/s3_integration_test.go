//go:build integration

package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/occccad/occccad/internal/config"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Uses the selected S3 backend and only removes this test's random object.
func TestS3IntegrationStreaming(t *testing.T) {
	if !strings.EqualFold(os.Getenv("OCCCCAD_ARTIFACT_BACKEND"), "S3") {
		t.Skip("NOT_APPLICABLE: configured artifact backend is not S3")
	}
	if _, err := config.LoadEnvironment(); err != nil {
		t.Fatal(err)
	}
	store, _, err := OpenConfigured(context.Background(), config.Load().DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if store.Backend() != "S3" {
		t.Fatal("S3 backend required")
	}
	// The upload buffer belongs to this call; persistent objects still use the
	// one configured bucket. Concurrent application uploads are not test failures.
	privateSpool, err := os.MkdirTemp(store.(*S3Store).temporaryDirectory, "probe-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(privateSpool)
	store.(*S3Store).temporaryDirectory = privateSpool
	size := int64(17 << 20)
	if raw := os.Getenv("OCCCCAD_TEST_S3_BYTES"); raw != "" {
		size, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	hash := sha256.New()
	input := io.TeeReader(io.LimitReader(rand.New(rand.NewSource(time.Now().UnixNano())), size), hash)
	object, err := store.Put(ctx, KindExchangeSource, "application/octet-stream", input)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Delete(context.Background(), object.Key); err != nil {
			t.Error(err)
		}
	}()
	if object.Size != size || object.SHA256 != hex.EncodeToString(hash.Sum(nil)) {
		t.Fatal("upload integrity mismatch")
	}
	reader, err := store.Open(ctx, object.Key)
	if err != nil {
		t.Fatal(err)
	}
	hash.Reset()
	n, err := io.Copy(hash, reader)
	closeErr := reader.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("download %v %v", err, closeErr)
	}
	if n != size || hex.EncodeToString(hash.Sum(nil)) != object.SHA256 {
		t.Fatal("download integrity mismatch")
	}
	spool := store.(*S3Store).temporaryDirectory
	entries, _ := filepath.Glob(filepath.Join(spool, "upload-*"))
	if len(entries) > 0 {
		t.Fatal("spools leaked")
	}
	t.Logf("verified %d bytes through S3 multipart upload and streaming download", n)
}
