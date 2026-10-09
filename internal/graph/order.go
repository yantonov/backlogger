package graph

import "sort"

// Order returns every known ticket ID in a stable topological order: a
// dependency precedes every ticket that depends on it, and independent tickets
// keep the caller's preferred order.
//
// The deps adjacency is A -> B ("A depends on B"), so the blocking direction is
// the reverse edge B -> A. Kahn's algorithm therefore drains the set of tickets
// that block nothing (in-degree zero, i.e. no unmet dependency) and, on each
// pass, picks the ready ticket that comes first in pref. pref is the caller's
// desired tiebreak (for `bl dep order`, the detail order of priority, created,
// id); the sort is deterministic and never depends on map iteration, so the
// output is stable for equal inputs.
//
// An unresolved dependency that names no known ticket is ignored for ordering:
// it is already surfaced as a warning and must not stall the sort. A cycle
// introduced by a race or a manual edit is impossible for the tickets that can
// still be drained; any ticket left in a cycle is appended at the end in pref
// order, so the walk always terminates and never drops a ticket (spec 5.2).
func (g *Graph) Order(pref []string) []string {
	// rank is the preferred position of each id. Ids absent from pref sort last
	// in lexicographic order, which keeps the output deterministic even when the
	// caller's list is incomplete or a dep names an unknown ticket.
	rank := make(map[string]int, len(pref))
	for i, id := range pref {
		rank[id] = i
	}
	less := func(a, b string) bool {
		ra, oka := rank[a]
		rb, okb := rank[b]
		switch {
		case oka && okb:
			return ra < rb
		case oka:
			return true
		case okb:
			return false
		default:
			return a < b
		}
	}

	// Unmet counts each ticket's dependencies that name a known ticket; a known
	// blocker is what keeps the ticket from being ready. dependents is the
	// reverse adjacency used to decrement a blocker's dependents when it drains.
	unmet := make(map[string]int, len(g.ids))
	dependents := make(map[string][]string)
	for _, id := range g.IDs() {
		for _, dep := range g.deps[id] {
			if !g.ids[dep] {
				continue // unresolved: warned elsewhere, never blocks the sort
			}
			unmet[id]++
			dependents[dep] = append(dependents[dep], id)
		}
	}

	ready := make([]string, 0, len(g.ids))
	for id := range g.ids {
		if unmet[id] == 0 {
			ready = append(ready, id)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })

	out := make([]string, 0, len(g.ids))
	emitted := make(map[string]bool, len(g.ids))
	for len(ready) > 0 {
		// Pop the preferred ready ticket: the slice is kept sorted, so it is
		// always the first element.
		id := ready[0]
		ready = ready[1:]
		out = append(out, id)
		emitted[id] = true

		newly := make([]string, 0, len(dependents[id]))
		for _, dependent := range dependents[id] {
			unmet[dependent]--
			if unmet[dependent] == 0 {
				newly = append(newly, dependent)
			}
		}
		if len(newly) == 0 {
			continue
		}
		ready = append(ready, newly...)
		sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
	}

	// Anything not emitted sits in a cycle. Finish it in pref order so a corrupt
	// graph still yields every ticket, deterministically (spec 5.2).
	if len(out) < len(g.ids) {
		left := make([]string, 0, len(g.ids)-len(out))
		for id := range g.ids {
			if !emitted[id] {
				left = append(left, id)
			}
		}
		sort.Slice(left, func(i, j int) bool { return less(left[i], left[j]) })
		out = append(out, left...)
	}
	return out
}
