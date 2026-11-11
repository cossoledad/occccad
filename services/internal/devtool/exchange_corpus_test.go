package devtool

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestExchangeCorpusVerifiesArchivesAndFilesBeforePublication(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	content := []byte("ISO-10303-21; pinned source")
	w, _ := zw.Create("../../native/sample.stp")
	w.Write(content)
	zw.Close()
	digest := func(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.Write(archive.Bytes()) }))
	defer server.Close()
	pkg := exchangeCorpusPackage{ID: "fixture", URL: server.URL, SHA256: digest(archive.Bytes()), Files: []exchangeCorpusFile{{Path: "../../native/sample.stp", SHA256: digest(content), Bytes: int64(len(content))}}}
	directory := t.TempDir()
	if err := prepareExchangePackage(context.Background(), server.Client(), pkg, directory); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, pkg.Files[0].SHA256+".stp"))
	if err != nil || !bytes.Equal(data, content) {
		t.Fatal("content was not published by hash", err)
	}
	if err = prepareExchangePackage(context.Background(), server.Client(), pkg, directory); err != nil || requests.Load() != 1 {
		t.Fatal("valid corpus downloaded again", err)
	}
	os.Remove(filepath.Join(directory, pkg.Files[0].SHA256+".stp"))
	pkg.Files[0].SHA256 = strings.Repeat("a", 64)
	if err = prepareExchangePackage(context.Background(), server.Client(), pkg, directory); err == nil {
		t.Fatal("wrong source digest was accepted")
	}
	if _, err = os.Stat(filepath.Join(directory, pkg.Files[0].SHA256+".stp")); !os.IsNotExist(err) {
		t.Fatal("corrupt source was published")
	}
	pkg.SHA256 = strings.Repeat("b", 64)
	if err = prepareExchangePackage(context.Background(), server.Client(), pkg, directory); err == nil {
		t.Fatal("wrong archive digest was accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err = prepareExchangePackage(canceled, server.Client(), pkg, directory); err == nil {
		t.Fatal("canceled preparation succeeded")
	}
}
