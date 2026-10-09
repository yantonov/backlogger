package frontmatter

import (
	"bytes"
	"testing"

	"github.com/yantonov/backlogger/internal/model"
)

// TestParse_NoLeadingFence_DefaultsFieldsAndKeepsBodyVerbatim verifies that a
// file without a leading fence is a body-only ticket with default fields and is
// never a data-losing error (spec 8.2).
func TestParse_NoLeadingFence_DefaultsFieldsAndKeepsBodyVerbatim(t *testing.T) {
	in := readFixture(t, "body_only.md")
	doc, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse body-only: %v", err)
	}
	want := model.DefaultTicket()
	if doc.Ticket.Status != want.Status || doc.Ticket.Priority != want.Priority {
		t.Fatalf("body-only ticket defaults = %+v, want %+v", doc.Ticket, want)
	}
	if !bytes.Equal(doc.Body, in) {
		t.Fatalf("body-only content must be preserved verbatim")
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

// TestParse_BlankLineBeforeFence_YieldsDefaults verifies that whitespace before
// the opening fence means there is no frontmatter block at all.
func TestParse_BlankLineBeforeFence_YieldsDefaults(t *testing.T) {
	in := []byte("\n---\nstatus: closed\n---\nbody\n")
	doc, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Ticket.Status != model.DefaultStatus {
		t.Fatalf("leading blank line must yield defaults, got %q", doc.Ticket.Status)
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
	in := []byte("---\r\nstatus: closed\r\npriority: 3\r\n---\r\nbody\r\n")
	doc, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse CRLF: %v", err)
	}
	if doc.Ticket.Status != model.StatusClosed || doc.Ticket.Priority != 3 {
		t.Fatalf("CRLF frontmatter not parsed: %+v", doc.Ticket)
	}
}
