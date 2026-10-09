// Package test holds the end-to-end CLI integration tests. It builds the bl
// binary once and drives it as a subprocess in temporary directories, so the
// tests exercise the real dispatch, exit codes, stderr, and on-disk format.
package test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNoArgs_PrintsUsage(t *testing.T) {
	dir := t.TempDir()

	res := mustRun(t, dir)

	if !strings.Contains(res.Stdout, "usage: bl") {
		t.Fatalf("usage missing from stdout: %q", res.Stdout)
	}
}

func TestHelp_PrintsUsage(t *testing.T) {
	dir := t.TempDir()

	res := mustRun(t, dir, "help")

	if !strings.Contains(res.Stdout, "usage: bl") {
		t.Fatalf("usage missing from stdout: %q", res.Stdout)
	}
}

func TestUnknownCommand_FailsWithUsageOnStderr(t *testing.T) {
	dir := t.TempDir()

	res := mustFail(t, dir, "bogus")

	if res.Code != 2 {
		t.Fatalf("exit code = %d, want 2", res.Code)
	}
	if !strings.Contains(res.Stderr, "usage: bl") {
		t.Fatalf("usage missing from stderr: %q", res.Stderr)
	}
}

func TestAdd_FreshDir_CreatesTicketsAndIndex(t *testing.T) {
	dir := t.TempDir()

	mustRun(t, dir, "add", "First")

	if _, err := os.Stat(filepath.Join(dir, ".tickets")); err != nil {
		t.Fatalf(".tickets not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backlog.md")); err != nil {
		t.Fatalf("backlog.md not created: %v", err)
	}
}

func TestList_MixedPriorities_SortsByPriority(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "Later", "--id", "later", "-p", "1")
	mustRun(t, dir, "add", "Urgent", "--id", "urgent", "-p", "0")

	out := mustRun(t, dir, "list", "-vv").Stdout

	lines := nonEmptyLines(out)
	if len(lines) != 2 || trailingID(t, lines[0]) != "urgent" || trailingID(t, lines[1]) != "later" {
		t.Fatalf("list = %q, want urgent then later", out)
	}
}

func TestShow_GeneratedID_PrintsTicket(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "Add login flow")
	id := trailingID(t, nonEmptyLines(mustRun(t, dir, "list", "-vv").Stdout)[0])

	out := mustRun(t, dir, "show", id).Stdout

	if !strings.Contains(out, "# Add login flow") {
		t.Fatalf("show = %q, want the title heading", out)
	}
}

func TestShow_UppercasePartialID_MatchesFullID(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "login")
	full := showOf(t, dir, "login")

	out := mustRun(t, dir, "show", "LOG").Stdout

	if out != full {
		t.Fatalf("partial show differs:\n%s\nwant:\n%s", out, full)
	}
}

func TestEdit_PriorityAndNote_UpdatesDetail(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "flow")

	mustRun(t, dir, "edit", "flow", "-p", "3", "--note", "waiting")

	show := showOf(t, dir, "flow")
	if !strings.Contains(show, "priority: 3") {
		t.Fatalf("priority not applied:\n%s", show)
	}
	if !strings.Contains(show, "## Notes\nwaiting") {
		t.Fatalf("note not applied:\n%s", show)
	}
}

func TestEdit_Priority_RegeneratesIndexOrder(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "Alpha", "--id", "alpha", "-p", "1")
	mustRun(t, dir, "add", "Bravo", "--id", "bravo", "-p", "2")

	mustRun(t, dir, "edit", "alpha", "-p", "3")

	index, err := os.ReadFile(filepath.Join(dir, "backlog.md"))
	if err != nil {
		t.Fatal(err)
	}
	bravo, alpha := strings.Index(string(index), "#bravo"), strings.Index(string(index), "#alpha")
	if bravo < 0 || alpha < 0 || bravo > alpha {
		t.Fatalf("index not re-sorted to bravo then alpha:\n%s", index)
	}
}

// The transition tests cover every status verb: one test per verb and source
// status. A refused transition must exit nonzero and change nothing.

