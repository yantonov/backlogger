package store

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path without ever exposing a partial file.
//
// The data is written to a temp file in the same directory, fsynced, and then
// moved over the destination atomically. Readers therefore always see either
// the whole old file or the whole new file (spec 6.2/8.3).
//
// The destination's existing mode is preserved; when the destination does not
// exist, defaultPerm is used.
func writeFileAtomic(path string, data []byte, defaultPerm fs.FileMode) error {
	return writeFileAtomicMode(path, data, 0, defaultPerm)
}

// writeFileAtomicMode is writeFileAtomic with an optional explicit mode. When
// explicitPerm is non-zero it overrides both the existing mode and defaultPerm.
// It is used when the caller needs a specific mode (for example copying an
// attachment bit-for-bit).
func writeFileAtomicMode(path string, data []byte, explicitPerm, defaultPerm fs.FileMode) error {
	dir := filepath.Dir(path)

	perm := defaultPerm
	if explicitPerm != 0 {
		perm = explicitPerm
	} else if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	} else if !os.IsNotExist(err) {
		// A stat error other than "does not exist" means we cannot trust the
		// current mode; fail rather than silently change permissions.
		return fmt.Errorf("stat %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := replaceFile(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
