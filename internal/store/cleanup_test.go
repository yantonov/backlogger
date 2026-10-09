package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yantonov/backlogger/internal/store"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSweepStaleTemp_TempAndRealFiles_RemovesOnlyTemp seeds temp files in every
// backlog directory and proves the sweep removes exactly those and leaves real
// files alone.
func TestSweepStaleTemp_TempAndRealFiles_RemovesOnlyTemp(t *testing.T) {
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	if err := paths.EnsureArchive(); err != nil {
		t.Fatal(err)
	}
	if err := paths.EnsureAttachments(); err != nil {
		t.Fatal(err)
	}

	// Real files that must survive.
	keep := []string{
		filepath.Join(paths.TicketsDir, "login.md"),
		filepath.Join(paths.ArchiveDir, "archive.md"),
		filepath.Join(paths.AttachmentsDir, "login__shot.png"),
	}
	for _, p := range keep {
		writeFile(t, p, "real")
	}

	// Stale temp files that must be removed.
	stale := []string{
		filepath.Join(paths.TicketsDir, ".login.md.tmp-123"),
		filepath.Join(paths.ArchiveDir, ".archive.md.tmp-9"),
		filepath.Join(paths.AttachmentsDir, ".login__shot.png.tmp-1"),
	}
	for _, p := range stale {
		writeFile(t, p, "stale")
	}

	if err := paths.SweepStaleTemp(); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	for _, p := range stale {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("stale temp not removed: %s (err %v)", p, err)
		}
	}
	for _, p := range keep {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("real file removed: %s (%v)", p, err)
		}
	}
}

// TestSweepStaleTemp_MissingDirs_Succeeds proves a backlog with no directory at
// all sweeps cleanly instead of erroring.
func TestSweepStaleTemp_MissingDirs_Succeeds(t *testing.T) {
	paths := store.NewPaths(t.TempDir())
	if err := paths.SweepStaleTemp(); err != nil {
		t.Fatalf("sweep with missing dirs: %v", err)
	}
}

// TestSweepStaleTemp_TempNamedDirectory_KeepsIt proves a directory whose name
// matches the temp pattern is never deleted.
func TestSweepStaleTemp_TempNamedDirectory_KeepsIt(t *testing.T) {
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}

	dirPath := filepath.Join(paths.TicketsDir, ".keep.md.tmp-dir")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := paths.SweepStaleTemp(); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if fi, err := os.Stat(dirPath); err != nil || !fi.IsDir() {
		t.Fatalf("matching directory was removed or changed: %v", err)
	}
}

// TestBegin_StaleTempFile_SweepsBeforeReturning proves Begin itself removes a
// stale temp file under the lock, before the caller writes or commits.
func TestBegin_StaleTempFile_SweepsBeforeReturning(t *testing.T) {
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(paths.TicketsDir, ".zzz.md.tmp-1")
	writeFile(t, stale, "stale")

	tx := mustBegin(t, paths, store.TxOptions{})
	defer func() {
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
	}()

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale temp survived Begin: %v", err)
	}
}
