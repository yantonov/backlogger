package store_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/frontmatter"
	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

const (
	crashHelperEnv = "BL_CRASH_HELPER"
	crashDirEnv    = "BL_CRASH_DIR"
	crashOldLen    = 64 * 1024
	crashNewLen    = 256 * 1024
)

// TestCrashHelper is the subprocess body used by
// TestKill_MidTransaction_LeavesWholeFileAndNextTxSucceeds. It is skipped
// unless the parent sets the marker environment variable. It opens one
// transaction (holding the lock) and then rewrites the same detail file in a
// tight loop forever, so whenever the parent kills it, it is inside the
// transaction.
func TestCrashHelper(t *testing.T) {
	if os.Getenv(crashHelperEnv) != "1" {
		t.Skip("crash helper; runs only as a subprocess")
	}
	dir := os.Getenv(crashDirEnv)
	if dir == "" {
		t.Fatal("missing crash directory")
	}
	paths := store.NewPaths(dir)
	tx, err := store.Begin(paths, store.TxOptions{})
	if err != nil {
		t.Fatalf("helper begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Signal readiness only after the lock is held.
	fmt.Println("ready")

	ts := model.FormatTimestamp(time.Now())
	meta := &store.TicketMeta{
		ID: "victim",
		Ticket: model.Ticket{
			Status:   model.StatusOpen,
			Priority: model.DefaultPriority,
			Created:  ts,
			Updated:  ts,
		},
	}
	newBody := bytes.Repeat([]byte("n"), crashNewLen)
	for {
		meta.Body = newBody
		if err := tx.WriteDetail(meta); err != nil {
			t.Fatalf("helper write: %v", err)
		}
	}
}

// TestKill_MidTransaction_LeavesWholeFileAndNextTxSucceeds kills a process
// while it holds the lock and rewrites a detail file. The file on disk must be
// whole (the old or the new content, never a mix), and the next transaction
// must proceed.
func TestKill_MidTransaction_LeavesWholeFileAndNextTxSucceeds(t *testing.T) {
	dir := t.TempDir()
	paths := store.NewPaths(dir)
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}

	// Seed the "old" whole file.
	oldBody := bytes.Repeat([]byte("o"), crashOldLen)
	seed := mustBegin(t, paths, store.TxOptions{})
	if err := seed.WriteDetail(newTicket("victim", string(oldBody), time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := seed.Commit(); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestCrashHelper", "-test.v")
	cmd.Env = append(os.Environ(), crashHelperEnv+"=1", crashDirEnv+"="+dir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	waitReady(t, stdout)

	// Let it get into the write loop, then kill it mid-transaction.
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}
	_ = cmd.Wait()

	// The destination must be whole: exactly the old or the new content.
	data, err := paths.ReadDetailBytes(paths.ActivePath("victim"))
	if err != nil {
		t.Fatalf("read victim after kill: %v", err)
	}
	doc, err := frontmatter.Parse(data)
	if err != nil {
		t.Fatalf("victim is not a whole parseable file after kill: %v", err)
	}
	switch {
	case bytes.Equal(doc.Body, oldBody):
		// Old version survived; fine.
	case len(doc.Body) == crashNewLen && allByte(doc.Body, 'n'):
		// New version won; fine.
	default:
		t.Fatalf("torn file: body length %d (old %d, new %d) first=%q last=%q",
			len(doc.Body), len(oldBody), crashNewLen, firstByte(doc.Body), lastByte(doc.Body))
	}

	// The next operation must acquire the lock (the OS released it on death)
	// and succeed.
	next := mustBegin(t, paths, store.TxOptions{Timeout: 5 * time.Second})
	if err := next.WriteDetail(newTicket("after", "ok\n", time.Now())); err != nil {
		t.Fatalf("next write: %v", err)
	}
	if err := next.Commit(); err != nil {
		t.Fatalf("next commit: %v", err)
	}
	if _, err := paths.ReadDetail("after"); err != nil {
		t.Fatalf("next ticket missing: %v", err)
	}

	// No unexpected files besides the two tickets and the lock. Any temp file
	// the killed writer left behind must have been swept by the next Begin.
	entries, err := os.ReadDir(paths.TicketsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if name == "victim.md" || name == "after.md" || name == ".lock" {
			continue
		}
		t.Fatalf("unexpected leftover file after crash: %q", name)
	}

	// A leftover temp file must never be surfaced as a ticket.
	metas, err := paths.ListActive()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range metas {
		if strings.HasPrefix(m.ID, ".") || strings.Contains(m.ID, "tmp-") {
			t.Fatalf("temp file surfaced as a ticket: %q", m.ID)
		}
	}
}

func waitReady(t *testing.T, stdout io.Reader) {
	t.Helper()
	scanner := bufio.NewScanner(stdout)
	deadline := time.After(10 * time.Second)
	type result struct {
		ready bool
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		for scanner.Scan() {
			if scanner.Text() == "ready" {
				ch <- result{ready: true}
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = fmt.Errorf("helper exited before signalling ready")
		}
		ch <- result{err: err}
	}()
	select {
	case r := <-ch:
		if !r.ready {
			t.Fatalf("crash helper did not signal ready: %v", r.err)
		}
	case <-deadline:
		t.Fatal("timed out waiting for crash helper to signal ready")
	}
}

func allByte(b []byte, c byte) bool {
	for _, x := range b {
		if x != c {
			return false
		}
	}
	return true
}

func firstByte(b []byte) byte {
	if len(b) == 0 {
		return 0
	}
	return b[0]
}

func lastByte(b []byte) byte {
	if len(b) == 0 {
		return 0
	}
	return b[len(b)-1]
}
