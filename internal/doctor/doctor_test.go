package doctor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/doctor"
	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// ts is the fixed timestamp test tickets use so their bytes are deterministic.
var ts = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func modelTicket(status string, deps []string) model.Ticket {
	stamp := model.FormatTimestamp(ts)
	return model.Ticket{
		Status:   model.Status(status),
		Priority: model.DefaultPriority,
		Deps:     deps,
		Created:  stamp,
		Updated:  stamp,
	}
}

func newProject(t *testing.T) *store.Paths {
	t.Helper()
	paths := store.NewPaths(t.TempDir())
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	return paths
}

func writeDetail(t *testing.T, dir, id, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// addTicket creates a valid active ticket with the given status and deps, then
// reindexes.
func addTicket(t *testing.T, paths *store.Paths, id, status string, deps ...string) {
	t.Helper()
	meta := &store.TicketMeta{
		ID:     id,
		Ticket: modelTicket(status, deps),
		Body:   []byte("# " + id + "\n"),
	}
	if err := paths.WriteDetail(meta); err != nil {
		t.Fatal(err)
	}
	if err := index.Reindex(paths); err != nil {
		t.Fatal(err)
	}
}

// cleanProject returns a project with two valid tickets and a matching index.
func cleanProject(t *testing.T) *store.Paths {
	t.Helper()
	paths := newProject(t)
	addTicket(t, paths, "alpha", "open")
	addTicket(t, paths, "bravo", "open")
	return paths
}

func mustRun(t *testing.T, paths *store.Paths) *doctor.Report {
	t.Helper()
	report, err := doctor.Run(paths)
	if err != nil {
		t.Fatalf("doctor.Run: %v", err)
	}
	return report
}

// TestRun_CleanRepo_ReportsNoIssues is the positive control: a repo built
// through the normal write path must be clean.
func TestRun_CleanRepo_ReportsNoIssues(t *testing.T) {
	report := mustRun(t, cleanProject(t))
	if len(report.Issues) != 0 {
		t.Fatalf("clean repo reported issues: %+v", report.Issues)
	}
}

// TestRun_BadFilename_FlagsFilename is one injected corruption class: a
// filename stem that violates the ID grammar.
func TestRun_BadFilename_FlagsFilename(t *testing.T) {
	paths := cleanProject(t)
	writeDetail(t, paths.TicketsDir, "bad_name", "# x\n")

	report := mustRun(t, paths)
	if !report.Has(doctor.CheckFilename) {
		t.Fatalf("bad filename not flagged: %+v", report.Issues)
	}
}

// TestRun_SameIDInActiveAndArchive_FlagsDuplicate is one injected corruption
// class: the same stem in active and archive.
func TestRun_SameIDInActiveAndArchive_FlagsDuplicate(t *testing.T) {
	paths := cleanProject(t)
	if err := paths.EnsureArchive(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(paths.ActivePath("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ArchivedPath("alpha"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	report := mustRun(t, paths)
	if !report.Has(doctor.CheckDuplicate) {
		t.Fatalf("duplicate id not flagged: %+v", report.Issues)
	}
}

// TestRun_TamperedIndex_FlagsIndexDriftAsError is one injected corruption
// class: backlog.md no longer equals f(details).
func TestRun_TamperedIndex_FlagsIndexDriftAsError(t *testing.T) {
	paths := cleanProject(t)
	if err := os.WriteFile(paths.IndexFile, []byte("## Active\n- [ ] tampered #alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := mustRun(t, paths)
	if !report.Has(doctor.CheckIndex) {
		t.Fatalf("index drift not flagged: %+v", report.Issues)
	}
	if report.Errors() == 0 {
		t.Fatal("index drift should be an error")
	}
}

// TestRun_UnknownDep_FlagsDepAsWarningOnly is one injected corruption class: a
// dep ID that resolves to no ticket. It is a warning, not a silent skip.
func TestRun_UnknownDep_FlagsDepAsWarningOnly(t *testing.T) {
	paths := cleanProject(t)
	addTicket(t, paths, "charlie", "open", "ghost")

	report := mustRun(t, paths)
	if !report.Has(doctor.CheckDep) {
		t.Fatalf("missing dep not flagged: %+v", report.Issues)
	}
	if report.Errors() != 0 {
		t.Fatalf("missing dep should be a warning, got errors: %+v", report.Issues)
	}
	if report.Warnings() == 0 {
		t.Fatal("missing dep should produce a warning")
	}
}

// TestRun_CycleWrittenToDisk_FlagsCycle is one injected corruption class: a
// dependency cycle written directly to disk (the dep add guard cannot see a
// manual edit).
func TestRun_CycleWrittenToDisk_FlagsCycle(t *testing.T) {
	paths := cleanProject(t)
	addTicket(t, paths, "aaa", "open", "bbb")
	addTicket(t, paths, "bbb", "open", "aaa")

	report := mustRun(t, paths)
	if !report.Has(doctor.CheckCycle) {
		t.Fatalf("cycle not flagged: %+v", report.Issues)
	}
}

// TestRun_UnmarkedFileInArchive_FlagsArchiveMismatch is one injected corruption
// class: a file under archive/ with no archived field.
func TestRun_UnmarkedFileInArchive_FlagsArchiveMismatch(t *testing.T) {
	paths := cleanProject(t)
	if err := paths.EnsureArchive(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(paths.ActivePath("alpha"), paths.ArchivedPath("alpha")); err != nil {
		t.Fatal(err)
	}

	report := mustRun(t, paths)
	if !report.Has(doctor.CheckArchive) {
		t.Fatalf("archive mismatch not flagged: %+v", report.Issues)
	}
}

// TestRun_OrphanAttachmentFile_FlagsAttachment is one injected corruption
// class: a sidecar file with no frontmatter link.
func TestRun_OrphanAttachmentFile_FlagsAttachment(t *testing.T) {
	paths := cleanProject(t)
	if err := paths.EnsureAttachments(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.AttachmentsDir, "alpha__orphan.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := mustRun(t, paths)
	if !report.Has(doctor.CheckAttachment) {
		t.Fatalf("orphan attachment not flagged: %+v", report.Issues)
	}
}

// TestRun_DanglingAttachmentLink_FlagsAttachment is the other attachment
// corruption class: a frontmatter link with no sidecar file.
func TestRun_DanglingAttachmentLink_FlagsAttachment(t *testing.T) {
	paths := newProject(t)
	meta := &store.TicketMeta{
		ID:     "alpha",
		Ticket: modelTicket("open", nil),
		Body:   []byte("# alpha\n"),
	}
	meta.Ticket.Attachments = []string{"gone.md"}
	if err := paths.WriteDetail(meta); err != nil {
		t.Fatal(err)
	}
	if err := index.Reindex(paths); err != nil {
		t.Fatal(err)
	}

	report := mustRun(t, paths)
	if !report.Has(doctor.CheckAttachment) {
		t.Fatalf("missing attachment not flagged: %+v", report.Issues)
	}
}

// TestFix_TamperedIndex_RebuildsIndexAndLeavesDetailsByteIdentical proves Fix
// regenerates backlog.md from the details and never rewrites an unrelated
// detail file (byte compare).
func TestFix_TamperedIndex_RebuildsIndexAndLeavesDetailsByteIdentical(t *testing.T) {
	paths := cleanProject(t)

	before := map[string][]byte{}
	for _, p := range []string{paths.ActivePath("alpha"), paths.ActivePath("bravo")} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		before[p] = data
	}

	drift := []byte("## Active\n- [ ] tampered #alpha\n")
	if err := os.WriteFile(paths.IndexFile, drift, 0o644); err != nil {
		t.Fatal(err)
	}
	report := mustRun(t, paths)
	if !report.Has(doctor.CheckIndex) {
		t.Fatalf("precondition: index drift not detected: %+v", report.Issues)
	}

	fixed, err := doctor.Fix(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixed) == 0 {
		t.Fatal("fix reported nothing fixed")
	}

	// The index now matches f(details) and a rerun is clean.
	after := mustRun(t, paths)
	if after.Has(doctor.CheckIndex) {
		t.Fatalf("index still drifted after fix: %+v", after.Issues)
	}
	if after.Errors() != 0 {
		t.Fatalf("repo still has errors after fix: %+v", after.Issues)
	}

	// Details are byte-identical: an index-only fix must not touch them.
	for p, want := range before {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("detail %s was modified by fix", p)
		}
	}
}

// The idempotence tests prove fix reports no work when nothing is left to
// repair: on a clean repo, and on a repo that an earlier fix already repaired.

func TestFix_CleanRepo_ReportsNoWork(t *testing.T) {
	paths := cleanProject(t)

	fixed, err := doctor.Fix(paths)

	if err != nil {
		t.Fatal(err)
	}
	if len(fixed) != 0 {
		t.Fatalf("fix reported work on a clean repo: %v", fixed)
	}
}

func TestFix_AlreadyFixedRepo_ReportsNoWork(t *testing.T) {
	paths := cleanProject(t)
	if err := os.WriteFile(paths.IndexFile, []byte("## Active\n- [ ] tampered #alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fixed, err := doctor.Fix(paths); err != nil || len(fixed) == 0 {
		t.Fatalf("setup fix: fixed %v, err %v", fixed, err)
	}

	fixed, err := doctor.Fix(paths)

	if err != nil {
		t.Fatal(err)
	}
	if len(fixed) != 0 {
		t.Fatalf("second fix reported work: %v", fixed)
	}
}

// TestFix_UnknownDep_DropsItFromDeps proves fix removes a dependency reference
// that names no ticket, turning the warning into a clean repo.
func TestFix_UnknownDep_DropsItFromDeps(t *testing.T) {
	paths := cleanProject(t)
	addTicket(t, paths, "charlie", "open", "ghost")
	if !mustRun(t, paths).Has(doctor.CheckDep) {
		t.Fatal("precondition: missing dep not detected")
	}

	if _, err := doctor.Fix(paths); err != nil {
		t.Fatal(err)
	}
	if after := mustRun(t, paths); after.Has(doctor.CheckDep) {
		t.Fatalf("unresolved dep survived fix: %+v", after.Issues)
	}
	meta, err := paths.ReadDetail("charlie")
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Ticket.Deps) != 0 {
		t.Fatalf("deps = %v, want empty", meta.Ticket.Deps)
	}
}

// The attachment-fix tests prove fix repairs both directions of the
// attachment invariant: it drops a dangling link and adopts an orphan file.

// givenAlphaAttachments returns a project whose one ticket "alpha" links the
// given attachments and has the given sidecar files, with the index rebuilt.
func givenAlphaAttachments(t *testing.T, links, files []string) *store.Paths {
	t.Helper()
	paths := newProject(t)
	meta := &store.TicketMeta{
		ID:     "alpha",
		Ticket: modelTicket("open", nil),
		Body:   []byte("# alpha\n"),
	}
	meta.Ticket.Attachments = links
	if err := paths.WriteDetail(meta); err != nil {
		t.Fatal(err)
	}
	if err := paths.EnsureAttachments(); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(paths.AttachmentsDir, "alpha__"+f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := index.Reindex(paths); err != nil {
		t.Fatal(err)
	}
	if !mustRun(t, paths).Has(doctor.CheckAttachment) {
		t.Fatal("precondition: attachment issue not detected")
	}
	return paths
}

// assertAlphaAttachments fails unless fix left no attachment finding and
// alpha links exactly want.
func assertAlphaAttachments(t *testing.T, paths *store.Paths, want []string) {
	t.Helper()
	if after := mustRun(t, paths); after.Has(doctor.CheckAttachment) {
		t.Fatalf("attachment issues survived fix: %+v", after.Issues)
	}
	got, err := paths.ReadDetail("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Ticket.Attachments, ",") != strings.Join(want, ",") {
		t.Fatalf("attachments = %v, want %v", got.Ticket.Attachments, want)
	}
}

func TestFix_DanglingLink_DropsIt(t *testing.T) {
	paths := givenAlphaAttachments(t, []string{"gone.md"}, nil)

	if _, err := doctor.Fix(paths); err != nil {
		t.Fatal(err)
	}

	assertAlphaAttachments(t, paths, nil)
}

func TestFix_OrphanFile_AdoptsIt(t *testing.T) {
	paths := givenAlphaAttachments(t, nil, []string{"orphan.md"})

	if _, err := doctor.Fix(paths); err != nil {
		t.Fatal(err)
	}

	assertAlphaAttachments(t, paths, []string{"orphan.md"})
}

// TestFix_UnmarkedFileInArchive_MovesItBackToActive proves fix moves a
// misplaced file so its location agrees with its archived field.
func TestFix_UnmarkedFileInArchive_MovesItBackToActive(t *testing.T) {
	paths := cleanProject(t)
	if err := paths.EnsureArchive(); err != nil {
		t.Fatal(err)
	}
	// Move alpha under archive/ without setting the field: a mismatch.
	if err := os.Rename(paths.ActivePath("alpha"), paths.ArchivedPath("alpha")); err != nil {
		t.Fatal(err)
	}
	if !mustRun(t, paths).Has(doctor.CheckArchive) {
		t.Fatal("precondition: archive mismatch not detected")
	}

	if _, err := doctor.Fix(paths); err != nil {
		t.Fatal(err)
	}
	if after := mustRun(t, paths); after.Has(doctor.CheckArchive) {
		t.Fatalf("archive mismatch survived fix: %+v", after.Issues)
	}
	if _, err := os.Stat(paths.ActivePath("alpha")); err != nil {
		t.Fatalf("alpha not moved back to the active directory: %v", err)
	}
}

// The identity tests prove fix leaves identity corruption in place: changing
// an ID is a human decision.

// TestFix_BadFilename_LeavesFileAndFinding proves fix keeps a malformed
// filename and still reports it.
func TestFix_BadFilename_LeavesFileAndFinding(t *testing.T) {
	paths := cleanProject(t)
	writeDetail(t, paths.TicketsDir, "bad_name", "# x\n")
	if !mustRun(t, paths).Has(doctor.CheckFilename) {
		t.Fatal("precondition: bad filename not detected")
	}

	if _, err := doctor.Fix(paths); err != nil {
		t.Fatal(err)
	}

	if after := mustRun(t, paths); !after.Has(doctor.CheckFilename) {
		t.Fatal("fix silently removed the bad-filename finding")
	}
	if _, err := os.Stat(filepath.Join(paths.TicketsDir, "bad_name.md")); err != nil {
		t.Fatalf("fix removed a malformed detail: %v", err)
	}
}

// TestFix_DuplicateID_LeavesBothCopiesAndFinding proves fix keeps both the
// active and the archived copy of a duplicated ID, byte for byte, and still
// reports it. The archived copy has no archived field, so a location repair
// would move it over the active copy.
func TestFix_DuplicateID_LeavesBothCopiesAndFinding(t *testing.T) {
	paths := cleanProject(t)
	if err := paths.EnsureArchive(); err != nil {
		t.Fatal(err)
	}
	active, err := os.ReadFile(paths.ActivePath("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	archived := append(append([]byte(nil), active...), "hand-edited copy\n"...)
	if err := os.WriteFile(paths.ArchivedPath("alpha"), archived, 0o644); err != nil {
		t.Fatal(err)
	}
	if !mustRun(t, paths).Has(doctor.CheckDuplicate) {
		t.Fatal("precondition: duplicate id not detected")
	}

	if _, err := doctor.Fix(paths); err != nil {
		t.Fatal(err)
	}

	if after := mustRun(t, paths); !after.Has(doctor.CheckDuplicate) {
		t.Fatal("fix silently removed the duplicate-id finding")
	}
	for p, want := range map[string][]byte{paths.ActivePath("alpha"): active, paths.ArchivedPath("alpha"): archived} {
		if got, err := os.ReadFile(p); err != nil || string(got) != string(want) {
			t.Fatalf("fix changed or removed %s: %v", p, err)
		}
	}
}
