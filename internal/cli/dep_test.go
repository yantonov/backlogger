package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func givenAaaDependsOnBbbAndCcc(t *testing.T) *testCLI {
	t.Helper()
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "A", "--id", "aaa")
	c.must(t, "add", "B", "--id", "bbb")
	c.must(t, "add", "C", "--id", "ccc")
	c.must(t, "dep", "add", "aaa", "bbb")
	c.must(t, "dep", "add", "aaa", "ccc")
	return c
}

// TestDepAdd_DepAddedOutOfOrder_SortsDetailAndIndexMirror proves dep add
// writes the detail file and that both the detail and the index mirror keep
// deps sorted, whatever order they were added in (spec 5.2).
func TestDepAdd_DepAddedOutOfOrder_SortsDetailAndIndexMirror(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "A", "--id", "aaa")
	c.must(t, "add", "B", "--id", "bbb")
	c.must(t, "add", "C", "--id", "ccc")
	c.must(t, "dep", "add", "aaa", "ccc")

	c.must(t, "dep", "add", "aaa", "bbb")

	if show := c.must(t, "show", "aaa"); !strings.Contains(show, "deps: [bbb, ccc]") {
		t.Fatalf("detail deps not sorted:\n%s", show)
	}
	if idx := readFile(t, dir, "backlog.md"); !strings.Contains(idx, "#aaa <- [bbb, ccc]") {
		t.Fatalf("index deps not mirrored/sorted:\n%s", idx)
	}
}

// TestDepRm_ExistingDep_RemovesOnlyThatDep proves dep rm drops the named dep
// from the detail file and keeps the others (spec 5.2).
func TestDepRm_ExistingDep_RemovesOnlyThatDep(t *testing.T) {
	c := givenAaaDependsOnBbbAndCcc(t)

	c.must(t, "dep", "rm", "aaa", "bbb")

	if show := c.must(t, "show", "aaa"); !strings.Contains(show, "deps: [ccc]") {
		t.Fatalf("dep rm not applied:\n%s", show)
	}
}

// TestDepRm_SameDepTwice_SecondFails proves dep rm is guarded: once an edge is
// removed, removing it again is refused (spec 5.2).
func TestDepRm_SameDepTwice_SecondFails(t *testing.T) {
	c := givenAaaDependsOnBbbAndCcc(t)
	c.must(t, "dep", "rm", "aaa", "bbb")

	c.fail(t, "dep", "rm", "aaa", "bbb")
}

// TestDepRm_AbsentDep_FailsAndChangesNothing proves removing a dep the ticket
// does not have is refused and writes nothing (spec 5.2). Both tickets exist,
// so the refusal comes from the dep guard, not from ID resolution.
func TestDepRm_AbsentDep_FailsAndChangesNothing(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "A", "--id", "aaa")
	c.must(t, "add", "B", "--id", "bbb")
	c.must(t, "add", "C", "--id", "ccc")
	c.must(t, "dep", "add", "aaa", "bbb")
	before := treeSnapshot(t, dir)

	errOut := c.fail(t, "dep", "rm", "aaa", "ccc")

	if !strings.Contains(errOut, "aaa does not depend on ccc") {
		t.Fatalf("dep rm error = %q", errOut)
	}

	assertTreeUnchanged(t, "rejected dep rm", before, treeSnapshot(t, dir))
}

// TestDepAdd_SelfLoop_RefusedNamingCycleAndNotWritten proves `dep A A` is
// refused, names the cycle, and writes nothing (spec 5.2).
func TestDepAdd_SelfLoop_RefusedNamingCycleAndNotWritten(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "A", "--id", "aaa")

	before := c.must(t, "show", "aaa")
	errOut := c.fail(t, "dep", "add", "aaa", "aaa")
	if !strings.Contains(errOut, "would create cycle: aaa -> aaa") {
		t.Fatalf("self-loop error = %q", errOut)
	}
	after := c.must(t, "show", "aaa")
	if before != after {
		t.Fatalf("refused self-loop changed the detail:\n%s\n%s", before, after)
	}
}

// The unknown-ticket tests prove dep add rejects an operand that names no
// ticket, on either side, before anything is written. The unresolved-dep
// warning path (TestAvailable_UnresolvedDep_WarnsAndExcludesTicket) is only
// reachable through a manual edit, never through dep add (spec 5.1/5.2).

