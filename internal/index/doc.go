// Package index builds the committed backlog.md view as the pure, deterministic
// function f(details) and parses it back for the fast read path.
//
// The render is byte-stable: stable sort, fixed field and line order, no
// timestamps added at render time. reindex runs inside the store lock.
package index
