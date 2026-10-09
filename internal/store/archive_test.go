package store_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// agreeingMetas returns an active detail with no archived field and an
// archived detail with the field: both locations agree with their field.
func agreeingMetas() (active, archived *store.TicketMeta) {
	active = newTicket("active", "", time.Now())
	archived = newTicket("field", "", time.Now())
	archived.Archived = true
	archived.Ticket.Archived = model.FormatTimestamp(time.Now())
	return active, archived
}

// TestLocationMatchesField_LocationAndField_TrueOnlyWhenTheyAgree proves the
// location-field agreement predicate catches both disagreement directions (spec
// 5.4).
func TestLocationMatchesField_LocationAndField_TrueOnlyWhenTheyAgree(t *testing.T) {
	active, archivedFieldOnly := agreeingMetas()

	cases := []struct {
		name string
		meta *store.TicketMeta
		want bool
	}{
		{"active file, no field", active, true},
		{"archive file, archived field", archivedFieldOnly, true},
		{"archive path but no field", &store.TicketMeta{ID: "a", Archived: true}, false},
		{"active path but archived field", &store.TicketMeta{ID: "b", Ticket: model.Ticket{Archived: "2026-01-01T00:00:00Z"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := store.LocationMatchesField(tc.meta); got != tc.want {
				t.Fatalf("LocationMatchesField = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestArchiveAgreement_AllAgree_ReturnsNil proves the doctor helper is quiet
// when every detail's location matches its field.
func TestArchiveAgreement_AllAgree_ReturnsNil(t *testing.T) {
	active, archived := agreeingMetas()

	err := store.ArchiveAgreement([]*store.TicketMeta{active, archived})

	if err != nil {
		t.Fatalf("agreement expected, got %v", err)
	}
}

// TestArchiveAgreement_ArchivePathWithoutField_ReportsIt proves the helper
// names the disagreeing detail and which side of the invariant failed.
func TestArchiveAgreement_ArchivePathWithoutField_ReportsIt(t *testing.T) {
	active, _ := agreeingMetas()

	err := store.ArchiveAgreement([]*store.TicketMeta{active, {ID: "bad", Archived: true}})

	var age *store.ArchiveAgreementError
	if !asArchiveError(err, &age) {
		t.Fatalf("agreement error = %v (%T), want *ArchiveAgreementError", err, err)
	}
	if age.ID != "bad" || !age.InArchive {
		t.Fatalf("agreement error = %+v", age)
	}
}

func asArchiveError(err error, target **store.ArchiveAgreementError) bool {
	if e, ok := err.(*store.ArchiveAgreementError); ok {
		*target = e
		return true
	}
	return false
}

// The move tests prove a write follows the archived field: setting it moves
// the file into archive/, clearing it moves the file back, and the old copy
// is removed.

// givenStoredDetail returns paths with an active detail "mv" and that detail
// as read back from disk.
func givenStoredDetail(t *testing.T) (*store.Paths, *store.TicketMeta) {
	t.Helper()
	paths := store.NewPaths(t.TempDir())
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteDetail(newTicket("mv", "# mv\n", time.Now())); err != nil {
		t.Fatal(err)
	}
	meta, err := paths.ReadDetail("mv")
	if err != nil {
		t.Fatal(err)
	}
	return paths, meta
}

func assertOnlyAt(t *testing.T, want, gone string) {
	t.Helper()
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected file at %s: %v", want, err)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Fatalf("old copy at %s should be gone, stat err = %v", gone, err)
	}
}

func TestWriteDetailMoved_ArchivedFieldSet_MovesFileToArchive(t *testing.T) {
	paths, meta := givenStoredDetail(t)
	meta.Ticket.Archived = model.FormatTimestamp(time.Now())

	err := paths.WriteDetailMoved(meta)

	if err != nil {
		t.Fatal(err)
	}
	assertOnlyAt(t, paths.ArchivedPath("mv"), paths.ActivePath("mv"))
}

func TestWriteDetailMoved_ArchivedFieldCleared_MovesFileBack(t *testing.T) {
	paths, meta := givenStoredDetail(t)
	meta.Ticket.Archived = model.FormatTimestamp(time.Now())
	if err := paths.WriteDetailMoved(meta); err != nil {
		t.Fatal(err)
	}
	meta, err := paths.ReadDetail("mv")
	if err != nil {
		t.Fatal(err)
	}
	meta.Ticket.Archived = ""

	err = paths.WriteDetailMoved(meta)

	if err != nil {
		t.Fatal(err)
	}
	assertOnlyAt(t, paths.ActivePath("mv"), paths.ArchivedPath("mv"))
}

// The duplicate-move tests prove a move never overwrites a different file
// already at the destination: it fails and leaves both copies byte for byte.

// givenDuplicateInArchive returns the stored detail "mv" and the bytes of a
// different hand-placed copy of "mv" under archive/.
func givenDuplicateInArchive(t *testing.T) (*store.Paths, *store.TicketMeta, []byte) {
	t.Helper()
	paths, meta := givenStoredDetail(t)
	if err := paths.EnsureArchive(); err != nil {
		t.Fatal(err)
	}
	other := []byte("---\nid: mv\ntitle: Archived copy\nstatus: closed\n---\nprecious archived notes\n")
	if err := os.WriteFile(paths.ArchivedPath("mv"), other, 0o644); err != nil {
		t.Fatal(err)
	}
	return paths, meta, other
}

func assertBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s changed:\n got %q\nwant %q", path, got, want)
	}
}

func TestWriteDetailMoved_DuplicateInArchive_RefusesAndKeepsBothCopies(t *testing.T) {
	paths, meta, archived := givenDuplicateInArchive(t)
	active, err := os.ReadFile(paths.ActivePath("mv"))
	if err != nil {
		t.Fatal(err)
	}
	meta.Ticket.Archived = model.FormatTimestamp(time.Now())

	err = paths.WriteDetailMoved(meta)

	if err == nil || !strings.Contains(err.Error(), "bl doctor") {
		t.Fatalf("err = %v, want a duplicate error pointing to bl doctor", err)
	}
	assertBytes(t, paths.ActivePath("mv"), active)
	assertBytes(t, paths.ArchivedPath("mv"), archived)
}

func TestWriteDetailMoved_DuplicateInActive_RefusesAndKeepsBothCopies(t *testing.T) {
	paths, active, archived := givenDuplicateInArchive(t)
	activeBytes, err := os.ReadFile(paths.ActivePath("mv"))
	if err != nil {
		t.Fatal(err)
	}
	meta := &store.TicketMeta{ID: "mv", Archived: true, Ticket: active.Ticket, Path: paths.ArchivedPath("mv")}

	err = paths.WriteDetailMoved(meta)

	if err == nil || !strings.Contains(err.Error(), "bl doctor") {
		t.Fatalf("err = %v, want a duplicate error pointing to bl doctor", err)
	}
	assertBytes(t, paths.ActivePath("mv"), activeBytes)
	assertBytes(t, paths.ArchivedPath("mv"), archived)
}

// The name-only tests prove archive membership is decidable from file names
// alone: an unparseable archived detail does not break the name-only scans
// (ListArchiveIDs, CandidateIDs), which return sorted stems, but does break
// the full listing that opens every file.

// givenUnparseableArchive returns paths with one active detail "aaa" and two
// unparseable archived files, "zzz" and "mmm", written out of order.
func givenUnparseableArchive(t *testing.T) *store.Paths {
	t.Helper()
	paths := store.NewPaths(t.TempDir())
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	if err := paths.EnsureArchive(); err != nil {
		t.Fatal(err)
	}
	if err := paths.WriteDetail(newTicket("aaa", "", time.Now())); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"zzz", "mmm"} {
		if err := os.WriteFile(paths.ArchivedPath(id), []byte("---\nstatus: nope\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func TestListArchiveIDs_UnparseableArchive_ReturnsSortedStems(t *testing.T) {
	paths := givenUnparseableArchive(t)

	ids, err := paths.ListArchiveIDs()

	if err != nil {
		t.Fatalf("ListArchiveIDs must not open archive files: %v", err)
	}
	if strings.Join(ids, ",") != "mmm,zzz" {
		t.Fatalf("ListArchiveIDs = %v, want [mmm zzz]", ids)
	}
}

func TestCandidateIDs_UnparseableArchive_ReturnsActiveAndArchivedSorted(t *testing.T) {
	paths := givenUnparseableArchive(t)

	ids, err := paths.CandidateIDs()

	if err != nil {
		t.Fatalf("CandidateIDs must not open archive files: %v", err)
	}
	if strings.Join(ids, ",") != "aaa,mmm,zzz" {
		t.Fatalf("CandidateIDs = %v, want [aaa mmm zzz]", ids)
	}
}

func TestListAll_UnparseableArchive_Fails(t *testing.T) {
	paths := givenUnparseableArchive(t)

	_, err := paths.ListAll()

	if err == nil {
		t.Fatal("ListAll should surface the corrupt archived file")
	}
}

// TestArchiveCandidates_MixedTickets_SelectsOnlyOldClosed proves the age
// selection takes only closed, not-yet-archived tickets whose closed-at is
// strictly older than the cutoff (spec 5.4).
func TestArchiveCandidates_MixedTickets_SelectsOnlyOldClosed(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	old := newTicket("old", "", now)
	old.Ticket.Status = model.StatusClosed
	old.Ticket.ClosedAt = model.FormatTimestamp(now.Add(-40 * 24 * time.Hour))
	recent := newTicket("recent", "", now)
	recent.Ticket.Status = model.StatusClosed
	recent.Ticket.ClosedAt = model.FormatTimestamp(now.Add(-20 * 24 * time.Hour))
	open := newTicket("open", "", now)
	already := newTicket("already", "", now)
	already.Archived = true
	already.Ticket.Archived = model.FormatTimestamp(now)

	got := store.ArchiveCandidates([]*store.TicketMeta{old, recent, open, already}, now.Add(-30*24*time.Hour))

	if len(got) != 1 || got[0].ID != "old" {
		t.Fatalf("ArchiveCandidates = %v, want [old]", idsOf(got))
	}
}

// TestArchiveCandidates_MalformedClosedAt_NeverQualifies proves an
// unparseable closed-at is never treated as old enough (spec 5.4).
func TestArchiveCandidates_MalformedClosedAt_NeverQualifies(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	bad := newTicket("bad", "", now)
	bad.Ticket.Status = model.StatusClosed
	bad.Ticket.ClosedAt = "not-a-timestamp"

	got := store.ArchiveCandidates([]*store.TicketMeta{bad}, now)

	if len(got) != 0 {
		t.Fatalf("malformed closed-at qualified: %v", idsOf(got))
	}
}

func idsOf(metas []*store.TicketMeta) []string {
	out := make([]string, 0, len(metas))
	for _, m := range metas {
		out = append(out, m.ID)
	}
	return out
}
