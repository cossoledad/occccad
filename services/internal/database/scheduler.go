package database

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	perf "github.com/occccad/occccad/internal/performance"
	"golang.org/x/sync/semaphore"
)

var ErrBusy = errors.New("database admission is busy")

type backgroundKey struct{}

// Background gives polling and maintenance a separate bounded waiting budget
// and caps their active connections, leaving capacity for interactive work.
func Background(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundKey{}, true)
}

type Limits struct {
	Concurrent           int
	Queued               int
	BackgroundConcurrent int
	BackgroundQueued     int
	WaitTimeout          time.Duration
}

func (l Limits) validate() error {
	if l.Concurrent < 1 || l.Queued < 0 || l.BackgroundConcurrent < 1 || l.BackgroundConcurrent > l.Concurrent || l.BackgroundQueued < 0 || l.WaitTimeout <= 0 {
		return errors.New("invalid database scheduling limits")
	}
	return nil
}

type scheduler struct {
	limits                             Limits
	active, background                 *semaphore.Weighted
	foregroundBudget, backgroundBudget chan struct{}
	running, waiting                   atomic.Int64
	rejected                           atomic.Uint64
}

func newScheduler(l Limits) *scheduler {
	return &scheduler{limits: l, active: semaphore.NewWeighted(int64(l.Concurrent)), background: semaphore.NewWeighted(int64(l.BackgroundConcurrent)), foregroundBudget: make(chan struct{}, l.Queued), backgroundBudget: make(chan struct{}, l.BackgroundQueued)}
}

func (s *scheduler) acquire(ctx context.Context) (func(), error) {
	finish := perf.Start(ctx, "db-queue-wait")
	defer finish()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	background, _ := ctx.Value(backgroundKey{}).(bool)
	// The fast path requires no waiting slot. TryAcquire respects existing
	// semaphore waiters, preventing fresh arrivals from bypassing queued work.
	acquired := false
	if background {
		if s.background.TryAcquire(1) {
			acquired = s.active.TryAcquire(1)
			if !acquired {
				s.background.Release(1)
			}
		}
	} else {
		acquired = s.active.TryAcquire(1)
	}
	if !acquired {
		budget := s.foregroundBudget
		if background {
			budget = s.backgroundBudget
		}
		select {
		case budget <- struct{}{}:
		default:
			s.rejected.Add(1)
			return nil, ErrBusy
		}
		s.waiting.Add(1)
		defer func() { s.waiting.Add(-1); <-budget }()
		wait, cancel := context.WithTimeout(ctx, s.limits.WaitTimeout)
		defer cancel()
		if background {
			if err := s.background.Acquire(wait, 1); err != nil {
				return nil, admissionWaitError(ctx, err)
			}
		}
		if err := s.active.Acquire(wait, 1); err != nil {
			if background {
				s.background.Release(1)
			}
			return nil, admissionWaitError(ctx, err)
		}
	}

	s.running.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() {
			s.running.Add(-1)
			s.active.Release(1)
			if background {
				s.background.Release(1)
			}
		})
	}, nil
}

func admissionWaitError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("%w: %w", ErrBusy, err)
}
