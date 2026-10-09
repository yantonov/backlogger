package graph

import (
	"testing"

	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// TestWouldCycle_SelfEdge_ReportsSelfLoop proves a ticket depending on itself
// is always a cycle and is named as a self-loop (spec 5.2).
func TestWouldCycle_SelfEdge_ReportsSelfLoop(t *testing.T) {
	g := New([]*store.TicketMeta{})
	cycle := g.WouldCycle("a", "a")
	if cycle == nil {
		t.Fatal("self-loop was not detected")
	}
	if got, want := FormatCycle(cycle), "a -> a"; got != want {
		t.Fatalf("cycle = %q, want %q", got, want)
	}
}

// TestWouldCycle_BackEdgeOnChain_ReportsFullCycle proves a three-node back edge
// is refused and the reported path is the full cycle (spec 5.2).
func TestWouldCycle_BackEdgeOnChain_ReportsFullCycle(t *testing.T) {
	// a -> b -> c exists; adding c -> a closes a -> b -> c -> a.
	g := New([]*store.TicketMeta{
		meta("a", model.StatusOpen, 2, "b"),
		meta("b", model.StatusOpen, 2, "c"),
		meta("c", model.StatusOpen, 2),
	})

	cycle := g.WouldCycle("c", "a")

	if cycle == nil {
		t.Fatal("back edge was not detected")
	}
	if got, want := FormatCycle(cycle), "c -> a -> b -> c"; got != want {
		t.Fatalf("cycle = %q, want %q", got, want)
	}
}

// TestWouldCycle_EdgeToIndependentNode_IsSafe proves an edge to a node that
// cannot reach the source is allowed (spec 5.2).
func TestWouldCycle_EdgeToIndependentNode_IsSafe(t *testing.T) {
	g := New([]*store.TicketMeta{
		meta("a", model.StatusOpen, 2, "b"),
		meta("b", model.StatusOpen, 2, "c"),
		meta("c", model.StatusOpen, 2),
		meta("d", model.StatusOpen, 2),
	})

	got := g.WouldCycle("c", "d")

	if got != nil {
		t.Fatalf("safe edge refused: %v", got)
	}
}

// diamond returns a -> {b, c} -> d.
func diamond() *Graph {
	return New([]*store.TicketMeta{
		meta("a", model.StatusOpen, 2, "b", "c"),
		meta("b", model.StatusOpen, 2, "d"),
		meta("c", model.StatusOpen, 2, "d"),
		meta("d", model.StatusOpen, 2),
	})
}

// TestWouldCycle_BackEdgeAcrossDiamond_IsDetected proves reachability follows
// a longer path (spec 5.2).
func TestWouldCycle_BackEdgeAcrossDiamond_IsDetected(t *testing.T) {
	g := diamond()

	got := g.WouldCycle("d", "a")

	if got == nil {
		t.Fatal("long back edge not detected")
	}
}

// TestWouldCycle_ExistingDiamondEdge_IsSafe proves re-adding an edge the
// diamond already has is not mistaken for a cycle (spec 5.2).
func TestWouldCycle_ExistingDiamondEdge_IsSafe(t *testing.T) {
	g := diamond()

	got := g.WouldCycle("a", "b")

	if got != nil {
		t.Fatalf("existing edge refused: %v", got)
	}
}

// TestFindCycle_ThreeNodeCycle_ReportsClosedPath proves the backstop reports a
// cycle that is present in the data, not one being added (spec 5.2).
func TestFindCycle_ThreeNodeCycle_ReportsClosedPath(t *testing.T) {
	metas := []*store.TicketMeta{
		meta("a", model.StatusOpen, 2, "b"),
		meta("b", model.StatusOpen, 2, "c"),
		meta("c", model.StatusOpen, 2, "a"),
	}
	g := New(metas)
	cycle := g.FindCycle()
	if cycle == nil {
		t.Fatal("injected cycle not found")
	}
	if cycle[0] != cycle[len(cycle)-1] {
		t.Fatalf("cycle does not close: %v", cycle)
	}
	if len(cycle) != 4 {
		t.Fatalf("cycle = %v, want 3 nodes plus the closing node", cycle)
	}
}

// TestFindCycle_Diamond_ReportsNone proves a shared dependency reached by two
// paths is not mistaken for a cycle.
func TestFindCycle_Diamond_ReportsNone(t *testing.T) {
	metas := []*store.TicketMeta{
		meta("a", model.StatusOpen, 2, "b", "c"),
		meta("b", model.StatusOpen, 2, "d"),
		meta("c", model.StatusOpen, 2, "d"),
		meta("d", model.StatusOpen, 2),
	}

	got := New(metas).FindCycle()

	if got != nil {
		t.Fatalf("acyclic graph reported a cycle: %v", got)
	}
}

// TestFindCycle_DAGWithForwardAndCrossEdges_ReportsNone proves a larger DAG
// reports no cycle: a forward edge (a -> c beside a -> b -> c), and a second
// root e whose edges reach nodes already fully explored from a. A DFS that
// confuses "explored" with "on the current path" reports a false cycle here.
func TestFindCycle_DAGWithForwardAndCrossEdges_ReportsNone(t *testing.T) {
	metas := []*store.TicketMeta{
		meta("a", model.StatusOpen, 2, "b", "c", "d"),
		meta("b", model.StatusOpen, 2, "c"),
		meta("c", model.StatusOpen, 2, "d"),
		meta("d", model.StatusOpen, 2),
		meta("e", model.StatusOpen, 2, "a", "d"),
		meta("f", model.StatusOpen, 2),
	}

	got := New(metas).FindCycle()

	if got != nil {
		t.Fatalf("acyclic graph reported a cycle: %v", got)
	}
}

// TestFindCycle_SelfLoop_ReportsAToA proves a self-loop in the data is found.
func TestFindCycle_SelfLoop_ReportsAToA(t *testing.T) {
	metas := []*store.TicketMeta{meta("a", model.StatusOpen, 2, "a")}
	cycle := New(metas).FindCycle()
	if cycle == nil || FormatCycle(cycle) != "a -> a" {
		t.Fatalf("self-loop = %v, want a -> a", cycle)
	}
}
