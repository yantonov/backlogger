package attach

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yantonov/backlogger/internal/ids"
)

// The suffix tests pin the traversal guard: only plain filename suffixes are
// usable, so a suffix can never escape the attachments directory.

func TestValidateSuffix_PlainFilename_Accepts(t *testing.T) {
	for _, s := range []string{"screenshot.png", "plan.md", "a", "A1.tar.gz", "under_score", "dash-name", ".hidden"} {
		t.Run(s, func(t *testing.T) {
			err := ValidateSuffix(s)

			if err != nil {
				t.Errorf("ValidateSuffix(%q) = %v, want nil", s, err)
			}
		})
	}
}

func TestValidateSuffix_PathOrEmpty_RejectsWithErrInvalidSuffix(t *testing.T) {
	for _, s := range []string{
		"",
		".",
		"..",
		"../escape",
		"a/../b",
		"a/b",
		`a\b`,
		"/abs/path",
		`C:\abs`,
		"C:drive-relative",
		"../..",
	} {
		t.Run(s, func(t *testing.T) {
			err := ValidateSuffix(s)

			if !errors.Is(err, ErrInvalidSuffix) {
				t.Errorf("ValidateSuffix(%q) = %v, want ErrInvalidSuffix", s, err)
			}
		})
	}
}

// TestSourceSuffix_NestedPath_ReturnsBaseName proves a source path contributes
// only its base name, so a caller-chosen directory never influences storage.
func TestSourceSuffix_NestedPath_ReturnsBaseName(t *testing.T) {
	if got, err := SourceSuffix(filepath.Join("some", "dir", "shot.png")); err != nil || got != "shot.png" {
		t.Fatalf("SourceSuffix = (%q, %v), want (shot.png, nil)", got, err)
	}
}

// The Name/Split tests prove the ID is everything before the first "__", so
// the delimiter is unambiguous and suffix underscores are preserved.

func TestName_SuffixWithUnderscore_JoinsWithDoubleUnderscore(t *testing.T) {
	got := Name("my-task", "a_b.png")

	if got != "my-task__a_b.png" {
		t.Fatalf("Name = %q", got)
	}
}

func TestSplit_SuffixWithUnderscore_KeepsItInSuffix(t *testing.T) {
	id, suffix, ok := Split("my-task__a_b.png")

	if !ok || id != "my-task" || suffix != "a_b.png" {
		t.Fatalf("Split = (%q, %q, %v)", id, suffix, ok)
	}
}

func TestSplit_MalformedName_ReportsNotOK(t *testing.T) {
	for _, bad := range []string{"nodashes", "__suffix", "id__", "id"} {
		t.Run(bad, func(t *testing.T) {
			_, _, ok := Split(bad)

			if ok {
				t.Errorf("Split(%q) reported ok, want false", bad)
			}
		})
	}
}

// TestIDsValidate_Underscore_FailsWithErrGrammar cross-checks the attachment
// delimiter claim against the ID grammar: underscore is forbidden in IDs, which
// is what makes "__" unambiguous (spec 3.5/5.1).
func TestIDsValidate_Underscore_FailsWithErrGrammar(t *testing.T) {
	err := ids.Validate("my_task")
	if err == nil {
		t.Fatal("ids.Validate accepted an underscore; the delimiter is not unambiguous")
	}
	if !errors.Is(err, ids.ErrGrammar) {
		t.Fatalf("underscore error = %v, want ErrGrammar", err)
	}
}

// The copy/list/remove tests prove a file is stored under <id>__<suffix>,
// listed back, and removed, and that the directory is created lazily.

