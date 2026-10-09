package graph

// TreeLine is one line of a dependency tree.
type TreeLine struct {
	// ID is the ticket at this line.
	ID string
	// Depth is the indentation level; the root is 0.
	Depth int
	// Note is "(cycle)" for a node already on the current path and "(seen)"
	// for a node already emitted when full is false. It is empty otherwise.
	Note string
	// Unknown reports whether the id does not name a known ticket.
	Unknown bool
}

// Tree walks the dependency subtree rooted at root, following A -> deps.
//
// A visited set and an on-path set are always maintained, so the walk
// terminates even when the graph is corrupt (a cycle). With full=false an
// already-visited node is emitted once with the "(seen)" note and not expanded,
// which keeps a shared dependency from being printed repeatedly. With
// full=true every node is expanded and only the on-path set stops a cycle
// (spec 5.2).
//
// The root is always emitted, even when it is unknown.
func (g *Graph) Tree(root string, full bool) []TreeLine {
	var out []TreeLine
	visited := make(map[string]bool)
	onPath := make(map[string]bool)

	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		if onPath[id] {
			out = append(out, TreeLine{ID: id, Depth: depth, Note: "(cycle)"})
			return
		}
		if visited[id] && !full {
			out = append(out, TreeLine{ID: id, Depth: depth, Note: "(seen)"})
			return
		}
		out = append(out, TreeLine{ID: id, Depth: depth, Unknown: !g.Has(id)})
		visited[id] = true
		onPath[id] = true
		for _, dep := range g.Deps(id) {
			walk(dep, depth+1)
		}
		delete(onPath, id)
	}

	walk(root, 0)
	return out
}
