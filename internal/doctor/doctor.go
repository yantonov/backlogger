package doctor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yantonov/backlogger/internal/attach"
	"github.com/yantonov/backlogger/internal/frontmatter"
	"github.com/yantonov/backlogger/internal/graph"
	"github.com/yantonov/backlogger/internal/ids"
	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/store"
)

// Check names one invariant the doctor validates. They are stable strings so
// scripts can match on them.
const (
	// CheckFilename validates a detail filename stem against the ID grammar.
	CheckFilename = "filename"
	// CheckDuplicate reports the same stem present in active and archive.
	CheckDuplicate = "duplicate"
	// CheckDetail reports a detail file that could not be read or parsed.
	CheckDetail = "detail"
	// CheckIndex reports backlog.md drifting from f(details), or missing.
	CheckIndex = "index"
	// CheckDep reports a dependency ID that resolves to no ticket (warning).
	CheckDep = "dep"
	// CheckCycle reports a dependency cycle (the graph must be a DAG).
	CheckCycle = "cycle"
	// CheckArchive reports a location-field archive disagreement.
	CheckArchive = "archive"
	// CheckAttachment reports a missing or orphaned attachment link.
	CheckAttachment = "attachment"
)

// Severity separates a problem from a diagnostic.
type Severity string

// The two severities. Only errors make doctor exit nonzero; warnings are still
// reported (spec 5.2: an unresolved dep is a warning, never silent).
const (
	SevError   Severity = "error"
	SevWarning Severity = "warning"
)

// Issue is one finding. Check is one of the Check* constants, ID names the
// ticket when the finding is ticket-scoped, and Message is the human sentence.
type Issue struct {
	Severity Severity
	Check    string
	ID       string
	Message  string
}

// Report is the deterministic result of a doctor run.
type Report struct {
	// Issues holds every finding, sorted by (check, id, message).
	Issues []Issue
}

// Errors returns the number of error-severity issues.
func (r *Report) Errors() int {
	n := 0
	for _, issue := range r.Issues {
		if issue.Severity == SevError {
			n++
		}
	}
	return n
}

// Warnings returns the number of warning-severity issues.
func (r *Report) Warnings() int {
	n := 0
	for _, issue := range r.Issues {
		if issue.Severity == SevWarning {
			n++
		}
	}
	return n
}

// Has reports whether any issue was recorded for the named check.
func (r *Report) Has(check string) bool {
	for _, issue := range r.Issues {
		if issue.Check == check {
			return true
		}
	}
	return false
}

func (r *Report) add(sev Severity, check, id, format string, args ...any) {
	r.Issues = append(r.Issues, Issue{
		Severity: sev,
		Check:    check,
		ID:       id,
		Message:  fmt.Sprintf(format, args...),
	})
}

// Run validates every invariant and returns the collected report. It only
// returns an error when the backlog cannot be inspected at all (for example a
// missing .tickets/ directory); an unreadable or corrupt detail is reported as
// an issue and never treated as an empty backlog.
func Run(paths *store.Paths) (*Report, error) {
	report, _, err := inspect(paths)
	return report, err
}

// inspect validates every invariant and returns the report together with the
// detail files it could parse. The fix path reuses those metas so it does not
// read the details a second time; a file it cannot parse is reported and
// omitted from the metas rather than aborting the whole inspection.
func inspect(paths *store.Paths) (*Report, []*store.TicketMeta, error) {
	if paths == nil {
		return nil, nil, fmt.Errorf("doctor: nil paths")
	}
	if err := paths.RequireTickets(); err != nil {
		return nil, nil, err
	}

	report := &Report{}

	// Filename/ID integrity and uniqueness need only the names, so they run
	// before any file is opened.
	active, err := detailStems(paths.TicketsDir)
	if err != nil {
		return nil, nil, err
	}
	archived, err := detailStems(paths.ArchiveDir)
	if err != nil {
		return nil, nil, err
	}
	for _, id := range active {
		checkFilename(report, id)
	}
	for _, id := range archived {
		checkFilename(report, id)
	}
	checkUnique(report, active, archived)

	// Load the details. A single unreadable file is reported and skipped so the
	// remaining checks still run.
	metas := make([]*store.TicketMeta, 0, len(active)+len(archived))
	loadDetails(report, paths, paths.TicketsDir, false, &metas)
	loadDetails(report, paths, paths.ArchiveDir, true, &metas)

	checkIndex(report, paths)

	checkDeps(report, metas)
	checkCycle(report, metas)
	checkArchive(report, metas)

	checkAttachments(report, paths, metas)

	sortIssues(report.Issues)
	return report, metas, nil
}

// detailStems returns the .md filename stems directly inside dir, sorted. A
// missing directory is empty, not an error. Only regular files are considered.
func detailStems(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read directory %s: %w", dir, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(out)
	return out, nil
}

