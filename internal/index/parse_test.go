package index_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/model"
)

// TestParse_GoldenIndex_ReturnsEveryEntry parses the golden backlog.md and
// checks every entry's id, glyph, title, and deps. Parsing is tolerant of
// headings, the `---` separator, and blank lines.
func TestParse_GoldenIndex_ReturnsEveryEntry(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fixtureRoot(t), "backlog.md"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	entries := index.Parse(data)

	want := []index.Entry{
		{ID: "design", Glyph: " ", Title: "Design the API"},
		{ID: "login", Glyph: ">", Title: "Add login flow", Deps: []string{"build", "design"}},
		{ID: "build", Glyph: " ", Title: "Set up CI"},
		{ID: "deploy", Glyph: "x", Title: "Deploy to prod"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("parse mismatch.\ngot:  %#v\nwant: %#v", entries, want)
	}
}

// TestParse_RenderedDetails_RecoversIDGlyphTitleAndDeps proves
// parse(render(details)) recovers id, glyph, title, and deps for every ticket
// (spec 10).
func TestParse_RenderedDetails_RecoversIDGlyphTitleAndDeps(t *testing.T) {
	details := []*index.Detail{
		detail("design", model.StatusOpen, 0, "2026-10-08T08:00:00Z", "Design the API"),
		detail("login", model.StatusInProgress, 1, "2026-10-08T09:00:00Z", "Add login flow", "design", "build"),
		detail("build", model.StatusOpen, 2, "2026-10-08T07:00:00Z", "Set up CI"),
		detail("deploy", model.StatusClosed, 2, "2026-10-07T00:00:00Z", "Deploy to prod"),
	}

	entries := index.Parse(index.Render(details))
	byID := map[string]index.Entry{}
	for _, e := range entries {
		byID[e.ID] = e
	}

	for _, d := range details {
		got, ok := byID[d.ID]
		if !ok {
			t.Fatalf("id %s missing after round-trip", d.ID)
		}
		if got.Title != d.Title {
			t.Errorf("%s title %q, want %q", d.ID, got.Title, d.Title)
		}
		wantGlyph := map[model.Status]string{
			model.StatusOpen:       " ",
			model.StatusInProgress: ">",
			model.StatusClosed:     "x",
		}[d.Status]
		if got.Glyph != wantGlyph {
			t.Errorf("%s glyph %q, want %q", d.ID, got.Glyph, wantGlyph)
		}
		wantDeps := append([]string(nil), d.Deps...)
		if len(wantDeps) > 0 {
			sort.Strings(wantDeps)
		} else {
			wantDeps = nil
		}
		if !reflect.DeepEqual(got.Deps, wantDeps) {
			t.Errorf("%s deps %v, want %v", d.ID, got.Deps, wantDeps)
		}
	}
}

// TestParse_HandEditedIndex_RecoversTitlesAndSkipsLinesWithoutID proves a
// hand-edited title is recovered and that a checkbox line without a trailing
// #id is skipped rather than treated as a ticket.
func TestParse_HandEditedIndex_RecoversTitlesAndSkipsLinesWithoutID(t *testing.T) {
	data := []byte("## Active\n" +
		"- [ ] A hand edited title   #login\n" +
		"- [ ] missing id token\n" +
		"- [>] tabbed title #tabbed\n" +
		"random prose\n")

	entries := index.Parse(data)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %#v", len(entries), entries)
	}
	if entries[0].ID != "login" || entries[0].Title != "A hand edited title" {
		t.Fatalf("title/id not recovered: %#v", entries[0])
	}
	if entries[1].ID != "tabbed" || entries[1].Title != "tabbed title" {
		t.Fatalf("second line not parsed: %#v", entries[1])
	}
}

// TestParse_StaleArchivedSection_KeepsEntriesWithoutArchivedMarker proves a
// stale `## Archived` section left in an old index is tolerated: its lines
// parse as ordinary closed entries, and no archived marker is set (archived
// tickets are not part of the index).
func TestParse_StaleArchivedSection_KeepsEntriesWithoutArchivedMarker(t *testing.T) {
	data := []byte("## Active\n" +
		"- [ ] Live #live\n" +
		"## Closed\n" +
		"- [x] Done #done\n" +
		"---\n" +
		"## Archived\n" +
		"- [x] Old #old\n")
	entries := index.Parse(data)
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3: %#v", len(entries), entries)
	}
	for _, e := range entries {
		if e.ArchivedAt != "" {
			t.Fatalf("parse set an archived marker: %#v", e)
		}
	}
}

// TestRenderEntries_UnsortedEntries_KeepsFileOrder proves the navigation-only
// render path keeps file order within a section and does not re-sort by
// priority (spec 3.1/7.1).
func TestRenderEntries_UnsortedEntries_KeepsFileOrder(t *testing.T) {
	entries := []index.Entry{
		{ID: "low", Glyph: " ", Title: "Low"},
		{ID: "high", Glyph: " ", Title: "High"},
		{ID: "mid", Glyph: " ", Title: "Mid"},
	}
	got := index.Parse(index.RenderEntries(entries))
	var ids []string
	for _, e := range got {
		ids = append(ids, e.ID)
	}
	if !reflect.DeepEqual(ids, []string{"low", "high", "mid"}) {
		t.Fatalf("render reordered entries: %v", ids)
	}
}

// TestParseIndex_MissingFile_ReturnsNoEntriesAndNoError proves a missing index
// is not an error on the fast read path: the caller falls back to a reindex.
func TestParseIndex_MissingFile_ReturnsNoEntriesAndNoError(t *testing.T) {
	paths := newTempPaths(t)

	entries, err := index.ParseIndex(paths)

	if err != nil {
		t.Fatalf("parse missing index: %v", err)
	}
	if entries != nil {
		t.Fatalf("missing index yielded %#v, want nil", entries)
	}
}

// TestParse_EmptyBody_ReturnsNoEntries proves scanning an empty index body
// yields no entries.
func TestParse_EmptyBody_ReturnsNoEntries(t *testing.T) {
	entries := index.Parse([]byte{})

	if len(entries) != 0 {
		t.Fatalf("empty body yielded %#v, want no entries", entries)
	}
}
