package cli

import (
	"fmt"
	"strings"

	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

type listFilter struct {
	// status is an explicit --status value; empty means no status filter.
	status string
	// archived selects only archived tickets.
	archived bool
	// all selects every ticket, archived included.
	all bool
}

// cmdList implements `bl list`. It is a navigation view (spec 3.1/7): the
// default active view, --status, and --all read only backlog.md, preserve the
// committed index order, and never open a detail file. --archived and --all are
// the sanctioned exception: archived tickets are not in the index, so their
// details are read from .tickets/archive/ (spec 3.1). A missing index is
// rebuilt under the lock first; a missing .tickets/ is an error with guidance
// (spec 7.1).
func (a *App) cmdList(args []string) int {
	fs := newFlagSet()
	statusFlag := fs.String("", "--status")
	archivedFlag := fs.Bool("--archived")
	allFlag := fs.Bool("--all")
	jsonFlag := fs.Bool("--json")
	vFlag := fs.Bool("-v")
	vvFlag := fs.Bool("-vv")
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	if extra := fs.positionals(); len(extra) > 0 {
		return a.usageError(fmt.Sprintf("list takes no positional arguments: %q", strings.Join(extra, " ")))
	}

	filter := listFilter{
		status:   *statusFlag,
		archived: *archivedFlag,
		all:      *allFlag,
	}
	if filter.status != "" && !model.Status(filter.status).Valid() {
		return a.usageError(fmt.Sprintf("invalid status %q (want open, in_progress, or closed)", filter.status))
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}
	if err := a.ensureIndex(paths); err != nil {
		return a.fail(err)
	}

	entries, err := index.ParseIndex(paths)
	if err != nil {
		return a.fail(err)
	}
	selected, err := a.selectEntries(paths, entries, filter)
	if err != nil {
		return a.fail(err)
	}

	if *jsonFlag {
		return a.renderNavJSON("list", listJSONFilter(fs, filter), selected)
	}
	if len(selected) == 0 {
		// The sentinel is for a backlog with no tickets at all; a filter that
		// simply matches nothing stays silent (spec 7.1).
		if len(entries) == 0 && !filter.archived {
			fmt.Fprintln(a.Stdout, "(backlog is empty)")
		}
		return exitOK
	}
	level := navVerbosity(*vFlag, *vvFlag)
	for _, e := range selected {
		fmt.Fprintln(a.Stdout, renderNavLine(e, level))
	}
	return exitOK
}

// listJSONFilter converts the resolved list filters into the JSON filter
// record, or nil when no filter flag was given. The record is additive schema:
// it documents what the caller asked for without changing the ticket payload.
func listJSONFilter(fs *flagSet, f listFilter) *jsonFilter {
	if !fs.Set("--status") && !fs.Set("--archived") && !fs.Set("--all") {
		return nil
	}
	return &jsonFilter{Status: f.status, Archived: f.archived, All: f.all}
}

// ensureIndex rebuilds backlog.md when it is absent. It runs under the lock so
// a concurrent writer cannot observe a half-regenerated index. A present index
// is left alone: the navigation views read it as it is and never treat a
// detail file as an alternate source.
func (a *App) ensureIndex(paths *store.Paths) error {
	if _, err := paths.ReadDetailBytes(paths.IndexFile); err == nil {
		return nil
	} else if !store.IsNotFound(err) {
		return err
	}
	tx, err := store.Begin(paths, store.TxOptions{Reindex: index.ReindexFunc()})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	return tx.Commit()
}

// selectEntries applies the view and filters to parsed index entries. It never
// re-sorts: file order is the committed (priority, created, id) order (spec
// 3.1/7.1). The default and --status views are index-only. --archived reads the
// archive directory, and --all appends it, because archived tickets are not in
// the index (spec 3.1).
func (a *App) selectEntries(paths *store.Paths, entries []index.Entry, f listFilter) ([]index.Entry, error) {
	if f.archived || f.all {
		archived, err := archivedEntries(paths)
		if err != nil {
			return nil, err
		}
		if f.archived {
			return archived, nil
		}
		entries = append(entries, archived...)
	}
	out := make([]index.Entry, 0, len(entries))
	for _, e := range entries {
		if !matchEntry(e, f) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// archivedEntries reads every archived detail and renders it as a closed index
// entry carrying the archived marker. It is the one list path that reads a
// detail file, and only the archive directory (spec 3.1).
func archivedEntries(paths *store.Paths) ([]index.Entry, error) {
	metas, err := paths.ListArchived()
	if err != nil {
		return nil, err
	}
	out := make([]index.Entry, 0, len(metas))
	for _, meta := range metas {
		out = append(out, index.Entry{
			ID:         meta.ID,
			Glyph:      "x",
			Title:      bodyTitle(meta.Body),
			Deps:       append([]string(nil), meta.Ticket.Deps...),
			ArchivedAt: index.ArchivedMarker,
		})
	}
	return out, nil
}

func matchEntry(e index.Entry, f listFilter) bool {
	archived := e.ArchivedAt != ""
	switch {
	case f.all:
		// Every ticket, archived included.
	case f.archived:
		if !archived {
			return false
		}
	case f.status != "":
		if archived {
			return false
		}
		if string(index.StatusOfGlyph(e.Glyph)) != f.status {
			return false
		}
	default:
		if archived || !isActiveStatus(index.StatusOfGlyph(e.Glyph)) {
			return false
		}
	}
	return true
}

func isActiveStatus(s model.Status) bool {
	return s == model.StatusOpen || s == model.StatusInProgress
}

// titleOf returns the display title stored on an index entry. It exists for the
// JSON show path (which reads a detail file); navigation responses use the
// entry's title directly.
func titleOf(meta *store.TicketMeta) string {
	if t := bodyTitle(meta.Body); t != "" {
		return t
	}
	return meta.ID
}
