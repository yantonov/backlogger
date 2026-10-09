package store_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/store"
)

// TestBegin_LockHeld_TimesOutAndCreatesNothing proves that when the lock
// cannot be acquired within the timeout, Begin fails with a timeout error and
// no detail file is created or modified.
func TestBegin_LockHeld_TimesOutAndCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}

	held := mustBegin(t, paths, store.TxOptions{})
	defer func() { _ = held.Rollback() }()

	_, err := store.Begin(paths, store.TxOptions{Timeout: 100 * time.Millisecond})
	if err == nil {
		t.Fatal("expected a timeout when the lock is held")
	}
	if !store.IsLockTimeout(err) {
		t.Fatalf("expected a lock timeout, got %v", err)
	}

	entries, readErr := os.ReadDir(paths.TicketsDir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, e := range entries {
		if e.Name() != ".lock" {
			t.Fatalf("timeout path created %q", e.Name())
		}
	}
}

// TestCommit_WrittenDetail_RunsReindexOnceAfterWrite proves Commit invokes the
// injected reindex callback exactly once and after the detail is written.
func TestCommit_WrittenDetail_RunsReindexOnceAfterWrite(t *testing.T) {
	paths := store.NewPaths(t.TempDir())
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	calls := 0
	seenTicket := false
	reindex := func(p *store.Paths) error {
		calls++
		if _, err := p.ReadDetail("r"); err == nil {
			seenTicket = true
		}
		return nil
	}
	tx := mustBegin(t, paths, store.TxOptions{Reindex: reindex})
	if err := tx.WriteDetail(newTicket("r", "", time.Now())); err != nil {
		t.Fatal(err)
	}

	err := tx.Commit()

	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("reindex called %d times, want 1", calls)
	}
	if !seenTicket {
		t.Fatal("reindex ran before the detail was visible")
	}
}

// TestRollback_AfterCommit_IsNoOp proves a deferred Rollback after a
// successful Commit does nothing and reports no error.
func TestRollback_AfterCommit_IsNoOp(t *testing.T) {
	paths := store.NewPaths(t.TempDir())
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	tx := mustBegin(t, paths, store.TxOptions{})
	if err := tx.WriteDetail(newTicket("r", "", time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	err := tx.Rollback()

	if err != nil {
		t.Fatalf("rollback after commit: %v", err)
	}
	if _, err := paths.ReadDetail("r"); err != nil {
		t.Fatalf("rollback after commit undid the write: %v", err)
	}
}

// The Begin-create tests prove Begin refuses to create state unless the
// caller opts in.

func TestBegin_MissingTicketsWithoutCreate_FailsWithErrNoTickets(t *testing.T) {
	paths := store.NewPaths(t.TempDir())

	_, err := store.Begin(paths, store.TxOptions{})

	if !errors.Is(err, store.ErrNoTickets) {
		t.Fatalf("expected ErrNoTickets, got %v", err)
	}
	if paths.TicketsExist() {
		t.Fatal("Begin without Create made .tickets/")
	}
}

func TestBegin_MissingTicketsWithCreate_CreatesTickets(t *testing.T) {
	paths := store.NewPaths(t.TempDir())

	tx, err := store.Begin(paths, store.TxOptions{Create: true})

	if err != nil {
		t.Fatalf("create begin: %v", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
	}()
	if !paths.TicketsExist() {
		t.Fatal(".tickets/ should exist after a create transaction")
	}
}
