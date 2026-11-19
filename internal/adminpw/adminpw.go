// Package adminpw owns the admin password of the panel: the argon2id hash it
// is stored as, the admin.hash file on the config volume, and the password
// that is generated on the first start when nobody configured one.
//
// The password is never written to disk and never kept in memory in the
// clear: only the hash is stored, and only the hash is compared against.
//
// The base64(SHA-256) format releases before argon2id used is refused, not
// migrated: it is a single fast digest of the password, and a hash is worth
// exactly what it costs to attack. Nothing accepts it any more.
package adminpw

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"

	"amneziawg-web-ui/internal/atomicfile"
)

const (
	// FileName is the file on the config volume that holds the stored hash.
	FileName = "admin.hash"

	// PasswordLength is how many characters a generated password has.
	PasswordLength = 24

	// SaltLength and KeyLength are the sizes of the salt and the derived key
	// of a hash this package writes.
	SaltLength = 16
	KeyLength  = 32

	// The argon2id parameters this package writes: the second recommended
	// option of RFC 9106 - 19 MiB, two passes, one lane - which is what the
	// rest of the ecosystem settled on too, so a hash generated here has the
	// same size and shape as one generated elsewhere.
	Memory      uint32 = 19456 // KiB
	Time        uint32 = 2
	Parallelism uint8  = 1

	// The ceilings on the parameters this package will run. The hash can be
	// attacker-supplied when WEB_UI_PASSWORD is, and argon2id allocates what
	// the hash asks for: without a limit, one line in a compose file turns
	// every login attempt into a memory bomb.
	MaxMemory      uint32 = 256 * 1024 // KiB, 256 MiB
	MaxTime        uint32 = 10
	MaxParallelism uint8  = 8

	// MinMemory and MinSaltLength keep a hash from being cheap to verify, and
	// keep the decoder from accepting a degenerate one.
	MinMemory     uint32 = 8
	MinSaltLength int    = 8
	minKeyLength  int    = 16
	maxSaltLength int    = 64
	maxKeyLength  int    = 64

	// checkInterval is how often the store looks at the hash file for a
	// change. Rewriting the file is how "reset-password" reaches a panel that
	// is already running, and once a second is often enough for that without
	// putting a stat() on every request.
	checkInterval = time.Second
)

// ErrDefaultPassword is returned when the configured hash turns out to be the
// password the image used to ship. It is a sentinel rather than a string
// comparison: the check is "does this hash verify changeme", so a hash of it
// is refused whatever format it is in.
var ErrDefaultPassword = errors.New(`the configured password is the old default "changeme"`)

// alphabet of a generated password: [A-Za-z0-9], the set that survives being
// read out of a log line and typed into a phone.
const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// Hash is a password hash: an argon2id PHC string and the parameters it
// carries.
type Hash struct {
	// The argon2id parameters, as the PHC string carries them.
	memory      uint32
	time        uint32
	parallelism uint8
	salt        []byte
	key         []byte
}

// ErrLegacyFormat is returned for a hash in the base64(SHA-256) format that
// earlier releases used. It is refused rather than migrated, and named as its
// own error so that the message an operator reads says what to do about it.
var ErrLegacyFormat = errors.New(`the base64(SHA-256) password hash format is no longer supported; make an argon2id hash with "api hash-password" and use that instead`)

// Parse reads an argon2id PHC string.
//
// The parameters are checked against the ceilings; anything that does not
// parse exactly is an error, because a hash nobody can verify is a panel
// nobody can log in to, and the start is the moment to say so.
func Parse(s string) (Hash, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Hash{}, errors.New("empty password hash")
	}
	if !strings.HasPrefix(s, "$") {
		// The one other format this project ever wrote: base64 of a bare
		// SHA-256 digest. Recognise it to say what it is, rather than
		// answering a migration problem with a syntax error.
		if raw, err := base64.StdEncoding.DecodeString(s); err == nil && len(raw) == sha256.Size {
			return Hash{}, ErrLegacyFormat
		}
		return Hash{}, errors.New("not an argon2id hash: want $argon2id$v=19$m=..,t=..,p=..$salt$hash, which is what \"api hash-password\" prints")
	}

	return parsePHC(s)
}

