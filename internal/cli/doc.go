// Package cli implements the command handlers and the human and JSON rendering
// for the bl command surface.
//
// It is the only package that talks to users and the only one that reaches
// down into store, model, index, graph, ids, and lock.
package cli
