package devtool

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/occccad/occccad/internal/config"
	"golang.org/x/sync/errgroup"
)

type exchangeCorpusFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type exchangeCorpusPackage struct {
	ID     string               `json:"id"`
	URL    string               `json:"url"`
	SHA256 string               `json:"sha256"`
	Files  []exchangeCorpusFile `json:"files"`
}

func exchangeCorpusMatches(data []byte, digest string, size int64) bool {
	sum := sha256.Sum256(data)
	return (size < 0 || int64(len(data)) == size) && hex.EncodeToString(sum[:]) == digest
}
func prepareExchangePackage(ctx context.Context, client *http.Client, pkg exchangeCorpusPackage, directory string) error {
	complete := true
	for _, file := range pkg.Files {
		data, err := os.ReadFile(filepath.Join(directory, file.SHA256+".stp"))
		if err != nil || !exchangeCorpusMatches(data, file.SHA256, file.Bytes) {
			complete = false
			break
		}
	}
	if complete {
		return nil
	}
	archivePath := filepath.Join(directory, pkg.SHA256+".zip")
	data, _ := os.ReadFile(archivePath)
	if !exchangeCorpusMatches(data, pkg.SHA256, -1) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pkg.URL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "occccad-exchange-corpus/1")
		response, err := client.Do(req)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("NIST archive %s returned HTTP %d", pkg.ID, response.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(response.Body, (512<<20)+1))
		if err != nil {
			return err
		}
		if len(data) > 512<<20 || !exchangeCorpusMatches(data, pkg.SHA256, -1) {
			return fmt.Errorf("NIST archive %s integrity mismatch", pkg.ID)
		}
		if err = writeExchangeCorpusFile(archivePath, data); err != nil {
			return err
		}
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	entries := map[string]*zip.File{}
	for _, entry := range archive.File {
		entries[entry.Name] = entry
	}
	for _, file := range pkg.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := entries[file.Path]
		if entry == nil {
			return fmt.Errorf("NIST archive %s missing %s", pkg.ID, file.Path)
		}
		stream, err := entry.Open()
		if err != nil {
			return err
		}
		content, err := io.ReadAll(io.LimitReader(stream, file.Bytes+1))
		closeErr := stream.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if !exchangeCorpusMatches(content, file.SHA256, file.Bytes) {
			return fmt.Errorf("NIST file %s integrity mismatch", file.Path)
		}
		// Only content hashes become destination paths. ZIP member paths are never
		// extracted onto disk; unrelated native models/PDFs remain in the archive.
		if err = writeExchangeCorpusFile(filepath.Join(directory, file.SHA256+".stp"), content); err != nil {
			return err
		}
	}
	return nil
}
func writeExchangeCorpusFile(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".prepare-")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err = temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}
func (a *App) PrepareExchangeCorpus() error {
	if _, err := config.LoadEnvironment(); err != nil {
		return err
	}
	var manifest struct {
		Schema   string                  `json:"schema"`
		Packages []exchangeCorpusPackage `json:"packages"`
	}
	data, err := os.ReadFile(filepath.Join(a.Root, "tests/exchange-corpus/manifest.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.Schema != "occccad.exchange-corpus.v1" || len(manifest.Packages) == 0 {
		return fmt.Errorf("invalid exchange corpus manifest")
	}
	directory := filepath.Join(os.Getenv("OCCCCAD_TEST_RESOURCES_DIR"), "nist-step")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	group, ctx := errgroup.WithContext(a.Context)
	group.SetLimit(4)
	files := map[string]bool{}
	for _, pkg := range manifest.Packages {
		u, err := url.Parse(pkg.URL)
		if err != nil || u.Scheme != "https" || u.Hostname() != "www.nist.gov" {
			return fmt.Errorf("untrusted NIST archive URL")
		}
		validDigest := func(value string) bool { decoded, e := hex.DecodeString(value); return e == nil && len(decoded) == 32 }
		if !validDigest(pkg.SHA256) {
			return fmt.Errorf("invalid NIST archive digest")
		}
		for _, file := range pkg.Files {
			if !validDigest(file.SHA256) || file.Bytes < 1 {
				return fmt.Errorf("invalid NIST file manifest")
			}
			files[file.SHA256] = true
		}
	}
	for _, pkg := range manifest.Packages {
		group.Go(func() error { return prepareExchangePackage(ctx, client, pkg, directory) })
	}
	if err = group.Wait(); err != nil {
		return err
	}
	return json.NewEncoder(a.Out).Encode(map[string]any{"schema": "occccad.exchange-corpus-preparation.v1", "status": "PASS", "archives": len(manifest.Packages), "verifiedFiles": len(files), "directory": directory})
}
