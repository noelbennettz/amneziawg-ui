package adminpw

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// changemeLegacy is base64(SHA-256("changeme")), the hash of the password the
// image used to ship, in the format WEB_UI_PASSWORD used to hold. The format
// is no longer accepted, and this constant is what proves that.
const changemeLegacy = "BXugPWxEEEhj3HNh/kV4ll0YhzYPkKCJWILlimJI/IY="

// testPassword is what most tests log in with.
const testPassword = "correct horse battery staple"

// hashOf is a hash of testPassword in the format this package writes.
func hashOf(t *testing.T) string {
	t.Helper()

	encoded, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	return encoded
}

func TestHashPasswordRoundTrip(t *testing.T) {
	encoded := hashOf(t)

	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash = %q, want the documented argon2id parameters", encoded)
	}

	hash, err := Parse(encoded)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !hash.Verify(testPassword) {
		t.Error("the hash does not verify the password it was made from")
	}
	if hash.Verify(testPassword + " ") {
		t.Error("the hash verifies a different password")
	}
	if hash.Verify("") {
		t.Error("the hash verifies an empty password")
	}
	if hash.String() != encoded {
		t.Errorf("String() = %q, want the hash it was parsed from", hash.String())
	}
}

func TestParseRejectsTheLegacyFormat(t *testing.T) {
	if _, err := Parse(changemeLegacy); !errors.Is(err, ErrLegacyFormat) {
		t.Fatalf("Parse(legacy hash) = %v, want ErrLegacyFormat", err)
	}

	// The message is what an operator sees at startup, so it has to say both
	// what was wrong and what to do.
	err := ErrLegacyFormat
	for _, want := range []string{"SHA-256", "argon2id", "hash-password"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the legacy error does not mention %q: %v", want, err)
		}
	}
}

func TestParseRejects(t *testing.T) {
	salt := base64.RawStdEncoding.EncodeToString(make([]byte, SaltLength))
	key := base64.RawStdEncoding.EncodeToString(make([]byte, KeyLength))
	with := func(params string) string {
		return "$argon2id$v=19$" + params + "$" + salt + "$" + key
	}

	cases := map[string]string{
		"empty":                   "",
		"blank":                   "   ",
		"not base64":              "!!!not-a-hash!!!",
		"short base64 digest":     base64.StdEncoding.EncodeToString(make([]byte, 16)),
		"unknown algorithm":       "$argon2i$v=19$m=19456,t=2,p=1$" + salt + "$" + key,
		"wrong version":           "$argon2id$v=16$m=19456,t=2,p=1$" + salt + "$" + key,
		"no version":              "$argon2id$m=19456,t=2,p=1$" + salt + "$" + key,
		"missing field":           "$argon2id$v=19$m=19456,t=2$" + salt + "$" + key,
		"unknown parameter":       with("m=19456,t=2,p=1,x=1"),
		"parameter without value": with("m=19456,t=2,p="),
		"not a number":            with("m=lots,t=2,p=1"),
		"missing parameters":      with("m=19456"),
		"memory over the ceiling": with("m=262145,t=2,p=1"),
		"time over the ceiling":   with("m=19456,t=11,p=1"),
		"parallelism over":        with("m=19456,t=2,p=9"),
		"memory under the floor":  with("m=4,t=2,p=1"),
		"time zero":               with("m=19456,t=0,p=1"),
		"salt not base64":         "$argon2id$v=19$m=19456,t=2,p=1$!!!$" + key,
		"key not base64":          "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$!!!",
		"short salt":              "$argon2id$v=19$m=19456,t=2,p=1$" + base64.RawStdEncoding.EncodeToString(make([]byte, 4)) + "$" + key,
		"short key":               "$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$" + base64.RawStdEncoding.EncodeToString(make([]byte, 8)),
	}

	for name, hash := range cases {
		if _, err := Parse(hash); err == nil {
			t.Errorf("%s: Parse accepted %q", name, hash)
		}
	}
}

// A hash inside the parameter ceilings is accepted: the limits bound what the
// process can be made to allocate, they do not demand this package's own
// numbers.
func TestParseAcceptsParametersAtTheCeiling(t *testing.T) {
	salt := base64.RawStdEncoding.EncodeToString(make([]byte, SaltLength))
	key := base64.RawStdEncoding.EncodeToString(make([]byte, KeyLength))
	hash := "$argon2id$v=19$m=262144,t=10,p=8$" + salt + "$" + key

	if _, err := Parse(hash); err != nil {
		t.Fatalf("Parse rejected a hash at the ceilings: %v", err)
	}
}

func TestGenerate(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		password, err := Generate()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if len(password) != PasswordLength {
			t.Fatalf("password %q is %d characters, want %d", password, len(password), PasswordLength)
		}
		for _, r := range password {
			if !strings.ContainsRune(alphabet, r) {
				t.Fatalf("password %q contains %q, which is not in A-Za-z0-9", password, r)
			}
		}
		if seen[password] {
			t.Fatalf("Generate returned %q twice", password)
		}
		seen[password] = true
	}
}

func TestReadPassword(t *testing.T) {
	password, err := ReadPassword(strings.NewReader("hunter2\n"))
	if err != nil || password != "hunter2" {
		t.Fatalf("ReadPassword = %q, %v", password, err)
	}
	// A CRLF from a Windows pipe is a line ending, not part of the password.
	password, err = ReadPassword(strings.NewReader("hunter2\r\n"))
	if err != nil || password != "hunter2" {
		t.Fatalf("ReadPassword with CRLF = %q, %v", password, err)
	}
	// Spaces inside the password are the password.
	password, err = ReadPassword(strings.NewReader(" two words \n"))
	if err != nil || password != " two words " {
		t.Fatalf("ReadPassword = %q, %v", password, err)
	}
	if _, err := ReadPassword(strings.NewReader("\n")); err == nil {
		t.Error("ReadPassword accepted an empty password")
	}
}
