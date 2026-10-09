package doctor

import (
	"fmt"
	"sort"

	"github.com/yantonov/backlogger/internal/attach"
	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/store"
)

// FixAction names one repair Fix applied. The Action is a stable verb scripts
// can match on; ID names the ticket when the repair was ticket-scoped.
type FixAction struct {
	// Action is a short stable verb, for example "regenerated index".
	Action string
	// ID is the ticket the repair touched, empty for a whole-backlog repair.
	ID string
	// Detail describes what changed in one human sentence.
	Detail string
}

// Fix repairs every finding that can be repaired without changing ticket
// identity, then returns the actions it applied. It is lossless by design: it
// regenerates the derived index, keeps an archive file's location and stored
// `archived` field in step, reconciles attachment links against the files on
// disk, and drops dependency references that name no ticket.
//
// It deliberately cannot repair the identity-level findings (a malformed
// filename, the same ID in active and archive, an unparseable detail) or a
// dependency cycle: those have no unique, safe rewrite and are left for a
// human. Both copies of a duplicated ID are also exempt from the detail
// repairs above. Callers re-run Run after Fix to see what remains.
//
// Every mutation happens inside one lock, so a concurrent writer cannot
// interleave, and a read error propagates before anything is written.
func Fix(paths *store.Paths) ([]FixAction, error) {
	if paths == nil {
		return nil, fmt.Errorf("doctor: nil paths")
	}
	if err := paths.RequireTickets(); err != nil {
		return nil, err
	}

	tx, err := store.Begin(paths, store.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	report, metas, err := inspect(paths)
	if err != nil {
		return nil, err
	}

	known := make(map[string]bool, len(metas))
	for _, meta := range metas {
		known[meta.ID] = true
	}

	// A cycle has no safe local repair (dropping an arbitrary edge could break
	// a real dependency), so when one exists every detail edit below is
	// skipped and only the derived index is regenerated. This keeps a corrupt
	// graph from being rewritten in an arbitrary order.
	cyclic := report.Has(CheckCycle)

	// A duplicated ID means someone placed two files under one identity, most
	// likely by hand. Which copy is right is the user's decision, so neither
	// copy is repaired: a location repair would move one file over the other.
	duplicated := make(map[string]bool)
	for _, issue := range report.Issues {
		if issue.Check == CheckDuplicate {
			duplicated[issue.ID] = true
		}
	}

	var actions []FixAction
	if !cyclic {
		for _, meta := range metas {
			if duplicated[meta.ID] {
				continue
			}
			changed, detailActions := fixDetail(paths, meta, known)
			if !changed {
				continue
			}
			if err := tx.WriteDetailMoved(meta); err != nil {
				return nil, err
			}
			actions = append(actions, detailActions...)
		}
	}

	// Regenerating the index is always safe; it runs after every detail edit so
	// the index reflects the repairs. A failure to compute f(details) (a
	// corrupt detail that the doctor already reported) is not fatal: the
	// finding simply remains for the next run.
	if changed, err := fixIndex(paths); err != nil {
		return nil, err
	} else if changed {
		actions = append(actions, FixAction{
			Action: "regenerated index",
			Detail: "backlog.md regenerated from the detail files",
		})
	}

	sortActions(actions)
	return actions, nil
}

// fixDetail applies the detail-level repairs to one parsed ticket and reports
// whether it changed anything, together with one action per applied repair. It
// never edits a file it could not parse (those are excluded from metas entirely)
// and never touches identity.
func fixDetail(paths *store.Paths, meta *store.TicketMeta, known map[string]bool) (bool, []FixAction) {
	var actions []FixAction

	// Archive location <-> field agreement. WriteDetailMoved reconciles the
	// location to the stored field, which is authoritative (spec 5.4).
	if !store.LocationMatchesField(meta) {
		actions = append(actions, FixAction{
			Action: "reconciled archive location",
			ID:     meta.ID,
			Detail: fmt.Sprintf("%s: file moved to match its archived field", meta.ID),
		})
	}

	// Dependency integrity: drop references that name no ticket. They are
	// already a warning and always block; removing them is the repair.
	kept := make([]string, 0, len(meta.Ticket.Deps))
	for _, dep := range meta.Ticket.Deps {
		if known[dep] {
			kept = append(kept, dep)
			continue
		}
		actions = append(actions, FixAction{
			Action: "dropped unresolved dep",
			ID:     meta.ID,
			Detail: fmt.Sprintf("%s: dropped dependency %q (no such ticket)", meta.ID, dep),
		})
	}
	meta.Ticket.Deps = kept

	// Attachment links: drop links whose file is gone, and adopt files that
	// exist but are not linked. Both directions are the two halves of the
	// attachment invariant (spec 3.5).
	links, err := attach.CheckLinks(paths.AttachmentsDir, meta.ID, meta.Ticket.Attachments)
	if err == nil && !links.Empty() {
		missing := make(map[string]bool, len(links.Missing))
		for _, suffix := range links.Missing {
			missing[suffix] = true
			actions = append(actions, FixAction{
				Action: "dropped missing attachment link",
				ID:     meta.ID,
				Detail: fmt.Sprintf("%s: dropped link to missing attachment %q", meta.ID, suffix),
			})
		}
		linked := make(map[string]bool, len(meta.Ticket.Attachments))
		for _, suffix := range meta.Ticket.Attachments {
			linked[suffix] = true
		}
		merged := make([]string, 0, len(meta.Ticket.Attachments)+len(links.Orphans))
		for _, suffix := range meta.Ticket.Attachments {
			if !missing[suffix] {
				merged = append(merged, suffix)
			}
		}
		for _, suffix := range links.Orphans {
			if !linked[suffix] {
				merged = append(merged, suffix)
				actions = append(actions, FixAction{
					Action: "adopted orphan attachment",
					ID:     meta.ID,
					Detail: fmt.Sprintf("%s: linked orphaned attachment file %q", meta.ID, suffix),
				})
			}
		}
		meta.Ticket.Attachments = merged
	}

	return len(actions) > 0, actions
}

// fixIndex regenerates backlog.md from the detail files and reports whether the
// bytes changed. It is the one repair that always runs, so a stale or missing
// index is healed even when no detail-level repair applies.
func fixIndex(paths *store.Paths) (bool, error) {
	want, err := index.Build(paths)
	if err != nil {
		// A corrupt detail makes f(details) uncomputable; the doctor already
		// reported it, so skip the regeneration rather than fail the repair.
		return false, nil
	}
	got, readErr := paths.ReadDetailBytes(paths.IndexFile)
	existed := readErr == nil
	if readErr != nil && !store.IsNotFound(readErr) {
		return false, fmt.Errorf("read backlog.md: %w", readErr)
	}
	if existed && string(got) == string(want) {
		return false, nil
	}
	if err := paths.WriteIndex(want); err != nil {
		return false, err
	}
	return true, nil
}

// sortActions orders the applied repairs deterministically so two identical
// backlogs produce identical fix output.
func sortActions(actions []FixAction) {
	sort.SliceStable(actions, func(i, j int) bool {
		a, b := actions[i], actions[j]
		if a.Action != b.Action {
			return a.Action < b.Action
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Detail < b.Detail
	})
}
