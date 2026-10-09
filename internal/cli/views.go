package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/model"
)

// The recomputed navigation views. None of them is stored: started, blocked, and
// available are derived from the glyphs and dependency edges on the index lines
// every time (spec 5.2). They are index-only: they read backlog.md, preserve the
// committed order, and never open a detail file. Dependency satisfaction is
// decidable from the index alone because an archived ticket is definitionally
// closed and its index line already carries the `x` glyph (spec 3.1/5.2).

// navIndex is the parsed index plus the known-ID set the dependency views need.
type navIndex struct {
	entries []index.Entry
	// known is every ID in every section (active, closed, archived). A dep that
	// is absent from it is unresolved and therefore unsatisfied (spec 5.2).
	known map[string]bool
	// satisfied reports whether an ID's glyph is `x` (closed/archived).
	satisfied map[string]bool
}

// loadNavIndex parses backlog.md, rebuilding it first when absent. A missing
// .tickets/ is an error with guidance. It never reads a detail file: archived
// IDs are added from the archive directory by name only, so a dep on an
// archived ticket still counts as satisfied (spec 3.1/5.2).
func (a *App) loadNavIndex() ([]index.Entry, *navIndex, error) {
	paths, err := a.discover()
	if err != nil {
		return nil, nil, err
	}
	if err := a.requireTickets(paths); err != nil {
		return nil, nil, err
	}
	if err := a.ensureIndex(paths); err != nil {
		return nil, nil, err
	}
	entries, err := index.ParseIndex(paths)
	if err != nil {
		return nil, nil, err
	}
	nav := &navIndex{
		entries:   entries,
		known:     make(map[string]bool, len(entries)),
		satisfied: make(map[string]bool, len(entries)),
	}
	for _, e := range entries {
		nav.known[e.ID] = true
		nav.satisfied[e.ID] = e.Glyph == "x"
	}
	// Archived tickets are not in the index; they are definitionally closed, so
	// every archived ID is known and satisfied. Listing them reads names only.
	archivedIDs, err := paths.ListArchiveIDs()
	if err != nil {
		return nil, nil, err
	}
	for _, id := range archivedIDs {
		nav.known[id] = true
		nav.satisfied[id] = true
	}
	return entries, nav, nil
}

// warnings returns the unresolved dependency references across every index
// section, sorted by (ticket, dep) so the output is deterministic. It mirrors
// graph.Warnings but works from parsed entries, so the view path never reads a
// detail file (spec 5.2).
func (n *navIndex) warnings() []indexWarning {
	var out []indexWarning
	for _, e := range n.entries {
		for _, dep := range e.Deps {
			if !n.known[dep] {
				out = append(out, indexWarning{ID: e.ID, Dep: dep})
			}
		}
	}
	sortWarnings(out)
	return out
}

type indexWarning struct {
	ID  string
	Dep string
}

func (w indexWarning) String() string {
	return fmt.Sprintf("%s: unresolved dependency %q", w.ID, w.Dep)
}

func sortWarnings(out []indexWarning) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Dep < out[j].Dep
	})
}

// allSatisfied reports whether every dep of e is satisfied. A dep missing from
// the known set is not satisfied, so a broken reference blocks (spec 5.2).
func (n *navIndex) allSatisfied(e index.Entry) bool {
	for _, dep := range e.Deps {
		if !n.satisfied[dep] {
			return false
		}
	}
	return true
}

// cmdStarted implements `bl started`: the claimed tickets, i.e. every ticket
// whose status is in_progress (spec 5.2). A claim is independent of the
// dependency graph, so the view does not apply the deps-satisfied filter.
func (a *App) cmdStarted(args []string) int {
	return a.cmdNavView(args, "started")
}

// cmdBlocked implements `bl blocked`: active tickets with at least one
// dependency still open or in progress (spec 5.2).
func (a *App) cmdBlocked(args []string) int {
	return a.cmdNavView(args, "blocked")
}

// cmdAvailable implements `bl available`: open tickets whose dependencies are
// satisfied, which excludes anything already taken (spec 5.2).
func (a *App) cmdAvailable(args []string) int {
	return a.cmdNavView(args, "available")
}

