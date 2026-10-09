package store_test

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// TestParallelStart_OpenTicket_ExactlyOneWinsAndLoserSeesInProgress runs two
// goroutines that both try to claim the same free ticket via the
// compare-and-set Update primitive. Exactly one must succeed (spec 6.3 / AE2).
func TestParallelStart_OpenTicket_ExactlyOneWinsAndLoserSeesInProgress(t *testing.T) {
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}

	// Seed a free (open) ticket.
	seed := mustBegin(t, paths, store.TxOptions{})
	if err := seed.WriteDetail(newTicket("claim", "body\n", time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := seed.Commit(); err != nil {
		t.Fatal(err)
	}

	start := func() error {
		tx, err := store.Begin(paths, store.TxOptions{})
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		_, err = tx.Update("claim",
			store.RequireStatus(model.StatusOpen),
			store.SetStatus(model.StatusInProgress, time.Now()),
			time.Now(),
		)
		if err != nil {
			return err
		}
		return tx.Commit()
	}

	var successes atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := start(); err != nil {
				errs <- err
				return
			}
			successes.Add(1)
		}()
	}
	wg.Wait()
	close(errs)

	if got := successes.Load(); got != 1 {
		t.Fatalf("expected exactly one successful start, got %d (errors: %v)", got, drain(errs))
	}
	for err := range errs {
		// The loser must fail on the state precondition, so the message names
		// the observed status. Any other failure (for example a lock timeout)
		// means the CAS contract is not what we think it is.
		if !strings.Contains(err.Error(), "in_progress") {
			t.Fatalf("loser failed for an unexpected reason: %v", err)
		}
	}

	meta, err := paths.ReadDetail("claim")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Ticket.Status != model.StatusInProgress {
		t.Fatalf("final status = %s, want in_progress", meta.Ticket.Status)
	}
}

func drain(ch chan error) []error {
	var out []error
	for err := range ch {
		out = append(out, err)
	}
	return out
}
