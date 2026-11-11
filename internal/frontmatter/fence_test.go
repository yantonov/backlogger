package frontmatter

import (
	"bytes"
	"errors"
	"testing"

	"github.com/yantonov/backlogger/internal/model"
)

// TestParse_MissingTitle_ReturnsErrTitleMissing verifies that a file without a
// title is loud on read. A title has no default, so an unfenced file (which has
// no fields at all) is an error and never a silently empty title (spec 5.3,
// 8.2).
func TestParse_MissingTitle_ReturnsErrTitleMissing(t *testing.T) {
	in := readFixture(t, "no_title.md")
	if _, err := Parse(in); !errors.Is(err, model.ErrTitleMissing) {
		t.Fatalf("Parse body-only = %v, want ErrTitleMissing", err)
	}
}

// TestParse_BlankTitle_ReturnsErrTitleMissing verifies that an explicitly empty
// or whitespace-only title is rejected. The subset can spell both, so the check
// cannot be "did the key appear".
func TestParse_BlankTitle_ReturnsErrTitleMissing(t *testing.T) {
	for _, in := range []string{
		"---\ntitle: \"\"\n---\nbody\n",
		"---\ntitle: \"   \"\n---\nbody\n",
	} {
		if _, err := Parse([]byte(in)); !errors.Is(err, model.ErrTitleMissing) {
			t.Fatalf("Parse %q = %v, want ErrTitleMissing", in, err)
		}
	}
}

// TestParse_BodyHeading_IsProseNotTitle verifies the body is pure prose: a file
// whose body starts with a `## ` heading parses, and the title comes from the
// field, not from the body.
func TestParse_BodyHeading_IsProseNotTitle(t *testing.T) {
	in := []byte("---\ntitle: \"Real title\"\n---\n## Design\n\nprose\n")
	doc, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Ticket.Title != "Real title" {
		t.Fatalf("title = %q, want the frontmatter title", doc.Ticket.Title)
	}
	if !bytes.Equal(doc.Body, []byte("## Design\n\nprose\n")) {
		t.Fatalf("body must be kept verbatim, got %q", doc.Body)
	}
}

// TestParse_DashLinesInBody_KeepsWholeBody verifies the body after the closing
// fence may contain "---" freely; only the first exact fence line ends the
// block.
func TestParse_DashLinesInBody_KeepsWholeBody(t *testing.T) {
	in := readFixture(t, "good.md")
	doc, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !bytes.Contains(doc.Body, []byte("---")) {
		t.Fatalf("body should retain its own --- lines")
	}
	if !bytes.Contains(doc.Body, []byte("More prose.")) {
		t.Fatalf("body after a later --- must not be truncated:\n%s", doc.Body)
	}
}

// TestParse_BlankLineBeforeFence_NoFrontmatterSoNoTitle verifies that
// whitespace before the opening fence means there is no frontmatter block at
// all, and therefore no title.
func TestParse_BlankLineBeforeFence_NoFrontmatterSoNoTitle(t *testing.T) {
	in := []byte("\n---\ntitle: t\nstatus: closed\n---\nbody\n")
	if _, err := Parse(in); !errors.Is(err, model.ErrTitleMissing) {
		t.Fatalf("leading blank line must yield no frontmatter, got %v", err)
	}
}

// TestParse_UnclosedFence_ReturnsError verifies that an opened-but-unclosed
// block is loud.
func TestParse_UnclosedFence_ReturnsError(t *testing.T) {
	in := []byte("---\nstatus: open\n")
	if _, err := Parse(in); err == nil {
		t.Fatalf("unclosed frontmatter fence was accepted")
	}
}

// TestParse_CRLFFences_ParsesFields verifies Windows line endings are accepted.
func TestParse_CRLFFences_ParsesFields(t *testing.T) {
	in := []byte("---\r\ntitle: crlf\r\nstatus: closed\r\npriority: 3\r\n---\r\nbody\r\n")
	doc, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse CRLF: %v", err)
	}
	if doc.Ticket.Title != "crlf" || doc.Ticket.Status != model.StatusClosed || doc.Ticket.Priority != 3 {
		t.Fatalf("CRLF frontmatter not parsed: %+v", doc.Ticket)
	}
}
