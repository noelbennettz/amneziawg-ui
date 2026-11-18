package auth

import (
	"sync"
	"time"
)

// The login limiter. Off by default (WEB_UI_LOGIN_RATE_LIMIT), because a
// panel behind a proxy that does not forward the client address sees one
// address for everybody - and then a limiter locks the operator out along with
// the attacker. Turning it on is the right choice whenever the panel is on the
// internet and the real address arrives.
const (
	limitWindow   = 15 * time.Minute
	limitFailures = 5
	// A blocked address waits this long before it may try again, and each
	// further block doubles it. The ceiling keeps a mistake - or a shared
	// address - from becoming a permanent lockout.
	limitBlock = 15 * time.Minute
	limitMax   = 24 * time.Hour
)

// attempt is what the limiter remembers about one address.
type attempt struct {
	// windowStart is when the current run of failures began. A failure that
	// arrives after the window slides starts a new run at zero.
	windowStart time.Time
	failures    int
	// strikes counts how many times this address has been blocked, and is
	// what the backoff doubles.
	strikes      int
	blockedUntil time.Time
}

// limiter counts failed logins per address. A nil one allows everything, so
// the disabled case costs a nil check rather than a branch at every call site.
type limiter struct {
	mu      sync.Mutex
	entries map[string]*attempt
	now     func() time.Time
}

func newLimiter() *limiter {
	return &limiter{entries: map[string]*attempt{}, now: time.Now}
}

// retryAfter reports how long the address still has to wait, or zero when it
// may try now.
func (l *limiter) retryAfter(ip string) time.Duration {
	if l == nil {
		return 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[ip]
	if !ok {
		return 0
	}
	if wait := entry.blockedUntil.Sub(l.now()); wait > 0 {
		return wait
	}
	return 0
}

// fail records a wrong password. The fifth one inside the window blocks the
// address, and every block after that doubles.
func (l *limiter) fail(ip string) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	entry, ok := l.entries[ip]
	if !ok {
		entry = &attempt{windowStart: now}
		l.entries[ip] = entry
	}
	if now.Sub(entry.windowStart) > limitWindow {
		entry.windowStart = now
		entry.failures = 0
	}

	entry.failures++
	if entry.failures < limitFailures {
		return
	}

	entry.strikes++
	entry.failures = 0
	entry.windowStart = now
	entry.blockedUntil = now.Add(backoff(entry.strikes))
}

// succeed forgets the address: a right password is the end of the matter.
func (l *limiter) succeed(ip string) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, ip)
}

// backoff is the block a strike count is worth: the base, doubled once per
// further strike, capped.
func backoff(strikes int) time.Duration {
	wait := limitBlock
	for i := 1; i < strikes && wait < limitMax; i++ {
		wait *= 2
	}
	if wait > limitMax {
		wait = limitMax
	}
	return wait
}

// sweep drops the addresses that are neither blocked nor inside a window.
// It runs on a failure, which is the only path that adds entries: the map
// stays as small as the number of addresses currently being guessed at.
func (l *limiter) sweep(now time.Time) {
	for ip, entry := range l.entries {
		if now.Before(entry.blockedUntil) {
			continue
		}
		if now.Sub(entry.windowStart) > limitWindow {
			delete(l.entries, ip)
		}
	}
}
