// Package graph computes dependency traversal and cycle detection over the
// ticket DAG.
//
// It provides dependency-tree traversal with a visited set, the dep cycle
// backstop, and unresolved-dependency warnings. The recomputed navigation views
// (started, blocked, available, closed) are index-only reads in internal/cli and
// are not part of this package (spec 3.1/5.2).
package graph
