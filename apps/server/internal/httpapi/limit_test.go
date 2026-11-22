package httpapi

import (
	"testing"
	"time"
)

func TestAttemptLimiterLocksAfterRepeatedFailures(t *testing.T) {
	lim := newAttemptLimiter(3, time.Minute)
	now := time.Now()
	if lim.blocked("a", now) {
		t.Fatal("fresh key should be allowed")
	}
	lim.recordFailure("a", now)
	lim.recordFailure("a", now)
	if lim.blocked("a", now) {
		t.Fatal("two failures should not lock a limit of three")
	}
	lim.recordFailure("a", now)
	if !lim.blocked("a", now) {
		t.Fatal("expected the key to be locked")
	}
	if lim.blocked("a", now.Add(2*time.Minute)) {
		t.Fatal("expected the window to expire")
	}
	lim.reset("a")
	if lim.blocked("a", now) {
		t.Fatal("reset should clear the key")
	}
}
