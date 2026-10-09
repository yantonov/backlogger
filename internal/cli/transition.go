package cli

import (
	"fmt"
	"time"

	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

type transitionKind int

const (
	transitionStart transitionKind = iota
	transitionDone
	transitionReopen
)

// cmdTransition implements the guarded status verbs. Each one reads the
// authoritative detail file under the lock, verifies the precondition, and
// writes only on success; a failed guard changes nothing and exits nonzero
// (spec 6.3).
func (a *App) cmdTransition(args []string, kind transitionKind) int {
	fs := newFlagSet()
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	positionals := fs.positionals()
	if len(positionals) != 1 {
		return a.usageError(fmt.Sprintf("%s needs exactly one ticket id", transitionVerb(kind)))
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}
	id, err := resolveID(paths, positionals[0])
	if err != nil {
		return a.fail(err)
	}

	tx, err := store.Begin(paths, store.TxOptions{Reindex: index.ReindexFunc()})
	if err != nil {
		return a.fail(err)
	}
	defer func() { _ = tx.Rollback() }()

	now := a.now()
	var verify, mutate func(*store.TicketMeta) error
	switch kind {
	case transitionStart:
		verify = store.RequireStatus(model.StatusOpen)
		mutate = store.SetStatus(model.StatusInProgress, now)
	case transitionDone:
		verify = store.RequireStatus(model.StatusInProgress)
		mutate = store.SetStatus(model.StatusClosed, now)
	case transitionReopen:
		verify = requireReopenState
		mutate = reopenMutate(now)
	}

	if _, err := tx.Update(id, verify, mutate, now); err != nil {
		return a.fail(err)
	}
	if err := tx.Commit(); err != nil {
		return a.fail(err)
	}

	fmt.Fprintf(a.Stdout, "%s %s\n", transitionVerb(kind), id)
	return exitOK
}

// requireReopenState accepts in_progress or closed and rejects an open ticket
// (spec 6.3). An archived ticket is closed, so reopen also brings it back.
func requireReopenState(meta *store.TicketMeta) error {
	if meta.Ticket.Status == model.StatusInProgress || meta.Ticket.Status == model.StatusClosed {
		return nil
	}
	return fmt.Errorf("ticket %s is %s, expected in_progress or closed", meta.ID, meta.Ticket.Status)
}

// reopenMutate clears closed-at and archived when set, and sets the status to
// open (spec 5.4/6.3).
func reopenMutate(_ time.Time) func(*store.TicketMeta) error {
	return func(meta *store.TicketMeta) error {
		meta.Ticket.Status = model.StatusOpen
		meta.Ticket.ClosedAt = ""
		meta.Ticket.Archived = ""
		return nil
	}
}

func transitionVerb(kind transitionKind) string {
	switch kind {
	case transitionStart:
		return "start"
	case transitionDone:
		return "done"
	case transitionReopen:
		return "reopen"
	default:
		return "transition"
	}
}
