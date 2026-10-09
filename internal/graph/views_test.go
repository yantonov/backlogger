package graph

import (
	"testing"

	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// TestWarnings_UnresolvedDeps_SortedByTicketThenDep proves unresolved warnings
// are sorted by (ticket, dep).
func TestWarnings_UnresolvedDeps_SortedByTicketThenDep(t *testing.T) {
	metas := []*store.TicketMeta{
		meta("b", model.StatusOpen, 2, "z", "y"),
		meta("a", model.StatusOpen, 2, "y"),
	}
	warnings := Warnings(metas)
	want := []Warning{{ID: "a", Dep: "y"}, {ID: "b", Dep: "y"}, {ID: "b", Dep: "z"}}
	if len(warnings) != len(want) {
		t.Fatalf("warnings = %v", warnings)
	}
	for i := range want {
		if warnings[i] != want[i] {
			t.Fatalf("warning %d = %v, want %v", i, warnings[i], want[i])
		}
	}
}

// TestDeps_UnsortedDuplicateDeps_ReturnsSortedUnique proves the adjacency list
// is deterministic regardless of the order the detail file lists deps (spec
// 5.2/8.2).
func TestDeps_UnsortedDuplicateDeps_ReturnsSortedUnique(t *testing.T) {
	metas := []*store.TicketMeta{
		meta("a", model.StatusOpen, 2, "c", "b", "b", "a2"),
	}
	g := New(metas)
	if got := g.Deps("a"); !equalStrings(got, []string{"a2", "b", "c"}) {
		t.Fatalf("deps = %v, want sorted unique", got)
	}
}
