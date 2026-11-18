package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteLeavesNoTemporariesAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web_config.json")

	for _, body := range []string{"first", "second, rather longer"} {
		if err := Write(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != body {
			t.Errorf("content = %q, want %q", got, body)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 - the config holds private keys", info.Mode().Perm())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("temporary files left behind: %v", entries)
	}
}

// A failed write must not destroy what is already on disk - that file is the
// only copy of every server and client private key.
func TestWriteKeepsTheOldFileWhenTheWriteFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web_config.json")
	if err := Write(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A directory that does not exist makes the temporary file fail.
	missing := filepath.Join(dir, "gone", "web_config.json")
	if err := Write(missing, []byte("nope"), 0o600); err == nil {
		t.Error("expected an error writing into a missing directory")
	}

	got, err := os.ReadFile(path)
	if err != nil || string(got) != "original" {
		t.Errorf("original file damaged: %q, %v", got, err)
	}
}
