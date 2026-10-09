package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yantonov/backlogger/internal/attach"
	"github.com/yantonov/backlogger/internal/ids"
	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/store"
)

// Attachment commands. The authoritative link is the `attachments:` list in a
// ticket's frontmatter; the sidecar file `.tickets/attachments/<id>__<suffix>`
// is only storage. Every mutation runs in the same store transaction as every
// other mutation, so the lock covers the copy and the frontmatter write and
// backlog.md regenerates inside the lock (spec 3.5/6.1).

// cmdAttach implements `bl attach <id> <file...>`: for each file it copies the
// bytes to `.tickets/attachments/<id>__<base>` and records the suffix in the
// ticket's frontmatter, under the lock (spec 3.5).
//
// Copying the same ticket + suffix again replaces the stored bytes and reports
// it as a replacement. Because the ID is part of the filename, a copy can never
// overwrite a different ticket's attachment.
func (a *App) cmdAttach(args []string) int {
	fs := newFlagSet()
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	pos := fs.positionals()
	if len(pos) < 2 {
		return a.usageError("attach needs a ticket id and at least one file: bl attach <id> <file...>")
	}
	operand, files := pos[0], pos[1:]
	if err := rejectUnderscoreOperand(operand); err != nil {
		return a.fail(err)
	}

	// Validate every source suffix before touching the backlog, so an invalid
	// file name is a clean usage error with no writes.
	suffixes := make([]string, 0, len(files))
	for _, file := range files {
		suffix, err := attach.SourceSuffix(file)
		if err != nil {
			return a.fail(err)
		}
		suffixes = append(suffixes, suffix)
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}
	id, err := resolveID(paths, operand)
	if err != nil {
		return a.fail(err)
	}

	tx, err := store.Begin(paths, store.TxOptions{Reindex: index.ReindexFunc()})
	if err != nil {
		return a.fail(err)
	}
	defer func() { _ = tx.Rollback() }()

	meta, err := tx.ReadDetail(id)
	if err != nil {
		return a.fail(err)
	}
	existing := make(map[string]bool, len(meta.Ticket.Attachments))
	for _, s := range meta.Ticket.Attachments {
		existing[s] = true
	}

	// Copy under the lock. Track files that did not exist before so a failure
	// can undo exactly the new files and leave a same-suffix replacement alone.
	var created []string
	cleanup := func() {
		for _, suffix := range created {
			_, _ = attach.Remove(paths.AttachmentsDir, id, suffix)
		}
	}
	had := make([]bool, len(suffixes))
	for i, file := range files {
		suffix := suffixes[i]
		before, err := attach.Exists(paths.AttachmentsDir, id, suffix)
		if err != nil {
			cleanup()
			return a.fail(err)
		}
		had[i] = before
		if _, err := attach.Copy(paths.AttachmentsDir, id, file); err != nil {
			cleanup()
			return a.fail(err)
		}
		if !before {
			created = append(created, suffix)
		}
	}

	now := a.now()
	merged := append(append([]string(nil), meta.Ticket.Attachments...), suffixes...)
	merged = uniqueSortedStrings(merged)
	if _, err := tx.Update(id, nil, func(m *store.TicketMeta) error {
		m.Ticket.Attachments = merged
		return nil
	}, now); err != nil {
		cleanup()
		return a.fail(err)
	}
	if err := tx.Commit(); err != nil {
		cleanup()
		return a.fail(err)
	}

	for i, suffix := range suffixes {
		if had[i] || existing[suffix] {
			fmt.Fprintf(a.Stdout, "replaced %s on %s\n", suffix, id)
		} else {
			fmt.Fprintf(a.Stdout, "attached %s to %s\n", suffix, id)
		}
	}
	return exitOK
}