// cmdNavView is the shared handler for started/blocked/available. It parses the
// index, applies the view in file order, and renders each selected entry like
// `list` does. Beyond --json it takes the -v/-vv verbosity flags (spec 7).
func (a *App) cmdNavView(args []string, name string) int {
	fs := newFlagSet()
	jsonFlag := fs.Bool("--json")
	vFlag := fs.Bool("-v")
	vvFlag := fs.Bool("-vv")
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	if extra := fs.positionals(); len(extra) > 0 {
		return a.usageError(fmt.Sprintf("%s takes no positional arguments: %q", name, strings.Join(extra, " ")))
	}

	_, nav, err := a.loadNavIndex()
	if err != nil {
		return a.fail(err)
	}
	selected := viewEntries(nav, name)

	if *jsonFlag {
		code := a.renderNavJSON(name, nil, selected)
		printIndexWarnings(a.Stderr, nav.warnings())
		return code
	}
	level := navVerbosity(*vFlag, *vvFlag)
	for _, e := range selected {
		fmt.Fprintln(a.Stdout, renderNavLine(e, level))
	}
	printIndexWarnings(a.Stderr, nav.warnings())
	return exitOK
}

// viewEntries selects the entries for one named view, preserving file order.
// A view never re-sorts (spec 3.1/7.1).
func viewEntries(nav *navIndex, name string) []index.Entry {
	out := make([]index.Entry, 0, len(nav.entries))
	for _, e := range nav.entries {
		status := index.StatusOfGlyph(e.Glyph)
		archived := e.ArchivedAt != ""
		switch name {
		case "started":
			// in_progress only; a claim is independent of deps.
			if status == model.StatusInProgress && !archived {
				out = append(out, e)
			}
		case "blocked":
			if isActiveStatus(status) && !archived && len(e.Deps) > 0 && !nav.allSatisfied(e) {
				out = append(out, e)
			}
		case "available":
			if status == model.StatusOpen && nav.allSatisfied(e) {
				out = append(out, e)
			}
		}
	}
	return out
}

// cmdClosed implements `bl closed`: the Closed section (non-archived closed) in
// committed index order (spec 5.2/7). It is index-only and takes no --limit.
func (a *App) cmdClosed(args []string) int {
	fs := newFlagSet()
	jsonFlag := fs.Bool("--json")
	vFlag := fs.Bool("-v")
	vvFlag := fs.Bool("-vv")
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	if extra := fs.positionals(); len(extra) > 0 {
		return a.usageError(fmt.Sprintf("closed takes no positional arguments: %q", strings.Join(extra, " ")))
	}

	_, nav, err := a.loadNavIndex()
	if err != nil {
		return a.fail(err)
	}
	selected := closedEntries(nav)

	if *jsonFlag {
		return a.renderNavJSON("closed", nil, selected)
	}
	level := navVerbosity(*vFlag, *vvFlag)
	for _, e := range selected {
		fmt.Fprintln(a.Stdout, renderNavLine(e, level))
	}
	return exitOK
}

// closedEntries returns the non-archived closed entries in file order. An
// archived entry is closed but belongs to the Archived section, so it is not
// part of this view (spec 5.4/7).
func closedEntries(nav *navIndex) []index.Entry {
	out := make([]index.Entry, 0, len(nav.entries))
	for _, e := range nav.entries {
		if e.ArchivedAt != "" {
			continue
		}
		if index.StatusOfGlyph(e.Glyph) == model.StatusClosed {
			out = append(out, e)
		}
	}
	return out
}

// navVerbosity maps the -v/-vv flags to a render level: 0 = bare id,
// 1 = id + title, 2 = the canonical index line. -vv wins when both are given,
// because it is the more verbose request (spec 7).
func navVerbosity(v, vv bool) int {
	switch {
	case vv:
		return 2
	case v:
		return 1
	default:
		return 0
	}
}

// renderNavLine renders one selected entry at the requested verbosity. Level 0
// is the bare filename stem so the line can be piped straight to another
// command (operands resolve as bare IDs, never `#id`); level 1 adds the title;
// level 2 is the canonical index line (spec 3.1/7).
func renderNavLine(e index.Entry, level int) string {
	switch level {
	case 0:
		return e.ID
	case 1:
		return e.ID + " " + e.Title
	default:
		return index.RenderEntry(e)
	}
}

func printIndexWarnings(w io.Writer, warnings []indexWarning) {
	for _, warn := range warnings {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}
}
