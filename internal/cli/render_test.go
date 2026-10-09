package cli

import (
	"strings"
	"testing"
)

// The section-editor tests cover setSection: replace, append, and remove.

func descAndNotes() []byte {
	return newBody("Title", []bodySection{
		{Name: sectionDescription, Text: "old desc"},
		{Name: sectionNotes, Text: "old note"},
	})
}

func TestSetSection_Replace_ReplacesOnlyTargetSection(t *testing.T) {
	body := descAndNotes()

	body = setSection(body, sectionDescription, "new desc", false)

	if got := sectionText(body, sectionDescription); got != "new desc" {
		t.Fatalf("replace: got %q", got)
	}
	if got := sectionText(body, sectionNotes); got != "old note" {
		t.Fatalf("unrelated section changed: %q", got)
	}
}

func TestSetSection_Append_AddsBlockAfterExistingText(t *testing.T) {
	body := descAndNotes()

	body = setSection(body, sectionNotes, "first", true)

	if got := sectionText(body, sectionNotes); got != "old note\n\nfirst" {
		t.Fatalf("append: got %q", got)
	}
}

func TestSetSection_EmptyText_RemovesSectionAndHeading(t *testing.T) {
	body := descAndNotes()

	body = setSection(body, sectionNotes, "", false)

	if sectionText(body, sectionNotes) != "" {
		t.Fatalf("remove failed: %q", body)
	}
	if got := strings.Count(string(body), "## "); got != 1 {
		t.Fatalf("expected 1 heading after removal, got %d: %q", got, body)
	}
}

func TestSetBodyTitle_ExistingHeading_ReplacesItAndKeepsContent(t *testing.T) {
	out := setBodyTitle([]byte("# Old\n\n## Notes\nx\n"), "New")

	if bodyTitle(out) != "New" {
		t.Fatalf("title not replaced: %q", out)
	}
	if !strings.Contains(string(out), "## Notes") {
		t.Fatalf("content lost: %q", out)
	}
}

func TestSetBodyTitle_NoHeading_PrependsItAndKeepsContent(t *testing.T) {
	out := setBodyTitle([]byte("plain paragraph\n"), "Fresh")

	if bodyTitle(out) != "Fresh" {
		t.Fatalf("heading not prepended: %q", out)
	}
	if !strings.Contains(string(out), "plain paragraph") {
		t.Fatalf("paragraph lost: %q", out)
	}
}

// TestFlagSet_InterleavedFlagsAndPositionals_ParsesValuesAndTracksSet covers
// flags before, after, and interleaved with positionals, the `=` form, and
// which flags count as set.
func TestFlagSet_InterleavedFlagsAndPositionals_ParsesValuesAndTracksSet(t *testing.T) {
	fs := newFlagSet()
	prio := fs.Int(2, "-p", "--priority")
	ref := fs.String("", "--notes")
	all := fs.Bool("--all")

	err := fs.Parse([]string{"Title", "-p", "1", "--notes=first", "--all"})

	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := fs.positionals(); len(got) != 1 || got[0] != "Title" {
		t.Fatalf("positionals = %v", got)
	}
	if *prio != 1 || *ref != "first" || !*all {
		t.Fatalf("values = %d %q %v", *prio, *ref, *all)
	}
	if !fs.Set("-p") || !fs.Set("--notes") || !fs.Set("--all") {
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
		{"missing_value", []string{"--notes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFlagSet()
			fs.Int(2, "-p", "--priority")
			fs.String("", "--notes")

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

// sectionText returns a section's prose (without the heading) for assertions.
func sectionText(body []byte, name string) string {
	start, end := sectionBounds(body, name)
	if start < 0 || start+1 > end {
		return ""
	}
	return strings.TrimSpace(strings.Join(splitLines(body)[start+1:end], "\n"))
}
