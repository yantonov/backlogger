package graph

import (
	"math/rand"
	"testing"

	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// TestWouldCycle_RandomDepAdds_RefusesOnlyCyclesAndKeepsGraphAcyclic is the DAG
// property test (spec 10). It applies a random sequence of dep adds; the
// in-lock refusal logic is simulated with WouldCycle. Whenever an edge is
// refused the accepted graph must be unchanged, and the final accepted graph
// must be acyclic. A cycle the check wrongly accepted would make FindCycle
// report it.
func TestWouldCycle_RandomDepAdds_RefusesOnlyCyclesAndKeepsGraphAcyclic(t *testing.T) {
	const (
		nodes = 7
		steps = 400
	)
	idsAll := []string{"a", "b", "c", "d", "e", "f", "g"}

	edgesByNode := make(map[string][]string, nodes)
	for _, id := range idsAll {
		edgesByNode[id] = nil
	}
	contains := func(deps []string, dep string) bool {
		for _, d := range deps {
			if d == dep {
				return true
			}
		}
		return false
	}
	build := func() []*store.TicketMeta {
		metas := make([]*store.TicketMeta, 0, nodes)
		for _, id := range idsAll {
			metas = append(metas, meta(id, model.StatusOpen, 2, edgesByNode[id]...))
		}
		return metas
	}

	rng := rand.New(rand.NewSource(20261008))
	refusals := 0
	for i := 0; i < steps; i++ {
		a := idsAll[rng.Intn(nodes)]
		b := idsAll[rng.Intn(nodes)]
		g := New(build())
		if g.WouldCycle(a, b) != nil {
			refusals++
			if a != b && g.findPath(b, a) == nil {
				t.Fatalf("refused safe edge %s -> %s", a, b)
			}
			continue
		}
		if !contains(edgesByNode[a], b) {
			edgesByNode[a] = append(edgesByNode[a], b)
		}
	}

	if refusals == 0 {
		t.Fatal("property test never exercised a refusal; check the generator")
	}
	final := New(build())
	if cycle := final.FindCycle(); cycle != nil {
		t.Fatalf("accepted graph contains a cycle: %s", FormatCycle(cycle))
	}
}