func TestStart_OpenTicket_BecomesInProgress(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "flow")

	mustRun(t, dir, "start", "flow")

	if got := statusOf(t, dir, "flow"); got != "in_progress" {
		t.Fatalf("status = %q, want in_progress", got)
	}
}

func TestStart_InProgressTicket_FailsAndKeepsStatus(t *testing.T) {
	dir := t.TempDir()
	addInProgress(t, dir, "flow")

	mustFail(t, dir, "start", "flow")

	if got := statusOf(t, dir, "flow"); got != "in_progress" {
		t.Fatalf("status = %q, want in_progress", got)
	}
}

func TestReopen_InProgressTicket_BecomesOpen(t *testing.T) {
	dir := t.TempDir()
	addInProgress(t, dir, "flow")

	mustRun(t, dir, "reopen", "flow")

	if got := statusOf(t, dir, "flow"); got != "open" {
		t.Fatalf("status = %q, want open", got)
	}
}

func TestDone_OpenTicket_FailsAndKeepsStatus(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "flow")

	mustFail(t, dir, "done", "flow")

	if got := statusOf(t, dir, "flow"); got != "open" {
		t.Fatalf("status = %q, want open", got)
	}
}

func TestDone_InProgressTicket_ClosesAndSetsClosedAt(t *testing.T) {
	dir := t.TempDir()
	addInProgress(t, dir, "flow")

	mustRun(t, dir, "done", "flow")

	if got := statusOf(t, dir, "flow"); got != "closed" {
		t.Fatalf("status = %q, want closed", got)
	}
	if show := showOf(t, dir, "flow"); !strings.Contains(show, "closed-at: ") {
		t.Fatalf("closed-at not set:\n%s", show)
	}
}

func TestDone_ClosedTicket_FailsAndKeepsStatus(t *testing.T) {
	dir := t.TempDir()
	addClosed(t, dir, "flow")

	mustFail(t, dir, "done", "flow")

	if got := statusOf(t, dir, "flow"); got != "closed" {
		t.Fatalf("status = %q, want closed", got)
	}
}

func TestReopen_ClosedTicket_BecomesOpenAndClearsClosedAt(t *testing.T) {
	dir := t.TempDir()
	addClosed(t, dir, "flow")

	mustRun(t, dir, "reopen", "flow")

	if got := statusOf(t, dir, "flow"); got != "open" {
		t.Fatalf("status = %q, want open", got)
	}
	if show := showOf(t, dir, "flow"); strings.Contains(show, "closed-at:") {
		t.Fatalf("closed-at not cleared:\n%s", show)
	}
}

func TestReopen_OpenTicket_FailsAndKeepsStatus(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "flow")

	mustFail(t, dir, "reopen", "flow")

	if got := statusOf(t, dir, "flow"); got != "open" {
		t.Fatalf("status = %q, want open", got)
	}
}

// TestStatus_ExistingTicket_FailsAsUnknownCommand proves the explicit `status`
// verb is gone; the guarded verbs are the only transitions (spec 6.3).
func TestStatus_ExistingTicket_FailsAsUnknownCommand(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "S", "--id", "sst")
	res := run(t, dir, "status", "sst", "in_progress")
	if res.Code == 0 {
		t.Fatalf("status should be an unknown command:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "unknown command") {
		t.Fatalf("status error = %q", res.Stderr)
	}
}

// TestNonAddCommands_MissingTickets_FailWithGuidanceAndCreateNothing proves
// every command but add errors with guidance and only add creates state (spec
// 7.1). The add side is TestAdd_FreshDir_CreatesTicketsAndIndex.
func TestNonAddCommands_MissingTickets_FailWithGuidanceAndCreateNothing(t *testing.T) {
	for _, args := range [][]string{
		{"list"},
		{"show", "foo"},
		{"start", "foo"},
		{"edit", "foo", "-p", "1"},
	} {
		t.Run(args[0], func(t *testing.T) {
			dir := t.TempDir()

			res := mustFail(t, dir, args...)

			if !strings.Contains(res.Stderr, "bl add") {
				t.Fatalf("guidance missing from %q", res.Stderr)
			}
			if _, err := os.Stat(filepath.Join(dir, ".tickets")); !os.IsNotExist(err) {
				t.Fatalf(".tickets created: %v", err)
			}
		})
	}
}