// parsePHC reads "$argon2id$v=19$m=...,t=...,p=...$<salt>$<key>".
func parsePHC(s string) (Hash, error) {
	parts := strings.Split(s, "$")
	if len(parts) != 6 || parts[0] != "" {
		return Hash{}, fmt.Errorf("argon2id hash has %d fields, want 6: $argon2id$v=..$m=..,t=..,p=..$salt$hash", len(parts))
	}
	if parts[1] != "argon2id" {
		return Hash{}, fmt.Errorf("unsupported hash algorithm %q, want argon2id", parts[1])
	}

	version, ok := strings.CutPrefix(parts[2], "v=")
	if !ok {
		return Hash{}, fmt.Errorf("unsupported version field %q, want v=19", parts[2])
	}
	if version != strconv.Itoa(argon2.Version) {
		return Hash{}, fmt.Errorf("unsupported argon2 version %q, want v=%d", version, argon2.Version)
	}

	h := Hash{}
	for _, field := range strings.Split(parts[3], ",") {
		name, value, ok := strings.Cut(field, "=")
		if !ok {
			return Hash{}, fmt.Errorf("malformed argon2id parameter %q", field)
		}
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return Hash{}, fmt.Errorf("argon2id parameter %s=%q is not a number", name, value)
		}
		switch name {
		case "m":
			h.memory = uint32(n)
		case "t":
			h.time = uint32(n)
		case "p":
			if n > 255 {
				return Hash{}, fmt.Errorf("argon2id parallelism is %d, want at most %d", n, MaxParallelism)
			}
			h.parallelism = uint8(n)
		default:
			return Hash{}, fmt.Errorf("unknown argon2id parameter %q", name)
		}
	}

	switch {
	case h.memory == 0 || h.time == 0 || h.parallelism == 0:
		return Hash{}, errors.New("argon2id hash is missing one of m, t or p")
	case h.memory < MinMemory || h.memory > MaxMemory:
		return Hash{}, fmt.Errorf("argon2id memory is %d KiB, want %d..%d", h.memory, MinMemory, MaxMemory)
	case h.time > MaxTime:
		return Hash{}, fmt.Errorf("argon2id time is %d, want at most %d", h.time, MaxTime)
	case h.parallelism > MaxParallelism:
		return Hash{}, fmt.Errorf("argon2id parallelism is %d, want at most %d", h.parallelism, MaxParallelism)
	}

	var err error
	if h.salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return Hash{}, fmt.Errorf("argon2id salt is not base64: %w", err)
	}
	if h.key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return Hash{}, fmt.Errorf("argon2id key is not base64: %w", err)
	}
	if len(h.salt) < MinSaltLength || len(h.salt) > maxSaltLength {
		return Hash{}, fmt.Errorf("argon2id salt is %d bytes, want %d..%d", len(h.salt), MinSaltLength, maxSaltLength)
	}
	if len(h.key) < minKeyLength || len(h.key) > maxKeyLength {
		return Hash{}, fmt.Errorf("argon2id key is %d bytes, want %d..%d", len(h.key), minKeyLength, maxKeyLength)
	}

	return h, nil
}

// String renders the hash the way it is stored: rebuilt from its parameters,
// which is what makes a hash that parsed exactly also print exactly.
func (h Hash) String() string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, h.memory, h.time, h.parallelism,
		base64.RawStdEncoding.EncodeToString(h.salt),
		base64.RawStdEncoding.EncodeToString(h.key))
}

// Verify reports whether password matches the hash. The comparison is constant
// time: a wrong guess learns that it was wrong, and nothing about how close it
// was.
func (h Hash) Verify(password string) bool {
	got := argon2.IDKey([]byte(password), h.salt, h.time, h.memory, h.parallelism, uint32(len(h.key)))

	return subtle.ConstantTimeCompare(got, h.key) == 1
}

// HashPassword hashes a password with the parameters this package writes and
// returns the PHC string to store.
func HashPassword(password string) (string, error) {
	salt := make([]byte, SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, Time, Memory, Parallelism, KeyLength)

	return Hash{memory: Memory, time: Time, parallelism: Parallelism, salt: salt, key: key}.String(), nil
}

// Generate returns a random password.
//
// The characters are drawn with rejection sampling rather than by taking a
// byte modulo 62: 256 is not a multiple of 62, so the modulo would make the
// first eight letters of the alphabet a little likelier than the rest.
func Generate() (string, error) {
	const limit = 256 - 256%len(alphabet) // 248: bytes at or above it are redrawn

	out := make([]byte, 0, PasswordLength)
	buf := make([]byte, PasswordLength)
	for len(out) < PasswordLength {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate password: %w", err)
		}
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			out = append(out, alphabet[int(b)%len(alphabet)])
			if len(out) == PasswordLength {
				break
			}
		}
	}

	return string(out), nil
}

// ReadPassword reads one line from r, which is how "hash-password" takes the
// password: never from the command line, where it would end up in the shell
// history and in the process list.
func ReadPassword(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, 4096))
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	password := strings.TrimRight(string(data), "\r\n")
	if password == "" {
		return "", errors.New("no password on standard input")
	}

	return password, nil
}

