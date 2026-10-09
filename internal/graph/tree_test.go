package graph

import (
	"testing"

	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

type treeStep struct {
	id    string
	depth int
	note  string
}

func steps(lines []TreeLine) []treeStep {
	out := make([]treeStep, 0, len(lines))
	for _, l := range lines {
		out = append(out, treeStep{id: l.ID, depth: l.Depth, note: l.Note})
	}
	return out
}

// TestTree_Diamond_PrintsSharedDepOnceThenSeen proves a shared dependency is
// printed once with a "(seen)" note and is not re-expanded, and that the walk
// terminates on a diamond (spec 5.2).
func TestTree_Diamond_PrintsSharedDepOnceThenSeen(t *testing.T) {
	metas := []*store.TicketMeta{
		meta("a", model.StatusOpen, 2, "b", "c"),
		meta("b", model.StatusOpen, 2, "d"),
		meta("c", model.StatusOpen, 2, "d"),
		meta("d", model.StatusOpen, 2),
	}
	lines := New(metas).Tree("a", false)
	got := steps(lines)
	want := []treeStep{
		{"a", 0, ""},
		{"b", 1, ""},
		{"d", 2, ""},
		{"c", 1, ""},
		{"d", 2, "(seen)"},
	}
	if len(got) != len(want) {
		t.Fatalf("tree = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %v, want %v (tree %v)", i, got[i], want[i], got)
		}
	}
}

func assertTree(t *testing.T, lines []TreeLine, want []treeStep) {
	t.Helper()
	got := steps(lines)
	if len(got) != len(want) {
		t.Fatalf("tree = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %v, want %v (tree %v)", i, got[i], want[i], got)
		}
	}
}

// TestTreeFull_Diamond_ReExpandsSharedDep proves --full expands a shared
// dependency under every parent instead of marking it "(seen)" (spec 5.2).
func TestTreeFull_Diamond_ReExpandsSharedDep(t *testing.T) {
	metas := []*store.TicketMeta{
		meta("a", model.StatusOpen, 2, "b", "c"),
		meta("b", model.StatusOpen, 2, "d"),
		meta("c", model.StatusOpen, 2, "d"),
		meta("d", model.StatusOpen, 2, "e"),
		meta("e", model.StatusOpen, 2),
	}

	lines := New(metas).Tree("a", true)

	assertTree(t, lines, []treeStep{
		{"a", 0, ""},
		{"b", 1, ""},
		{"d", 2, ""},
		{"e", 3, ""},
		{"c", 1, ""},
		{"d", 2, ""},
		{"e", 3, ""},
	})
}

// TestTreeFull_InjectedCycle_StopsAtCycle proves --full still stops at a node
// already on the current path and marks it "(cycle)" (spec 5.2).
func TestTreeFull_InjectedCycle_StopsAtCycle(t *testing.T) {
	metas := []*store.TicketMeta{
		meta("a", model.StatusOpen, 2, "b"),
		meta("b", model.StatusOpen, 2, "c"),
		meta("c", model.StatusOpen, 2, "a"),
	}

	lines := New(metas).Tree("a", true)

	assertTree(t, lines, []treeStep{
		{"a", 0, ""},
		{"b", 1, ""},
		{"c", 2, ""},
		{"a", 3, "(cycle)"},
	})
}

// TestTree_InjectedCycle_TerminatesAndMarksCycle proves a corrupt cyclic graph
// does not make the tree hang or recurse forever, and that the cycle is marked
// (spec 5.2).
func TestTree_InjectedCycle_TerminatesAndMarksCycle(t *testing.T) {
	metas := []*store.TicketMeta{
		meta("a", model.StatusOpen, 2, "b"),
		meta("b", model.StatusOpen, 2, "c"),
		meta("c", model.StatusOpen, 2, "a"),
	}
	done := make(chan []TreeLine, 1)
	go func() { done <- New(metas).Tree("a", false) }()
	lines := <-done

	if len(lines) == 0 {
		t.Fatal("empty tree")
	}
	sawCycle := false
	for _, l := range lines {
		if l.Note == "(cycle)" {
			sawCycle = true
		}
	}
	if !sawCycle {
		t.Fatalf("cycle not marked: %v", lines)
	}
}

// TestTree_UnknownDep_MarksItUnknown proves an unresolved dependency is marked
// rather than silently dropped (spec 5.2).
func TestTree_UnknownDep_MarksItUnknown(t *testing.T) {
	metas := []*store.TicketMeta{meta("a", model.StatusOpen, 2, "ghost")}
	lines := New(metas).Tree("a", false)
	if len(lines) != 2 {
		t.Fatalf("tree = %v", lines)
	}
	if !lines[1].Unknown || lines[1].ID != "ghost" {
		t.Fatalf("unknown dep not marked: %+v", lines[1])
	}
}
