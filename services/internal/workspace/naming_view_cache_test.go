package workspace

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

type namingWaitContext struct {
	context.Context
	ready chan struct{}
	once  sync.Once
}

func (ctx *namingWaitContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.ready) })
	return ctx.Context.Done()
}

func TestNamingViewCacheBudgetEvictionAndLeases(t *testing.T) {
	cache := &namingViewCache{}
	loads := map[string]int{}
	read := func(key string) *topologyManifest {
		t.Helper()
		value, err := cache.read(t.Context(), key, 30, func() (*topologyManifest, int64, error) {
			loads[key]++
			return &topologyManifest{GeometryID: key}, 10, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	a, b := read("a"), read("b")
	read("c")
	if read("a") != a {
		t.Fatal("immutable view was decoded again")
	}
	read("d")
	if cache.bytes > 30 || loads["a"] != 1 || b.GeometryID != "b" {
		t.Fatal("budget or borrowed lease invalid", cache.bytes, loads)
	}
	read("b")
	if loads["b"] != 2 {
		t.Fatal("LRU did not release the oldest root", loads)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := cache.read(ctx, "b", 30, func() (*topologyManifest, int64, error) {
		t.Fatal("cancelled hit attempted a load")
		return nil, 0, nil
	}); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestNamingViewCacheSharesOversizedFlightWithoutRetainingIt(t *testing.T) {
	cache := &namingViewCache{}
	started, release := make(chan struct{}), make(chan struct{})
	value := &topologyManifest{GeometryID: "large"}
	var loads atomic.Int32
	owner := make(chan error, 1)
	go func() {
		_, err := cache.read(t.Context(), "large", 10, func() (*topologyManifest, int64, error) {
			loads.Add(1)
			close(started)
			<-release
			return value, 20, nil
		})
		owner <- err
	}()
	<-started
	ctx := &namingWaitContext{Context: t.Context(), ready: make(chan struct{})}
	waiter := make(chan *topologyManifest, 1)
	go func() {
		got, err := cache.read(ctx, "large", 10, func() (*topologyManifest, int64, error) { loads.Add(1); return value, 20, nil })
		if err != nil {
			waiter <- nil
			return
		}
		waiter <- got
	}()
	<-ctx.ready
	close(release)
	if err := <-owner; err != nil {
		t.Fatal(err)
	}
	if got := <-waiter; got != value || loads.Load() != 1 || cache.bytes != 0 {
		t.Fatal("oversized single flight was recomputed or retained", loads.Load(), cache.bytes)
	}
}

func TestNamingViewCacheCancelledOwnerLetsWaiterRetry(t *testing.T) {
	cache := &namingViewCache{}
	ownerCtx, cancel := context.WithCancel(t.Context())
	started, release := make(chan struct{}), make(chan struct{})
	owner := make(chan error, 1)
	go func() {
		_, err := cache.read(ownerCtx, "same", 30, func() (*topologyManifest, int64, error) {
			close(started)
			<-release
			return &topologyManifest{GeometryID: "cancelled"}, 10, nil
		})
		owner <- err
	}()
	<-started
	ctx := &namingWaitContext{Context: t.Context(), ready: make(chan struct{})}
	waiter := make(chan *topologyManifest, 1)
	go func() {
		got, err := cache.read(ctx, "same", 30, func() (*topologyManifest, int64, error) { return &topologyManifest{GeometryID: "valid"}, 10, nil })
		if err != nil {
			waiter <- nil
			return
		}
		waiter <- got
	}()
	<-ctx.ready
	cancel()
	close(release)
	if err := <-owner; err != context.Canceled {
		t.Fatal("cancelled owner published a result", err)
	}
	if got := <-waiter; got == nil || got.GeometryID != "valid" {
		t.Fatal("waiter inherited cancellation", got)
	}
	if cache.bytes != 10 {
		t.Fatal("failed attempt retained data", cache.bytes)
	}
}

func TestNamingViewCacheWaitingCancellationAndFailuresDoNotPoisonOwner(t *testing.T) {
	cache := &namingViewCache{}
	started, release := make(chan struct{}), make(chan struct{})
	owner := make(chan error, 1)
	go func() {
		_, err := cache.read(t.Context(), "same", 30, func() (*topologyManifest, int64, error) {
			close(started)
			<-release
			return &topologyManifest{GeometryID: "valid"}, 10, nil
		})
		owner <- err
	}()
	<-started
	base, cancel := context.WithCancel(t.Context())
	ctx := &namingWaitContext{Context: base, ready: make(chan struct{})}
	waiter := make(chan error, 1)
	go func() {
		_, err := cache.read(ctx, "same", 30, func() (*topologyManifest, int64, error) { return nil, 0, errors.New("unexpected load") })
		waiter <- err
	}()
	<-ctx.ready
	cancel()
	if err := <-waiter; err != context.Canceled {
		t.Fatal(err)
	}
	close(release)
	if err := <-owner; err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		_, err := cache.read(t.Context(), "failure", 30, func() (*topologyManifest, int64, error) { return nil, 0, errors.New("retryable") })
		if err == nil {
			t.Fatal("failure was hidden")
		}
	}
	if cache.bytes != 10 {
		t.Fatal("infrastructure failure became a cached support", cache.bytes)
	}
}
