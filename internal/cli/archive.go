package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// cmdArchive implements `bl archive <id>...` and
// `bl archive --closed-older-than D`. Archive is stored state: it sets the
// `archived` timestamp and moves the file to .tickets/archive/ in one locked
// transaction (spec 5.4). Only `status: closed` tickets may be archived.
func (a *App) cmdArchive(args []string) int {
	fs := newFlagSet()
	olderFlag := fs.String("", "--closed-older-than")
	dryRunFlag := fs.Bool("--dry-run")
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	ids := fs.positionals()

	byAge := fs.Set("--closed-older-than")
	if byAge && len(ids) > 0 {
		return a.usageError("archive takes either ids or --closed-older-than, not both")
	}
	if !byAge && len(ids) == 0 {
		return a.usageError("archive needs ids or --closed-older-than D: bl archive <id>... | bl archive --closed-older-than 30d")
	}
	var age time.Duration
	if byAge {
		d, err := parseAge(*olderFlag)
		if err != nil {
			return a.usageError(err.Error())
		}
		age = d
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}

	now := a.now()

	if *dryRunFlag {
		plan, err := a.planArchive(paths, ids, byAge, age, now)
		if err != nil {
			return a.fail(err)
		}
		for _, meta := range plan {
			fmt.Fprintf(a.Stdout, "would archive %s\n", meta.ID)
		}
		return exitOK
	}

	tx, err := store.Begin(paths, store.TxOptions{Reindex: index.ReindexFunc()})
	if err != nil {
		return a.fail(err)
	}
	defer func() { _ = tx.Rollback() }()

	plan, err := a.planArchiveLocked(tx, paths, ids, byAge, age, now)
	if err != nil {
		return a.fail(err)
	}
	for _, meta := range plan {
		meta.Ticket.Archived = model.FormatTimestamp(now)
		if err := tx.WriteDetailMoved(meta); err != nil {
			return a.fail(err)
		}
	}
	if len(plan) > 0 {
		if err := tx.Commit(); err != nil {
			return a.fail(err)
		}
	}
	for _, meta := range plan {
		fmt.Fprintf(a.Stdout, "archived %s\n", meta.ID)
	}
	return exitOK
}

// cmdUnarchive implements `bl unarchive <id>`: it clears the `archived` field
// and moves the file back under .tickets/ in one locked transaction
// (spec 5.4). `closed-at` is untouched; only `reopen` clears that.
func (a *App) cmdUnarchive(args []string) int {
	fs := newFlagSet()
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	positionals := fs.positionals()
	if len(positionals) != 1 {
		return a.usageError("unarchive needs exactly one ticket id: bl unarchive <id>")
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}
	id, err := resolveID(paths, positionals[0])
	if err != nil {
		return a.fail(err)
	}

	tx, err := store.Begin(paths, store.TxOptions{Reindex: index.ReindexFunc()})
	if err != nil {
		return a.fail(err)
	}
	defer func() { _ = tx.Rollback() }()

	now := a.now()
	if _, err := tx.Update(id, requireArchived, func(meta *store.TicketMeta) error {
		meta.Ticket.Archived = ""
		return nil
	}, now); err != nil {
		return a.fail(err)
	}
	if err := tx.Commit(); err != nil {
		return a.fail(err)
	}

	fmt.Fprintf(a.Stdout, "unarchived %s\n", id)
	return exitOK
}

// requireArchived is the unarchive precondition: the ticket must carry the
// archive field. An active ticket is an error and changes nothing.
func requireArchived(meta *store.TicketMeta) error {
	if !store.Archived(meta) {
		return fmt.Errorf("ticket %s is not archived", meta.ID)
	}
	return nil
}

// archiveEligible is the archive precondition: only a closed, not-yet-archived
// ticket may be archived (spec 5.4).
func archiveEligible(meta *store.TicketMeta) error {
	if meta.Archived || meta.Ticket.ArchivedAt() {
		return fmt.Errorf("ticket %s is already archived", meta.ID)
	}
	if meta.Ticket.Status != model.StatusClosed {
		return fmt.Errorf("ticket %s is %s, only closed tickets can be archived", meta.ID, meta.Ticket.Status)
	}
	return nil
}

// planArchive resolves and validates the tickets to archive without holding the
// lock. It is used only by --dry-run, which must touch nothing. The detailed
// reads it performs are safe because it never mutates.
func (a *App) planArchive(paths *store.Paths, ids []string, byAge bool, age time.Duration, now time.Time) ([]*store.TicketMeta, error) {
	if byAge {
		metas, err := paths.ListActive()
		if err != nil {
			return nil, err
		}
		return store.ArchiveCandidates(metas, now.Add(-age)), nil
	}
	plan := make([]*store.TicketMeta, 0, len(ids))
	for _, operand := range ids {
		id, err := resolveID(paths, operand)
		if err != nil {
			return nil, err
		}
		meta, err := paths.ReadDetail(id)
		if err != nil {
			return nil, err
		}
		if err := archiveEligible(meta); err != nil {
			return nil, err
		}
		plan = append(plan, meta)
	}
	return plan, nil
}

// planArchiveLocked is planArchive inside the transaction. It validates every
// requested ticket before anything is written, so a rejected request changes
// nothing (spec 5.4).
func (a *App) planArchiveLocked(tx *store.Tx, paths *store.Paths, ids []string, byAge bool, age time.Duration, now time.Time) ([]*store.TicketMeta, error) {
	if byAge {
		metas, err := tx.ListActive()
		if err != nil {
			return nil, err
		}
		return store.ArchiveCandidates(metas, now.Add(-age)), nil
	}
	plan := make([]*store.TicketMeta, 0, len(ids))
	for _, operand := range ids {
		id, err := resolveID(paths, operand)
		if err != nil {
			return nil, err
		}
		meta, err := tx.ReadDetail(id)
		if err != nil {
			return nil, err
		}
		if err := archiveEligible(meta); err != nil {
			return nil, err
		}
		plan = append(plan, meta)
	}
	return plan, nil
}

// parseAge parses a threshold for --closed-older-than. It accepts the Go
// duration units (h, m, s) plus a day suffix, so "30d", "12h", and "45m" are
// all valid.
func parseAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("--closed-older-than needs a duration such as 30d or 12h")
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid duration %q (want a non-negative number of days)", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid duration %q (want 30d, 12h, or 45m)", s)
	}
	return d, nil
}
