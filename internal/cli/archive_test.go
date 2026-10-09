package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArchive_OpenTicket_FailsAndChangesNothing(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Open", "--id", "open")

	before := treeSnapshot(t, dir)
	errOut := c.fail(t, "archive", "open")
	if !strings.Contains(errOut, "only closed tickets can be archived") {
		t.Fatalf("error = %q", errOut)
	}
	assertTreeUnchanged(t, "rejected archive", before, treeSnapshot(t, dir))
}

// The location-field tests prove a detail's location and its archived field
// agree after archive, unarchive, and reopen (spec 5.4).

// assertActive fails unless id lives in .tickets/ and not in archive/.
func assertActive(t *testing.T, dir, id string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, ".tickets", id+".md")); err != nil {
		t.Fatalf("%s not in .tickets/: %v", id, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tickets", "archive", id+".md")); !os.IsNotExist(err) {
		t.Fatalf("%s still in archive/: %v", id, err)
	}
}

func TestArchive_ClosedTicket_MovesFileAndSetsField(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Thing", "--id", "thing")
	c.closeTicket(t, "thing")

	c.must(t, "archive", "thing")

	if out := c.must(t, "show", "thing"); !strings.Contains(out, "archived: ") {
		t.Fatalf("archived field missing:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tickets", "archive", "thing.md")); err != nil {
		t.Fatalf("file not moved to archive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tickets", "thing.md")); !os.IsNotExist(err) {
		t.Fatalf("active copy should be gone: %v", err)
	}
}

func TestArchive_ClosedTicket_DropsItFromIndex(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Thing", "--id", "thing")
	c.closeTicket(t, "thing")

	c.must(t, "archive", "thing")

	if idx := readFile(t, dir, "backlog.md"); strings.Contains(idx, "## Archived") || strings.Contains(idx, "#thing") {
		t.Fatalf("archived ticket must not be in the index:\n%s", idx)
	}
}

func TestUnarchive_ArchivedTicket_ClearsFieldKeepsClosedAtAndMovesBack(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Thing", "--id", "thing")
	c.archiveTicket(t, "thing")

	c.must(t, "unarchive", "thing")

	show := c.must(t, "show", "thing")
	if strings.Contains(show, "archived:") {
		t.Fatalf("archived field not cleared:\n%s", show)
	}
	if !strings.Contains(show, "closed-at: ") {
		t.Fatalf("unarchive must keep closed-at:\n%s", show)
	}
	assertActive(t, dir, "thing")
}

func TestReopen_ArchivedTicket_ClearsArchivedAndClosedAtAndMovesBack(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Thing", "--id", "thing")
	c.archiveTicket(t, "thing")

	c.must(t, "reopen", "thing")

	if show := c.must(t, "show", "thing"); strings.Contains(show, "archived:") || strings.Contains(show, "closed-at:") {
		t.Fatalf("reopen did not clear archived+closed-at:\n%s", show)
	}
	assertActive(t, dir, "thing")
}

// TestArchive_ClosedTicket_RemovesOnlyItsIndexLine proves archiving drops the
// archived ticket's index line and leaves every other line, in both sections,
// byte-identical and in place (spec 3.1/5.4).
func TestArchive_ClosedTicket_RemovesOnlyItsIndexLine(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Alpha", "--id", "alpha", "-p", "0")
	c.must(t, "add", "Bravo", "--id", "bravo", "-p", "1")
	c.must(t, "add", "Charlie", "--id", "charlie", "-p", "2")
	c.must(t, "add", "Delta", "--id", "delta", "-p", "3")
	c.closeTicket(t, "bravo")
	c.closeTicket(t, "delta")
	var want []string
	for _, line := range strings.Split(readFile(t, dir, "backlog.md"), "\n") {
		if !strings.HasSuffix(line, "#bravo") {
			want = append(want, line)
		}
	}

	c.must(t, "archive", "bravo")

	if got := readFile(t, dir, "backlog.md"); got != strings.Join(want, "\n") {
		t.Fatalf("index after archive:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
}

// TestArchiveDryRun_ByID_ListsTicketAndChangesNothing proves --dry-run names
// the candidate and leaves the whole tree, index included, byte-identical.
func TestArchiveDryRun_ByID_ListsTicketAndChangesNothing(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "T", "--id", "ttt")
	c.closeTicket(t, "ttt")
	before := treeSnapshot(t, dir)

	out := c.must(t, "archive", "ttt", "--dry-run")

	if !strings.Contains(out, "would archive ttt") {
		t.Fatalf("dry-run output = %q", out)
	}
	assertTreeUnchanged(t, "dry-run", before, treeSnapshot(t, dir))
}

// TestArchiveDryRun_ByAgeWithOldTicket_ListsTicketAndChangesNothing proves
// the age-based --dry-run names a ticket old enough to archive and still
// touches nothing.
func TestArchiveDryRun_ByAgeWithOldTicket_ListsTicketAndChangesNothing(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "T", "--id", "ttt")
	c.closeTicketAgo(t, "ttt", 40*24*time.Hour)
	before := treeSnapshot(t, dir)

	out := c.must(t, "archive", "--closed-older-than", "30d", "--dry-run")

	if !strings.Contains(out, "would archive ttt") {
		t.Fatalf("age dry-run output = %q", out)
	}
	assertTreeUnchanged(t, "age dry-run", before, treeSnapshot(t, dir))
}

