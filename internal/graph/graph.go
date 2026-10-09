// Package graph computes dependency views and cycle detection over the ticket
// DAG.
//
// An edge A -> B means A depends on B (spec 5.2). The graph is built from the
// authoritative detail files; it is never stored. All adjacency lists are
// sorted and deduplicated so every traversal and every rendered result is
// deterministic.
package graph

import (
	"sort"

	"github.com/yantonov/backlogger/internal/store"
)

// Graph is an adjacency view of the ticket dependency DAG. It holds no state
// beyond the detail files it was built from, so every derived answer is
// recomputed (spec 5.2).
type Graph struct {
	// deps maps a ticket ID to its dependencies (the nodes it points at).
	deps map[string][]string
	ids  map[string]bool
}

// New builds a graph from a set of detail files. Dependencies are copied,
// deduplicated, and sorted; a dep that does not name a known ticket stays in
// the adjacency list (it is surfaced as an unresolved warning, never silently
// dropped).
func New(metas []*store.TicketMeta) *Graph {
	g := &Graph{
		deps: make(map[string][]string, len(metas)),
		ids:  make(map[string]bool, len(metas)),
	}
	for _, m := range metas {
		g.ids[m.ID] = true
	}
	for _, m := range metas {
		if deps := uniqueSorted(m.Ticket.Deps); len(deps) > 0 {
			g.deps[m.ID] = deps
		}
	}
	return g
}

// Has reports whether id names a known ticket.
func (g *Graph) Has(id string) bool { return g.ids[id] }

// Deps returns the sorted, unique dependencies of id. An unknown or
// dependency-free id returns nil.
func (g *Graph) Deps(id string) []string { return g.deps[id] }

// IDs returns every known ticket ID in sorted order. The order is fixed so
// cycle and traversal output does not depend on map iteration.
func (g *Graph) IDs() []string {
	out := make([]string, 0, len(g.ids))
	for id := range g.ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
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
