package api

import (
	"bytes"
	"github.com/occccad/occccad/internal/thumbnail"
	"image/png"
	"net/http/httptest"
	"testing"
)

func TestDefaultThumbnailResponseIsPNG(t *testing.T) {
	response := httptest.NewRecorder()
	writeThumbnail(response, thumbnail.DefaultForType("PART"), "default")
	if response.Header().Get("Content-Type") != "image/png" || response.Header().Get("X-OCCCCAD-Thumbnail-State") != "default" {
		t.Fatal(response.Header())
	}
	config, err := png.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
	if err != nil || config.Width != thumbnail.Width || config.Height != thumbnail.Height {
		t.Fatalf("invalid default PNG: %v %v", config, err)
	}
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("pending placeholder must not become a permanently cached image")
	}
}
