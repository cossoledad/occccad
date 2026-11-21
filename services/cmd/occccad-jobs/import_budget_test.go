package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestImportBudgetSharedAcrossFiles(t *testing.T) {
	budget := newImportBudget(3)
	var active, peak atomic.Int32
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	var group sync.WaitGroup
	for file := 0; file < 2; file++ {
		for component := 0; component < 5; component++ {
			group.Go(func() {
				err := budget.run(t.Context(), func() error {
					count := active.Add(1)
					for previous := peak.Load(); count > previous; previous = peak.Load() {
						if peak.CompareAndSwap(previous, count) {
							break
						}
					}
					select {
					case started <- struct{}{}:
					default:
					}
					<-release
					active.Add(-1)
					return nil
				})
				if err != nil {
					t.Error(err)
				}
			})
		}
	}
	for range 3 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("budget did not fill")
		}
	}
	if active.Load() != 3 {
		t.Errorf("active=%d", active.Load())
	}
	close(release)
	group.Wait()
	if peak.Load() != 3 {
		t.Fatalf("shared import peak=%d, expected 3", peak.Load())
	}
}

func TestImportBudgetCancellationAndFailureRelease(t *testing.T) {
	budget := newImportBudget(1)
	if err := budget.slots.Acquire(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := budget.run(ctx, func() error { t.Fatal("canceled work ran"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	budget.slots.Release(1)
	failure := errors.New("component failed")
	if err := budget.run(t.Context(), func() error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if !budget.slots.TryAcquire(1) {
		t.Fatal("failed operation leaked budget")
	}
	budget.slots.Release(1)
}

func TestUnsupportedImportDisplayRequiresWorkerDiagnosis(t *testing.T) {
	const detail = "VISUAL_MESH_FAILED:status=4"
	const message = "EXCHANGE_UNSUPPORTED_GEOMETRY:" + detail
	for _, tc := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("import BREP exchange: %w", status.Error(codes.FailedPrecondition, message)), detail},
		{status.Error(codes.FailedPrecondition, "source integrity mismatch"), ""},
		{status.Error(codes.Unavailable, message), ""},
		{status.Error(codes.Internal, message), ""},
		{context.Canceled, ""},
	} {
		if got := unsupportedImportDisplay(tc.err); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.err, got, tc.want)
		}
	}
}
