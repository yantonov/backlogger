package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/yantonov/backlogger/internal/graph"
	"github.com/yantonov/backlogger/internal/ids"
	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/store"
)

func (a *App) cmdDep(args []string) int {
	if len(args) == 0 {
		return a.usageError("dep needs a subcommand: add, rm, tree, cycle, or order")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return a.cmdDepAdd(rest)
	case "rm":
		return a.cmdDepRm(rest)
	case "tree":
		return a.cmdDepTree(rest)
	case "cycle":
		return a.cmdDepCycle(rest)
	case "order":
		return a.cmdDepOrder(rest)
	default:
		return a.usageError(fmt.Sprintf("unknown dep subcommand %q (want add, rm, tree, cycle, or order)", sub))
	}
}

// resolvePair resolves two operands against the same active+archived candidate
// set using partial matching (spec 5.1). Only the file names are listed; an
// archived file is never opened just to resolve its ID.
func resolvePair(paths *store.Paths, first, second string) (string, string, error) {
	candidates, err := paths.CandidateIDs()
	if err != nil {
		return "", "", err
	}
	a, err := ids.Resolve(candidates, first)
	if err != nil {
		return "", "", err
	}
	b, err := ids.Resolve(candidates, second)
	if err != nil {
		return "", "", err
	}
	return a, b, nil
}

// cmdDepAdd implements `bl dep add <id> <dep>`: A depends on B. The cycle check
// and the detail write happen in the same lock, so the graph is an invariant
// DAG and a refused edge writes nothing (spec 5.2).
func (a *App) cmdDepAdd(args []string) int {
	fs := newFlagSet()
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	positionals := fs.positionals()
	if len(positionals) != 2 {
		return a.usageError("dep add needs two ids: bl dep add <id> <dep>")
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}
	aID, bID, err := resolvePair(paths, positionals[0], positionals[1])
	if err != nil {
		return a.fail(err)
	}

	tx, err := store.Begin(paths, store.TxOptions{Reindex: index.ReindexFunc()})
	if err != nil {
		return a.fail(err)
	}
	defer func() { _ = tx.Rollback() }()

	metas, err := tx.ListAll()
	if err != nil {
		return a.fail(err)
	}
	g := graph.New(metas)
	if cycle := g.WouldCycle(aID, bID); cycle != nil {
		return a.fail(fmt.Errorf("would create cycle: %s", graph.FormatCycle(cycle)))
	}

	now := a.now()
	if _, err := tx.Update(aID, nil, func(meta *store.TicketMeta) error {
		meta.Ticket.Deps = addDep(meta.Ticket.Deps, bID)
		return nil
	}, now); err != nil {
		return a.fail(err)
	}
	if err := tx.Commit(); err != nil {
		return a.fail(err)
	}

	fmt.Fprintf(a.Stdout, "dep %s -> %s\n", aID, bID)
	return exitOK
}

// cmdDepRm implements `bl dep rm <id> <dep>`: remove the edge A -> B. Removing a
// dependency can never create a cycle, so no reachability check is needed.
func (a *App) cmdDepRm(args []string) int {
	fs := newFlagSet()
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	positionals := fs.positionals()
	if len(positionals) != 2 {
		return a.usageError("dep rm needs two ids: bl dep rm <id> <dep>")
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}
	aID, bID, err := resolvePair(paths, positionals[0], positionals[1])
	if err != nil {
		return a.fail(err)
	}

	tx, err := store.Begin(paths, store.TxOptions{Reindex: index.ReindexFunc()})
	if err != nil {
		return a.fail(err)
	}
	defer func() { _ = tx.Rollback() }()

	now := a.now()
	if _, err := tx.Update(aID, func(meta *store.TicketMeta) error {
		if !hasDep(meta.Ticket.Deps, bID) {
			return fmt.Errorf("%s does not depend on %s", aID, bID)
		}
		return nil
	}, func(meta *store.TicketMeta) error {
		meta.Ticket.Deps = removeDep(meta.Ticket.Deps, bID)
		return nil
	}, now); err != nil {
		return a.fail(err)
	}
	if err := tx.Commit(); err != nil {
		return a.fail(err)
	}

	fmt.Fprintf(a.Stdout, "dep %s -/-> %s\n", aID, bID)
	return exitOK
}

