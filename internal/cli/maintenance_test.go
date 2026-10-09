package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVersion_InProcess_PrintsOneBlPrefixedLine proves `bl version` prints
// exactly one line, with no tty dependence and no extra output.
func TestVersion_InProcess_PrintsOneBlPrefixedLine(t *testing.T) {
	app, stdout, stderr := newTestApp(t, t.TempDir())
	if code := app.Run([]string{"version"}); code != exitOK {
		t.Fatalf("version exit %d: %s", code, stderr.String())
	}
	out := stdout.String()
	if strings.Count(strings.TrimRight(out, "\n"), "\n") != 0 {
		t.Fatalf("version printed more than one line: %q", out)
	}
	if !strings.HasPrefix(out, "bl ") {
		t.Fatalf("version output = %q", out)
	}
}

// The doctor tests prove `doctor` exits clean on a healthy repo and nonzero
// on drift, and `doctor fix` repairs the index without touching details.

// givenTamperedIndex returns a harness with ticket "alpha" whose backlog.md
// was overwritten, so the index drifts from the details.
func givenTamperedIndex(t *testing.T) *testCLI {
	t.Helper()
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Alpha", "--id", "alpha")
	if err := os.WriteFile(filepath.Join(c.app.Dir, "backlog.md"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDoctor_CleanRepo_ExitsOKWithCleanReport(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Alpha", "--id", "alpha")

	out, code := c.run("doctor")

	if code != exitOK {
		t.Fatalf("clean doctor exit %d: %s\nstdout: %s", code, c.err.String(), out)
	}
	if !strings.Contains(out, `"clean": true`) {
		t.Fatalf("clean doctor was not a clean JSON report: %s", out)
	}
}

func TestDoctor_TamperedIndex_ExitsErrorWithFinding(t *testing.T) {
	c := givenTamperedIndex(t)

	out, code := c.run("doctor")

	if code != exitError {
		t.Fatalf("drift doctor exit %d, want %d\nstdout: %s", code, exitError, out)
	}
	if !strings.Contains(out, `"clean": false`) {
		t.Fatalf("drift doctor did not report a finding: %s", out)
	}
}

func TestDoctorFix_TamperedIndex_ReportsRepairAndLeavesDetail(t *testing.T) {
	c := givenTamperedIndex(t)
	before := readFile(t, c.app.Dir, ".tickets/alpha.md")

	out, code := c.run("doctor", "fix")

	if code != exitOK {
		t.Fatalf("doctor fix exit %d: %s\nstdout: %s", code, c.err.String(), out)
	}
	if !strings.Contains(out, `"regenerated index"`) {
		t.Fatalf("fix did not report the repair: %s", out)
	}
	if after := readFile(t, c.app.Dir, ".tickets/alpha.md"); after != before {
		t.Fatal("doctor fix modified a detail file")
	}
}

func TestReindex_JunkIndex_RegeneratesItFromDetails(t *testing.T) {
	dir := t.TempDir()
	app, _, stderr := newTestApp(t, dir)
	if code := app.Run([]string{"add", "Alpha", "--id", "alpha"}); code != exitOK {
		t.Fatalf("add exit %d: %s", code, stderr.String())
	}
	if err := os.WriteFile(filepath.Join(dir, "backlog.md"), []byte("junk\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := app.Run([]string{"reindex"}); code != exitOK {
		t.Fatalf("reindex exit %d: %s", code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "backlog.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "#alpha") {
		t.Fatalf("reindex did not regenerate the index:\n%s", data)
	}
}
