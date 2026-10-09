package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDepOrder_UrgentTicketBlockedByLowPriority_ListsBlockerFirst proves `dep
// order` puts a blocker before its dependent even when the dependent has the
// higher priority (spec 5.2). This is the whole point: the command reorders
// where a navigation view would not.
func TestDepOrder_UrgentTicketBlockedByLowPriority_ListsBlockerFirst(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	// first is urgent but blocked; blocker is low priority but must come first.
	c.must(t, "add", "Urgent blocked", "--id", "first", "-p", "0")
	c.must(t, "add", "Low blocker", "--id", "blocker", "-p", "4")
	c.must(t, "dep", "add", "first", "blocker")

	got := lineIDs(t, c.must(t, "dep", "order", "-vv"))
	if !equalStringSlices(got, []string{"blocker", "first"}) {
		t.Fatalf("dep order = %v, want [blocker first]", got)
	}
}

// TestDepOrder_IndependentTickets_ListsByPriority proves independent tickets
// keep the (priority, created, id) order, so the command is not just "some
// topological order".
func TestDepOrder_IndependentTickets_ListsByPriority(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "High", "--id", "high", "-p", "0")
	c.must(t, "add", "Low", "--id", "low", "-p", "4")
	c.must(t, "add", "Mid", "--id", "mid", "-p", "2")

	got := lineIDs(t, c.must(t, "dep", "order", "-vv"))
	if !equalStringSlices(got, []string{"high", "mid", "low"}) {
		t.Fatalf("dep order = %v, want [high mid low]", got)
	}
}

// givenClosedBlocker returns a harness where open "todo" depends on closed
// "done": a satisfied blocker, not work that remains.
func givenClosedBlocker(t *testing.T) *testCLI {
	t.Helper()
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Done", "--id", "done")
	c.must(t, "add", "Todo", "--id", "todo")
	c.must(t, "dep", "add", "todo", "done")
	c.closeTicket(t, "done")
	return c
}

// TestDepOrder_ClosedBlocker_ListsOnlyDependent proves a closed ticket is
// never listed in the text output, and its dependent still is.
func TestDepOrder_ClosedBlocker_ListsOnlyDependent(t *testing.T) {
	c := givenClosedBlocker(t)

	got := lineIDs(t, c.must(t, "dep", "order", "-vv"))

	if !equalStringSlices(got, []string{"todo"}) {
		t.Fatalf("dep order = %v, want [todo]", got)
	}
}

// TestDepOrderJSON_ClosedBlocker_ListsOnlyDependent proves a closed ticket is
// never an entry of the JSON order either.
func TestDepOrderJSON_ClosedBlocker_ListsOnlyDependent(t *testing.T) {
	c := givenClosedBlocker(t)

	out := c.must(t, "dep", "order", "--json")

	var resp struct {
		Order []struct {
			ID string `json:"id"`
		} `json:"order"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if len(resp.Order) != 1 || resp.Order[0].ID != "todo" {
		t.Fatalf("order = %+v, want only todo", resp.Order)
	}
}

// TestDepOrder_UnresolvedDep_WarnsAndStillListsTicket proves a broken dep is
// surfaced as a warning like the other graph commands and never stalls or hides
// the order (spec 5.2).
func TestDepOrder_UnresolvedDep_WarnsAndStillListsTicket(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "A", "--id", "aaa")
	c.setDeps(t, "aaa", "ghost")

	out, code := c.run("dep", "order", "-vv")
	if code != 0 {
		t.Fatalf("dep order exit %d: %s", code, c.err.String())
	}
	if !strings.Contains(c.err.String(), `unresolved dependency "ghost"`) {
		t.Fatalf("warning missing: %q", c.err.String())
	}
	if !strings.Contains(out, "#aaa") {
		t.Fatalf("ticket dropped because of an unresolved dep: %q", out)
	}
}

// TestDepOrderJSON_BlockedAndBlocker_ListsOneBasedPositionsWithDeps proves the
// --json response carries the stable sequence and a 1-based position per
// ticket, in the lean navigation shape.
func TestDepOrderJSON_BlockedAndBlocker_ListsOneBasedPositionsWithDeps(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Blocker", "--id", "blocker")
	c.must(t, "add", "Blocked", "--id", "blocked", "-p", "0")
	c.must(t, "dep", "add", "blocked", "blocker")

	out := c.must(t, "dep", "order", "--json")
	var resp struct {
		Version int    `json:"version"`
		Command string `json:"command"`
		Order   []struct {
			Position int      `json:"position"`
			ID       string   `json:"id"`
			Status   string   `json:"status"`
			Title    string   `json:"title"`
			Deps     []string `json:"deps"`
		} `json:"order"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if resp.Command != "dep order" {
		t.Fatalf("command = %q", resp.Command)
	}
	if len(resp.Order) != 2 {
		t.Fatalf("order length = %d, want 2", len(resp.Order))
	}
	if resp.Order[0].ID != "blocker" || resp.Order[0].Position != 1 {
		t.Fatalf("first = %+v, want blocker at position 1", resp.Order[0])
	}
	if resp.Order[1].ID != "blocked" || resp.Order[1].Position != 2 {
		t.Fatalf("second = %+v, want blocked at position 2", resp.Order[1])
	}
	if !equalStringSlices(resp.Order[1].Deps, []string{"blocker"}) {
		t.Fatalf("deps = %v, want [blocker]", resp.Order[1].Deps)
	}
}

// TestDepOrder_TicketsWithDep_LeavesIndexUnchanged proves dep order is a pure
// read: it leaves backlog.md byte-identical.
func TestDepOrder_TicketsWithDep_LeavesIndexUnchanged(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "A", "--id", "aaa")
	c.must(t, "add", "B", "--id", "bbb")
	c.must(t, "dep", "add", "bbb", "aaa")

	before, err := os.ReadFile(filepath.Join(dir, "backlog.md"))
	if err != nil {
		t.Fatal(err)
	}
	c.must(t, "dep", "order")
	after, err := os.ReadFile(filepath.Join(dir, "backlog.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("dep order rewrote the index:\n%s\n%s", before, after)
	}
}

// TestDepOrder_MissingTickets_FailsWithGuidanceAndCreatesNothing proves a read
// on a missing .tickets/ errors with guidance rather than creating state (spec
// 7.1).
func TestDepOrder_MissingTickets_FailsWithGuidanceAndCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	errOut := c.fail(t, "dep", "order")
	if !strings.Contains(errOut, "tickets") {
		t.Fatalf("missing-tickets error = %q", errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tickets")); !os.IsNotExist(err) {
		t.Fatalf("dep order created .tickets")
	}
}

// TestDepOrder_ArchivedBlocker_ListsOnlyDependentWithoutWarning proves an
// archived ticket is never listed and is never reported as an unresolved dep
// either: archive membership is known by name and is satisfied by definition.
func TestDepOrder_ArchivedBlocker_ListsOnlyDependentWithoutWarning(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "Old", "--id", "old")
	c.must(t, "add", "New", "--id", "new")
	c.must(t, "dep", "add", "new", "old")
	c.archiveTicket(t, "old")

	out, code := c.run("dep", "order", "-vv")
	if code != 0 {
		t.Fatalf("dep order exit %d: %s", code, c.err.String())
	}
	if got := lineIDs(t, out); !equalStringSlices(got, []string{"new"}) {
		t.Fatalf("dep order = %v, want [new]", got)
	}
	if strings.Contains(c.err.String(), "unresolved") {
		t.Fatalf("archived dep reported unresolved: %q", c.err.String())
	}
}
