package performance

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Recorder is request-scoped and intentionally stores only bounded, low-cardinality
// phase names. Entity IDs belong in traces, never metric labels or Server-Timing.
type Recorder struct {
	mu     sync.Mutex
	phases map[string]time.Duration
}

type recorderKey struct{}

func WithRecorder(ctx context.Context) (context.Context, *Recorder) {
	recorder := &Recorder{phases: map[string]time.Duration{}}
	return context.WithValue(ctx, recorderKey{}, recorder), recorder
}

func Start(ctx context.Context, phase string) func() {
	started := time.Now()
	return func() {
		if recorder, ok := ctx.Value(recorderKey{}).(*Recorder); ok {
			recorder.mu.Lock()
			recorder.phases[phase] += time.Since(started)
			recorder.mu.Unlock()
		}
	}
}

func (recorder *Recorder) Header() string {
	if recorder == nil {
		return ""
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	names := make([]string, 0, len(recorder.phases))
	for name := range recorder.phases {
		names = append(names, name)
	}
	sort.Strings(names)
	values := make([]string, 0, len(names))
	for _, name := range names {
		safe := strings.Map(func(value rune) rune {
			if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '-' || value == '_' {
				return value
			}
			return '-'
		}, strings.ToLower(name))
		values = append(values, fmt.Sprintf("%s;dur=%.3f", safe, float64(recorder.phases[name].Microseconds())/1000))
	}
	return strings.Join(values, ", ")
}

func (recorder *Recorder) SnapshotMilliseconds() map[string]float64 {
	result := map[string]float64{}
	if recorder == nil {
		return result
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for name, duration := range recorder.phases {
		result[name] = float64(duration.Microseconds()) / 1000
	}
	return result
}
