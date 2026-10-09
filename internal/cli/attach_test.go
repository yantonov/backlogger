package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The attachment-surface tests cover copy into the sidecar, the
// authoritative frontmatter link, listing, show, and removing both sides on
// detach (spec 3.5).

// givenLoginWithPlanAndShot returns a harness whose "login" ticket has
// plan.md and screenshot.png attached, and the plan.md source path.
func givenLoginWithPlanAndShot(t *testing.T) (c *testCLI, plan string) {
	t.Helper()
	c = newCLI(t, t.TempDir())
	c.must(t, "add", "Login", "--id", "login")
	plan = writeSource(t, "plan.md", "PLAN")
	c.must(t, "attach", "login", writeSource(t, "screenshot.png", "PNG"), plan)
	return c, plan
}

func TestAttach_TwoFiles_ReportsCopiesStoresBytesAndLinksSorted(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Login", "--id", "login")
	shot := writeSource(t, "screenshot.png", "PNG")
	plan := writeSource(t, "plan.md", "PLAN")

	out := c.must(t, "attach", "login", shot, plan)

	if !strings.Contains(out, "attached screenshot.png to login") || !strings.Contains(out, "attached plan.md to login") {
		t.Fatalf("attach output = %q", out)
	}
	if got := readAttachmentFile(t, dir, "login__screenshot.png"); got != "PNG" {
		t.Fatalf("screenshot sidecar bytes = %q", got)
	}
	if got := readAttachmentFile(t, dir, "login__plan.md"); got != "PLAN" {
		t.Fatalf("plan sidecar bytes = %q", got)
	}
	if show := c.must(t, "show", "login"); !strings.Contains(show, "attachments: [plan.md, screenshot.png]") {
		t.Fatalf("frontmatter link missing or unsorted:\n%s", show)
	}
}

func TestAttachments_TwoAttached_ListsSorted(t *testing.T) {
	c, _ := givenLoginWithPlanAndShot(t)

	out := c.must(t, "attachments", "login")

	if strings.TrimSpace(out) != "plan.md\nscreenshot.png" {
		t.Fatalf("attachments output = %q", out)
	}
}

func TestShowAttachments_TwoAttached_AppendsBlock(t *testing.T) {
	c, _ := givenLoginWithPlanAndShot(t)

	out := c.must(t, "show", "login", "--attachments")

	if !strings.Contains(out, "Attachments:") || !strings.Contains(out, "  plan.md") || !strings.Contains(out, "  screenshot.png") {
		t.Fatalf("show --attachments output = %q", out)
	}
}

func TestDetach_AttachedFile_RemovesFileAndLink(t *testing.T) {
	c, plan := givenLoginWithPlanAndShot(t)

	c.must(t, "detach", "login", plan)

	if _, err := os.Stat(filepath.Join(c.app.Dir, ".tickets", "attachments", "login__plan.md")); !os.IsNotExist(err) {
		t.Fatalf("plan.md not removed: %v", err)
	}
	if show := c.must(t, "show", "login"); !strings.Contains(show, "attachments: [screenshot.png]") {
		t.Fatalf("detach did not clear the link:\n%s", show)
	}
}

func TestDetach_AlreadyDetached_FailsWithNoAttachment(t *testing.T) {
	c, plan := givenLoginWithPlanAndShot(t)
	c.must(t, "detach", "login", plan)

	errOut := c.fail(t, "detach", "login", plan)

	if !strings.Contains(errOut, "no attachment") {
		t.Fatalf("second detach error = %q", errOut)
	}
}

// The optional-directory tests prove the sidecar directory is created only
// when a ticket actually gets an attachment, and that a ticket with none
// reports it cleanly.

func TestAdd_Ticket_CreatesNoAttachmentsDir(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)

	c.must(t, "add", "Bare", "--id", "bare")

	if _, err := os.Stat(filepath.Join(dir, ".tickets", "attachments")); !os.IsNotExist(err) {
		t.Fatalf("attachments dir exists before any attach: %v", err)
	}
}

func TestAttachments_NoAttachments_ReportsNone(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Bare", "--id", "bare")

	out := c.must(t, "attachments", "bare")

	if !strings.Contains(out, "(no attachments)") {
		t.Fatalf("attachments output = %q", out)
	}
}

func TestShowAttachments_NoAttachments_ReportsNone(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Bare", "--id", "bare")

	out := c.must(t, "show", "bare", "--attachments")

	if !strings.Contains(out, "(none)") {
		t.Fatalf("show --attachments output = %q", out)
	}
}

