package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/yantonov/backlogger/internal/attach"
	"github.com/yantonov/backlogger/internal/doctor"
	"github.com/yantonov/backlogger/internal/graph"
	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/store"
)

// This file implements the machine-readable output for every read command. The
// schema itself lives in schema.go. JSON is emitted only when --json was passed
// explicitly: no command branches on whether stdout is a tty, so piping the
// output never changes the bytes (spec 7). Every response carries the
// schema version. Navigation views (list/started/blocked/available/closed) emit
// the lean index-line shape only and never read a detail file; `show` is the
// one full-detail response and is the only place a content hash appears.

// renderNavJSON writes the selected index entries as the lean navigation shape.
// command is the view name (`list`, `started`, `blocked`, `available`, `closed`);
// filter is nil when no filter flag was given. It builds the response purely
// from parsed index entries, so it never opens a detail file (spec 3.1/7).
func (a *App) renderNavJSON(command string, filter *jsonFilter, entries []index.Entry) int {
	resp := jsonList{
		Version: jsonSchemaVersion,
		Command: command,
		Filter:  filter,
		Tickets: make([]jsonNavTicket, 0, len(entries)),
	}
	for _, e := range entries {
		resp.Tickets = append(resp.Tickets, toNavTicket(e))
	}
	return a.writeJSON(resp)
}

// renderShowJSON writes one ticket, its body, and a derived content hash as
// JSON. The hash field is reserved for a future --if-match (spec 6.4).
func (a *App) renderShowJSON(meta *store.TicketMeta) int {
	resp := jsonShow{
		Version: jsonSchemaVersion,
		Command: "show",
		Ticket:  toJSONTicket(meta),
		Body:    string(meta.Body),
	}
	return a.writeJSON(resp)
}

// renderTreeJSON writes a `dep tree --json` response. Lines are the graph's
// traversal order; the root and full flag are echoed so the response is
// self-describing.
func (a *App) renderTreeJSON(root string, full bool, lines []graph.TreeLine) int {
	resp := jsonDepTree{
		Version: jsonSchemaVersion,
		Command: "dep tree",
		Root:    root,
		Full:    full,
		Lines:   make([]jsonTreeLn, 0, len(lines)),
	}
	for _, line := range lines {
		resp.Lines = append(resp.Lines, jsonTreeLn{
			ID:      line.ID,
			Depth:   line.Depth,
			Note:    line.Note,
			Unknown: line.Unknown,
		})
	}
	return a.writeJSON(resp)
}

// renderCycleJSON writes a `dep cycle --json` response. cycle is nil when the
// graph is acyclic; the field is then the empty string, never omitted, so a
// consumer can branch on one stable field.
func (a *App) renderCycleJSON(cycle []string) int {
	text := ""
	if cycle != nil {
		text = graph.FormatCycle(cycle)
	}
	return a.writeJSON(jsonDepCycle{
		Version: jsonSchemaVersion,
		Command: "dep cycle",
		Cycle:   text,
	})
}

// renderDepOrderJSON writes a `dep order --json` response. The order is the
// stable topological sequence restricted to the selected tickets; each ticket
// carries its 1-based position within that sequence and the lean navigation
// shape, so a consumer can both read the order and act on an ID (spec 7).
func (a *App) renderDepOrderJSON(ids []string, byID map[string]*store.TicketMeta) int {
	resp := jsonDepOrder{
		Version: jsonSchemaVersion,
		Command: "dep order",
		Order:   make([]jsonOrderTicket, 0, len(ids)),
	}
	for i, id := range ids {
		t := toNavTicket(entryFor(byID[id]))
		resp.Order = append(resp.Order, jsonOrderTicket{
			Position:      i + 1,
			jsonNavTicket: t,
		})
	}
	return a.writeJSON(resp)
}

// renderAttachmentsJSON writes an `attachments --json` response. entries and
// orphans are always arrays, so an empty result is [] rather than null.
func (a *App) renderAttachmentsJSON(id string, entries []attach.Entry, issues attach.LinkIssues) int {
	resp := jsonAttachments{
		Version: jsonSchemaVersion,
		Command: "attachments",
		ID:      id,
		Entries: make([]jsonAttachment, 0, len(entries)),
		Orphans: nonNil(issues.Orphans),
	}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, jsonAttachment{Suffix: e.Suffix, Present: e.Present})
	}
	return a.writeJSON(resp)
}

