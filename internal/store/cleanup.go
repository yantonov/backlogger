package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// tempPatternFragment is the distinctive part of the temp-file name that the
// atomic writers produce: "." + base + ".tmp-" + random. Matching on the
// dot-prefix plus ".tmp-" cannot collide with a real ticket or attachment,
// because an ID may not start with a dot and an attachment sidecar is
// "<id>__<suffix>" with no leading dot.
const tempPatternFragment = ".tmp-"

func isStaleTempName(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, tempPatternFragment)
}

// SweepStaleTemp removes temp files left behind by a writer that was killed
// before it could clean up. A writer killed with SIGKILL cannot run its
// cleanup, so a ".<name>.tmp-*" file can remain in .tickets/, archive/, or
// attachments/.
//
// The caller must hold the exclusive backlog lock: the lock proves no other
// writer is mid-write, so every matching file present is stale. A missing
// directory is treated as empty. Directories are never removed even when their
// name matches. A file that vanished concurrently is not an error; the first
// other error is returned.
func (p *Paths) SweepStaleTemp() error {
	for _, dir := range []string{p.TicketsDir, p.ArchiveDir, p.AttachmentsDir} {
		if err := sweepDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func sweepDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("sweep %s: %w", dir, err)
	}
	for _, e := range entries {
		if !isStaleTempName(e.Name()) {
			continue
		}
		// A temp file is always a regular file. Never delete a directory that
		// merely happens to match the pattern.
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale temp %s: %w", path, err)
		}
	}
	return nil
}
