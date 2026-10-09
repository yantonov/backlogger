package store

import (
	"fmt"
	"time"

	"github.com/yantonov/backlogger/internal/model"
)

// Archive is stored state in the detail file: the `archived` field holds an
// ISO-8601 timestamp and its presence means archived (spec 5.4). The location
// under .tickets/archive/ must agree with that field. This file holds the
// shared selection and agreement helpers; the move itself is WriteDetailMoved
// in detail.go, which keeps the field and the location in step in one atomic
// write (plus one removal) inside the caller's lock.

// Archived reports whether meta is archived by its stored field or by its
// location under .tickets/archive/. The field is authoritative; the location is
// a derived invariant that doctor checks (spec 5.4).
func Archived(meta *TicketMeta) bool {
	if meta == nil {
		return false
	}
	return meta.Ticket.ArchivedAt() || meta.Archived
}

// LocationMatchesField reports whether a detail's location agrees with its
// stored archive field: a file under archive/ must carry `archived`, and a file
// under .tickets/ must not. It is the predicate doctor uses for the
// location-field agreement check (spec 5.4).
func LocationMatchesField(meta *TicketMeta) bool {
	if meta == nil {
		return false
	}
	return meta.Archived == meta.Ticket.ArchivedAt()
}

// ArchiveAgreementError describes a location-field disagreement. It is returned
// only by the helper below; doctor renders it.
type ArchiveAgreementError struct {
	ID string
	// InArchive reports which side of the invariant failed: true when the file
	// lives in archive/ but has no `archived` field, false when it lives in the
	// active directory but carries the field.
	InArchive bool
}

func (e *ArchiveAgreementError) Error() string {
	if e.InArchive {
		return fmt.Sprintf("%s: file is under archive/ but has no archived field", e.ID)
	}
	return fmt.Sprintf("%s: file has an archived field but lives under .tickets/", e.ID)
}

// ArchiveAgreement reports the first location-field disagreement in metas, or
// nil when every detail agrees. It is the support predicate the doctor calls;
// the unit test for it lives in this package.
func ArchiveAgreement(metas []*TicketMeta) error {
	for _, meta := range metas {
		if !LocationMatchesField(meta) {
			return &ArchiveAgreementError{ID: meta.ID, InArchive: meta.Archived}
		}
	}
	return nil
}

// ArchiveCandidates returns the closed, not-yet-archived active tickets whose
// `closed-at` is strictly older than cutoff, sorted by ID. Only status closed
// may be archived, and only active-directory files can be archived (an already
// archived ticket is skipped). It is the selection for
// `archive --closed-older-than D` (spec 5.4).
func ArchiveCandidates(metas []*TicketMeta, cutoff time.Time) []*TicketMeta {
	var out []*TicketMeta
	for _, meta := range metas {
		if meta.Archived || meta.Ticket.Status != model.StatusClosed {
			continue
		}
		ts, ok := parseTimestamp(meta.Ticket.ClosedAt)
		if !ok {
			continue
		}
		if ts.Before(cutoff) {
			out = append(out, meta)
		}
	}
	return out
}

// parseTimestamp parses the canonical UTC ISO-8601 form the serializer emits
// (spec 8.2). An unparseable or empty value is reported as not-ok so a
// malformed closed-at never qualifies for archive selection.
func parseTimestamp(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(model.TimestampLayout, s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}
