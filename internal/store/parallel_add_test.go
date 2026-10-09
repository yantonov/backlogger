package store_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/store"
)

// TestParallelAdd_DistinctTickets_EachPresentOnceInStoreAndIndex runs N writers
// in parallel through the store transaction, each adding a distinct ticket and
// rewriting the shared index. Every ticket must be present exactly once and the
// index must contain exactly one line per ticket (no lost lines).
func TestParallelAdd_DistinctTickets_EachPresentOnceInStoreAndIndex(t *testing.T) {
	const n = 24
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}

	indexPath := filepath.Join(dir, "backlog.md")
	reindex := func(p *store.Paths) error {
		metas, err := p.ListActive()
		if err != nil {
			return err
		}
		var buf []byte
		for _, m := range metas {
			buf = append(buf, []byte("- [ ] "+m.ID+"\n")...)
		}
		return p.WriteDetailBytes(indexPath, buf)
	}

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("t%02d", i)
			tx, err := store.Begin(paths, store.TxOptions{Reindex: reindex})
			if err != nil {
				errs <- err
				return
			}
			defer func() { _ = tx.Rollback() }()
			if err := tx.WriteDetail(newTicket(id, "# "+id+"\n", time.Now())); err != nil {
				errs <- err
				return
			}
			if err := tx.Commit(); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("writer failed: %v", err)
	}

	metas, err := paths.ListActive()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != n {
		t.Fatalf("expected %d tickets, got %d", n, len(metas))
	}
	seen := map[string]int{}
	for _, m := range metas {
		seen[m.ID]++
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("t%02d", i)
		if seen[id] != 1 {
			t.Fatalf("ticket %s present %d times", id, seen[id])
		}
	}

	// The index must have exactly one line per ticket.
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]int{}
	for _, line := range splitLines(string(data)) {
		lines[line]++
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("t%02d", i)
		want := "- [ ] " + id
		if lines[want] != 1 {
			t.Fatalf("index line %q present %d times", want, lines[want])
		}
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// TestListActive_SequentialAdds_ReturnsEachOnceSorted is a deterministic
// companion to the parallel test: after a straight sequence of transactions,
// ListActive returns every written ticket exactly once, sorted.
func TestListActive_SequentialAdds_ReturnsEachOnceSorted(t *testing.T) {
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	var want []string
	for i := 9; i >= 0; i-- {
		id := fmt.Sprintf("u%02d", i)
		want = append([]string{id}, want...)
		tx := mustBegin(t, paths, store.TxOptions{})
		if err := tx.WriteDetail(newTicket(id, "", time.Now())); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	metas, err := paths.ListActive()

	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(metas))
	for i, m := range metas {
		got[i] = m.ID
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ListActive = %v, want %v", got, want)
	}
}
