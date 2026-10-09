package graph

import "strings"

// WouldCycle reports whether adding the edge a -> b (a depends on b) would
// close a cycle in the existing graph. It returns the cycle path starting and
// ending at a (for example [a, b, a]) or nil when the edge is safe.
//
// A self-loop (a == b) is always a cycle: [a, a]. Otherwise the new edge closes
// a cycle exactly when b can already reach a, so the check is a single
// reachability search from b (spec 5.2). It is O(V+E) and only runs on
// `dep add`, inside the lock.
func (g *Graph) WouldCycle(a, b string) []string {
	if a == b {
		return []string{a, a}
	}
	path := g.findPath(b, a)
	if path == nil {
		return nil
	}
	out := make([]string, 0, len(path)+1)
	out = append(out, a)
	out = append(out, path...)
	return out
}

// FormatCycle renders a cycle path as "a -> b -> a". It is the exact text a
// refused `dep add` reports (spec 5.2).
func FormatCycle(path []string) string {
	return strings.Join(path, " -> ")
}

// FindCycle returns one dependency cycle in the graph, or nil when the graph is
// acyclic. The result starts and ends at the same node, so FormatCycle renders
// a closed loop.
//
// This is the backstop for a cycle that entered through a race or a manual file
// edit; the graph is an invariant DAG on the happy path. It uses the standard
// three-colour DFS, so it terminates on any input, corrupt or not (spec 5.2).
func (g *Graph) FindCycle() []string {
	const (
		white = iota // unvisited
		gray         // on the current path
		black        // fully explored
	)
	color := make(map[string]int, len(g.ids))
	stack := make([]string, 0, len(g.ids))
	var cycle []string

	var visit func(id string) bool
	visit = func(id string) bool {
		color[id] = gray
		stack = append(stack, id)
		for _, dep := range g.Deps(id) {
			switch color[dep] {
			case white:
				if visit(dep) {
					return true
				}
			case gray:
				// A back edge to a node on the current path closes a cycle.
				cycle = append([]string(nil), stack[stackIndex(stack, dep):]...)
				cycle = append(cycle, dep)
				return true
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}

	for _, id := range g.IDs() {
		if color[id] == white && visit(id) {
			return cycle
		}
	}
	return nil
}

// findPath returns a path from -> ... -> to following dependency edges, or nil
// when to is unreachable. The visited set makes it terminate on a cyclic graph,
// so the reachability check can never hang (spec 5.2).
func (g *Graph) findPath(from, to string) []string {
	visited := make(map[string]bool, len(g.ids))
	path := make([]string, 0, len(g.ids))

	var dfs func(id string) bool
	dfs = func(id string) bool {
		if visited[id] {
			return false
		}
		visited[id] = true
		path = append(path, id)
		if id == to {
			return true
		}
		for _, dep := range g.Deps(id) {
			if dfs(dep) {
				return true
			}
		}
		path = path[:len(path)-1]
		return false
	}

	if dfs(from) {
		return append([]string(nil), path...)
	}
	return nil
}

// stackIndex returns the position of id in stack, or 0 when it is absent.
func stackIndex(stack []string, id string) int {
	for i, v := range stack {
		if v == id {
			return i
		}
	}
	return 0
}
