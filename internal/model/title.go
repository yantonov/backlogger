package model

import "strings"

// The title is not a stored field: it is the body's first ATX heading (spec
// 3.2), written by `add` as `# <title>`. These helpers own that rule so the
// reader (`show`), the writer (`edit --title`), and the index (`f(details)`) can
// never disagree about what counts as a title. Only a single-`#` heading
// qualifies; `## ` and deeper are prose the author wrote.

// isTitleLine reports whether line is a title heading. The space after `#` is
// required, so `#tag` is not a title.
func isTitleLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "# ")
}

// titleText returns the heading text of line, or "" when line is not a title
// heading or carries no text. `Trim` also drops an ATX closing sequence, so
// `# Name #` yields `Name`.
func titleText(line string) string {
	if !isTitleLine(line) {
		return ""
	}
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "#"))
}

// TitleLine returns the index of the body's title heading within the lines of
// body, or -1 when the body has none. Lines are split on "\n" with a trailing
// CR trimmed, so the index matches a plain line split of the body.
//
// This is the structural boundary: everything after the returned line is prose.
// A heading needs the space after `#`, so a bare `#` line is not a title at all.
func TitleLine(body []byte) int {
	for i, line := range strings.Split(string(body), "\n") {
		if isTitleLine(line) {
			return i
		}
	}
	return -1
}

// Title returns the display title of body, or "" when the body has no title
// heading. It skips a heading with empty text and keeps scanning, so an
// accidental `# ` line does not blank out the title.
func Title(body []byte) string {
	for _, line := range strings.Split(string(body), "\n") {
		if text := titleText(line); text != "" {
			return text
		}
	}
	return ""
}
