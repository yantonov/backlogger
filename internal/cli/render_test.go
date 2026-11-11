package cli

import (
	"strings"
	"testing"
)

// The body-editor tests cover the prose model: the body is one opaque block and
// the title lives in frontmatter, so no payload is ever scanned for headings.

func descBody() []byte {
	return newBody("old desc")
}

func TestSetDescription_ReplacesWholeProseBlock(t *testing.T) {
	body := setDescription("new desc")

	if got := strings.TrimSpace(string(body)); got != "new desc" {
		t.Fatalf("replace: %q", got)
	}
}

func TestSetDescription_HeadingInPayload_IsStoredVerbatim(t *testing.T) {
	body := setDescription("part one\n## Part 1\npart two")

	want := "part one\n## Part 1\npart two\n"
	if string(body) != want {
		t.Fatalf("payload not verbatim:\n got %q\nwant %q", body, want)
	}

	// A second identical edit must not grow the body.
	if again := setDescription("part one\n## Part 1\npart two"); string(again) != want {
		t.Fatalf("edit is not idempotent:\n got %q\nwant %q", again, want)
	}
}

func TestSetDescription_EmptyText_EmptiesTheBody(t *testing.T) {
	body := setDescription("")

	if got := string(body); got != "" {
		t.Fatalf("expected an empty body, got %q", got)
	}
}

func TestAppendProse_KeepsExistingProseAndAddsParagraph(t *testing.T) {
	body := appendProse(descBody(), "more")

	if got := string(body); got != "old desc\n\nmore\n" {
		t.Fatalf("append: %q", got)
	}
}

func TestAppendProse_HeadingInPayload_IsStoredVerbatim(t *testing.T) {
	body := appendProse(descBody(), "see:\n## Detail\nx")

	if got := string(body); got != "old desc\n\nsee:\n## Detail\nx\n" {
		t.Fatalf("append not verbatim: %q", got)
	}
}

// TestFlagSet_InterleavedFlagsAndPositionals_ParsesValuesAndTracksSet covers
// flags before, after, and interleaved with positionals, the `=` form, and
// which flags count as set.
func TestFlagSet_InterleavedFlagsAndPositionals_ParsesValuesAndTracksSet(t *testing.T) {
	fs := newFlagSet()
	prio := fs.Int(2, "-p", "--priority")
	ref := fs.String("", "--append")
	all := fs.Bool("--all")

	err := fs.Parse([]string{"Title", "-p", "1", "--append=first", "--all"})

	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := fs.positionals(); len(got) != 1 || got[0] != "Title" {
		t.Fatalf("positionals = %v", got)
	}
	if *prio != 1 || *ref != "first" || !*all {
		t.Fatalf("values = %d %q %v", *prio, *ref, *all)
	}
	if !fs.Set("-p") || !fs.Set("--append") || !fs.Set("--all") {
		t.Fatalf("Set tracking failed")
	}
	if fs.Set("--description") {
		t.Fatalf("unset flag reported as set")
	}
}

// TestFlagSet_BadInput_FailsToParse covers the parse errors: a non-integer
// value, an unknown flag, and a missing value.
func TestFlagSet_BadInput_FailsToParse(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"non_integer", []string{"-p", "notint"}},
		{"unknown_flag", []string{"--nope"}},
		{"missing_value", []string{"--append"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFlagSet()
			fs.Int(2, "-p", "--priority")
			fs.String("", "--append")

			err := fs.Parse(tc.args)

			if err == nil {
				t.Fatalf("Parse(%q) accepted bad input", tc.args)
			}
		})
	}
}

func TestFlagSet_DoubleDash_TreatsRestAsPositionals(t *testing.T) {
	fs := newFlagSet()
	prio := fs.Int(2, "-p")
	if err := fs.Parse([]string{"--", "-p", "not-a-flag"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if *prio != 2 {
		t.Fatalf("flag after -- was parsed: %d", *prio)
	}
	if got := fs.positionals(); len(got) != 2 || got[0] != "-p" {
		t.Fatalf("positionals = %v", got)
	}
}

// The title lives in frontmatter and is required: the tracker no longer derives
// it from the body, so these tests pin the write and read halves of that rule.

// TestEditTitle_SetsFrontmatterField_WithoutTouchingBody verifies --title writes
// the field and never scans the body for a heading.
func TestEditTitle_SetsFrontmatterField_WithoutTouchingBody(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Old title", "--id", "abc", "-d", "prose")

	c.must(t, "edit", "abc", "--title", "New title")

	paths := c.paths(t)
	meta, err := paths.ReadDetail("abc")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Ticket.Title != "New title" {
		t.Fatalf("title = %q, want New title", meta.Ticket.Title)
	}
	if string(meta.Body) != "prose\n" {
		t.Fatalf("body changed: %q", meta.Body)
	}
	// The index is f(details), so the new title must be on the index line.
	if got := c.must(t, "list", "-vv"); !strings.Contains(got, "New title #abc") {
		t.Fatalf("index line not regenerated: %q", got)
	}
}

// TestEditTitle_BlankTitle_FailsAndChangesNothing verifies the empty-title
// invariant is enforced on the write path too, with a message that names it.
func TestEditTitle_BlankTitle_FailsAndChangesNothing(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Keep me", "--id", "abc")
	before := c.must(t, "show", "abc")

	errOut := c.fail(t, "edit", "abc", "--title", "   ")

	if !strings.Contains(errOut, "non-empty title") {
		t.Fatalf("error does not name the invariant: %q", errOut)
	}
	if after := c.must(t, "show", "abc"); after != before {
		t.Fatalf("failed edit changed the detail:\n%s", after)
	}
}

// TestShow_MissingTitle_ErrorsNamingTheInvariant verifies a hand-written detail
// without a title is loud on every read path instead of rendering an empty
// title (spec 5.3).
func TestShow_MissingTitle_ErrorsNamingTheInvariant(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Real", "--id", "abc")
	writeRawDetail(t, dir, "abc", "---\nstatus: open\npriority: 2\ndeps: []\nattachments: []\n---\nprose\n")

	errOut := c.fail(t, "show", "abc")

	if !strings.Contains(errOut, "title is required") {
		t.Fatalf("error does not name the invariant: %q", errOut)
	}
}

// TestAdd_BodyWithShellComment_IsNotTheTitle is the regression test for the old
// convention: a `# ` line inside a fence used to become the display title.
func TestAdd_BodyWithShellComment_IsNotTheTitle(t *testing.T) {
	c := newCLI(t, t.TempDir())
	body := "```sh\n# install\nmake check\n```\n"

	c.must(t, "add", "Install steps", "--id", "abc", "-d", body)

	if got := c.must(t, "list", "-vv"); !strings.Contains(got, "Install steps #abc") {
		t.Fatalf("index title = %q, want the title field", got)
	}
	meta, err := c.paths(t).ReadDetail("abc")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Ticket.Title != "Install steps" || string(meta.Body) != body {
		t.Fatalf("title %q body %q", meta.Ticket.Title, meta.Body)
	}
}