func TestDepAdd_UnknownDependency_FailsAndChangesNothing(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "A", "--id", "aaa")
	before := treeSnapshot(t, dir)

	errOut := c.fail(t, "dep", "add", "aaa", "ghost")

	if !strings.Contains(errOut, "ticket not found") {
		t.Fatalf("unknown dep error = %q", errOut)
	}
	assertTreeUnchanged(t, "refused dep add", before, treeSnapshot(t, dir))
}

func TestDepAdd_UnknownSource_FailsAndChangesNothing(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "B", "--id", "bbb")
	before := treeSnapshot(t, dir)

	errOut := c.fail(t, "dep", "add", "ghost", "bbb")

	if !strings.Contains(errOut, "ticket not found") {
		t.Fatalf("unknown source error = %q", errOut)
	}
	assertTreeUnchanged(t, "refused dep add", before, treeSnapshot(t, dir))
}

// TestDepAdd_ThreeNodeBackEdge_RefusedNamingFullCycleAndNotWritten proves a
// back edge in a three-node chain is refused with the full path and writes
// nothing (spec 5.2).
func TestDepAdd_ThreeNodeBackEdge_RefusedNamingFullCycleAndNotWritten(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	for _, id := range []string{"aaa", "bbb", "ccc"} {
		c.must(t, "add", id, "--id", id)
	}
	c.must(t, "dep", "add", "aaa", "bbb")
	c.must(t, "dep", "add", "bbb", "ccc")

	before := c.must(t, "show", "ccc")
	errOut := c.fail(t, "dep", "add", "ccc", "aaa")
	if !strings.Contains(errOut, "would create cycle: ccc -> aaa -> bbb -> ccc") {
		t.Fatalf("back-edge error = %q", errOut)
	}
	after := c.must(t, "show", "ccc")
	if before != after {
		t.Fatalf("refused back edge changed the detail:\n%s\n%s", before, after)
	}
}

// TestDepCycle_CleanGraph_ReportsNoCycles proves dep cycle reports none when
// the graph is a DAG (spec 5.2).
func TestDepCycle_CleanGraph_ReportsNoCycles(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "A", "--id", "aaa")
	c.must(t, "add", "B", "--id", "bbb")
	c.must(t, "dep", "add", "aaa", "bbb")

	out := c.must(t, "dep", "cycle")

	if !strings.Contains(out, "no cycles") {
		t.Fatalf("clean graph cycle output = %q", out)
	}
}

// TestDepCycle_InjectedCycle_ReportsCycle proves dep cycle is the backstop for
// a cycle written to disk by a hand edit or a race (spec 5.2).
func TestDepCycle_InjectedCycle_ReportsCycle(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "A", "--id", "aaa")
	c.must(t, "add", "B", "--id", "bbb")
	c.must(t, "dep", "add", "aaa", "bbb")
	c.setDeps(t, "bbb", "aaa")

	out := c.must(t, "dep", "cycle")

	if !strings.Contains(out, "cycle:") {
		t.Fatalf("injected cycle not detected: %q", out)
	}
}

// TestDepTree_InjectedCycle_TerminatesAndMarksCycle proves dep tree does not
// hang and marks the cycle when one is present on disk (spec 5.2).
func TestDepTree_InjectedCycle_TerminatesAndMarksCycle(t *testing.T) {
	dir := t.TempDir()
	c := newCLI(t, dir)
	c.must(t, "add", "A", "--id", "aaa")
	c.must(t, "add", "B", "--id", "bbb")
	c.setDeps(t, "aaa", "bbb")
	c.setDeps(t, "bbb", "aaa")

	out := c.must(t, "dep", "tree", "aaa")
	if !strings.Contains(out, "(cycle)") {
		t.Fatalf("tree did not mark the cycle: %q", out)
	}
}

// givenUnresolvedDep returns a harness where "aaa" depends on "ghost", which
// names no ticket. Such a dep can only enter through a manual edit, so it is
// injected on disk and the index regenerated to mirror it. "bbb" is a plain
// open ticket, so available is never empty.
func givenUnresolvedDep(t *testing.T) *testCLI {
	t.Helper()
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "A", "--id", "aaa")
	c.must(t, "add", "B", "--id", "bbb")
	c.setDeps(t, "aaa", "ghost")
	c.must(t, "reindex")
	return c
}