// TestList_EmptyBacklog_PrintsEmptySentinel proves an existing but empty
// backlog prints the sentinel and exits 0.
func TestList_EmptyBacklog_PrintsEmptySentinel(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".tickets"), 0o755); err != nil {
		t.Fatal(err)
	}
	res := mustRun(t, dir, "list")
	if strings.TrimSpace(res.Stdout) != "(backlog is empty)" {
		t.Fatalf("empty backlog output = %q", res.Stdout)
	}
}

// The explicit-ID guards: each refused add exits nonzero and writes nothing.

func TestAdd_DuplicateID_FailsAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "aaa")

	mustFail(t, dir, "add", "B", "--id", "aaa")

	if got := detailFiles(t, dir); len(got) != 1 {
		t.Fatalf("detail files = %v, want only aaa.md", got)
	}
}

func TestAdd_UnderscoreID_FailsAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "aaa")

	mustFail(t, dir, "add", "C", "--id", "BAD_ID")

	if got := detailFiles(t, dir); len(got) != 1 {
		t.Fatalf("detail files = %v, want only aaa.md", got)
	}
}

func TestAdd_ReservedID_FailsAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "aaa")

	mustFail(t, dir, "add", "D", "--id", "archive")

	if got := detailFiles(t, dir); len(got) != 1 {
		t.Fatalf("detail files = %v, want only aaa.md", got)
	}
}

// The list filter tests cover --status and --all. list is an index-only
// navigation view: the committed order is preserved and no priority flag
// exists (spec 3.1/7).

// givenListFixture returns a backlog with closed "high" and in-progress "low".
func givenListFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustRun(t, dir, "add", "High", "--id", "high", "-p", "0")
	mustRun(t, dir, "add", "Low", "--id", "low", "-p", "4")
	mustRun(t, dir, "start", "low")
	mustRun(t, dir, "start", "high")
	mustRun(t, dir, "done", "high")
	return dir
}

func TestList_Default_ShowsOnlyActive(t *testing.T) {
	dir := givenListFixture(t)

	got := nonEmptyLines(mustRun(t, dir, "list", "-vv").Stdout)

	if len(got) != 1 || !strings.HasSuffix(got[0], "#low") {
		t.Fatalf("list = %v, want only low", got)
	}
}

func TestList_StatusClosed_ShowsOnlyClosed(t *testing.T) {
	dir := givenListFixture(t)

	got := nonEmptyLines(mustRun(t, dir, "list", "--status", "closed", "-vv").Stdout)

	if len(got) != 1 || !strings.HasSuffix(got[0], "#high") {
		t.Fatalf("list --status closed = %v, want only high", got)
	}
}

func TestList_All_ShowsEveryTicket(t *testing.T) {
	dir := givenListFixture(t)

	got := nonEmptyLines(mustRun(t, dir, "list", "--all").Stdout)

	if len(got) != 2 {
		t.Fatalf("list --all = %v, want 2 tickets", got)
	}
}

func TestList_PriorityFlag_FailsAsUnknownFlag(t *testing.T) {
	dir := givenListFixture(t)

	res := run(t, dir, "list", "-p", "0")

	if res.Code == 0 {
		t.Fatalf("list -p succeeded:\n%s", res.Stdout)
	}
}

func TestList_BogusStatus_Fails(t *testing.T) {
	dir := givenListFixture(t)

	mustFail(t, dir, "list", "--status", "bogus")
}

// The index-only tests prove list and every view read backlog.md alone: a
// deleted detail file does not break them, and doctor still flags the missing
// detail (spec 3.1/5.2/7).

