// Package totp verifies six-digit time-based one-time passwords: RFC 6238 on
// top of RFC 4226, with the SHA-1 and 30-second defaults every authenticator
// app assumes. Nothing here is proprietary and nothing is fetched: HMAC-SHA1
// and a counter are all a TOTP is.
//
// The panel has one account and does not want a second dependency for one
// optional field, so this is the whole of it - parse a base32 secret, derive
// the code for a moment in time, compare against the code a person typed.
package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// Digits is what every authenticator app shows for this kind of secret.
	Digits = 6
	// Period is the step a code is valid for.
	Period = 30 * time.Second
	// Skew is how many steps either side of the current one are accepted:
	// one step covers the clock difference between a phone and a container
	// nobody synchronised. It is a window of three steps in total, which is
	// the usual compromise - wide enough for a slow finger, narrow enough
	// that a code is not worth stealing.
	Skew = 1

	// minSecretLength is 80 bits, the floor RFC 4226 puts on a shared secret.
	minSecretLength = 10
)

// Key is a parsed shared secret.
type Key struct {
	secret []byte
}

// Parse reads the base32 secret an authenticator app was given. Grouping
// spaces and dashes are how people copy one out of a QR code's text, and lower
// case is how they type it, so both are accepted; anything else that is not
// base32 is an error, because a secret nobody can use is a second factor
// nobody can pass.
func Parse(secret string) (Key, error) {
	clean := strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(strings.TrimSpace(secret)))
	if clean == "" {
		return Key{}, errors.New("empty TOTP secret")
	}
	clean = strings.TrimRight(clean, "=")

	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(clean)
	if err != nil {
		return Key{}, fmt.Errorf("TOTP secret is not base32: %w", err)
	}
	if len(decoded) < minSecretLength {
		return Key{}, fmt.Errorf("TOTP secret is %d bytes, want at least %d (80 bits)", len(decoded), minSecretLength)
	}

	return Key{secret: decoded}, nil
}

// Code returns the code for the step t falls in.
func (k Key) Code(t time.Time) string {
	return k.codeAt(uint64(t.Unix() / int64(Period/time.Second)))
}

// Verify reports whether code is the one for t, one step either side. The
// comparison is constant time, and a code of the wrong length is refused
// before any HMAC runs.
func (k Key) Verify(code string, t time.Time) bool {
	_, ok := k.Match(code, t)
	return ok
}

// Match is Verify with the answer made explicit: it reports the step counter
// the code belongs to, and whether the code is one of the ones the window
// accepts. A caller that has to refuse a code it has already accepted needs the
// counter, not just the verdict - the six digits alone do not say how old they
// are. The highest matching counter is the one returned, and the comparison is
// the same constant-time one Verify makes.
func (k Key) Match(code string, t time.Time) (uint64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return 0, false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return 0, false
		}
	}

	var (
		matched uint64
		found   bool
	)
	counter := uint64(t.Unix() / int64(Period/time.Second))
	for offset := -Skew; offset <= Skew; offset++ {
		candidate := counter + uint64(offset)
		if offset < 0 && counter < uint64(-offset) {
			// Before the epoch: nothing to step back to.
			continue
		}
		if subtle.ConstantTimeCompare([]byte(k.codeAt(candidate)), []byte(code)) == 1 {
			// Two steps can derive the same six digits; the later one
			// is the safer answer, because accepting the earlier one
			// would leave the other usable a second time.
			if !found || candidate > matched {
				matched, found = candidate, true
			}
		}
	}

	return matched, found
}

// codeAt is the HOTP of RFC 4226 section 5.3 for one counter value, with the
// dynamic truncation of section 5.4 and the last six digits of the result.
func (k Key) codeAt(counter uint64) string {
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], counter)

	mac := hmac.New(sha1.New, k.secret)
	mac.Write(message[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	return fmt.Sprintf("%0*d", Digits, value%1_000_000)
}
