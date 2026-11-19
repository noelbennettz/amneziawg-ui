package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"amneziawg-web-ui/internal/adminpw"
)

// A hash produced by "hash-password" is what the panel accepts, and the
// password itself never appears in the output.
func TestHashPassword(t *testing.T) {
	var out bytes.Buffer
	if err := hashPassword(strings.NewReader("a password\n"), &out); err != nil {
		t.Fatalf("hashPassword: %v", err)
	}

	encoded := strings.TrimSpace(out.String())
	if strings.Contains(encoded, "a password") {
		t.Fatalf("the output carries the password: %q", encoded)
	}
	hash, err := adminpw.Parse(encoded)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !hash.Verify("a password") {
		t.Error("the printed hash does not verify the password")
	}
}

func TestHashPasswordNeedsStdin(t *testing.T) {
	var out bytes.Buffer
	if err := hashPassword(strings.NewReader(""), &out); err == nil {
		t.Error("hashPassword accepted an empty password")
	}
	if out.Len() != 0 {
		t.Errorf("hashPassword printed %q for a failed run", out.String())
	}
}

// "reset-password" writes the new hash to the file the panel reads and prints
// the password once. With WEB_UI_PASSWORD set it also says that the variable
// still wins, which is the one way the command can look like it did nothing.
func TestResetPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), adminpw.FileName)

	var out bytes.Buffer
	if err := resetPassword(path, false, &out); err != nil {
		t.Fatalf("resetPassword: %v", err)
	}
	if strings.Contains(out.String(), "WEB_UI_PASSWORD") {
		t.Errorf("the note about WEB_UI_PASSWORD was printed although it is unset:\n%s", out.String())
	}

	// The password the command printed is the one the panel will check.
	printed := ""
	for line := range strings.SplitSeq(out.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, "New admin password: "); ok {
			printed = rest
		}
	}
	if len(printed) != adminpw.PasswordLength {
		t.Fatalf("the printed password is %q", printed)
	}
	store, generated, err := adminpw.Open(path, "")
	if err != nil {
		t.Fatalf("adminpw.Open: %v", err)
	}
	if generated != "" {
		t.Errorf("the file was missing its hash: %q", generated)
	}
	hash, _ := store.Hash()
	if !hash.Verify(printed) {
		t.Error("the password printed by reset-password does not verify against the file it wrote")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("%s has mode %o, want 600", path, perm)
	}

	out.Reset()
	if err := resetPassword(path, true, &out); err != nil {
		t.Fatalf("resetPassword: %v", err)
	}
	if !strings.Contains(out.String(), "WEB_UI_PASSWORD") {
		t.Errorf("resetPassword did not mention that WEB_UI_PASSWORD wins:\n%s", out.String())
	}
}

// The banner is the only copy of a generated password: it has to carry the
// user, the password, the fact that it is shown once, and how to reset it.
func TestAnnounceGeneratedPassword(t *testing.T) {
	var out bytes.Buffer
	announceGeneratedPassword(&out, "admin", "abc123DEF456ghi789JKL012")

	for _, want := range []string{"admin", "abc123DEF456ghi789JKL012", "shown once", "reset-password", "/etc/amnezia/admin.hash"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the banner does not mention %q:\n%s", want, out.String())
		}
	}
}

// The refusal says how to get a password, in both ways there are.
func TestExplainDefaultPassword(t *testing.T) {
	var out bytes.Buffer
	explainDefaultPassword(&out, "/etc/amnezia/admin.hash")

	for _, want := range []string{"changeme", "hash-password", "reset-password", "$$"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the refusal does not mention %q:\n%s", want, out.String())
		}
	}
}

func TestUsage(t *testing.T) {
	var out bytes.Buffer
	usage(&out)

	for _, want := range []string{"hash-password", "reset-password", "stdin"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the usage line does not mention %q:\n%s", want, out.String())
		}
	}
}
