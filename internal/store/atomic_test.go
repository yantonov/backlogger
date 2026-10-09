package store_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yantonov/backlogger/internal/store"
)

// TestWriteDetailBytes_ConcurrentReader_NeverSeesPartialFile proves a reader
// never observes a partial file: a large payload is only ever readable as the
// whole old or whole new content.
func TestWriteDetailBytes_ConcurrentReader_NeverSeesPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "detail.md")

	oldData := make([]byte, 256*1024)
	for i := range oldData {
		oldData[i] = 'a'
	}
	newData := make([]byte, 256*1024)
	for i := range newData {
		newData[i] = 'b'
	}

	paths := store.NewPaths(dir)
	if err := paths.WriteDetailBytes(path, oldData); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	const writes = 200
	done := make(chan error, 1)
	go func() {
		for i := 0; i < writes; i++ {
			if err := paths.WriteDetailBytes(path, newData); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("write: %v", err)
			}
			return
		default:
		}
		got, err := paths.ReadDetailBytes(path)
		if err != nil {
			t.Fatalf("read during write: %v", err)
		}
		if len(got) != len(oldData) {
			t.Fatalf("observed a partial file: %d bytes", len(got))
		}
		if got[0] != 'a' && got[0] != 'b' {
			t.Fatalf("observed unexpected first byte %q", got[0])
		}
		if got[0] == 'a' && got[len(got)-1] != 'a' {
			t.Fatal("observed a torn file (mixed old/new content)")
		}
		if got[0] == 'b' && got[len(got)-1] != 'b' {
			t.Fatal("observed a torn file (mixed new/old content)")
		}
	}
}

// TestWriteDetailBytes_ExistingFile_PreservesModeAndReplacesContent proves the
// destination mode survives a replace. On POSIX the full permission bits are
// compared; on Windows Go maps only the read-only bit, so the test compares the
// mode before and after, which is the strongest portable statement.
func TestWriteDetailBytes_ExistingFile_PreservesModeAndReplacesContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "detail.md")
	paths := store.NewPaths(dir)

	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := paths.WriteDetailBytes(path, []byte("new")); err != nil {
		t.Fatalf("write: %v", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != after.Mode().Perm() {
		t.Fatalf("mode not preserved across replace: before %o after %o",
			before.Mode().Perm(), after.Mode().Perm())
	}
	if runtime.GOOS != "windows" && after.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", after.Mode().Perm())
	}
	got, _ := paths.ReadDetailBytes(path)
	if string(got) != "new" {
		t.Fatalf("content not replaced: %q", got)
	}
}

// TestWriteDetailBytes_NewFile_LeavesNoTempFile proves the temp file is cleaned
// up.
func TestWriteDetailBytes_NewFile_LeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	path := filepath.Join(dir, "detail.md")
	if err := paths.WriteDetailBytes(path, []byte("x")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "detail.md" {
			t.Fatalf("unexpected leftover file %q", e.Name())
		}
	}
}