func TestAttach_FirstFile_CreatesAttachmentsDir(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Bare", "--id", "bare")

	c.must(t, "attach", "bare", writeSource(t, "x.bin", "X"))

	if _, err := os.Stat(filepath.Join(dir, ".tickets", "attachments")); err != nil {
		t.Fatalf("attachments dir not created lazily: %v", err)
	}
}

// The same-suffix tests prove a same ticket + suffix copy replaces the bytes
// and says so, and that the same suffix on another ticket never overwrites
// the first ticket's file.

// givenAlphaWithPlan returns a harness with tickets "alpha" and "bravo", and
// plan.md (bytes "ALPHA") attached to alpha.
func givenAlphaWithPlan(t *testing.T) *testCLI {
	t.Helper()
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Alpha", "--id", "alpha")
	c.must(t, "add", "Bravo", "--id", "bravo")
	c.must(t, "attach", "alpha", writeSource(t, "plan.md", "ALPHA"))
	return c
}

func TestAttach_SameTicketAndSuffix_ReplacesBytesAndReportsIt(t *testing.T) {
	c := givenAlphaWithPlan(t)

	out := c.must(t, "attach", "alpha", writeSource(t, "plan.md", "ALPHA2"))

	if !strings.Contains(out, "replaced plan.md on alpha") {
		t.Fatalf("replacement not reported: %q", out)
	}
	if got := readAttachmentFile(t, c.app.Dir, "alpha__plan.md"); got != "ALPHA2" {
		t.Fatalf("replacement bytes = %q", got)
	}
	if show := c.must(t, "show", "alpha"); !strings.Contains(show, "attachments: [plan.md]") {
		t.Fatalf("alpha link duplicated or lost:\n%s", show)
	}
}

func TestAttach_SameSuffixOnOtherTicket_LeavesFirstTicketsFile(t *testing.T) {
	c := givenAlphaWithPlan(t)

	c.must(t, "attach", "bravo", writeSource(t, "plan.md", "BRAVO"))

	if got := readAttachmentFile(t, c.app.Dir, "alpha__plan.md"); got != "ALPHA" {
		t.Fatalf("bravo attach clobbered alpha: %q", got)
	}
	if got := readAttachmentFile(t, c.app.Dir, "bravo__plan.md"); got != "BRAVO" {
		t.Fatalf("bravo bytes = %q", got)
	}
}

// The detach-repair tests prove detach repairs either half-broken side: it
// clears a link whose file is missing, and removes an orphan file whose link
// is missing (spec 3.5).

func TestDetach_LinkWithoutFile_ClearsLink(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Thing", "--id", "thing")
	c.setAttachments(t, "thing", "ghost.md")

	c.must(t, "detach", "thing", "ghost.md")

	if show := c.must(t, "show", "thing"); !strings.Contains(show, "attachments: []") {
		t.Fatalf("frontmatter link not cleared:\n%s", show)
	}
}

func TestDetach_OrphanFile_RemovesFile(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Thing", "--id", "thing")
	orphan := c.writeOrphan(t, "thing", "orphan.md")

	c.must(t, "detach", "thing", "orphan.md")

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan not removed: %v", err)
	}
}

// The disagreement tests prove `bl attachments` and `show --attachments`
// surface a missing link as (missing) and an orphan file as (orphan), the
// disagreement doctor checks.

// givenMissingAndOrphan returns a harness whose "thing" declares a link with
// no file and has a file with no link.
func givenMissingAndOrphan(t *testing.T) *testCLI {
	t.Helper()
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Thing", "--id", "thing")
	c.setAttachments(t, "thing", "declared-but-missing.md")
	c.writeOrphan(t, "thing", "orphan.md")
	return c
}

func TestAttachments_MissingAndOrphan_FlagsBoth(t *testing.T) {
	c := givenMissingAndOrphan(t)

	out := c.must(t, "attachments", "thing")

	if !strings.Contains(out, "declared-but-missing.md (missing)") {
		t.Fatalf("missing link not flagged: %q", out)
	}
	if !strings.Contains(out, "orphan.md (orphan)") {
		t.Fatalf("orphan not flagged: %q", out)
	}
}

func TestShowAttachments_MissingAndOrphan_FlagsBoth(t *testing.T) {
	c := givenMissingAndOrphan(t)

	out := c.must(t, "show", "thing", "--attachments")

	if !strings.Contains(out, "declared-but-missing.md (missing)") || !strings.Contains(out, "orphan.md (orphan)") {
		t.Fatalf("show --attachments did not flag links: %q", out)
	}
}

