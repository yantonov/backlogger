package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/store"
)

// fixedNow is the deterministic clock every in-process test uses.
var fixedNow = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

// newTestApp returns an App whose working directory is dir, whose clock is
// fixed, and whose output is captured in the returned buffers.
func newTestApp(t *testing.T, dir string) (*App, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := &App{
		Stdout: &stdout,
		Stderr: &stderr,
		Dir:    dir,
		Now:    func() time.Time { return fixedNow },
	}
	return app, &stdout, &stderr
}

// testCLI is an in-process CLI harness with per-run output buffers.
type testCLI struct {
	app *App
	out *bytes.Buffer
	err *bytes.Buffer
}

// newCLI returns a harness rooted at dir with a deterministic clock.
func newCLI(t *testing.T, dir string) *testCLI {
	t.Helper()
	app, out, errBuf := newTestApp(t, dir)
	return &testCLI{app: app, out: out, err: errBuf}
}

// run resets the buffers and executes args, returning stdout and the code.
func (c *testCLI) run(args ...string) (string, int) {
	c.out.Reset()
	c.err.Reset()
	code := c.app.Run(args)
	return c.out.String(), code
}

// must runs args and fails the test on a nonzero exit. It returns stdout.
func (c *testCLI) must(t *testing.T, args ...string) string {
	t.Helper()
	out, code := c.run(args...)
	if code != 0 {
		t.Fatalf("bl %s: exit %d\nstdout: %s\nstderr: %s", strings.Join(args, " "), code, out, c.err.String())
	}
	return out
}

// fail runs args and fails the test if it succeeds. It returns stderr.
func (c *testCLI) fail(t *testing.T, args ...string) string {
	t.Helper()
	out, code := c.run(args...)
	if code == 0 {
		t.Fatalf("bl %s unexpectedly succeeded\nstdout: %s", strings.Join(args, " "), out)
	}
	return c.err.String()
}

// closeTicket moves an open ticket to closed through the guarded verbs
// (start, then done), the only path to closed.
func (c *testCLI) closeTicket(t *testing.T, id string) {
	t.Helper()
	c.must(t, "start", id)
	c.must(t, "done", id)
}

// archiveTicket closes an open ticket and archives it.
func (c *testCLI) archiveTicket(t *testing.T, id string) {
	t.Helper()
	c.closeTicket(t, id)
	c.must(t, "archive", id)
}

// closeTicketAgo closes an open ticket with the clock set age before
// fixedNow, then restores the clock, so closed-at is age in the past.
func (c *testCLI) closeTicketAgo(t *testing.T, id string, age time.Duration) {
	t.Helper()
	c.app.Now = func() time.Time { return fixedNow.Add(-age) }
	c.closeTicket(t, id)
	c.app.Now = func() time.Time { return fixedNow }
}

func (c *testCLI) paths(t *testing.T) *store.Paths {
	t.Helper()
	return store.NewPaths(c.app.Dir)
}

// setDeps overwrites the deps of id directly on disk, bypassing the cycle
// guard so a corrupt graph can be injected (spec 5.2 backstop tests).
func (c *testCLI) setDeps(t *testing.T, id string, deps ...string) {
	t.Helper()
	paths := c.paths(t)
	meta, err := paths.ReadDetail(id)
	if err != nil {
		t.Fatal(err)
	}
	meta.Ticket.Deps = deps
	if err := paths.WriteDetail(meta); err != nil {
		t.Fatal(err)
	}
}

// setAttachments overwrites a ticket's attachments list directly on disk,
// bypassing the command surface to inject a missing link or an orphan.
func (c *testCLI) setAttachments(t *testing.T, id string, links ...string) {
	t.Helper()
	paths := c.paths(t)
	meta, err := paths.ReadDetail(id)
	if err != nil {
		t.Fatal(err)
	}
	meta.Ticket.Attachments = links
	if err := paths.WriteDetail(meta); err != nil {
		t.Fatal(err)
	}
}

// writeOrphan writes a sidecar file for id with no frontmatter link, the
// orphan side of the attachment invariant (spec 3.5). It returns the path.
func (c *testCLI) writeOrphan(t *testing.T, id, suffix string) string {
	t.Helper()
	dir := filepath.Join(c.app.Dir, ".tickets", "attachments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, id+"__"+suffix)
	if err := os.WriteFile(p, []byte("O"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeSource creates a file with body under a fresh temp dir and returns its
// path.
func writeSource(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readAttachmentFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".tickets", "attachments", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// treeDirMarker is the snapshot value of a directory, so a created or removed
// empty directory shows up as a tree change.
const treeDirMarker = "<dir>"

// treeSnapshot records every path under root: a file maps to its bytes, a
// directory to treeDirMarker.
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			out[rel] = treeDirMarker
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// assertTreeUnchanged fails when after differs from before in any file:
// removed, changed, or created. what names the action for the message.
func assertTreeUnchanged(t *testing.T, what string, before, after map[string]string) {
	t.Helper()
	for k, v := range before {
		got, ok := after[k]
		if !ok {
			t.Fatalf("%s removed %s", what, k)
		}
		if got != v {
			t.Fatalf("%s changed %s", what, k)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			t.Fatalf("%s created %s", what, k)
		}
	}
}

func nonEmptyAppLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// viewLineIDs extracts the #id tokens from a rendered view in output order.
func viewLineIDs(t *testing.T, out string) []string {
	t.Helper()
	var ids []string
	for _, line := range nonEmptyAppLines(out) {
		ids = append(ids, idFromLine(t, line))
	}
	return ids
}

// idFromLine returns the trailing #id token of a rendered list/view line.
func idFromLine(t *testing.T, line string) string {
	t.Helper()
	i := strings.LastIndex(line, "#")
	if i < 0 {
		t.Fatalf("no trailing id in %q", line)
	}
	return strings.TrimSpace(line[i+1:])
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// lineIDs extracts the trailing #id tokens from rendered -vv lines, stopping
// at the id so a following `<- [deps]` suffix is not absorbed.
func lineIDs(t *testing.T, out string) []string {
	t.Helper()
	var ids []string
	for _, line := range nonEmptyAppLines(out) {
		i := strings.LastIndex(line, "#")
		if i < 0 {
			t.Fatalf("no trailing id in %q", line)
		}
		token := line[i+1:]
		if sp := strings.IndexAny(token, " \t"); sp >= 0 {
			token = token[:sp]
		}
		ids = append(ids, token)
	}
	return ids
}