// Store is the password the panel checks a login against.
//
// It prefers WEB_UI_PASSWORD when that is set, and otherwise the hash file on
// the config volume - which it re-reads when the file changes, so a new
// password written by "reset-password" from another process takes effect in
// the running panel without a restart.
type Store struct {
	path string

	// fromEnv is WEB_UI_PASSWORD. When it is set the file is not read at
	// all: an environment variable an operator spelled out beats whatever a
	// previous run happened to store.
	fromEnv bool

	mu      sync.Mutex
	hash    Hash
	modTime time.Time
	checked time.Time
	loadErr error
	// generation counts the password changes this process has seen. Anything
	// cached against a password is dropped when it moves.
	generation uint64

	now func() time.Time
}

// Open resolves the password source. envHash is WEB_UI_PASSWORD as configured,
// and empty when it is unset.
//
// With no environment variable and no stored hash it generates a password,
// stores only its hash, and returns the plaintext - which the caller prints
// once, and which is the one moment the password exists in the clear.
func Open(path, envHash string) (store *Store, generated string, err error) {
	s := &Store{path: path, now: time.Now}

	if envHash = strings.TrimSpace(envHash); envHash != "" {
		hash, err := Parse(envHash)
		if err != nil {
			return nil, "", fmt.Errorf("WEB_UI_PASSWORD: %w", err)
		}
		s.hash, s.fromEnv = hash, true

		return s, "", nil
	}

	hash, modTime, err := readFile(path)
	switch {
	case err == nil:
		s.hash, s.modTime = hash, modTime
		return s, "", nil
	case !errors.Is(err, os.ErrNotExist):
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}

	password, err := Generate()
	if err != nil {
		return nil, "", err
	}
	encoded, err := HashPassword(password)
	if err != nil {
		return nil, "", err
	}
	if err := WriteHash(path, encoded); err != nil {
		return nil, "", err
	}
	// Read the file back for its modification time, so the first refresh does
	// not mistake our own write for somebody else's.
	if hash, modTime, err := readFile(path); err == nil {
		s.hash, s.modTime = hash, modTime
	} else if s.hash, err = Parse(encoded); err != nil {
		return nil, "", err
	}

	return s, password, nil
}

// Hash returns the hash in force and the generation it belongs to. The
// generation changes whenever the stored hash is rewritten, which is what lets
// a cached credential and an issued session be recognised as stale.
func (s *Store) Hash() (Hash, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.refreshLocked()

	return s.hash, s.generation
}

// FromEnv reports whether WEB_UI_PASSWORD is what is in force.
func (s *Store) FromEnv() bool { return s.fromEnv }

// Err returns the error of the last attempt to reload the hash file, when that
// attempt failed. A broken file keeps the previous password in force: a
// half-written hash must not lock the operator out, so the error is reported
// and not acted on.
func (s *Store) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadErr
}

// CheckDefault refuses the password the image used to ship. It is a
// verification and not a string comparison on purpose: the point is that no
// hash of "changeme" - in either format - is what the panel starts with.
func (s *Store) CheckDefault() error {
	hash, _ := s.Hash()
	if hash.Verify("changeme") {
		return ErrDefaultPassword
	}

	return nil
}

// refreshLocked re-reads the hash file when it has changed, at most once per
// checkInterval.
func (s *Store) refreshLocked() {
	if s.fromEnv {
		return
	}
	now := s.now()
	if now.Sub(s.checked) < checkInterval {
		return
	}
	s.checked = now

	hash, modTime, err := readFile(s.path)
	if err != nil {
		s.loadErr = err
		return
	}
	s.loadErr = nil
	if modTime.Equal(s.modTime) || hash.String() == s.hash.String() {
		s.modTime = modTime
		return
	}
	// A new password is a new generation: sessions signed under the old one
	// stop verifying, and cached credentials stop counting.
	s.hash, s.modTime = hash, modTime
	s.generation++
}

// Reset writes a newly generated password's hash to path and returns the
// plaintext. A panel running against that file picks it up within a second.
func Reset(path string) (string, error) {
	password, err := Generate()
	if err != nil {
		return "", err
	}
	encoded, err := HashPassword(password)
	if err != nil {
		return "", err
	}
	if err := WriteHash(path, encoded); err != nil {
		return "", err
	}

	return password, nil
}

// WriteHash stores a hash with the permissions it deserves: an operator with a
// shell in the container can read the file anyway, but nothing else on the
// host or in a backup needs to.
func WriteHash(path, encoded string) error {
	hash, err := Parse(encoded)
	if err != nil {
		return err
	}

	return atomicfile.Write(path, []byte(hash.String()+"\n"), 0o600)
}

// readFile loads and parses the hash file.
func readFile(path string) (Hash, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Hash{}, time.Time{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Hash{}, time.Time{}, err
	}
	hash, err := Parse(string(data))
	if err != nil {
		return Hash{}, info.ModTime(), fmt.Errorf("%s: %w", path, err)
	}

	return hash, info.ModTime(), nil
}
