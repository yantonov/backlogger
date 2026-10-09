package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/yantonov/backlogger/internal/lock"
	"github.com/yantonov/backlogger/internal/model"
)

// ReindexFunc regenerates the derived index from the on-disk detail files. It
// runs inside the lock, after details are written; internal/index supplies it
// without changing the transaction shape.
type ReindexFunc func(*Paths) error

// ErrTxDone is returned by Commit on a transaction that has already ended.
var ErrTxDone = errors.New("transaction already ended")

// TxOptions configures a transaction.
type TxOptions struct {
	// Timeout bounds the wait for the exclusive lock. Zero means
	// lock.DefaultTimeout.
	Timeout time.Duration
	// Reindex regenerates backlog.md. A nil value leaves the index untouched.
	Reindex ReindexFunc
	// Create makes Begin create .tickets/ when it is missing. Only the write
	// commands that own state creation (add, migration) set this; every other
	// mutation fails with guidance on a missing .tickets/ (spec 7.1).
	Create bool
}

// Tx is a store mutation transaction. It holds the exclusive lock for its whole
// lifetime, so callers must not perform user interaction or slow external work
// while it is open. The lock is released exactly once, by Commit or Rollback.
type Tx struct {
	paths   *Paths
	lock    *lock.Lock
	reindex ReindexFunc
	done    bool
}

// Begin acquires the exclusive lock and returns a transaction. The lock scope
// is the whole read-verify-modify-write-reindex cycle (spec 6.1).
func Begin(paths *Paths, opts TxOptions) (*Tx, error) {
	if paths == nil {
		return nil, errors.New("nil paths")
	}
	if opts.Create {
		if err := paths.EnsureTickets(); err != nil {
			return nil, err
		}
	} else if err := paths.RequireTickets(); err != nil {
		return nil, err
	}
	fl, err := lock.Acquire(paths.LockFile, opts.Timeout)
	if err != nil {
		return nil, err
	}
	// Under the exclusive lock no other writer can be mid-write, so any temp
	// file left by a SIGKILLed writer is stale and safe to remove. Fail loud
	// on a sweep error rather than proceed on a dirty state.
	if err := paths.SweepStaleTemp(); err != nil {
		_ = fl.Release()
		return nil, err
	}
	return &Tx{paths: paths, lock: fl, reindex: opts.Reindex}, nil
}

// Paths returns the path set the transaction operates on.
func (tx *Tx) Paths() *Paths { return tx.paths }

// ReadDetail reads one detail file inside the lock. A read error propagates and
// is never treated as an empty backlog.
func (tx *Tx) ReadDetail(id string) (*TicketMeta, error) {
	if tx.done {
		return nil, ErrTxDone
	}
	return tx.paths.ReadDetail(id)
}

// ListActive reads every active detail file inside the lock.
func (tx *Tx) ListActive() ([]*TicketMeta, error) {
	if tx.done {
		return nil, ErrTxDone
	}
	return tx.paths.ListActive()
}

// ListAll reads active and archived detail files inside the lock.
func (tx *Tx) ListAll() ([]*TicketMeta, error) {
	if tx.done {
		return nil, ErrTxDone
	}
	return tx.paths.ListAll()
}

// WriteDetail writes one detail file atomically inside the lock. It does not
// regenerate the index; Commit does that.
func (tx *Tx) WriteDetail(meta *TicketMeta) error {
	if tx.done {
		return ErrTxDone
	}
	return tx.paths.WriteDetail(meta)
}

// WriteDetailMoved writes a detail file at the location implied by its archived
// field and removes the previous file when the location changes. Every
// mutation uses it, so the archive location and the stored field can never
// drift (spec 5.4).
func (tx *Tx) WriteDetailMoved(meta *TicketMeta) error {
	if tx.done {
		return ErrTxDone
	}
	return tx.paths.WriteDetailMoved(meta)
}

// Update is the minimal compare-and-set primitive. It reads the detail file,
// runs verify (which must return an error when the current state does not match
// the precondition), applies mutate, bumps the updated timestamp, and writes
// the detail atomically. Every step is inside the lock, so racing updates
// serialize and at most one can satisfy verify against the old state.
//
// A verification failure is returned before anything is written.
func (tx *Tx) Update(id string, verify, mutate func(*TicketMeta) error, now time.Time) (*TicketMeta, error) {
	meta, err := tx.ReadDetail(id)
	if err != nil {
		return nil, err
	}
	if verify != nil {
		if err := verify(meta); err != nil {
			return nil, err
		}
	}
	if mutate != nil {
		if err := mutate(meta); err != nil {
			return nil, err
		}
	}
	meta.Ticket.Updated = model.FormatTimestamp(now)
	if err := tx.WriteDetailMoved(meta); err != nil {
		return nil, err
	}
	return meta, nil
}

// SetStatus returns an Update mutate function that sets the ticket status and
// maintains the derived closed-at timestamp.
func SetStatus(status model.Status, now time.Time) func(*TicketMeta) error {
	return func(meta *TicketMeta) error {
		meta.Ticket.Status = status
		if status == model.StatusClosed {
			if meta.Ticket.ClosedAt == "" {
				meta.Ticket.ClosedAt = model.FormatTimestamp(now)
			}
		} else {
			meta.Ticket.ClosedAt = ""
		}
		return nil
	}
}

// RequireStatus returns an Update verify function that fails unless the ticket
// currently has the wanted status.
func RequireStatus(want model.Status) func(*TicketMeta) error {
	return func(meta *TicketMeta) error {
		if meta.Ticket.Status != want {
			return fmt.Errorf("ticket %s is %s, expected %s", meta.ID, meta.Ticket.Status, want)
		}
		return nil
	}
}

// Reindex runs the injected reindex callback. Commit calls it automatically.
func (tx *Tx) Reindex() error {
	if tx.done {
		return ErrTxDone
	}
	if tx.reindex == nil {
		return nil
	}
	return tx.reindex(tx.paths)
}

// Commit runs reindex and releases the lock. The lock is released even when
// reindex fails, so a failed regeneration never wedges the backlog.
func (tx *Tx) Commit() error {
	if tx.done {
		return ErrTxDone
	}
	defer tx.release()
	return tx.Reindex()
}

// Rollback releases the lock without reindexing. It is safe to call after
// Commit (it then does nothing), which lets callers `defer tx.Rollback()`.
func (tx *Tx) Rollback() error {
	if tx.done {
		return nil
	}
	tx.release()
	return nil
}

func (tx *Tx) release() {
	tx.done = true
	_ = tx.lock.Release()
}
