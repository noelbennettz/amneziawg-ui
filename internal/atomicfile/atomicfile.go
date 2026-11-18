// Package atomicfile writes a file so that a reader sees either the previous
// contents or the new ones, never a truncated file in between.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write writes data to a temporary file in the target directory and renames
// it over path. A plain write truncates the file first, so an interrupted one
// leaves the config - which holds every server and client private key -
// truncated or half-written, with no copy left to recover from. The rename is
// atomic, so a reader sees either the old file or the new one.
func Write(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeded

	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// Durability before visibility: rename can otherwise become visible while
	// the contents are still only in the page cache.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}

	// Persist the rename itself. Nothing depends on the result - the data is
	// already durable - so a directory that cannot be opened is not an error.
	if d, err := os.Open(dir); err == nil {
		d.Sync() //nolint:errcheck
		d.Close()
	}
	return nil
}