// cmdDetach implements `bl detach <id> <file>`: it removes the sidecar file and
// the frontmatter entry, under the lock. Detach repairs either side: a
// frontmatter entry with no file only clears the link, and an orphan file with
// no entry only removes the file. When neither exists it fails clearly
// (spec 3.5).
func (a *App) cmdDetach(args []string) int {
	fs := newFlagSet()
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	pos := fs.positionals()
	if len(pos) != 2 {
		return a.usageError("detach needs a ticket id and one file: bl detach <id> <file>")
	}
	operand := pos[0]
	suffix, err := attach.SourceSuffix(pos[1])
	if err != nil {
		return a.fail(err)
	}
	if err := rejectUnderscoreOperand(operand); err != nil {
		return a.fail(err)
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}
	id, err := resolveID(paths, operand)
	if err != nil {
		return a.fail(err)
	}

	tx, err := store.Begin(paths, store.TxOptions{Reindex: index.ReindexFunc()})
	if err != nil {
		return a.fail(err)
	}
	defer func() { _ = tx.Rollback() }()

	meta, err := tx.ReadDetail(id)
	if err != nil {
		return a.fail(err)
	}
	linked := containsString(meta.Ticket.Attachments, suffix)
	onDisk, err := attach.Exists(paths.AttachmentsDir, id, suffix)
	if err != nil {
		return a.fail(err)
	}
	if !linked && !onDisk {
		return a.fail(fmt.Errorf("ticket %s has no attachment %q (not in frontmatter and no file)", id, suffix))
	}

	if linked {
		if _, err := tx.Update(id, nil, func(m *store.TicketMeta) error {
			m.Ticket.Attachments = removeString(m.Ticket.Attachments, suffix)
			return nil
		}, a.now()); err != nil {
			return a.fail(err)
		}
	}
	if onDisk {
		if _, err := attach.Remove(paths.AttachmentsDir, id, suffix); err != nil {
			return a.fail(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return a.fail(err)
	}

	fmt.Fprintf(a.Stdout, "detached %s from %s\n", suffix, id)
	return exitOK
}

// cmdAttachments implements `bl attachments <id>`: it lists the ticket's
// authoritative attachment list (sorted, deduplicated), marking a link whose
// file is missing and an on-disk file whose link is missing. A ticket with no
// attachments prints `(no attachments)`.
func (a *App) cmdAttachments(args []string) int {
	fs := newFlagSet()
	jsonFlag := fs.Bool("--json")
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	pos := fs.positionals()
	if len(pos) != 1 {
		return a.usageError("attachments needs exactly one ticket id: bl attachments <id>")
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}
	id, err := resolveID(paths, pos[0])
	if err != nil {
		return a.fail(err)
	}
	meta, err := paths.ReadDetail(id)
	if err != nil {
		return a.fail(err)
	}

	entries, err := attach.Entries(paths.AttachmentsDir, id, meta.Ticket.Attachments)
	if err != nil {
		return a.fail(err)
	}
	issues, err := attach.CheckLinks(paths.AttachmentsDir, id, meta.Ticket.Attachments)
	if err != nil {
		return a.fail(err)
	}
	if *jsonFlag {
		return a.renderAttachmentsJSON(id, entries, issues)
	}
	for _, e := range entries {
		if e.Present {
			fmt.Fprintln(a.Stdout, e.Suffix)
		} else {
			fmt.Fprintf(a.Stdout, "%s (missing)\n", e.Suffix)
		}
	}
	for _, suffix := range issues.Orphans {
		fmt.Fprintf(a.Stdout, "%s (orphan)\n", suffix)
	}
	if len(entries) == 0 && len(issues.Orphans) == 0 {
		fmt.Fprintln(a.Stdout, "(no attachments)")
	}
	return exitOK
}

// renderAttachmentSection renders the human `show --attachments` block.
func renderAttachmentSection(entries []attach.Entry, issues attach.LinkIssues) string {
	var b strings.Builder
	b.WriteString("\nAttachments:\n")
	if len(entries) == 0 && len(issues.Orphans) == 0 {
		b.WriteString("  (none)\n")
		return b.String()
	}
	for _, e := range entries {
		if e.Present {
			fmt.Fprintf(&b, "  %s\n", e.Suffix)
		} else {
			fmt.Fprintf(&b, "  %s (missing)\n", e.Suffix)
		}
	}
	for _, suffix := range issues.Orphans {
		fmt.Fprintf(&b, "  %s (orphan)\n", suffix)
	}
	return b.String()
}

// uniqueSortedStrings removes duplicates and sorts, so a merged attachment list
// is deterministic regardless of the order files were given.
func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// rejectUnderscoreOperand turns an underscore-bearing operand into the ID
// grammar error it is. Such an operand can never resolve (IDs forbid
// underscore) and it is the natural way to attempt to smuggle the attachment
// delimiter into an ID, so the error names the grammar rule instead of a bare
// "not found" (spec 3.5/5.1).
func rejectUnderscoreOperand(operand string) error {
	if !strings.ContainsRune(operand, '_') {
		return nil
	}
	_, err := ids.Normalize(operand)
	return err
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func removeString(values []string, want string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != want {
			out = append(out, v)
		}
	}
	return out
}
