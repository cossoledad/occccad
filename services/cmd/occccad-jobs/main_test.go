package main

import (
	"fmt"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestJobConcurrencyBounds(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]int{"": 2, "1": 1, "8": 8, "0": 2, "9": 2, "bad": 2} {
		if got := jobConcurrency(input); got != want {
			t.Errorf("jobConcurrency(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestImportConcurrencyAndPermanentFailures(t *testing.T) {
	for raw, want := range map[string]int{"": 4, "2": 2, "8": 8, "0": 4, "9": 9, "32": 32} {
		if got := importConcurrency(raw); got != want {
			t.Fatalf("%q: %d", raw, got)
		}
	}
	if retryableImportError(fmt.Errorf("wrapped: %w", workspace.ErrValidation)) {
		t.Fatal("validation error must not retry")
	}
	if retryableImportError(fmt.Errorf("worker: %w", status.Error(codes.InvalidArgument, "invalid solid"))) {
		t.Fatal("invalid geometry must not retry")
	}
	if !retryableImportError(status.Error(codes.Unavailable, "worker restarting")) {
		t.Fatal("transient transport failure should retry")
	}
}
