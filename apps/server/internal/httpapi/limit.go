package httpapi

import (
	"sync"
	"time"
)

type attemptLimiter struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	failures map[string]attemptState
}

type attemptState struct {
	count int
	start time.Time
}

func newAttemptLimiter(max int, window time.Duration) *attemptLimiter {
	return &attemptLimiter{
		max:      max,
		window:   window,
		failures: map[string]attemptState{},
	}
}

func (l *attemptLimiter) blocked(key string, now time.Time) bool {
	if key == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	state, ok := l.failures[key]
	if !ok || now.Sub(state.start) > l.window {
		return false
	}
	return state.count >= l.max
}

func (l *attemptLimiter) recordFailure(key string, now time.Time) {
	if key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.failures) >= 4096 {
		for existing, state := range l.failures {
			if now.Sub(state.start) > l.window {
				delete(l.failures, existing)
			}
		}
	}
	if len(l.failures) >= 4096 {
		for existing := range l.failures {
			delete(l.failures, existing)
			break
		}
	}
	state, ok := l.failures[key]
	if !ok || now.Sub(state.start) > l.window {
		l.failures[key] = attemptState{count: 1, start: now}
		return
	}
	state.count++
	l.failures[key] = state
}

func (l *attemptLimiter) reset(key string) {
	if key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}