// checkFilename flags a stem that is not a legal ticket ID. A dot-prefixed,
// reserved, uppercase, or otherwise malformed name is a corruption; identity is
// the filename, so doctor can never repair it (spec 5.1/3.4).
func checkFilename(report *Report, id string) {
	if err := ids.Validate(id); err != nil {
		report.add(SevError, CheckFilename, id, "%s.md: %v", id, err)
	}
}

// checkUnique flags a stem that appears in both the active and the archive
// directory. Identity is the stem, so the same ID must name exactly one ticket.
func checkUnique(report *Report, active, archived []string) {
	seen := make(map[string]bool, len(active))
	for _, id := range active {
		seen[id] = true
	}
	for _, id := range archived {
		if seen[id] {
			report.add(SevError, CheckDuplicate, id,
				"%s.md exists in both .tickets/ and .tickets/archive/", id)
		}
	}
}

// loadDetails reads every detail file in dir and appends it to metas. A read or
// parse error is recorded as an issue and skipped; it is never treated as an
// empty backlog (spec 6.2).
func loadDetails(report *Report, paths *store.Paths, dir string, archived bool, metas *[]*store.TicketMeta) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		report.add(SevError, CheckDetail, "", "read directory %s: %v", dir, err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".md")
		path := filepath.Join(dir, e.Name())
		data, err := paths.ReadDetailBytes(path)
		if err != nil {
			report.add(SevError, CheckDetail, id, "%s: %v", path, err)
			continue
		}
		doc, err := frontmatter.Parse(data)
		if err != nil {
			report.add(SevError, CheckDetail, id, "parse %s: %v", path, err)
			continue
		}
		*metas = append(*metas, &store.TicketMeta{
			ID:       id,
			Archived: archived,
			Ticket:   doc.Ticket,
			Body:     doc.Body,
			Path:     path,
		})
	}
}

// checkIndex flags a backlog.md that is missing or not byte-equal to f(details).
// The index carries no information of its own, so this is always safe to
// regenerate (spec 3.1/6.4).
func checkIndex(report *Report, paths *store.Paths) {
	want, err := index.Build(paths)
	if err != nil {
		report.add(SevError, CheckIndex, "", "cannot compute index: %v", err)
		return
	}
	got, err := paths.ReadDetailBytes(paths.IndexFile)
	if err != nil {
		if store.IsNotFound(err) {
			report.add(SevError, CheckIndex, "", "backlog.md is missing")
			return
		}
		report.add(SevError, CheckIndex, "", "read backlog.md: %v", err)
		return
	}
	if !bytes.Equal(got, want) {
		report.add(SevError, CheckIndex, "", "backlog.md differs from f(details)")
	}
}

// checkDeps records every unresolved dependency as a warning (spec 5.2). A
// broken reference blocks rather than silently unblocks.
func checkDeps(report *Report, metas []*store.TicketMeta) {
	for _, warn := range graph.Warnings(metas) {
		report.add(SevWarning, CheckDep, warn.ID, "unresolved dependency %q", warn.Dep)
	}
}

// checkCycle records the first dependency cycle as an error. The graph is an
// invariant DAG; a cycle can still enter via a race or a manual edit, so this
// backstop must exist (spec 5.2).
func checkCycle(report *Report, metas []*store.TicketMeta) {
	if cycle := graph.New(metas).FindCycle(); cycle != nil {
		report.add(SevError, CheckCycle, cycle[0], "dependency cycle: %s", graph.FormatCycle(cycle))
	}
}

// checkArchive flags every location-field disagreement: a file under archive/
// that has no `archived` field, and a file under .tickets/ that carries one
// (spec 5.4).
func checkArchive(report *Report, metas []*store.TicketMeta) {
	for _, meta := range metas {
		if store.LocationMatchesField(meta) {
			continue
		}
		if meta.Archived {
			report.add(SevError, CheckArchive, meta.ID,
				"file is under archive/ but has no archived field")
		} else {
			report.add(SevError, CheckArchive, meta.ID,
				"file has an archived field but lives under .tickets/")
		}
	}
}

// checkAttachments compares each ticket's authoritative `attachments:` list with
// the sidecar files on disk. A missing entry and an orphan file are both errors
// (spec 3.5).
func checkAttachments(report *Report, paths *store.Paths, metas []*store.TicketMeta) {
	for _, meta := range metas {
		issues, err := attach.CheckLinks(paths.AttachmentsDir, meta.ID, meta.Ticket.Attachments)
		if err != nil {
			report.add(SevError, CheckAttachment, meta.ID, "%v", err)
			continue
		}
		for _, suffix := range issues.Missing {
			report.add(SevError, CheckAttachment, meta.ID,
				"attachment %q is linked but has no file", suffix)
		}
		for _, suffix := range issues.Orphans {
			report.add(SevError, CheckAttachment, meta.ID,
				"attachment file %q is not linked in frontmatter", suffix)
		}
	}
}

// sortIssues orders the report deterministically so two identical backlogs
// produce byte-identical doctor output.
func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Check != b.Check {
			return a.Check < b.Check
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Message < b.Message
	})
}
