package cli

import (
	"fmt"

	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/store"
)

// Version is the release version embedded in the binary. It is a single token
// so `bl version` stays a stable one-line script input; a release build may
// override it with -ldflags "-X ...cli.Version=<tag>".
var Version = "dev"

// cmdVersion implements `bl version`: one stable line, no leading noise,
// suitable for scripts (spec 7/10).
func (a *App) cmdVersion(args []string) int {
	fs := newFlagSet()
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	if extra := fs.positionals(); len(extra) > 0 {
		return a.usageError(fmt.Sprintf("version takes no arguments: %v", extra))
	}
	fmt.Fprintf(a.Stdout, "bl %s\n", Version)
	return exitOK
}

// cmdReindex implements `bl reindex`: it regenerates backlog.md from the
// authoritative detail files under the lock and never touches a detail
// (spec 3.1).
func (a *App) cmdReindex(args []string) int {
	fs := newFlagSet()
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	if extra := fs.positionals(); len(extra) > 0 {
		return a.usageError(fmt.Sprintf("reindex takes no positional arguments: %v", extra))
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}

	tx, err := store.Begin(paths, store.TxOptions{Reindex: index.ReindexFunc()})
	if err != nil {
		return a.fail(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.Commit(); err != nil {
		return a.fail(err)
	}
	fmt.Fprintln(a.Stdout, "reindexed")
	return exitOK
}
