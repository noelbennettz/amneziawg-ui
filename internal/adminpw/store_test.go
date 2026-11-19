package adminpw

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// storePath is a hash file path that does not exist yet.
func storePath(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), FileName)
}

// advance moves the store's clock past the reload interval, so its next call
// looks at a file that was just rewritten.
func advance(s *Store) {
	s.now = func() time.Time { return time.Now().Add(2 * time.Second) }
	s.checked = time.Time{}
}

func TestOpenFromEnv(t *testing.T) {
	path := storePath(t)
	// A file holding a different password: the environment variable has to
	// win over it, which is what lets an operator set the password from the
	// compose file without touching the volume.
	if err := WriteHash(path, hashOf(t)); err != nil {
		t.Fatalf("WriteHash: %v", err)
	}
	envHash, err := HashPassword("from-the-environment")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	store, generated, err := Open(path, envHash)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if generated != "" {
		t.Errorf("Open generated a password although WEB_UI_PASSWORD is set: %q", generated)
	}
	if !store.FromEnv() {
		t.Error("the store did not report WEB_UI_PASSWORD as its source")
	}
	hash, _ := store.Hash()
	if !hash.Verify("from-the-environment") {
		t.Error("the environment variable is not what the store verifies")
	}
	if hash.Verify(testPassword) {
		t.Error("the stored hash won over WEB_UI_PASSWORD")
	}
}

func TestOpenUsesTheStoredHash(t *testing.T) {
	path := storePath(t)
	if err := WriteHash(path, hashOf(t)); err != nil {
		t.Fatalf("WriteHash: %v", err)
	}

	store, generated, err := Open(path, "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if generated != "" {
		t.Errorf("Open printed a password although one was stored: %q", generated)
	}
	if store.FromEnv() {
		t.Error("the store claims WEB_UI_PASSWORD is set")
	}
	hash, _ := store.Hash()
	if !hash.Verify(testPassword) {
		t.Error("the stored hash is not what the store verifies")
	}
}

func TestOpenGeneratesOnFirstRun(t *testing.T) {
	path := storePath(t)

	store, generated, err := Open(path, "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(generated) != PasswordLength {
		t.Fatalf("generated password %q is %d characters, want %d", generated, len(generated), PasswordLength)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("%s has mode %o, want 600", path, perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if strings.Contains(string(data), generated) {
		t.Fatal("the password itself is in the hash file")
	}
	stored, err := Parse(string(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !stored.Verify(generated) {
		t.Error("the stored hash does not verify the generated password")
	}
	hash, _ := store.Hash()
	if !hash.Verify(generated) {
		t.Error("the store does not verify the password it just generated")
	}
}

func TestCheckDefault(t *testing.T) {
	argon2OfChangeme, err := HashPassword("changeme")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	cases := map[string]struct {
		hash string
		want bool
	}{
		"argon2id changeme": {argon2OfChangeme, true},
		"another password":  {hashOf(t), false},
	}

	for name, tc := range cases {
		store, _, err := Open(storePath(t), tc.hash)
		if err != nil {
			t.Fatalf("%s: Open: %v", name, err)
		}
		err = store.CheckDefault()
		switch {
		case tc.want && !errors.Is(err, ErrDefaultPassword):
			t.Errorf("%s: CheckDefault = %v, want ErrDefaultPassword", name, err)
		case !tc.want && err != nil:
			t.Errorf("%s: CheckDefault = %v, want no error", name, err)
		}
	}
}

// A hash in the format earlier releases used stops the panel from starting,
// whichever of the two sources it came from. There is no migration path on
// purpose: the format is a bare SHA-256 of the password, and nothing should go
// on accepting one.
func TestOpenRejectsTheLegacyFormat(t *testing.T) {
	// From WEB_UI_PASSWORD: the caller - main - turns this into a refusal to
	// start, naming the variable.
	if _, _, err := Open(storePath(t), changemeLegacy); !errors.Is(err, ErrLegacyFormat) {
		t.Errorf("Open with a legacy WEB_UI_PASSWORD = %v, want ErrLegacyFormat", err)
	}

	// From the file: the message names the file, because that is what an
	// operator has to go and look at.
	path := storePath(t)
	if err := os.WriteFile(path, []byte(changemeLegacy+"\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	_, _, err := Open(path, "")
	if !errors.Is(err, ErrLegacyFormat) {
		t.Fatalf("Open with a legacy hash file = %v, want ErrLegacyFormat", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error does not name the file it came from: %v", err)
	}
}

func TestStoreAdoptsARewrittenFile(t *testing.T) {
	path := storePath(t)
	store, generated, err := Open(path, "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	before, generation := store.Hash()

	password, err := Reset(path)
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	advance(store)

	after, nextGeneration := store.Hash()
	if nextGeneration <= generation {
		t.Fatal("the generation did not move after the hash file was rewritten")
	}
	if after.String() == before.String() {
		t.Fatal("the store kept the old hash")
	}
	if after.Verify(generated) {
		t.Error("the old password still verifies")
	}
	if !after.Verify(password) {
		t.Error("the new password does not verify")
	}
	if got := store.Err(); got != nil {
		t.Errorf("Err = %v, want nil", got)
	}
}

func TestStoreKeepsThePasswordWhenTheFileBreaks(t *testing.T) {
	path := storePath(t)
	store, generated, err := Open(path, "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := os.WriteFile(path, []byte("half a hash"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	advance(store)

	// Unreadable is not the same as unusable: the panel keeps the password it
	// had, and the failure is reported rather than acted on.
	hash, _ := store.Hash()
	if !hash.Verify(generated) {
		t.Error("a broken hash file dropped the password that was working")
	}
	if store.Err() == nil {
		t.Error("a broken hash file was not reported")
	}
}

func TestStoreReloadIsCheap(t *testing.T) {
	path := storePath(t)
	store, _, err := Open(path, "")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	first, generation := store.Hash()

	// A rewrite the store has not looked at yet is not seen until the
	// interval is up: the point of the interval is that there is no stat()
	// per request.
	if _, err := Reset(path); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	second, stillGeneration := store.Hash()
	if second.String() != first.String() || stillGeneration != generation {
		t.Error("the store re-read the file inside the interval")
	}
}

func TestReset(t *testing.T) {
	path := storePath(t)
	password, err := Reset(path)
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(password) != PasswordLength {
		t.Errorf("the new password is %d characters, want %d", len(password), PasswordLength)
	}
	store, generated, err := Open(path, "")
	if err != nil {
		t.Fatalf("Open after Reset: %v", err)
	}
	if generated != "" {
		t.Errorf("Open printed a password after Reset: %q", generated)
	}
	hash, _ := store.Hash()
	if !hash.Verify(password) {
		t.Error("the hash Reset wrote does not verify its password")
	}
}

func TestOpenRejectsABadHash(t *testing.T) {
	path := storePath(t)
	if err := os.WriteFile(path, []byte("not a hash\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := Open(path, ""); err == nil {
		t.Error("Open accepted a malformed hash file")
	}
	if _, _, err := Open(storePath(t), "$argon2id$broken"); err == nil {
		t.Error("Open accepted a malformed WEB_UI_PASSWORD")
	}
}
