package cli

import (
	"strings"
	"testing"
)

// The body-editor tests cover the prose model: the body is the title heading
// plus one opaque block, so no payload is ever scanned for headings.

func descBody() []byte {
	return newBody("Title", "old desc")
}

func TestSetDescription_ReplacesWholeProseBlock(t *testing.T) {
	body := descBody()

	body = setDescription(body, "new desc")

	if got := strings.TrimSpace(string(body)); got != "# Title\n\nnew desc" {
		t.Fatalf("replace: %q", got)
	}
}

func TestSetDescription_HeadingInPayload_IsStoredVerbatim(t *testing.T) {
	body := setDescription(descBody(), "part one\n## Part 1\npart two")

	want := "# Title\n\npart one\n## Part 1\npart two\n"
	if string(body) != want {
		t.Fatalf("payload not verbatim:\n got %q\nwant %q", body, want)
	}

	// A second identical edit must not grow the body.
	if again := setDescription(body, "part one\n## Part 1\npart two"); string(again) != want {
		t.Fatalf("edit is not idempotent:\n got %q\nwant %q", again, want)
	}
}

func TestSetDescription_EmptyText_RemovesProseKeepsTitle(t *testing.T) {
	body := setDescription(descBody(), "")

	if got := string(body); got != "# Title\n" {
		t.Fatalf("expected title only, got %q", got)
	}
}

func TestAppendProse_KeepsExistingProseAndAddsParagraph(t *testing.T) {
	body := appendProse(descBody(), "more")

	if got := string(body); got != "# Title\n\nold desc\n\nmore\n" {
		t.Fatalf("append: %q", got)
	}
}

func TestAppendProse_HeadingInPayload_IsStoredVerbatim(t *testing.T) {
	body := appendProse(descBody(), "see:\n## Detail\nx")

	if got := string(body); got != "# Title\n\nold desc\n\nsee:\n## Detail\nx\n" {
		t.Fatalf("append not verbatim: %q", got)
	}
}

func TestSetBodyTitle_ExistingHeading_ReplacesItAndKeepsContent(t *testing.T) {
	out := setBodyTitle([]byte("# Old\n\nprose\nx\n"), "New")

	if bodyTitle(out) != "New" {
		t.Fatalf("title not replaced: %q", out)
	}
	if !strings.Contains(string(out), "prose") {
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