// givenDeletedDetail returns a backlog whose index still lists alpha after
// alpha's detail file was removed. beta depends on alpha; gamma is closed.
func givenDeletedDetail(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustRun(t, dir, "add", "Alpha", "--id", "alpha", "-p", "0")
	mustRun(t, dir, "add", "Beta", "--id", "beta", "-p", "1")
	mustRun(t, dir, "add", "Gamma", "--id", "gamma", "-p", "2")
	mustRun(t, dir, "dep", "add", "beta", "alpha")
	mustRun(t, dir, "start", "gamma")
	mustRun(t, dir, "done", "gamma")
	if err := os.Remove(filepath.Join(dir, ".tickets", "alpha.md")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestViews_DetailDeleted_StillSucceed(t *testing.T) {
	for _, args := range [][]string{
		{"list"}, {"list", "--status", "closed"}, {"list", "--all"},
		{"started"}, {"blocked"}, {"available"}, {"closed"},
		{"list", "--json"}, {"started", "--json"}, {"closed", "--json"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			dir := givenDeletedDetail(t)

			res := run(t, dir, args...)

			if res.Code != 0 {
				t.Fatalf("exit %d (must read only backlog.md): %s", res.Code, res.Stderr)
			}
		})
	}
}

func TestListAll_DetailDeleted_StillListsTicket(t *testing.T) {
	dir := givenDeletedDetail(t)

	out := mustRun(t, dir, "list", "--all", "-vv").Stdout

	if !strings.Contains(out, "#alpha") {
		t.Fatalf("list --all dropped alpha, which the index still lists:\n%s", out)
	}
}

func TestDoctor_DetailDeleted_Fails(t *testing.T) {
	dir := givenDeletedDetail(t)

	res := run(t, dir, "doctor")

	if res.Code == 0 {
		t.Fatalf("doctor did not flag the missing detail:\n%s", res.Stdout)
	}
}

// TestParallelStart_OpenTicket_ExactlyOneSucceeds starts N processes that all
// try to start the same free ticket. Exactly one must succeed (spec 10 / AE2).
func TestParallelStart_OpenTicket_ExactlyOneSucceeds(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "Claim me", "--id", "claim")

	const workers = 8
	var wg sync.WaitGroup
	codes := make([]int, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = run(t, dir, "start", "claim").Code
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, c := range codes {
		if c == 0 {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one successful start, got %d (codes %v)", successes, codes)
	}
	show := mustRun(t, dir, "show", "claim").Stdout
	if !strings.Contains(show, "status: in_progress") {
		t.Fatalf("ticket status after race = %q", show)
	}
}

// The JSON tests cover the explicit-only flag and the basic schema: --json
// never appears unless asked.

func TestList_NoJSONFlag_PrintsPlainText(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "JSON", "--id", "jsn", "-p", "1")

	out := mustRun(t, dir, "list").Stdout

	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("plain list emitted JSON: %q", out)
	}
}

