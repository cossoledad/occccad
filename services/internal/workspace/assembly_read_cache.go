package workspace

import (
	"context"
	"sync"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
)

// One solve (including admission probes) shares immutable reads. Never cache
// mutable Heads, authorization, occurrence poses or solver outcomes here. Values
// are read-only; the cache dies with the operation, so no invalidation is needed.
type assemblyReadCache struct {
	mu     sync.Mutex
	values map[any]any
}
type assemblyReadCacheContextKey struct{}
type assemblyReadKey struct{ kind, document, revision string }
type assemblyTopologyKey struct {
	geometry, kind string
	localID        uint64
}
type assemblyManifestRead struct {
	manifest         *workerv1.PartTopologyManifest
	geometry, digest string
}

func withAssemblyReadCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, assemblyReadCacheContextKey{}, &assemblyReadCache{values: map[any]any{}})
}

func assemblyRead[T any](ctx context.Context, key any, read func() (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	cache, _ := ctx.Value(assemblyReadCacheContextKey{}).(*assemblyReadCache)
	if cache == nil {
		return read()
	}
	cache.mu.Lock()
	cached, ok := cache.values[key]
	cache.mu.Unlock()
	if ok {
		return cached.(T), nil
	}
	value, err := read()
	// In particular, never turn transient infrastructure failures into cached
	// missing supports during later admission attempts.
	if err == nil {
		cache.mu.Lock()
		cache.values[key] = value
		cache.mu.Unlock()
	}
	return value, err
}