// TestAvailable_UnresolvedDep_WarnsAndExcludesTicket proves an unknown dep ID
// is surfaced as a warning from the index alone and blocks the ticket, never
// silently ignored (spec 5.2).
func TestAvailable_UnresolvedDep_WarnsAndExcludesTicket(t *testing.T) {
	c := givenUnresolvedDep(t)

	out, code := c.run("available")

	if code != 0 {
		t.Fatalf("available exit %d: %s", code, c.err.String())
	}
	if !strings.Contains(c.err.String(), "unresolved dependency \"ghost\"") {
		t.Fatalf("warning missing: %q", c.err.String())
	}
	if out != "bbb\n" {
		t.Fatalf("available = %q, want only bbb", out)
	}
}

// TestDepTree_UnresolvedDep_MarksItMissing proves dep tree marks an unknown
// dep instead of dropping it (spec 5.2).
func TestDepTree_UnresolvedDep_MarksItMissing(t *testing.T) {
	c := givenUnresolvedDep(t)

	out := c.must(t, "dep", "tree", "aaa")

	if !strings.Contains(out, "ghost (missing)") {
		t.Fatalf("unknown dep not marked in tree: %q", out)
	}
}

// Dependency stages for givenBlockedOnDep, each one step further than the last.
const (
	stageDepOpen        = iota // blocked depends on open dep
	stageDepClosed             // dep closed
	stageBlockedStarted        // blocked claimed with start
	stageBothClosed            // blocked closed too
)

// givenBlockedOnDep returns a harness with "free" (priority 0) and "blocked"
// depending on "dep", advanced to stage.
func givenBlockedOnDep(t *testing.T, stage int) *testCLI {
	t.Helper()
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Blocked", "--id", "blocked")
	c.must(t, "add", "Dep", "--id", "dep")
	c.must(t, "add", "Free", "--id", "free", "-p", "0")
	c.must(t, "dep", "add", "blocked", "dep")
	if stage >= stageDepClosed {
		c.closeTicket(t, "dep")
	}
	if stage >= stageBlockedStarted {
		c.must(t, "start", "blocked")
	}
	if stage >= stageBothClosed {
		c.must(t, "done", "blocked")
	}
	return c
}

// TestViews_DepStatusStages_ListExpectedTickets proves started, blocked,
// available, and closed react to dep status and to taking a ticket, and that
// closed lists the closed section in committed order (spec 5.2).
func TestViews_DepStatusStages_ListExpectedTickets(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage int
		view  string
		want  string
	}{
		{"dep_open/blocked_lists_dependent", stageDepOpen, "blocked", "blocked\n"},
		{"dep_open/available_excludes_dependent", stageDepOpen, "available", "free\ndep\n"},
		{"dep_open/started_is_empty", stageDepOpen, "started", ""},
		{"dep_closed/blocked_drops_dependent", stageDepClosed, "blocked", ""},
		{"dep_closed/available_lists_dependent", stageDepClosed, "available", "free\nblocked\n"},
		{"blocked_started/available_excludes_it", stageBlockedStarted, "available", "free\n"},
		{"blocked_started/started_lists_it", stageBlockedStarted, "started", "blocked\n"},
		{"both_closed/closed_lists_both_in_order", stageBothClosed, "closed", "blocked\ndep\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := givenBlockedOnDep(t, tc.stage)

			got := c.must(t, tc.view)

			if got != tc.want {
				t.Fatalf("%s = %q, want %q", tc.view, got, tc.want)
			}
		})
	}
}

// TestViews_IndexOutOfPriorityOrder_KeepIndexOrder proves the navigation views
// never re-sort: they return entries in backlog.md line order. The committed
// index is hand-edited so its line order disagrees with priority; the views
// read only the index, so they must follow it (spec 3.1).
func TestViews_IndexOutOfPriorityOrder_KeepIndexOrder(t *testing.T) {
	for _, view := range []string{"list", "available"} {
		t.Run(view, func(t *testing.T) {
			dir := t.TempDir()
			c := newCLI(t, dir)
			c.must(t, "add", "High", "--id", "high", "-p", "0")
			c.must(t, "add", "Low", "--id", "low", "-p", "4")
			swapIndexLines(t, dir, "#high", "#low")

			got := viewLineIDs(t, c.must(t, view, "-vv"))

			if want := []string{"low", "high"}; !equalStringSlices(got, want) {
				t.Fatalf("%s order = %v, want index order %v", view, got, want)
			}
		})
	}
}