// givenLoginScreenshot returns an attachments dir where "login" has
// screenshot.png (bytes "PNG-BYTES") stored.
func givenLoginScreenshot(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "attachments")
	if _, err := Copy(dir, "login", writeSource(t, "screenshot.png", "PNG-BYTES")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCopy_FirstFile_CreatesDirAndStoresUnderIDSuffix(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	src := writeSource(t, "screenshot.png", "PNG-BYTES")

	suffix, err := Copy(dir, "login", src)

	if err != nil {
		t.Fatal(err)
	}
	if suffix != "screenshot.png" {
		t.Fatalf("suffix = %q", suffix)
	}
	if got := readAttachment(t, Path(dir, "login", "screenshot.png")); got != "PNG-BYTES" {
		t.Fatalf("stored bytes = %q", got)
	}
}

func TestFilesFor_StoredFile_ListsItsSuffix(t *testing.T) {
	dir := givenLoginScreenshot(t)

	files, err := FilesFor(dir, "login")

	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "screenshot.png" {
		t.Fatalf("FilesFor = %v", files)
	}
}

func TestEntries_StoredLinkedFile_IsPresent(t *testing.T) {
	dir := givenLoginScreenshot(t)

	entries, err := Entries(dir, "login", []string{"screenshot.png"})

	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Suffix != "screenshot.png" || !entries[0].Present {
		t.Fatalf("Entries = %+v", entries)
	}
}

func TestRemove_StoredFile_DeletesItAndReportsRemoved(t *testing.T) {
	dir := givenLoginScreenshot(t)

	removed, err := Remove(dir, "login", "screenshot.png")

	if err != nil || !removed {
		t.Fatalf("Remove = (%v, %v), want (true, nil)", removed, err)
	}
	if _, err := os.Stat(Path(dir, "login", "screenshot.png")); !os.IsNotExist(err) {
		t.Fatalf("file still present: %v", err)
	}
}

func TestRemove_MissingFile_IsNoOpWithoutError(t *testing.T) {
	dir := givenLoginScreenshot(t)
	if _, err := Remove(dir, "login", "screenshot.png"); err != nil {
		t.Fatal(err)
	}

	removed, err := Remove(dir, "login", "screenshot.png")

	if err != nil || removed {
		t.Fatalf("second Remove = (%v, %v), want (false, nil)", removed, err)
	}
}

// TestCopy_SameSuffixForAnotherTicket_KeepsBothFiles proves the ID is part of
// the filename, so one ticket's attachment can never collide with another's.
func TestCopy_SameSuffixForAnotherTicket_KeepsBothFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	if _, err := Copy(dir, "alpha", writeSource(t, "plan.md", "ALPHA")); err != nil {
		t.Fatal(err)
	}
	if _, err := Copy(dir, "bravo", writeSource(t, "plan.md", "BRAVO")); err != nil {
		t.Fatal(err)
	}
	if got := readAttachment(t, Path(dir, "alpha", "plan.md")); got != "ALPHA" {
		t.Fatalf("alpha file clobbered: %q", got)
	}
	if got := readAttachment(t, Path(dir, "bravo", "plan.md")); got != "BRAVO" {
		t.Fatalf("bravo file = %q", got)
	}
}

// TestFilesFor_PrefixSharingIDs_ListsOnlyExactIDFiles proves FilesFor filters
// by the exact ID, so a prefix ID does not pick up another ticket's files.
func TestFilesFor_PrefixSharingIDs_ListsOnlyExactIDFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "attachments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha__a.md", "alpha__b.md", "alpha-x__c.md", "other__d.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := FilesFor(dir, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.md", "b.md"}
	if strings.Join(files, ",") != strings.Join(want, ",") {
		t.Fatalf("FilesFor(alpha) = %v, want %v", files, want)
	}
}

// The CheckLinks tests prove the doctor support function reports a
// frontmatter entry with no file and a file with no frontmatter entry, and
// nothing else.

func attachmentsDirWith(t *testing.T, names ...string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "attachments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCheckLinks_MissingAndOrphan_ReportsBoth(t *testing.T) {
	dir := attachmentsDirWith(t, "thing__orphan.md")

	issues, err := CheckLinks(dir, "thing", []string{"linked.md"})

	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(issues.Missing, ",") != "linked.md" {
		t.Fatalf("Missing = %v", issues.Missing)
	}
	if strings.Join(issues.Orphans, ",") != "orphan.md" {
		t.Fatalf("Orphans = %v", issues.Orphans)
	}
	if issues.Empty() {
		t.Fatal("issues reported empty despite missing and orphan")
	}
}

func TestCheckLinks_LinksMatchFiles_ReportsNothing(t *testing.T) {
	dir := attachmentsDirWith(t, "thing__linked.md", "thing__orphan.md")

	issues, err := CheckLinks(dir, "thing", []string{"linked.md", "orphan.md"})

	if err != nil {
		t.Fatal(err)
	}
	if !issues.Empty() {
		t.Fatalf("consistent links reported issues: %+v", issues)
	}
}

func TestCheckLinks_NoDirAndNoLinks_ReportsNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nope")

	issues, err := CheckLinks(dir, "thing", nil)

	if err != nil || !issues.Empty() {
		t.Fatalf("CheckLinks on a missing dir = (%+v, %v)", issues, err)
	}
}

func TestCheckLinks_NoDirWithLink_ReportsMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nope")

	issues, err := CheckLinks(dir, "thing", []string{"gone.md"})

	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(issues.Missing, ",") != "gone.md" {
		t.Fatalf("Missing on a missing dir = %v", issues.Missing)
	}
}

// TestUniqueSorted_UnsortedDuplicates_ReturnsSortedUnique proves the entries
// helper drops duplicates and sorts, whatever the input order.
func TestUniqueSorted_UnsortedDuplicates_ReturnsSortedUnique(t *testing.T) {
	got := uniqueSorted([]string{"b", "a", "b", "c", "a"})

	if want := []string{"a", "b", "c"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("uniqueSorted = %v, want %v", got, want)
	}
}

// writeSource creates a file with body under a fresh temp dir and returns its
// path, the source side of a Copy.
func writeSource(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readAttachment(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
