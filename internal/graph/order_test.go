package graph

import (
	"sort"
	"testing"

	"github.com/yantonov/backlogger/internal/store"
)

// orderPref returns a preference list of 1..n so a tiebreak is visible.
func orderPref(ids ...string) []string { return ids }

// TestOrder_DependentPreferredFirst_EmitsDepFirst proves A -> B means B is
// emitted before A, even when A sorts first by preference (spec 5.2).
func TestOrder_DependentPreferredFirst_EmitsDepFirst(t *testing.T) {
	// aaa prefers to be first, but it depends on zzz.
	g := New([]*store.TicketMeta{
		meta("aaa", "open", 0, "zzz"),
		meta("zzz", "open", 2),
	})
	got := g.Order(orderPref("aaa", "zzz"))
	if !equalStrings(got, []string{"zzz", "aaa"}) {
		t.Fatalf("order = %v, want [zzz aaa]", got)
	}
}

// TestOrder_IndependentTickets_KeepsPreferenceOrder proves independent tickets
// keep the caller's preferred order (priority, created, id for the CLI), not
// map order.
func TestOrder_IndependentTickets_KeepsPreferenceOrder(t *testing.T) {
	g := New([]*store.TicketMeta{
		meta("bbb", "open", 2),
		meta("aaa", "open", 2),
		meta("ccc", "open", 0),
	})
	got := g.Order(orderPref("ccc", "aaa", "bbb"))
	if !equalStrings(got, []string{"ccc", "aaa", "bbb"}) {
		t.Fatalf("order = %v, want [ccc aaa bbb]", got)
	}
}

// TestOrder_Chain_EmitsInDependencyOrder proves a linear chain drains in
// dependency order.
func TestOrder_Chain_EmitsInDependencyOrder(t *testing.T) {
	g := New([]*store.TicketMeta{
		meta("ccc", "open", 2, "bbb"),
		meta("bbb", "open", 2, "aaa"),
		meta("aaa", "open", 2),
	})
	got := g.Order(orderPref("ccc", "bbb", "aaa"))
	if !equalStrings(got, []string{"aaa", "bbb", "ccc"}) {
		t.Fatalf("order = %v, want [aaa bbb ccc]", got)
	}
}

// TestOrder_Diamond_EmitsEveryDepBeforeItsDependent proves a diamond is emitted
// with both roots before the sink.
func TestOrder_Diamond_EmitsEveryDepBeforeItsDependent(t *testing.T) {
	// d depends on b and c; b and c depend on a.
	g := New([]*store.TicketMeta{
		meta("a", "open", 2),
		meta("b", "open", 2, "a"),
		meta("c", "open", 2, "a"),
		meta("d", "open", 2, "b", "c"),
	})
	got := g.Order(orderPref("a", "b", "c", "d"))
	if !equalStrings(got, []string{"a", "b", "c", "d"}) {
		t.Fatalf("order = %v, want [a b c d]", got)
	}
	if !strictlyPrecedes(got, "a", "b") || !strictlyPrecedes(got, "b", "d") {
		t.Fatalf("diamond order violates a dependency: %v", got)
	}
}

// injectedCycle returns a graph where aaa and bbb depend on each other, plus
// an independent ccc.
func injectedCycle() *Graph {
	return New([]*store.TicketMeta{
		meta("aaa", "open", 2, "bbb"),
		meta("bbb", "open", 2, "aaa"),
		meta("ccc", "open", 2),
	})
}

// TestOrder_InjectedCycle_YieldsEveryTicketExactlyOnce proves a cycle neither
// hangs the walk nor drops or repeats a ticket (spec 5.2 backstop).
func TestOrder_InjectedCycle_YieldsEveryTicketExactlyOnce(t *testing.T) {
	g := injectedCycle()

	got := g.Order(orderPref("aaa", "bbb", "ccc"))

	sorted := append([]string(nil), got...)
	sort.Strings(sorted)
	if !equalStrings(sorted, []string{"aaa", "bbb", "ccc"}) {
		t.Fatalf("order = %v, want each of aaa, bbb, ccc once", got)
	}
}

// TestOrder_InjectedCycle_IsDeterministic proves the cycle fallback gives the
// same order every time.
func TestOrder_InjectedCycle_IsDeterministic(t *testing.T) {
	g := injectedCycle()
	first := g.Order(orderPref("aaa", "bbb", "ccc"))

	second := g.Order(orderPref("aaa", "bbb", "ccc"))

	if !equalStrings(first, second) {
		t.Fatalf("order not deterministic: %v vs %v", first, second)
	}
}

// TestOrder_UnresolvedDep_StillEmitsTicket proves an unknown dep neither blocks
// nor stalls the sort; it is warned elsewhere (spec 5.2).
func TestOrder_UnresolvedDep_StillEmitsTicket(t *testing.T) {
	g := New([]*store.TicketMeta{
		meta("aaa", "open", 2, "ghost"),
	})
	got := g.Order(orderPref("aaa"))
	if !equalStrings(got, []string{"aaa"}) {
		t.Fatalf("order = %v, want [aaa]", got)
	}
}

func strictlyPrecedes(order []string, x, y string) bool {
	for _, id := range order {
		switch id {
		case x:
			return true
		case y:
			return false
		}
	}
	return false
}
