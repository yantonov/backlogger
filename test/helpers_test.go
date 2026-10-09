package test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// blBin is the path of the binary built once by TestMain.
var blBin string

// TestMain builds the bl binary into a temp dir and runs the tests.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "bl-cli-test-*")
	if err != nil {
		panic(err)
	}
	blBin = filepath.Join(tmp, "bl")
	if os.PathSeparator == '\\' {
		blBin += ".exe"
	}
	build := exec.Command("go", "build", "-o", blBin, "./cmd/bl")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		os.RemoveAll(tmp)
		panic("build bl: " + err.Error() + "\n" + string(out))
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

type result struct {
	Stdout string
	Stderr string
	Code   int
}

// run executes bl with args in dir. It never fails the test; callers assert on
// the result.
func run(t *testing.T, dir string, args ...string) result {
	t.Helper()
	cmd := exec.Command(blBin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return result{Stdout: stdout.String(), Stderr: stderr.String(), Code: code}
}

func mustRun(t *testing.T, dir string, args ...string) result {
	t.Helper()
	res := run(t, dir, args...)
	if res.Code != 0 {
		t.Fatalf("bl %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), res.Code, res.Stdout, res.Stderr)
	}
	return res
}

func mustFail(t *testing.T, dir string, args ...string) result {
	t.Helper()
	res := run(t, dir, args...)
	if res.Code == 0 {
		t.Fatalf("bl %s unexpectedly succeeded\nstdout: %s\nstderr: %s", strings.Join(args, " "), res.Stdout, res.Stderr)
	}
	return res
}

// trailingID returns the id token at the end of an index/list line.
func trailingID(t *testing.T, line string) string {
	t.Helper()
	idx := strings.LastIndex(line, "#")
	if idx < 0 {
		t.Fatalf("no trailing id in %q", line)
	}
	return strings.TrimSpace(line[idx+1:])
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// addOpen creates an open ticket whose id is also its title.
func addOpen(t *testing.T, dir, id string) {
	t.Helper()
	mustRun(t, dir, "add", id, "--id", id)
}

// addInProgress creates a ticket and claims it with start.
func addInProgress(t *testing.T, dir, id string) {
	t.Helper()
	addOpen(t, dir, id)
	mustRun(t, dir, "start", id)
}

// addClosed creates a ticket and closes it through start and done, the only
// path to closed.
func addClosed(t *testing.T, dir, id string) {
	t.Helper()
	addInProgress(t, dir, id)
	mustRun(t, dir, "done", id)
}

func showOf(t *testing.T, dir, id string) string {
	t.Helper()
	return mustRun(t, dir, "show", id).Stdout
}

func statusOf(t *testing.T, dir, id string) string {
	t.Helper()
	show := showOf(t, dir, id)
	for _, line := range strings.Split(show, "\n") {
		if strings.HasPrefix(line, "status: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "status: "))
		}
	}
	t.Fatalf("no status line in %q", show)
	return ""
}

// detailFiles returns the names of the active detail files under .tickets/.
func detailFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ".tickets"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			out = append(out, e.Name())
		}
	}
	return out
}
