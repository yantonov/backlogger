package index

import (
	"regexp"
	"sort"
	"strings"
)

// The index line grammar (spec 8.1). The glyph is the single character inside
// the brackets: space (todo), > (in progress), or x (done). The rest of the
// line is the title, trailing #id token, and optional `<- [deps]` suffix.
var indexLineRe = regexp.MustCompile(`^(\s*-\s*\[)([ >x])(\]\s*)(.*)$`)

// The trailing identity token (spec 8.1): lowercase, minus and digits allowed,
// at least three characters. It anchors at the end of the line.
var idTokenRe = regexp.MustCompile(`(?:^|\s)#([a-z][a-z0-9-]{2,})\s*$`)

// Parse reads a backlog.md index into its entries. It is deliberately tolerant
// of human edits: unknown non-ticket lines (headings, separators, prose) are
// ignored, and a line that matches the checkbox grammar but has no trailing
// #id is skipped rather than turned into a bogus entry. The index is never
// authoritative (spec 3.1): a caller that needs truth re-reads the detail file.
//
// Entries are returned in file order. Archived tickets are not in the index, so
// parsing never sets an archived marker (spec 3.1).
//
// Each entry's Deps is sorted and unique.
func Parse(data []byte) []Entry {
	var out []Entry
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if _, ok := sectionHeading(line); ok {
			continue
		}
		m := indexLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rest := m[4]
		// The deps suffix sits after the #id token in the rendered form, so it
		// must be stripped before the trailing #id can be recognized.
		core, deps := splitDeps(rest)
		id, title, ok := splitID(core)
		if !ok {
			continue
		}
		out = append(out, Entry{
			ID:    id,
			Glyph: m[2],
			Title: strings.TrimSpace(title),
			Deps:  deps,
		})
	}
	return out
}

// sectionHeading reports which section a heading line names, if any. Only the
// two canonical headings are recognized; any other heading is ignored.
func sectionHeading(line string) (Group, bool) {
	switch strings.TrimSpace(line) {
	case "## Active":
		return GroupActive, true
	case "## Closed":
		return GroupClosed, true
	default:
		return GroupActive, false
	}
}

// splitID splits the trailing #id token off rest. The title is everything
// before it. It returns ok=false when there is no valid trailing id, which
// makes Parse skip the line instead of inventing an entry.
func splitID(rest string) (id, title string, ok bool) {
	loc := idTokenRe.FindStringSubmatchIndex(rest)
	if loc == nil {
		return "", "", false
	}
	id = rest[loc[2]:loc[3]]
	title = rest[:loc[0]]
	return id, title, true
}

// splitDeps removes a trailing `<- [deps]` suffix from title and returns the
// cleaned title plus the parsed, sorted dep list. A malformed suffix is left in
// the title; the detail file remains authoritative for deps.
func splitDeps(title string) (string, []string) {
	trimmed := strings.TrimRight(title, " \t")
	open := strings.LastIndex(trimmed, "<- [")
	if open < 0 || !strings.HasSuffix(trimmed, "]") {
		return title, nil
	}
	// The suffix must start at a word boundary so a title that merely contains
	// "<- [" is not mangled.
	if open > 0 {
		switch trimmed[open-1] {
		case ' ', '\t':
		default:
			return title, nil
		}
	}
	inner := trimmed[open+len("<- [") : len(trimmed)-1]
	inner = strings.TrimSpace(inner)
	var deps []string
	if inner != "" {
		for _, part := range strings.Split(inner, ",") {
			dep := strings.TrimSpace(part)
			if dep == "" {
				return title, nil
			}
			deps = append(deps, dep)
		}
	}
	if deps != nil {
		deps = uniqueSorted(deps)
	}
	return strings.TrimRight(trimmed[:open], " \t"), deps
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
