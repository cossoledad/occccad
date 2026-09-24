package api

import (
	"github.com/occccad/occccad/internal/artifact"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestExchangeFormatsAndUploadLimit(t *testing.T) {
	if exchangeUploadLimit() < 1<<30 {
		t.Fatalf("exchange upload limit %d does not satisfy the 1 GiB product requirement", exchangeUploadLimit())
	}
	tests := []struct {
		input, format, extension string
		valid                    bool
	}{
		{"step", "STEP", ".step", true},
		{"STP", "STEP", ".step", true},
		{"brep", "BREP", ".brep", true},
		{"BRP", "BREP", ".brep", true},
		{"iges", "", "", false},
	}
	for _, test := range tests {
		format, extension, _, valid := exchangeFormat(test.input)
		if format != test.format || extension != test.extension || valid != test.valid {
			t.Fatalf("exchangeFormat(%q) = %q, %q, %v", test.input, format, extension, valid)
		}
	}
}

func TestExchangeUploadLimitConfiguration(t *testing.T) {
	t.Setenv("OCCCCAD_EXCHANGE_MAX_BYTES", "2147483648")
	if exchangeUploadLimit() != 2<<30 {
		t.Fatal("configured limit ignored")
	}
	for _, value := range []string{"", "-1", "+1024", "18446744073709551615", "invalid"} {
		t.Setenv("OCCCCAD_EXCHANGE_MAX_BYTES", value)
		if exchangeUploadLimit() != 16<<30 {
			t.Fatal("unsafe fallback")
		}
	}
}

func TestExchangeStreamingLimitRejectsBeforePublication(t *testing.T) {
	t.Setenv("OCCCCAD_EXCHANGE_MAX_BYTES", "1024")
	directory := t.TempDir()
	store, err := artifact.NewLocalStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{artifacts: artifact.NewService(nil, store)}
	request := httptest.NewRequest(http.MethodPost, "/api/exchange/imports?format=STEP&fileName=model.step", strings.NewReader(strings.Repeat("x", 1025)))
	request.ContentLength = -1 // chunked requests cannot bypass the limit
	writer := httptest.NewRecorder()
	server.startExchangeImport(writer, request)
	if writer.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d: %s", writer.Code, writer.Body.String())
	}
	files, _ := filepath.Glob(filepath.Join(directory, "artifacts", ".staging", "*"))
	if len(files) > 0 {
		t.Fatal("failed HTTP upload left scratch")
	}
}
