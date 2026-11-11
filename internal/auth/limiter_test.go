package auth

import (
	"testing"
	"time"
)

func newTestLimiter(now *time.Time) *limiter {
	l := newLimiter()
	l.now = func() time.Time { return *now }
	return l
}

// Five wrong passwords block the address; a sixth attempt is told how long to
// wait, and the address is free again once that has passed.
func TestLimiterBlocksAfterFiveFailures(t *testing.T) {
	now := time.Unix(testNow, 0)
	l := newTestLimiter(&now)

	for i := 0; i < limitFailures; i++ {
		if wait := l.retryAfter("10.0.0.1"); wait != 0 {
			t.Fatalf("blocked after %d failures", i)
		}
		l.fail("10.0.0.1")
	}

	wait := l.retryAfter("10.0.0.1")
	if wait <= 0 || wait > limitBlock {
		t.Fatalf("wait = %v, want (0, %v]", wait, limitBlock)
	}

	now = now.Add(limitBlock)
	if wait := l.retryAfter("10.0.0.1"); wait != 0 {
		t.Errorf("still blocked after the block expired: %v", wait)
	}
}

// The second block lasts twice the first: a guesser who comes back finds the
// door further away, not equally far.
func TestLimiterBackoffDoubles(t *testing.T) {
	now := time.Unix(testNow, 0)
	l := newTestLimiter(&now)

	for range limitFailures {
		l.fail("10.0.0.2")
	}
	first := l.retryAfter("10.0.0.2")

	now = now.Add(first)
	for range limitFailures {
		l.fail("10.0.0.2")
	}
	second := l.retryAfter("10.0.0.2")

	if second != 2*first {
		t.Errorf("second block = %v, want %v", second, 2*first)
	}
}

// One right password clears the count: somebody who mistypes four times and
// then gets it right is not one attempt from a lockout.
func TestLimiterForgetsOnSuccess(t *testing.T) {
	now := time.Unix(testNow, 0)
	l := newTestLimiter(&now)

	for range limitFailures - 1 {
		l.fail("10.0.0.3")
	}
	l.succeed("10.0.0.3")
	l.fail("10.0.0.3")

	if wait := l.retryAfter("10.0.0.3"); wait != 0 {
		t.Errorf("blocked after a success reset the count: %v", wait)
	}
}

// The count belongs to one address: the attacker's failures must not lock out
// the operator behind a different one.
func TestLimiterIsPerAddress(t *testing.T) {
	now := time.Unix(testNow, 0)
	l := newTestLimiter(&now)

	for range limitFailures {
		l.fail("10.0.0.4")
	}
	if wait := l.retryAfter("10.0.0.5"); wait != 0 {
		t.Errorf("another address was blocked: %v", wait)
	}
}

// A failure older than the window does not count towards the next one.
func TestLimiterWindowSlides(t *testing.T) {
	now := time.Unix(testNow, 0)
	l := newTestLimiter(&now)

	for range limitFailures - 1 {
		l.fail("10.0.0.6")
	}
	now = now.Add(limitWindow + time.Minute)
	l.fail("10.0.0.6")

	if wait := l.retryAfter("10.0.0.6"); wait != 0 {
		t.Errorf("failures from an old window still count: %v", wait)
	}
}

// Off is a nil limiter, and a nil limiter says yes to everything.
func TestDisabledLimiterAllowsEverything(t *testing.T) {
	var l *limiter
	for range limitFailures * 3 {
		l.fail("10.0.0.7")
	}
	if wait := l.retryAfter("10.0.0.7"); wait != 0 {
		t.Errorf("a disabled limiter blocked: %v", wait)
	}
	l.succeed("10.0.0.7") // must not panic on nil
}