// TestArchiveClosedOlderThan_MixedAges_ArchivesOnlyOlderClosed proves the age
// selection uses the injected clock and picks only closed tickets older than
// the threshold: not a newer closed ticket, and not an open one.
func TestArchiveClosedOlderThan_MixedAges_ArchivesOnlyOlderClosed(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Old", "--id", "old")
	c.must(t, "add", "New", "--id", "new")
	c.must(t, "add", "Open", "--id", "open")
	c.closeTicketAgo(t, "old", 40*24*time.Hour)
	c.closeTicketAgo(t, "new", 10*24*time.Hour)

	out := c.must(t, "archive", "--closed-older-than", "30d")

	if !strings.Contains(out, "archived old") {
		t.Fatalf("old not archived: %q", out)
	}
	if strings.Contains(out, "archived new") || strings.Contains(out, "archived open") {
		t.Fatalf("selection too broad: %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tickets", "archive", "old.md")); err != nil {
		t.Fatalf("old not moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tickets", "new.md")); err != nil {
		t.Fatalf("new should still be active: %v", err)
	}
}

// TestArchiveClosedOlderThan_EachUnit_SplitsAtThreshold proves every accepted
// unit is applied, not only parsed: a ticket closed a minute more than the
// threshold ago is selected, and one closed a minute less is not.
func TestArchiveClosedOlderThan_EachUnit_SplitsAtThreshold(t *testing.T) {
	for _, tc := range []struct {
		age       string
		threshold time.Duration
	}{
		{"1d", 24 * time.Hour},
		{"12h", 12 * time.Hour},
		{"45m", 45 * time.Minute},
	} {
		t.Run(tc.age, func(t *testing.T) {
			c := newCLI(t, t.TempDir())
			c.must(t, "add", "Over", "--id", "over")
			c.must(t, "add", "Under", "--id", "under")
			c.closeTicketAgo(t, "over", tc.threshold+time.Minute)
			c.closeTicketAgo(t, "under", tc.threshold-time.Minute)

			out := c.must(t, "archive", "--closed-older-than", tc.age, "--dry-run")

			if !strings.Contains(out, "would archive over") || strings.Contains(out, "would archive under") {
				t.Fatalf("--closed-older-than %s = %q, want only over", tc.age, out)
			}
		})
	}
}

// TestArchiveClosedOlderThan_AlreadyArchivedTicket_SkipsIt proves an already
// archived ticket is never selected again.
func TestArchiveClosedOlderThan_AlreadyArchivedTicket_SkipsIt(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Old", "--id", "old")
	c.closeTicketAgo(t, "old", 40*24*time.Hour)
	c.must(t, "archive", "old")

	out := c.must(t, "archive", "--closed-older-than", "30d")
	if strings.Contains(out, "archived old") {
		t.Fatalf("already archived ticket reselected: %q", out)
	}
}

func TestUnarchive_ActiveTicket_FailsAndChangesNothing(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "A", "--id", "aaa")

	before := treeSnapshot(t, dir)
	errOut := c.fail(t, "unarchive", "aaa")
	if !strings.Contains(errOut, "not archived") {
		t.Fatalf("error = %q", errOut)
	}
	assertTreeUnchanged(t, "rejected unarchive", before, treeSnapshot(t, dir))
}

