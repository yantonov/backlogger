package frontmatter

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// rejectedForms maps each out-of-subset construct to the fixture that
// exercises it. Every entry must produce a *ParseError, never a coerced value
// (spec 8.2 / 12).
var rejectedForms = []struct {
	form    string
	fixture string
}{
	{"block scalar |", "reject/block_scalar.md"},
	{"folded scalar >", "reject/folded_scalar.md"},
	{"anchor &", "reject/anchor.md"},
	{"alias *", "reject/alias.md"},
	{"merge key <<", "reject/merge_key.md"},
	{"flow map {}", "reject/flow_map.md"},
	{"nested/indented map", "reject/nested_map.md"},
	{"multi-line array", "reject/multiline_array.md"},
	{"implicit null", "reject/implicit_null.md"},
	{"tilde null", "reject/tilde_null.md"},
	{"bool-ish bare word", "reject/bool_word.md"},
	{"duplicate key", "reject/duplicate_key.md"},
	{"tab indentation", "reject/tab_indent.md"},
	{"trailing junk after list", "reject/trailing_junk.md"},
	{"invalid enum", "reject/bad_enum.md"},
	{"out-of-range int", "reject/bad_priority.md"},
}

func TestParse_OutOfSubsetFixture_ReturnsParseError(t *testing.T) {
	for _, tc := range rejectedForms {
		t.Run(tc.form, func(t *testing.T) {
			path := filepath.Join(testdataDir, filepath.FromSlash(tc.fixture))
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("missing fixture %s: %v", tc.fixture, err)
			}
			text, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			doc, err := Parse(text)
			if err == nil {
				t.Fatalf("Parse(%s) succeeded, want an error; got ticket %+v", tc.fixture, doc.Ticket)
			}
			var perr *ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("Parse(%s) error is %T, want *ParseError: %v", tc.fixture, err, err)
			}
			if perr.Msg == "" {
				t.Fatalf("Parse(%s) returned an empty error message", tc.fixture)
			}
		})
	}
}

// TestParse_NonIntegerPriority_FailsWithNoDocument checks the specific
// guarantee that a rejected value is not silently turned into something else:
// the error is returned and no document is produced.
func TestParse_NonIntegerPriority_FailsWithNoDocument(t *testing.T) {
	text := []byte("---\nstatus: open\npriority: notanint\n---\nbody\n")

	doc, err := Parse(text)

	if err == nil {
		t.Fatalf("Parse accepted a non-integer priority: %+v", doc.Ticket)
	}
	if doc != nil {
		t.Fatalf("Parse returned a document alongside the error: %+v", doc.Ticket)
	}
}

// TestRejectFixtures_EveryFile_IsListedInRejectedForms makes sure every file
// under testdata/reject is referenced by the reject table, so a new rejection
// construct cannot be added without a covering assertion.
func TestRejectFixtures_EveryFile_IsListedInRejectedForms(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(testdataDir, "reject"))
	if err != nil {
		t.Fatalf("read reject dir: %v", err)
	}
	listed := make(map[string]bool, len(rejectedForms))
	for _, tc := range rejectedForms {
		listed[tc.fixture] = true
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		rel := "reject/" + e.Name()
		if !listed[rel] {
			t.Errorf("fixture %s is not referenced by rejectedForms", rel)
		}
	}
}
