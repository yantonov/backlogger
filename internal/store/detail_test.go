package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// The read-error tests prove a missing file is ErrNotFound and a genuine read
// error propagates, so neither is silently turned into an empty backlog
// (spec 6.2).

func TestReadDetail_MissingFile_ReturnsErrNotFound(t *testing.T) {
	paths := store.NewPaths(t.TempDir())

	_, err := paths.ReadDetail("missing")

	if !store.IsNotFound(err) {
		t.Fatalf("expected ErrNotFound for a missing detail, got %v", err)
	}
}

func TestReadDetail_DirectoryInPlaceOfFile_ReturnsReadError(t *testing.T) {
	paths := store.NewPaths(t.TempDir())
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(paths.ActivePath("asdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := paths.ReadDetail("asdir")

	if err == nil {
		t.Fatal("expected a read error for a directory, got nil")
	}
	if store.IsNotFound(err) {
		t.Fatalf("a directory is a read error, not ErrNotFound: %v", err)
	}
}

// TestListActive_CorruptDetail_ReturnsParseError proves a corrupt detail file
// is surfaced, not skipped or treated as empty.
func TestListActive_CorruptDetail_ReturnsParseError(t *testing.T) {
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ActivePath("bad"), []byte("---\nstatus: nope\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := paths.ListActive(); err == nil {
		t.Fatal("expected a parse error from ListActive, got nil")
	}
}

// TestReadDetail_ArchivedFile_FindsArchivedCopy proves a ticket is located by
// stem in either the active directory or the archive directory.
func TestReadDetail_ArchivedFile_FindsArchivedCopy(t *testing.T) {
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	if err := paths.EnsureArchive(); err != nil {
		t.Fatal(err)
	}
	archived := "---\nstatus: closed\npriority: 2\ndeps: []\nattachments: []\ncreated: 2026-01-01T00:00:00Z\nupdated: 2026-01-01T00:00:00Z\n---\nbody\n"
	if err := paths.WriteDetailBytes(paths.ArchivedPath("old"), []byte(archived)); err != nil {
		t.Fatal(err)
	}
	got, err := paths.ReadDetail("old")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Archived {
		t.Fatal("expected the archived copy to be found")
	}
	if got.ID != "old" {
		t.Fatalf("ID = %q, want old", got.ID)
	}
}

// TestWriteDetail_TicketWithDeps_ReadsBackSameFieldsWithSortedDeps proves a
// written detail parses back to the same fields.
func TestWriteDetail_TicketWithDeps_ReadsBackSameFieldsWithSortedDeps(t *testing.T) {
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 8, 11, 40, 0, 0, time.UTC)
	meta := newTicket("round", "# Title\n\nprose\n", now)
	meta.Ticket.Deps = []string{"b", "a"}
	if err := paths.WriteDetail(meta); err != nil {
		t.Fatal(err)
	}
	got, err := paths.ReadDetail("round")
	if err != nil {
		t.Fatal(err)
	}
	if got.Ticket.Status != model.StatusOpen || got.Ticket.Priority != model.DefaultPriority {
		t.Fatalf("unexpected frontmatter: %+v", got.Ticket)
	}
	if len(got.Ticket.Deps) != 2 || got.Ticket.Deps[0] != "a" || got.Ticket.Deps[1] != "b" {
		t.Fatalf("deps not sorted: %v", got.Ticket.Deps)
	}
	if string(got.Body) != "# Title\n\nprose\n" {
		t.Fatalf("body = %q", got.Body)
	}
}

// TestDiscover_NestedDir_FindsTicketsRoot proves discovery walks up to the
// directory containing .tickets/.
func TestDiscover_NestedDir_FindsTicketsRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	paths, err := store.Discover(nested)

	if err != nil {
		t.Fatal(err)
	}
	got, _ := filepath.EvalSymlinks(paths.Root)
	want, _ := filepath.EvalSymlinks(root)
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
	if !paths.TicketsExist() {
		t.Fatal("TicketsExist should be true")
	}
}

// TestDiscover_GitRootWithoutTickets_CreatesNothing proves discovery never
// creates anything: in a git root with no .tickets/ it returns that root and
// leaves the tree as it was. The .git entry bounds the walk to the temp dir.
func TestDiscover_GitRootWithoutTickets_CreatesNothing(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	for _, d := range []string{filepath.Join(root, ".git"), nested} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	paths, err := store.Discover(nested)

	if err != nil {
		t.Fatal(err)
	}
	if paths.TicketsExist() {
		t.Fatal("discovery created .tickets/")
	}
	var entries []string
	err = filepath.Walk(root, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		entries = append(entries, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(entries, " "), ". .git a a/b"; got != want {
		t.Fatalf("tree after discovery = %q, want %q", got, want)
	}
}

// TestRequireTickets_MissingDir_FailsAndCreatesNothing proves the read guard
// errors on a missing .tickets/ and does not create it (spec 7.1). The write
// side, where a create transaction makes .tickets/, is
// TestBegin_MissingTicketsWithCreate_CreatesTickets.
func TestRequireTickets_MissingDir_FailsAndCreatesNothing(t *testing.T) {
	paths := store.NewPaths(t.TempDir())

	err := paths.RequireTickets()

	if !errors.Is(err, store.ErrNoTickets) {
		t.Fatalf("expected ErrNoTickets, got %v", err)
	}
	if paths.TicketsExist() {
		t.Fatal("RequireTickets created .tickets/")
	}
}
