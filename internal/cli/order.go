package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yantonov/backlogger/internal/graph"
	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/store"
)

// cmdDepOrder implements `bl dep order`: a stable topological order of the
// active tickets, then the ticket sort (priority, created, id) as the tiebreak.
// It is a graph command, not a navigation view: it reads the authoritative
// detail files, computes a new order, and never writes anything (spec 5.2/7).
//
// Unlike the views, which read the already-sorted index and never re-sort, this
// command must reorder: a low-priority blocker has to come before the ticket it
// unblocks. It therefore reads details — like dep tree / dep cycle — and is not
// bound by the "views never re-sort" rule.
//
// Only active tickets (open or in progress) are listed. A closed ticket imposes
// no constraint on the work that remains, and an archived ticket is closed by
// definition, so neither belongs in an ordering of what is left to do.
func (a *App) cmdDepOrder(args []string) int {
	fs := newFlagSet()
	jsonFlag := fs.Bool("--json")
	vFlag := fs.Bool("-v")
	vvFlag := fs.Bool("-vv")
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	if extra := fs.positionals(); len(extra) > 0 {
		return a.usageError(fmt.Sprintf("dep order takes no positional arguments: %q", strings.Join(extra, " ")))
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}

	// Every non-archived detail is read (closed tickets live beside the active
	// ones), so a dep on a closed ticket stays known and is never mis-reported
	// as unresolved; the closed nodes are then filtered from the output. Archived
	// tickets are known by name only: an archived ticket is definitionally
	// closed, so the archive directory is never opened (spec 5.4).
	metas, err := paths.ListActive()
	if err != nil {
		return a.fail(err)
	}
	archivedIDs, err := paths.ListArchiveIDs()
	if err != nil {
		return a.fail(err)
	}

	// Best-first is the ticket sort (priority, created, id). The graph returns
	// the same ids in topological order, breaking ties by that preferred order.
	pref := preferredOrder(metas)
	g := graph.New(metas)
	order := g.Order(pref)

	byID := make(map[string]*store.TicketMeta, len(metas))
	for _, meta := range metas {
		byID[meta.ID] = meta
	}

	// Drop the closed nodes the graph needed as blockers: the view is the work
	// that remains, in the order to do it.
	selected := make([]string, 0, len(order))
	for _, id := range order {
		if byID[id].Ticket.IsActive() {
			selected = append(selected, id)
		}
	}

	warnings := orderWarnings(metas, byID, archivedIDs)
	if *jsonFlag {
		code := a.renderDepOrderJSON(selected, byID)
		printWarnings(a.Stderr, warnings)
		return code
	}
	level := navVerbosity(*vFlag, *vvFlag)
	for _, id := range selected {
		fmt.Fprintln(a.Stdout, renderNavLine(entryFor(byID[id]), level))
	}
	printWarnings(a.Stderr, warnings)
	return exitOK
}

// preferredOrder is the dep-order tiebreak: every detail sorted by
// (priority, created, id), the same order `f` uses for the index. It is the
// caller's preference passed to graph.Order; the graph reorders only where the
// dependency edges require it (spec 3.1).
func preferredOrder(metas []*store.TicketMeta) []string {
	sorted := append([]*store.TicketMeta(nil), metas...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Ticket.Priority != b.Ticket.Priority {
			return a.Ticket.Priority < b.Ticket.Priority
		}
		if a.Ticket.Created != b.Ticket.Created {
			return a.Ticket.Created < b.Ticket.Created
		}
		return a.ID < b.ID
	})
	out := make([]string, 0, len(sorted))
	for _, meta := range sorted {
		out = append(out, meta.ID)
	}
	return out
}

// orderWarnings returns the unresolved dependency warnings for the ordering
// view. Only warnings for tickets the view can list are kept, so a closed
// ticket's broken dep does not add noise to an ordering of active work. A dep
// that names an archived ticket is satisfied by definition and is dropped too:
// archive membership is known by name only and the file is never opened
// (spec 5.2/5.4).
func orderWarnings(metas []*store.TicketMeta, byID map[string]*store.TicketMeta, archivedIDs []string) []graph.Warning {
	archived := make(map[string]bool, len(archivedIDs))
	for _, id := range archivedIDs {
		archived[id] = true
	}
	var out []graph.Warning
	for _, w := range graph.Warnings(metas) {
		if archived[w.Dep] {
			continue
		}
		if meta, ok := byID[w.ID]; !ok || !meta.Ticket.IsActive() {
			continue
		}
		out = append(out, w)
	}
	return out
}

// entryFor renders one detail as the lean index entry the navigation line
// renderer and the JSON view shape expect. It mirrors the fields already on an
// index line, so dep order stays a navigation-style output (spec 7). Only
// active details reach it, so its glyph is the open or in-progress glyph.
func entryFor(meta *store.TicketMeta) index.Entry {
	return index.Entry{
		ID:    meta.ID,
		Glyph: index.GlyphFor(meta.Ticket.Status),
		Title: titleOf(meta),
		Deps:  append([]string(nil), meta.Ticket.Deps...),
	}
}