// cmdDepTree implements `bl dep tree [--full] <id>`. It reads detail files,
// prints the dependency tree, and surfaces unresolved dep IDs as warnings. The
// traversal keeps a visited set, so it terminates even on a corrupt graph
// (spec 5.2).
func (a *App) cmdDepTree(args []string) int {
	fs := newFlagSet()
	fullFlag := fs.Bool("--full")
	jsonFlag := fs.Bool("--json")
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	positionals := fs.positionals()
	if len(positionals) != 1 {
		return a.usageError("dep tree needs exactly one ticket id: bl dep tree [--full] <id>")
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}
	root, err := resolveID(paths, positionals[0])
	if err != nil {
		return a.fail(err)
	}
	metas, err := paths.ListAll()
	if err != nil {
		return a.fail(err)
	}
	g := graph.New(metas)
	lines := g.Tree(root, *fullFlag)

	if *jsonFlag {
		code := a.renderTreeJSON(root, *fullFlag, lines)
		printWarnings(a.Stderr, graph.Warnings(metas))
		return code
	}
	for _, line := range lines {
		fmt.Fprintln(a.Stdout, renderTreeLine(line))
	}
	printWarnings(a.Stderr, graph.Warnings(metas))
	return exitOK
}

// cmdDepCycle implements `bl dep cycle`: the backstop that detects a cycle
// introduced by a race or a manual edit. It never hangs (spec 5.2).
func (a *App) cmdDepCycle(args []string) int {
	fs := newFlagSet()
	jsonFlag := fs.Bool("--json")
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	if extra := fs.positionals(); len(extra) > 0 {
		return a.usageError(fmt.Sprintf("dep cycle takes no positional arguments: %q", strings.Join(extra, " ")))
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}
	metas, err := paths.ListAll()
	if err != nil {
		return a.fail(err)
	}
	g := graph.New(metas)

	cycle := g.FindCycle()
	if *jsonFlag {
		code := a.renderCycleJSON(cycle)
		printWarnings(a.Stderr, graph.Warnings(metas))
		return code
	}
	if cycle == nil {
		fmt.Fprintln(a.Stdout, "no cycles")
		return exitOK
	}
	fmt.Fprintf(a.Stdout, "cycle: %s\n", graph.FormatCycle(cycle))
	printWarnings(a.Stderr, graph.Warnings(metas))
	return exitOK
}

// renderTreeLine formats one dependency tree line. Missing deps are marked so
// a broken reference is visible rather than silently expanded.
func renderTreeLine(line graph.TreeLine) string {
	text := strings.Repeat("  ", line.Depth) + line.ID
	switch {
	case line.Unknown:
		text += " (missing)"
	case line.Note != "":
		text += " " + line.Note
	}
	return text
}

func printWarnings(w io.Writer, warnings []graph.Warning) {
	for _, warn := range warnings {
		fmt.Fprintf(w, "warning: %s\n", warn)
	}
}

// addDep returns deps with dep added if absent. The result is not sorted here;
// the serializer and the index sort dep lists before writing (spec 5.2/8.2).
func addDep(deps []string, dep string) []string {
	if hasDep(deps, dep) {
		return deps
	}
	return append(append([]string(nil), deps...), dep)
}

func removeDep(deps []string, dep string) []string {
	out := make([]string, 0, len(deps))
	for _, d := range deps {
		if d != dep {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func hasDep(deps []string, dep string) bool {
	for _, d := range deps {
		if d == dep {
			return true
		}
	}
	return false
}