// The duplicate-ID tests prove archive and unarchive refuse an ID that has a
// hand-placed copy in both .tickets/ and archive/, and change nothing: which
// copy is right is the user's decision (spec 5.4).

// givenClosedTicketWithArchivedDuplicate returns a harness with closed ticket
// "dup" and a different hand-placed copy of "dup" under archive/.
func givenClosedTicketWithArchivedDuplicate(t *testing.T) (*testCLI, string) {
	t.Helper()
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Active copy", "--id", "dup")
	c.closeTicket(t, "dup")
	archiveDir := filepath.Join(dir, ".tickets", "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	other := "---\nid: dup\ntitle: Archived copy\nstatus: closed\narchived: 2026-01-01T00:00:00Z\n---\nprecious archived notes\n"
	if err := os.WriteFile(filepath.Join(archiveDir, "dup.md"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	return c, dir
}

func TestArchive_DuplicatedID_FailsAndChangesNothing(t *testing.T) {
	c, dir := givenClosedTicketWithArchivedDuplicate(t)
	before := treeSnapshot(t, dir)

	errOut := c.fail(t, "archive", "dup")

	if !strings.Contains(errOut, "bl doctor") {
		t.Fatalf("error = %q", errOut)
	}
	assertTreeUnchanged(t, "archive of duplicated id", before, treeSnapshot(t, dir))
}

func TestUnarchive_DuplicatedID_FailsAndChangesNothing(t *testing.T) {
	c, dir := givenClosedTicketWithArchivedDuplicate(t)
	before := treeSnapshot(t, dir)

	c.fail(t, "unarchive", "dup")

	assertTreeUnchanged(t, "unarchive of duplicated id", before, treeSnapshot(t, dir))
}

// The corrupt-archive tests prove the default navigation views (list,
// started, blocked, available, closed) are index-only: a corrupt archived
// detail is invisible to them. `list --archived`/`--all` are the sanctioned
// exception: they read archive/ and surface the corruption, and so does
// doctor (spec 3.1).

// givenCorruptArchivedDetail returns a harness with active ticket "live" and an
// unparseable archived detail "old".
func givenCorruptArchivedDetail(t *testing.T) *testCLI {
	t.Helper()
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Live", "--id", "live")
	archiveDir := filepath.Join(dir, ".tickets", "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "old.md"), []byte("---\nstatus: nope\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestViews_CorruptArchivedDetail_StillSucceed(t *testing.T) {
	for _, view := range []string{"list", "started", "blocked", "available", "closed"} {
		t.Run(view, func(t *testing.T) {
			c := givenCorruptArchivedDetail(t)

			_, code := c.run(view)

			if code != 0 {
				t.Fatalf("exit %d (must not read archive/): %s", code, c.err.String())
			}
		})
	}
}

func TestList_CorruptArchivedDetail_StillListsActiveTicket(t *testing.T) {
	c := givenCorruptArchivedDetail(t)

	out := c.must(t, "list", "-vv")

	if !strings.Contains(out, "#live") {
		t.Fatalf("list missing live ticket: %q", out)
	}
}

func TestListArchived_CorruptArchivedDetail_Fails(t *testing.T) {
	for _, flag := range []string{"--archived", "--all"} {
		t.Run(flag, func(t *testing.T) {
			c := givenCorruptArchivedDetail(t)

			_, code := c.run("list", flag)

			if code == 0 {
				t.Fatalf("list %s did not surface the corrupt archived detail", flag)
			}
		})
	}
}

func TestDoctor_CorruptArchivedDetail_Fails(t *testing.T) {
	c := givenCorruptArchivedDetail(t)

	_, code := c.run("doctor")

	if code == 0 {
		t.Fatal("doctor did not surface the corrupt archived detail")
	}
}

func TestArchive_PartialID_ArchivesMatchingTicket(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Partial", "--id", "partial")
	c.closeTicket(t, "partial")
	c.must(t, "archive", "part")
	if _, err := os.Stat(filepath.Join(dir, ".tickets", "archive", "partial.md")); err != nil {
		t.Fatalf("partial id did not archive: %v", err)
	}
}
