package graph

import (
	"fmt"
	"sort"

	"github.com/yantonov/backlogger/internal/store"
)

// Warning reports a dependency that does not resolve to a known ticket. A
// broken dep is surfaced, never silently ignored (spec 5.2).
type Warning struct {
	// ID is the ticket that names the dep.
	ID string
	// Dep is the unresolved dependency ID.
	Dep string
}

// String renders the warning in the form commands print.
func (w Warning) String() string {
	return fmt.Sprintf("%s: unresolved dependency %q", w.ID, w.Dep)
}

// Warnings returns the unresolved dependency references across every ticket,
// sorted by (ticket, dep) so the output is deterministic. The known-ID set is
// built from every meta the caller passes, so a caller that supplies all index
// sections (active, closed, archived) never reports a false unresolved dep.
//
// This package now keeps only the graph primitives and this warnings helper.
// The recomputed navigation views (started, blocked, available, closed) are
// index-only reads and live in internal/cli; they no longer read detail files
// and no longer carry view semantics here (spec 3.1/5.2).
func Warnings(metas []*store.TicketMeta) []Warning {
	known := make(map[string]bool, len(metas))
	for _, m := range metas {
		known[m.ID] = true
	}
	var out []Warning
	for _, m := range metas {
		for _, dep := range uniqueSorted(m.Ticket.Deps) {
			if !known[dep] {
				out = append(out, Warning{ID: m.ID, Dep: dep})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Dep < out[j].Dep
	})
	return out
}
