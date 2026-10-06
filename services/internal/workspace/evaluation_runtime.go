package workspace

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

// Runtime records use the existing graph's node keys, never a second graph.
type EvaluationExecution struct {
	Executions   int     `json:"executions"`
	Reuses       int     `json:"reuses"`
	OutputDigest string  `json:"outputDigest,omitempty"`
	DurationMS   float64 `json:"durationMs"`
}
type EvaluationRuntime struct {
	Nodes      map[modelcore.DependencyKey]EvaluationExecution `json:"nodes"`
	WorkerRuns []*workerv1.PartRuntimeStats                    `json:"workerRuns,omitempty"`
}
type evaluationRuntime struct {
	coldPrepared preparedCache
	mu           sync.Mutex
	records      EvaluationRuntime
	bodies       map[string]string
}
type evaluationRuntimeKey struct{}

func withEvaluationRuntime(ctx context.Context) context.Context {
	if _, ok := ctx.Value(evaluationRuntimeKey{}).(*evaluationRuntime); ok {
		return ctx
	}
	return context.WithValue(ctx, evaluationRuntimeKey{}, &evaluationRuntime{records: EvaluationRuntime{Nodes: map[modelcore.DependencyKey]EvaluationExecution{}}, bodies: map[string]string{}})
}

// WithColdEvaluation is a recovery/test path: bypasses preparation, stage,
// response, display and persisted result reuse, using the same operators.
func WithColdEvaluation(ctx context.Context) context.Context {
	options := geometry.PartRuntime(ctx)
	options.ForceCold = true
	return geometry.WithPartRuntime(ctx, options)
}
func recordEvaluation(ctx context.Context, key string, reused bool, digest string, elapsed time.Duration) {
	runtime, _ := ctx.Value(evaluationRuntimeKey{}).(*evaluationRuntime)
	if runtime == nil {
		return
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	k := modelcore.DependencyKey(key)
	r := runtime.records.Nodes[k]
	if reused {
		r.Reuses++
	} else {
		r.Executions++
	}
	r.OutputDigest = digest
	r.DurationMS += float64(elapsed) / float64(time.Millisecond)
	if !strings.HasPrefix(key, "@") {
		runtime.records.Nodes[k] = r
	}
}
func runtimeSnapshot(ctx context.Context) *EvaluationRuntime {
	r, _ := ctx.Value(evaluationRuntimeKey{}).(*evaluationRuntime)
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	b, _ := json.Marshal(r.records)
	var out EvaluationRuntime
	_ = json.Unmarshal(b, &out)
	return &out
}
func recordWorkerEvaluation(ctx context.Context, e *workerv1.EvaluatePartResponse) {
	r, _ := ctx.Value(evaluationRuntimeKey{}).(*evaluationRuntime)
	if r == nil {
		return
	}
	if stats := e.GetRuntimeStats(); stats != nil {
		for _, stage := range stats.Stages {
			recordEvaluation(ctx, "feature:"+stage.FeatureId, stage.Reused, stage.GeometryId, 0)
		}
		r.mu.Lock()
		r.records.WorkerRuns = append(r.records.WorkerRuns, stats)
		r.mu.Unlock()
	}
}

// Byte bounded immutable preparation results. In-flight entries are pinned;
// cancellation/failure never installs a result and waiting callers can retry.
type preparedEntry struct {
	data  []byte
	done  chan struct{}
	used  uint64
	bytes int
}
type preparedCache struct {
	mu      sync.Mutex
	entries map[string]*preparedEntry
	bytes   int
	clock   uint64
}

const preparedBudget = 32 * 1024 * 1024

func preparedResult[T any](ctx context.Context, c *preparedCache, node string, input any, compute func() (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return zero, err
	}
	key := evaluatorVersion + "|" + strings.SplitN(node, ":", 2)[0] + "|" + modelcore.ValueDigest(raw)
	started := time.Now()
	if geometry.PartRuntime(ctx).ForceCold {
		if runtime, ok := ctx.Value(evaluationRuntimeKey{}).(*evaluationRuntime); ok {
			// Cold means no cross-request heat. Identical preparation in this
			// fresh frozen request still has a single owner and execution.
			c = &runtime.coldPrepared
		} else {
			v, e := compute()
			recordEvaluation(ctx, node, false, "", time.Since(started))
			return v, e
		}
	}
	for {
		c.mu.Lock()
		if c.entries == nil {
			c.entries = map[string]*preparedEntry{}
		}
		if entry := c.entries[key]; entry != nil {
			c.clock++
			entry.used = c.clock
			if entry.done != nil {
				done := entry.done
				c.mu.Unlock()
				select {
				case <-ctx.Done():
					return zero, ctx.Err()
				case <-done:
				}
				continue
			}
			data := entry.data
			c.mu.Unlock()
			var result T
			err := json.Unmarshal(data, &result)
			recordEvaluation(ctx, node, true, modelcore.ValueDigest(data), time.Since(started))
			return result, err
		}
		entry := &preparedEntry{done: make(chan struct{})}
		c.entries[key] = entry
		c.mu.Unlock()
		result, e := compute()
		var data []byte
		if e == nil && ctx.Err() == nil {
			data, e = json.Marshal(result)
		}
		if e == nil && ctx.Err() != nil {
			e = ctx.Err()
		}
		c.mu.Lock()
		delete(c.entries, key)
		charge := len(data) + len(key) + 128
		if e == nil && charge <= preparedBudget && ctx.Err() == nil {
			for c.bytes+charge > preparedBudget {
				oldest := ""
				var stamp uint64 = ^uint64(0)
				for k, v := range c.entries {
					if v.done == nil && v.used < stamp {
						oldest = k
						stamp = v.used
					}
				}
				if oldest == "" {
					break
				}
				c.bytes -= c.entries[oldest].bytes
				delete(c.entries, oldest)
			}
			c.clock++
			entry.data = data
			entry.used = c.clock
			entry.bytes = charge
			c.bytes += charge
			c.entries[key] = entry
		}
		close(entry.done)
		entry.done = nil
		c.mu.Unlock()
		recordEvaluation(ctx, node, false, modelcore.ValueDigest(data), time.Since(started))
		return result, e
	}
}

func attachEvaluationRuntime(ctx context.Context, manifest *modelcore.EvaluationManifest) {
	if r := runtimeSnapshot(ctx); r != nil {
		manifest.Runtime, _ = json.Marshal(r)
	}
}

func (s *Service) projectExternalGeometry(ctx context.Context, requestID, sketchID, id string, source geometry.ExternalProjectionSource, frame geometry.ExternalProjectionFrame) (geometry.ProjectedExternalGeometry, error) {
	return preparedResult(ctx, &s.prepared, "projection:"+sketchID+"/"+id, struct {
		Source geometry.ExternalProjectionSource
		Frame  geometry.ExternalProjectionFrame
	}{source, frame}, func() (geometry.ProjectedExternalGeometry, error) {
		return s.worker.ProjectExternalGeometry(ctx, requestID+"/projection/"+id, source, frame)
	})
}

func resolveRuntimeParameters(ctx context.Context, m *PartModel) error {
	started := time.Now()
	err := validateAndResolvePartParameters(m)
	if err == nil {
		for _, p := range m.Parameters {
			b, _ := json.Marshal(p.EvaluatedValue)
			recordEvaluation(ctx, "parameter:"+p.ParameterID, false, modelcore.ValueDigest(b), time.Since(started))
		}
	}
	return err
}