// swapIndexLines swaps the backlog.md lines ending in a and b, so the
// committed order disagrees with the sort order.
func swapIndexLines(t *testing.T, dir, a, b string) {
	t.Helper()
	path := filepath.Join(dir, "backlog.md")
	lines := strings.Split(readFile(t, dir, "backlog.md"), "\n")
	ia, ib := -1, -1
	for i, line := range lines {
		switch {
		case strings.HasSuffix(line, a):
			ia = i
		case strings.HasSuffix(line, b):
			ib = i
		}
	}
	if ia < 0 || ib < 0 {
		t.Fatalf("index lacks %s or %s:\n%s", a, b, strings.Join(lines, "\n"))
	}
	lines[ia], lines[ib] = lines[ib], lines[ia]
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

// givenAlphaBlocksBeta returns a harness where open "beta" depends on open
// "alpha", so available lists alpha only and list lists both.
func givenAlphaBlocksBeta(t *testing.T) *testCLI {
	t.Helper()
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Alpha task", "--id", "alpha")
	c.must(t, "add", "Beta task", "--id", "beta")
	c.must(t, "dep", "add", "beta", "alpha")
	return c
}

// TestViews_VerbosityFlags_PrintLadderLevel covers the git-branch-style output
// ladder on the navigation views: bare id by default, id + title with -v, the
// canonical index line (deps included) with -vv, and -vv winning over -v in
// either order (spec 7).
func TestViews_VerbosityFlags_PrintLadderLevel(t *testing.T) {
	const (
		alphaLine = "- [ ] Alpha task #alpha\n"
		betaLine  = "- [ ] Beta task #beta <- [alpha]\n"
	)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"list_default", []string{"list"}, "alpha\nbeta\n"},
		{"available_default", []string{"available"}, "alpha\n"},
		{"list_v", []string{"list", "-v"}, "alpha Alpha task\nbeta Beta task\n"},
		{"available_v", []string{"available", "-v"}, "alpha Alpha task\n"},
		{"list_vv", []string{"list", "-vv"}, alphaLine + betaLine},
		{"available_vv", []string{"available", "-vv"}, alphaLine},
		{"list_v_then_vv", []string{"list", "-v", "-vv"}, alphaLine + betaLine},
		{"list_vv_then_v", []string{"list", "-vv", "-v"}, alphaLine + betaLine},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := givenAlphaBlocksBeta(t)

			got := c.must(t, tc.args...)

			if got != tc.want {
				t.Fatalf("%v = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// TestStart_AvailableDefaultOutput_ResolvesAsOperand proves the bare default
// is pipeable: a line of `available` output is a valid operand to a command
// that takes an ID (they parse bare stems, never `#id`).
func TestStart_AvailableDefaultOutput_ResolvesAsOperand(t *testing.T) {
	c := givenAlphaBlocksBeta(t)
	id := strings.TrimSpace(c.must(t, "available"))

	c.must(t, "start", id)

	if show := c.must(t, "show", "alpha"); !strings.Contains(show, "status: in_progress") {
		t.Fatalf("start %q did not claim alpha:\n%s", id, show)
	}
}

// TestViews_RemovedFlags_FailAsUsageErrors proves the removed priority and
// limit flags are gone from every view (spec 7).
func TestViews_RemovedFlags_FailAsUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"list", "-p", "0"},
		{"started", "-p", "0"},
		{"blocked", "-p", "0"},
		{"available", "-p", "0"},
		{"closed", "--limit", "1"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			c := newCLI(t, t.TempDir())
			c.must(t, "add", "A", "--id", "aaa")

			_, code := c.run(args...)

			if code != exitUsage {
				t.Fatalf("exit = %d, want usage error", code)
			}
		})
	}
}

// TestViewAndDepCommands_MissingTickets_FailWithGuidanceAndCreateNothing proves
// the views and dep commands, reads and writes alike, error with guidance on
// a missing .tickets/ and create nothing (spec 7.1).
func TestViewAndDepCommands_MissingTickets_FailWithGuidanceAndCreateNothing(t *testing.T) {
	for _, args := range [][]string{
		{"started"}, {"blocked"}, {"available"}, {"closed"},
		{"dep", "cycle"}, {"dep", "tree", "x"}, {"dep", "add", "aaa", "bbb"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			dir := t.TempDir()
			c := newCLI(t, dir)

			errOut := c.fail(t, args...)

			if !strings.Contains(errOut, "bl add") {
				t.Fatalf("guidance missing: %q", errOut)
			}
			if _, err := os.Stat(filepath.Join(dir, ".tickets")); !os.IsNotExist(err) {
				t.Fatalf(".tickets created: %v", err)
			}
		})
	}
}