// renderDoctorJSON writes a `doctor` response. The issues are already sorted by
// the doctor, so the JSON order is deterministic.
func (a *App) renderDoctorJSON(report *doctor.Report) int {
	resp := jsonDoctor{
		Version:  jsonSchemaVersion,
		Command:  "doctor",
		Issues:   make([]jsonIssue, 0, len(report.Issues)),
		Errors:   report.Errors(),
		Warnings: report.Warnings(),
		Clean:    len(report.Issues) == 0,
	}
	for _, issue := range report.Issues {
		resp.Issues = append(resp.Issues, jsonIssue{
			Severity: string(issue.Severity),
			Check:    issue.Check,
			ID:       issue.ID,
			Message:  issue.Message,
		})
	}
	return a.writeJSON(resp)
}

// renderDoctorFixJSON writes a `doctor fix` response: the repairs this run
// applied plus the findings that remain after them.
func (a *App) renderDoctorFixJSON(fixed []doctor.FixAction, report *doctor.Report) int {
	resp := jsonDoctorFix{
		Version:  jsonSchemaVersion,
		Command:  "doctor fix",
		Fixes:    make([]jsonFix, 0, len(fixed)),
		Issues:   make([]jsonIssue, 0, len(report.Issues)),
		Errors:   report.Errors(),
		Warnings: report.Warnings(),
		Clean:    len(report.Issues) == 0,
	}
	for _, fix := range fixed {
		resp.Fixes = append(resp.Fixes, jsonFix{Action: fix.Action, ID: fix.ID, Detail: fix.Detail})
	}
	for _, issue := range report.Issues {
		resp.Issues = append(resp.Issues, jsonIssue{
			Severity: string(issue.Severity),
			Check:    issue.Check,
			ID:       issue.ID,
			Message:  issue.Message,
		})
	}
	return a.writeJSON(resp)
}

// writeJSON encodes v on stdout with a trailing newline and two-space indent.
// It is used only when --json was explicit, so human output is never affected.
func (a *App) writeJSON(v any) int {
	enc := json.NewEncoder(a.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return a.fail(err)
	}
	return exitOK
}

// toNavTicket converts a parsed index entry into the lean navigation ticket.
// It carries exactly the fields an index line shows plus `archived` so a
// consumer can tell an archived ticket from a merely closed one; it never
// opens the detail file (spec 3.1/7).
func toNavTicket(e index.Entry) jsonNavTicket {
	return jsonNavTicket{
		ID:       e.ID,
		Status:   string(index.StatusOfGlyph(e.Glyph)),
		Title:    e.Title,
		Deps:     sortedNonNil(e.Deps),
		Archived: e.ArchivedAt != "",
	}
}

// toJSONTicket converts a detail file's metadata into the full wire ticket. The
// content hash is derived from the canonical serialization, so it changes
// exactly when the ticket changes and never depends on render time. It is used
// only by `show --json`, the one detail read path (spec 7).
func toJSONTicket(meta *store.TicketMeta) jsonTicket {
	if meta == nil {
		return jsonTicket{Deps: []string{}, Attachments: []string{}, Hash: contentHash(nil)}
	}
	return jsonTicket{
		ID:          meta.ID,
		Title:       titleOf(meta),
		Status:      string(meta.Ticket.Status),
		Priority:    meta.Ticket.Priority,
		Deps:        sortedNonNil(meta.Ticket.Deps),
		Attachments: sortedNonNil(meta.Ticket.Attachments),
		Created:     meta.Ticket.Created,
		Updated:     meta.Ticket.Updated,
		ClosedAt:    meta.Ticket.ClosedAt,
		Archived:    meta.Archived || meta.Ticket.ArchivedAt(),
		Hash:        contentHash(serializeDocument(meta)),
	}
}

// nonNil returns an empty slice for a nil list so JSON emits [] rather than
// null.
func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// sortedNonNil returns a sorted copy of list, or [] when it is empty, so the
// JSON list order is deterministic even for a hand-edited frontmatter list.
func sortedNonNil(list []string) []string {
	out := append([]string{}, list...)
	sort.Strings(out)
	return out
}

// contentHash derives a stable hash from a ticket's canonical serialization.
//
// It is content-addressed: an unchanged detail always hashes to the same value,
// and any change to a stored field, an unknown key, or the body changes it. It
// is the reserved --if-match token (spec 6.4); OCC itself is out of scope. The
// preimage is the canonical serializer output and includes no render-time
// value, so the hash is deterministic.
func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
