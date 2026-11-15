package workspace

import (
	"context"
	"sync"
)

const namingViewBudget int64 = 128 * 1024 * 1024

type namingViewEntry struct {
	value *topologyManifest
	done  chan struct{}
	bytes int64
	used  uint64
}

// Parsed immutable artifact views, not mutable Heads or per-selection results.
// Callers qualify source revisions, owners, policy and authorization separately.
// Leases returned to a reader survive cache eviction. Charges conservatively
// estimate decoded data/indexes; this is not a whole-process RSS limit.
type namingViewCache struct {
	mu      sync.Mutex
	entries map[string]*namingViewEntry
	bytes   int64
	clock   uint64
}

func (cache *namingViewCache) read(ctx context.Context, key string, budget int64, load func() (*topologyManifest, int64, error)) (*topologyManifest, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cache.mu.Lock()
		if cache.entries == nil {
			cache.entries = map[string]*namingViewEntry{}
		}
		if entry := cache.entries[key]; entry != nil {
			if entry.done != nil {
				done := entry.done
				cache.mu.Unlock()
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-done:
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					if entry.value != nil {
						return entry.value, nil
					}
					continue
				}
			}
			cache.clock++
			entry.used = cache.clock
			value := entry.value
			cache.mu.Unlock()
			return value, nil
		}
		entry := &namingViewEntry{done: make(chan struct{})}
		cache.entries[key] = entry
		cache.mu.Unlock()
		value, charge, err := load()
		if err == nil {
			err = ctx.Err()
		}
		cache.mu.Lock()
		if err == nil {
			err = ctx.Err()
		}
		delete(cache.entries, key)
		if err == nil {
			entry.value = value
		}
		if err == nil && value != nil && charge > 0 && charge <= budget {
			for cache.bytes+charge > budget {
				oldest := ""
				stamp := ^uint64(0)
				for k, v := range cache.entries {
					if v.done == nil && v.used < stamp {
						oldest, stamp = k, v.used
					}
				}
				if oldest == "" {
					break
				}
				cache.bytes -= cache.entries[oldest].bytes
				delete(cache.entries, oldest)
			}
			cache.clock++
			entry.value = value
			entry.bytes = charge
			entry.used = cache.clock
			cache.bytes += charge
			cache.entries[key] = entry
		}
		close(entry.done)
		entry.done = nil
		cache.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return value, nil
	}
}
