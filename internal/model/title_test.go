package model

import "testing"

// The title rule is shared by show, edit --title, and f(details), so it is
// pinned once here: only a `# ` line is a title, `## ` and deeper are prose, and
// a trailing ATX closing sequence is not part of the text (spec 3.2).
func TestTitle(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"empty body", "", ""},
		{"only newline", "\n", ""},
		{"plain title", "# Add login flow\n", "Add login flow"},
		{"no trailing newline", "# Add login flow", "Add login flow"},
		{"surrounding spaces", "#   Add login flow   \n", "Add login flow"},
		{"CRLF line", "# Add login flow\r\n", "Add login flow"},
		{"no space after hash", "#Add login flow\n", ""},
		{"h2 is prose, not a title", "## Description\n", ""},
		{"h3 is prose, not a title", "### Part 1\n", ""},
		{"bare hash", "#\n", ""},
		{"hash and spaces only", "#   \n", ""},
		{"empty heading before real title", "#\n# Real\n", "Real"},
		{"prose before title", "some prose\n# Later title\n", "Later title"},
		{"h2 before title is skipped", "## Sub\n# Real\n", "Real"},
		{"first of two titles wins", "# First\n# Second\n", "First"},
		{"trailing closing sequence", "# Name #\n", "Name"},
		{"hash inside text is kept", "# A ## B\n", "A ## B"},
		{"hash mid text is kept", "# A # B\n", "A # B"},
		{"tab after hash is not a title", "#\tTabbed\n", ""},
		{"blank lines before title", "\n\n# After blanks\n", "After blanks"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Title([]byte(tc.body)); got != tc.want {
				t.Fatalf("Title(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

// TestTitleLine pins the structural boundary Title's sibling reports, including
// the case where the heading has no text: it still marks where prose begins.
func TestTitleLine(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"empty body", "", -1},
		{"title first", "# T\n\nprose\n", 0},
		{"title after prose", "prose\n\n# T\n", 2},
		{"no title", "prose\n## Sub\n", -1},
		{"bare hash is not a heading", "#\nprose\n", -1},
		{"CRLF title", "\r\n# T\r\n", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := TitleLine([]byte(tc.body)); got != tc.want {
				t.Fatalf("TitleLine(%q) = %d, want %d", tc.body, got, tc.want)
			}
		})
	}
}