// The traversal tests prove a suffix may not escape the attachments
// directory: the path element `..` and any name whose base is not a plain
// filename are rejected before anything is written.

func TestAttach_TraversalSuffix_FailsAndChangesNothing(t *testing.T) {
	for _, tc := range []struct{ name, suffix string }{
		{"dotdot", ".."},
		{"a_dotdot", "a/.."},
		{"slash", "/"},
		{"backslash", `\`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			c := newCLI(t, dir)
			c.must(t, "add", "Thing", "--id", "thing")
			before := treeSnapshot(t, dir)

			c.fail(t, "attach", "thing", tc.suffix)

			assertTreeUnchanged(t, fmt.Sprintf("rejected suffix %q", tc.suffix), before, treeSnapshot(t, dir))
		})
	}
}

func TestDetach_TraversalSuffix_FailsAndChangesNothing(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Thing", "--id", "thing")
	before := treeSnapshot(t, dir)

	c.fail(t, "detach", "thing", "..")

	assertTreeUnchanged(t, "rejected detach", before, treeSnapshot(t, dir))
}

// TestAttach_UnderscoreID_FailsAndCreatesNothing proves the delimiter's grammar
// premise holds at the CLI boundary: an explicit underscore ID is a grammar
// error and no state is created.
func TestAttach_UnderscoreID_FailsAndCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Thing", "--id", "thing")
	src := writeSource(t, "a.md", "A")
	before := treeSnapshot(t, dir)

	errOut := c.fail(t, "attach", "my_task", src)

	if !strings.Contains(errOut, "invalid ticket ID") {
		t.Fatalf("underscore id error = %q", errOut)
	}
	assertTreeUnchanged(t, "rejected attach", before, treeSnapshot(t, dir))
}

// The no-attachments-dir tests prove the hot path never needs the attachments
// directory: with it removed, the views still answer from the index, and a
// mutation (which reindexes) neither fails nor recreates it (spec 3.5).

// givenAttachmentsDirRemoved returns a harness with open tickets "one" and
// "two", where one had an attachment and the attachments dir was then removed.
func givenAttachmentsDirRemoved(t *testing.T) *testCLI {
	t.Helper()
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "One", "--id", "one")
	c.must(t, "add", "Two", "--id", "two")
	c.must(t, "attach", "one", writeSource(t, "screenshot.png", "PNG"))
	if err := os.RemoveAll(filepath.Join(c.app.Dir, ".tickets", "attachments")); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestViews_AttachmentsDirRemoved_AnswerFromIndex(t *testing.T) {
	const both = "- [ ] One #one\n- [ ] Two #two\n"
	for _, tc := range []struct{ view, want string }{
		{"list", both},
		{"available", both},
		{"started", ""},
		{"blocked", ""},
	} {
		t.Run(tc.view, func(t *testing.T) {
			c := givenAttachmentsDirRemoved(t)

			out := c.must(t, tc.view, "-vv")

			if out != tc.want {
				t.Fatalf("%s -vv = %q, want %q", tc.view, out, tc.want)
			}
		})
	}
}

func TestStart_AttachmentsDirRemoved_DoesNotRecreateIt(t *testing.T) {
	c := givenAttachmentsDirRemoved(t)

	c.must(t, "start", "two")

	if _, err := os.Stat(filepath.Join(c.app.Dir, ".tickets", "attachments")); !os.IsNotExist(err) {
		t.Fatalf("a mutation recreated the attachments dir: %v", err)
	}
}

// The partial-archived-ID tests prove the attachment commands resolve a
// partial ID that names an archived ticket: the link lives in the frontmatter
// and the detail file is read on demand.

func TestAttach_PartialArchivedID_StoresFileAndLink(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Thing", "--id", "partial")
	c.archiveTicket(t, "partial")

	c.must(t, "attach", "part", writeSource(t, "plan.md", "P"))

	if got := readAttachmentFile(t, dir, "partial__plan.md"); got != "P" {
		t.Fatalf("sidecar bytes = %q", got)
	}
	if show := c.must(t, "show", "partial"); !strings.Contains(show, "attachments: [plan.md]") {
		t.Fatalf("link missing on archived ticket:\n%s", show)
	}
}

func TestAttachments_PartialArchivedID_ListsFile(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Thing", "--id", "partial")
	c.archiveTicket(t, "partial")
	c.must(t, "attach", "partial", writeSource(t, "plan.md", "P"))

	out := c.must(t, "attachments", "part")

	if strings.TrimSpace(out) != "plan.md" {
		t.Fatalf("archived attachments output = %q", out)
	}
}