func TestListJSON_OneTicket_PrintsVersionedTicket(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "JSON", "--id", "jsn", "-p", "1")

	res := mustRun(t, dir, "list", "--json")

	var list struct {
		Version int `json:"version"`
		Tickets []struct {
			ID       string   `json:"id"`
			Status   string   `json:"status"`
			Title    string   `json:"title"`
			Deps     []string `json:"deps"`
			Archived bool     `json:"archived"`
		} `json:"tickets"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &list); err != nil {
		t.Fatalf("list --json is not valid JSON: %v\n%s", err, res.Stdout)
	}
	if list.Version != 1 {
		t.Fatalf("version = %d, want 1", list.Version)
	}
	if len(list.Tickets) != 1 || list.Tickets[0].ID != "jsn" || list.Tickets[0].Status != "open" || list.Tickets[0].Title != "JSON" || list.Tickets[0].Archived {
		t.Fatalf("unexpected list JSON: %+v", list)
	}
}

func TestListJSON_OneTicket_OmitsDetailFields(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "JSON", "--id", "jsn", "-p", "1")

	out := mustRun(t, dir, "list", "--json").Stdout

	for _, field := range []string{`"priority"`, `"hash"`, `"created"`} {
		if strings.Contains(out, field) {
			t.Fatalf("navigation JSON leaked detail field %s:\n%s", field, out)
		}
	}
}

func TestShowJSON_Ticket_PrintsFullDetailAndBody(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "JSON", "--id", "jsn", "-p", "1")

	res := mustRun(t, dir, "show", "jsn", "--json")

	var one struct {
		Ticket struct {
			ID       string `json:"id"`
			Priority int    `json:"priority"`
			Created  string `json:"created"`
			Hash     string `json:"hash"`
		} `json:"ticket"`
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &one); err != nil {
		t.Fatalf("show --json is not valid JSON: %v\n%s", err, res.Stdout)
	}
	if one.Ticket.ID != "jsn" || one.Ticket.Priority != 1 || one.Ticket.Created == "" || !strings.Contains(one.Body, "# JSON") || one.Ticket.Hash == "" {
		t.Fatalf("unexpected show JSON: %+v", one)
	}
}

// The dependency tests drive dep add/rm, cycle refusal, dep tree, dep cycle,
// dep order, and the recomputed views through the real binary (spec 5.2).

// givenOpenDep returns a backlog where open "blocked" depends on open "dep".
func givenOpenDep(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	addOpen(t, dir, "blocked")
	addOpen(t, dir, "dep")
	mustRun(t, dir, "dep", "add", "blocked", "dep")
	return dir
}

// givenClosedDep is givenOpenDep with "dep" closed.
func givenClosedDep(t *testing.T) string {
	t.Helper()
	dir := givenOpenDep(t)
	mustRun(t, dir, "start", "dep")
	mustRun(t, dir, "done", "dep")
	return dir
}

func TestDepAdd_TwoTickets_MirrorsDepsIntoIndex(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "blocked")
	addOpen(t, dir, "dep")

	mustRun(t, dir, "dep", "add", "blocked", "dep")

	idx, err := os.ReadFile(filepath.Join(dir, "backlog.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(idx), "#blocked <- [dep]") {
		t.Fatalf("index missing deps mirror:\n%s", idx)
	}
}

func TestDepCycle_AcyclicGraph_ReportsNoCycles(t *testing.T) {
	dir := givenOpenDep(t)

	out := mustRun(t, dir, "dep", "cycle").Stdout

	if !strings.Contains(out, "no cycles") {
		t.Fatalf("dep cycle = %q", out)
	}
}

func TestDepAdd_BackEdge_RefusedNamingCycleAndNotWritten(t *testing.T) {
	dir := givenOpenDep(t)

	res := mustFail(t, dir, "dep", "add", "dep", "blocked")

	if !strings.Contains(res.Stderr, "would create cycle: dep -> blocked -> dep") {
		t.Fatalf("refusal = %q", res.Stderr)
	}
	if strings.Contains(showOf(t, dir, "dep"), "deps: [blocked]") {
		t.Fatalf("refused edge was written")
	}
}

func TestDepAdd_SelfLoop_RefusedAndNotWritten(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "aaa")

	res := mustFail(t, dir, "dep", "add", "aaa", "aaa")

	if !strings.Contains(res.Stderr, "would create cycle: aaa -> aaa") {
		t.Fatalf("self-loop error = %q", res.Stderr)
	}
	if out := mustRun(t, dir, "dep", "cycle").Stdout; !strings.Contains(out, "no cycles") {
		t.Fatalf("refused self-loop was written: %q", out)
	}
}

func TestBlocked_OpenDep_ListsDependent(t *testing.T) {
	dir := givenOpenDep(t)

	out := mustRun(t, dir, "blocked", "-vv").Stdout

	if !strings.Contains(out, "#blocked") {
		t.Fatalf("blocked view = %q", out)
	}
}

func TestAvailable_OpenDep_ExcludesDependent(t *testing.T) {
	dir := givenOpenDep(t)

	out := mustRun(t, dir, "available", "-vv").Stdout

	if strings.Contains(out, "#blocked") {
		t.Fatalf("blocked ticket appeared in available: %q", out)
	}
}

func TestAvailable_ClosedDep_ListsDependent(t *testing.T) {
	dir := givenClosedDep(t)

	out := mustRun(t, dir, "available", "-vv").Stdout

	if !strings.Contains(out, "#blocked") {
		t.Fatalf("available view = %q", out)
	}
}

func TestDepTree_Dependent_PrintsDependency(t *testing.T) {
	dir := givenOpenDep(t)

	out := mustRun(t, dir, "dep", "tree", "blocked").Stdout

	if !strings.Contains(out, "blocked") || !strings.Contains(out, "dep") {
		t.Fatalf("dep tree = %q", out)
	}
}

func TestDepOrder_ClosedDep_OmitsClosedKeepsDependent(t *testing.T) {
	dir := givenClosedDep(t)

	out := mustRun(t, dir, "dep", "order", "-vv").Stdout

	if strings.Contains(out, "#dep") {
		t.Fatalf("dep order listed a closed ticket: %q", out)
	}
	if !strings.Contains(out, "#blocked") {
		t.Fatalf("dep order dropped the active ticket: %q", out)
	}
}

func TestDepRm_Edge_ClearsDeps(t *testing.T) {
	dir := givenOpenDep(t)

	mustRun(t, dir, "dep", "rm", "blocked", "dep")

	if show := showOf(t, dir, "blocked"); !strings.Contains(show, "deps: []") {
		t.Fatalf("deps not cleared:\n%s", show)
	}
}

func TestClosed_ClosedTicket_IsListed(t *testing.T) {
	dir := givenClosedDep(t)

	out := mustRun(t, dir, "closed", "-vv").Stdout

	if !strings.Contains(out, "#dep") {
		t.Fatalf("closed view = %q", out)
	}
}

func TestClosed_LimitFlag_FailsAsUnknownFlag(t *testing.T) {
	dir := givenClosedDep(t)

	res := run(t, dir, "closed", "--limit", "1")

	if res.Code == 0 {
		t.Fatalf("closed --limit succeeded:\n%s", res.Stdout)
	}
}

// The attachment tests drive the sidecar through the real binary: attach
// copies and links, attachments lists, show --attachments includes the block,
// and detach removes both sides (spec 3.5).

// givenScreenshot returns a backlog with ticket "login" and the path of a
// source file screenshot.png next to it.
func givenScreenshot(t *testing.T) (dir, src string) {
	t.Helper()
	dir = t.TempDir()
	addOpen(t, dir, "login")
	src = filepath.Join(dir, "screenshot.png")
	if err := os.WriteFile(src, []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, src
}

// givenAttachedScreenshot is givenScreenshot with the file attached to login.
func givenAttachedScreenshot(t *testing.T) string {
	t.Helper()
	dir, src := givenScreenshot(t)
	mustRun(t, dir, "attach", "login", src)
	return dir
}

func TestAttach_File_CopiesSidecarAndLinksFrontmatter(t *testing.T) {
	dir, src := givenScreenshot(t)

	out := mustRun(t, dir, "attach", "login", src).Stdout

	if !strings.Contains(out, "attached screenshot.png to login") {
		t.Fatalf("attach output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tickets", "attachments", "login__screenshot.png")); err != nil {
		t.Fatalf("sidecar file missing: %v", err)
	}
	if show := showOf(t, dir, "login"); !strings.Contains(show, "attachments: [screenshot.png]") {
		t.Fatalf("frontmatter link missing:\n%s", show)
	}
}

func TestAttachments_AttachedTicket_ListsFile(t *testing.T) {
	dir := givenAttachedScreenshot(t)

	out := mustRun(t, dir, "attachments", "login").Stdout

	if strings.TrimSpace(out) != "screenshot.png" {
		t.Fatalf("attachments output = %q", out)
	}
}

func TestShowAttachments_AttachedTicket_PrintsBlock(t *testing.T) {
	dir := givenAttachedScreenshot(t)

	out := mustRun(t, dir, "show", "login", "--attachments").Stdout

	if !strings.Contains(out, "Attachments:") || !strings.Contains(out, "screenshot.png") {
		t.Fatalf("show --attachments output = %q", out)
	}
}

func TestDetach_AttachedFile_RemovesSidecarAndLink(t *testing.T) {
	dir := givenAttachedScreenshot(t)

	mustRun(t, dir, "detach", "login", "screenshot.png")

	if _, err := os.Stat(filepath.Join(dir, ".tickets", "attachments", "login__screenshot.png")); !os.IsNotExist(err) {
		t.Fatalf("sidecar file not removed: %v", err)
	}
	if show := showOf(t, dir, "login"); !strings.Contains(show, "attachments: []") {
		t.Fatalf("link not cleared:\n%s", show)
	}
}

func TestAttachments_NoAttachments_ReportsNone(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "bare")

	out := mustRun(t, dir, "attachments", "bare").Stdout

	if !strings.Contains(out, "(no attachments)") {
		t.Fatalf("attachments output = %q", out)
	}
}

func TestDetach_UnknownAttachment_FailsWithNoAttachment(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "thing")

	res := mustFail(t, dir, "detach", "thing", "ghost.md")

	if !strings.Contains(res.Stderr, "no attachment") {
		t.Fatalf("detach error = %q", res.Stderr)
	}
}

func TestAttach_DotDotSuffix_FailsAsInvalidSuffix(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "thing")

	res := mustFail(t, dir, "attach", "thing", "..")

	if !strings.Contains(res.Stderr, "invalid attachment suffix") {
		t.Fatalf("attach error = %q", res.Stderr)
	}
}

// The archive tests drive archive, unarchive, and reopen of an archived
// ticket through the real binary (spec 5.4).

func archivedPath(dir, id string) string {
	return filepath.Join(dir, ".tickets", "archive", id+".md")
}

// addArchived creates a closed ticket and archives it.
func addArchived(t *testing.T, dir, id string) {
	t.Helper()
	addClosed(t, dir, id)
	mustRun(t, dir, "archive", id)
}

func TestArchive_ClosedTicket_MovesToArchive(t *testing.T) {
	dir := t.TempDir()
	addClosed(t, dir, "thing")

	out := mustRun(t, dir, "archive", "thing").Stdout

	if !strings.Contains(out, "archived thing") {
		t.Fatalf("archive output = %q", out)
	}
	if _, err := os.Stat(archivedPath(dir, "thing")); err != nil {
		t.Fatalf("detail not in archive/: %v", err)
	}
}

func TestArchive_OpenTicket_FailsAndStaysActive(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "open")

	res := mustFail(t, dir, "archive", "open")

	if !strings.Contains(res.Stderr, "only closed tickets can be archived") {
		t.Fatalf("archive error = %q", res.Stderr)
	}
	if _, err := os.Stat(archivedPath(dir, "open")); !os.IsNotExist(err) {
		t.Fatalf("open ticket was archived: %v", err)
	}
}

func TestUnarchive_ArchivedTicket_KeepsClosedAndMovesBack(t *testing.T) {
	dir := t.TempDir()
	addArchived(t, dir, "thing")

	out := mustRun(t, dir, "unarchive", "thing").Stdout

	if !strings.Contains(out, "unarchived thing") {
		t.Fatalf("unarchive output = %q", out)
	}
	if got := statusOf(t, dir, "thing"); got != "closed" {
		t.Fatalf("status = %q, want closed", got)
	}
	if _, err := os.Stat(archivedPath(dir, "thing")); !os.IsNotExist(err) {
		t.Fatalf("detail still in archive/: %v", err)
	}
}

func TestReopen_ArchivedTicket_BecomesOpenAndMovesBack(t *testing.T) {
	dir := t.TempDir()
	addArchived(t, dir, "thing")

	mustRun(t, dir, "reopen", "thing")

	if got := statusOf(t, dir, "thing"); got != "open" {
		t.Fatalf("status = %q, want open", got)
	}
	if _, err := os.Stat(archivedPath(dir, "thing")); !os.IsNotExist(err) {
		t.Fatalf("detail still in archive/: %v", err)
	}
}

// TestReadCommands_PipedOrFileStdin_PrintSameBytesAndCode proves --json output
// is byte-identical whether stdout is a pipe (the test harness always pipes) or
// a terminal. A real tty cannot be allocated portably, so this drives the
// process the two ways a tty can change behavior without one: with stdin
// attached to a pipe and with stdin inherited from an empty file, and compares
// the stdout bytes. The key assertion is that no command ever branches on tty
// state; the same args must produce the same bytes (spec 7).
func TestReadCommands_PipedOrFileStdin_PrintSameBytesAndCode(t *testing.T) {
	commands := [][]string{
		{"list", "--json"},
		{"show", "alpha", "--json"},
		{"started", "--json"},
		{"blocked", "--json"},
		{"available", "--json"},
		{"closed", "--json"},
		{"dep", "tree", "beta", "--json"},
		{"dep", "cycle", "--json"},
		{"attachments", "alpha", "--json"},
		{"doctor"},
		{"doctor", "fix"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			dir := t.TempDir()
			mustRun(t, dir, "add", "Alpha", "--id", "alpha", "-p", "1")
			mustRun(t, dir, "add", "Beta", "--id", "beta")

			piped := run(t, dir, args...) // Stdout/Stderr are bytes.Buffers, i.e. pipes.
			fileBacked := runWithStdinFile(t, dir, args...)

			if piped.Stdout != fileBacked.Stdout {
				t.Fatalf("stdout differs by stdin/tty wiring:\npiped: %q\nfile:  %q", piped.Stdout, fileBacked.Stdout)
			}
			if piped.Code != fileBacked.Code {
				t.Fatalf("exit code differs: %d vs %d", piped.Code, fileBacked.Code)
			}
		})
	}
}

// runWithStdinFile runs bl with stdin connected to an empty regular file rather
// than the test's inherited stdin. Some programs use fstat on stdin to decide
// tty-ness; this is the closest portable approximation of a non-terminal stdin
// that is not a pipe. It must not change --json output.
func runWithStdinFile(t *testing.T, dir string, args ...string) result {
	t.Helper()
	empty, err := os.CreateTemp("", "bl-stdin-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(empty.Name())
	defer empty.Close()

	cmd := exec.Command(blBin, args...)
	cmd.Dir = dir
	cmd.Stdin = empty
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return result{Stdout: stdout.String(), Stderr: stderr.String(), Code: code}
}

// The maintenance tests drive version, doctor, and reindex through the real
// binary (spec 3.1/6.4/10).

// givenTamperedIndex returns a backlog with one ticket whose backlog.md was
// overwritten with junk, so the index drifts from the details.
func givenTamperedIndex(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	addOpen(t, dir, "alpha")
	if err := os.WriteFile(filepath.Join(dir, "backlog.md"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestVersion_PrintsOneLine(t *testing.T) {
	dir := t.TempDir()

	out := mustRun(t, dir, "version").Stdout

	if strings.TrimSpace(out) == "" || strings.Count(strings.TrimRight(out, "\n"), "\n") != 0 {
		t.Fatalf("version stdout = %q, want one non-empty line", out)
	}
}

func TestDoctor_CleanRepo_ReportsClean(t *testing.T) {
	dir := t.TempDir()
	addOpen(t, dir, "alpha")

	out := mustRun(t, dir, "doctor").Stdout

	if !strings.Contains(out, `"clean": true`) {
		t.Fatalf("doctor = %q", out)
	}
}

func TestDoctor_TamperedIndex_Fails(t *testing.T) {
	dir := givenTamperedIndex(t)

	res := run(t, dir, "doctor")

	if res.Code == 0 {
		t.Fatalf("doctor on drift exited 0:\n%s", res.Stdout)
	}
}

func TestDoctorFix_TamperedIndex_RegeneratesIndexAndLeavesClean(t *testing.T) {
	dir := givenTamperedIndex(t)

	out := mustRun(t, dir, "doctor", "fix").Stdout

	if !strings.Contains(out, `"regenerated index"`) {
		t.Fatalf("doctor fix = %q", out)
	}
	if after := mustRun(t, dir, "doctor").Stdout; !strings.Contains(after, `"clean": true`) {
		t.Fatalf("doctor after fix = %q", after)
	}
}

func TestReindex_TamperedIndex_LeavesClean(t *testing.T) {
	dir := givenTamperedIndex(t)

	mustRun(t, dir, "reindex")

	if out := mustRun(t, dir, "doctor").Stdout; !strings.Contains(out, `"clean": true`) {
		t.Fatalf("doctor after reindex = %q", out)
	}
}

// TestAdd_StaleTempFile_SweepsIt proves a mutating command removes a stale
// ".<name>.tmp-*" file left by a SIGKILLed writer (spec §6.2).
func TestAdd_StaleTempFile_SweepsIt(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "add", "Alpha", "--id", "alpha")

	stale := filepath.Join(dir, ".tickets", ".zzz.md.tmp-1")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	mustRun(t, dir, "add", "Beta", "--id", "beta")

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale temp survived a mutation: %v", err)
	}
}
