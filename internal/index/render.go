package index

import (
	"sort"
	"strings"

	"github.com/yantonov/backlogger/internal/model"
)

// Group describes which section of backlog.md a ticket belongs to. It is
// derived from the stored fields, never from the file's location, so f remains
// a pure function of the detail contents (spec 3.1/5.4).
type Group int

// The two index sections, in render order. Archived tickets are not part of
// the index (spec 3.1): archive is detail/directory state, read on demand.
const (
	GroupActive Group = iota
	GroupClosed
)

// sections is the fixed section order. It is a slice, not a map, so rendering
// cannot accidentally depend on map iteration order.
var sections = []struct {
	group Group
	head  string
}{
	{GroupActive, "## Active"},
	{GroupClosed, "## Closed"},
}

// Entry is one parsed or to-be-rendered index line. The title is everything
// before the trailing #id token and carries no trailing whitespace.
type Entry struct {
	ID    string
	Glyph string
	Title string
	Deps  []string
	// ArchivedAt is set to a non-empty marker on an entry the CLI built from the
	// archive directory. Index parsing never sets it: archived tickets are not in
	// the index. It is a marker, never a real timestamp.
	ArchivedAt string
}

// SortEntries orders entries by (priority, created, id). It is used only when
// rendering a detail list; the input slice is not modified.
func SortEntries(details []*Detail) []*Detail {
	out := make([]*Detail, len(details))
	copy(out, details)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if a.Created != b.Created {
			return a.Created < b.Created
		}
		return a.ID < b.ID
	})
	return out
}

// Render is the pure index function f(details). It groups the details into the
// Active/Closed sections, sorts each section by (priority, created, id), and
// emits one deterministic line per ticket. Archived details are skipped: the
// index carries only active and closed tickets (spec 3.1). The same inputs
// always produce the same bytes: the sort is stable, the section order is a
// fixed slice, dependency lists are sorted, and no timestamp is ever added at
// render time (spec 3.1/8.1).
func Render(details []*Detail) []byte {
	// A fixed slice indexed by Group: grouping never depends on map iteration
	// order, and the render visits sections in the explicit order below.
	groups := make([][]*Detail, len(sections))
	for _, d := range details {
		if d.ArchivedAt != "" {
			continue
		}
		g := groupOf(d)
		groups[g] = append(groups[g], d)
	}

	var b strings.Builder
	for i, sec := range sections {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(sec.head)
		b.WriteByte('\n')
		for _, d := range SortEntries(groups[sec.group]) {
			b.WriteString(renderEntry(d))
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

// RenderEntries renders a set of already-parsed index entries back to
// backlog.md bytes. It is the navigation-only render path: it never reads a
// detail file and never re-sorts, so file order is preserved. Archived entries
// (ArchivedAt set) are skipped; the deps suffix is re-emitted sorted and unique
// so the bytes stay canonical (spec 3.1/8.1).
//
// It is exercised by the index tests; the CLI renders one entry at a time with
// RenderEntry so it can interleave selection with output.
func RenderEntries(entries []Entry) []byte {
	// A fixed slice indexed by Group: grouping never depends on map iteration
	// order, and the render visits sections in the explicit order below.
	groups := make([][]Entry, len(sections))
	for _, e := range entries {
		if e.ArchivedAt != "" {
			continue
		}
		g := groupOfEntry(e)
		groups[g] = append(groups[g], e)
	}

	var b strings.Builder
	for i, sec := range sections {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(sec.head)
		b.WriteByte('\n')
		for _, e := range groups[sec.group] {
			b.WriteString(renderParsedEntry(e))
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

// groupOf maps a detail to its section. Archive membership is the stored
// `archived` field, not the directory, so a hand-moved file is corrected on the
// next reindex and flagged by doctor (spec 5.4). An archived detail is not
// rendered at all (Render skips it); this is only reached for non-archived ones.
func groupOf(d *Detail) Group {
	if d.Status == model.StatusClosed {
		return GroupClosed
	}
	return GroupActive
}

// groupOfEntry maps a parsed entry to its index section from its status glyph.
func groupOfEntry(e Entry) Group {
	if StatusOfGlyph(e.Glyph) == model.StatusClosed {
		return GroupClosed
	}
	return GroupActive
}

// StatusOfGlyph maps an index-line glyph to the model status it encodes: space
// is open, '>' is in progress, 'x' is closed. A caller that reaches this with
// an unknown glyph should treat it as open; a parsed entry always carries one
// of the three glyphs the grammar accepts (spec 8.1).
func StatusOfGlyph(glyph string) model.Status {
	switch glyph {
	case ">":
		return model.StatusInProgress
	case "x":
		return model.StatusClosed
	default:
		return model.StatusOpen
	}
}

// renderEntry renders one index line from an index detail:
//
//   - [ ] <title> #<id> <- [dep1, dep2]
//
// The #id token is always present as the cached pointer to the detail file. The
// `<- [deps]` suffix appears only when the ticket has dependencies and lists
// them in sorted order (spec 3.1/5.2).
func renderEntry(d *Detail) string {
	return renderParsedEntry(Entry{
		ID:    d.ID,
		Glyph: GlyphFor(d.Status),
		Title: d.Title,
		Deps:  d.Deps,
	})
}

func renderParsedEntry(e Entry) string {
	var b strings.Builder
	b.WriteString("- [")
	b.WriteString(e.Glyph)
	b.WriteString("] ")
	b.WriteString(e.Title)
	b.WriteString(" #")
	b.WriteString(e.ID)

	deps := append([]string(nil), e.Deps...)
	sort.Strings(deps)
	if len(deps) > 0 {
		b.WriteString(" <- [")
		for i, dep := range deps {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(dep)
		}
		b.WriteByte(']')
	}
	return b.String()
}

// RenderEntry renders one index line for e in canonical form. It is the
// navigation-only line renderer: it never reads a detail file (spec 3.1/7).
func RenderEntry(e Entry) string { return renderParsedEntry(e) }

// GlyphFor maps a status to its checkbox glyph: [ ] todo, [>] in progress,
// [x] done (spec 3.1). It is exported for the graph commands that build an
// index entry from a detail file (dep order).
func GlyphFor(s model.Status) string {
	switch s {
	case model.StatusInProgress:
		return ">"
	case model.StatusClosed:
		return "x"
	default:
		return " "
	}
}

func firstHeading(body []byte) string {
	for _, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			continue
		}
		text := strings.TrimSpace(strings.TrimLeft(line, "#"))
		text = strings.TrimSpace(strings.Trim(text, "#"))
		if text != "" {
			return text
		}
	}
	return ""
}

// RenderString is Render as a string, for callers that want the whole index
// body as text.
func RenderString(details []*Detail) string {
	return string(Render(details))
}

// ArchivedMarker is the non-empty placeholder an Entry carries when the CLI
// built it from the archive directory. Archived tickets are not in the index,
// so parsing never sets it; it is deliberately not a timestamp.
const ArchivedMarker = "archived"
