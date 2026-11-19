package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"sync"
	"time"
)

// credentialCache remembers Basic credentials that have already been verified,
// so a script that sends its Authorization header with every request does not
// start a fresh argon2id derivation each time.
//
// What it holds is an HMAC of the pair under a key generated when the process
// started: the password is not in memory in the clear, and the key dies with
// the process, so nothing here can be read back out of a core dump as a
// password. An entry is dropped when its five minutes are up, and every entry
// is dropped implicitly when the password changes - the entry carries the
// generation it was written under, and a lookup with a newer one misses.
type credentialCache struct {
	ttl time.Duration
	// secret is the per-process key the entries are keyed with.
	secret [sha256.Size]byte
	// enabled is off when the process could not read random bytes: a cache
	// keyed by something guessable is worse than no cache.
	enabled bool

	mu      sync.Mutex
	entries map[[sha256.Size]byte]cachedCredential
}

type cachedCredential struct {
	expires    time.Time
	generation uint64
}

// newCredentialCache builds the cache and draws its key. crypto/rand does not
// fail on a working system; when it does, the cache stays off and every check
// verifies against the hash as before.
func newCredentialCache(ttl time.Duration) *credentialCache {
	c := &credentialCache{ttl: ttl, entries: map[[sha256.Size]byte]cachedCredential{}}
	if _, err := rand.Read(c.secret[:]); err == nil {
		c.enabled = true
	}

	return c
}

// key is the HMAC of one credential pair. The two are separated by a NUL so
// that ("ab", "c") and ("a", "bc") cannot collide.
func (c *credentialCache) key(user, password string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, c.secret[:])
	mac.Write([]byte(user))
	mac.Write([]byte{0})
	mac.Write([]byte(password))

	var sum [sha256.Size]byte
	copy(sum[:], mac.Sum(nil))

	return sum
}

// get reports whether the pair behind key was verified under generation and
// has not expired at now.
func (c *credentialCache) get(key [sha256.Size]byte, generation uint64, now time.Time) bool {
	if !c.enabled {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return false
	}
	if entry.generation != generation || !now.Before(entry.expires) {
		// Stale in one of the two ways an entry can be: a new password, or
		// five minutes gone by. Either way it is no longer an answer.
		delete(c.entries, key)
		return false
	}

	return true
}

// put remembers a verified pair until ttl from now.
func (c *credentialCache) put(key [sha256.Size]byte, generation uint64, now time.Time) {
	if !c.enabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Expired entries are only worth sweeping when there is something to
	// sweep: the map holds one entry per distinct credential that worked, so
	// it is small either way.
	for k, entry := range c.entries {
		if !now.Before(entry.expires) {
			delete(c.entries, k)
		}
	}
	c.entries[key] = cachedCredential{expires: now.Add(c.ttl), generation: generation}
}
