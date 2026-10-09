package index

import (
	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// Detail is the index's view of one ticket: the identity and only the fields f
// consumes. It is deliberately a copy, not a store.TicketMeta, so rendering
// cannot reach back into storage.
type Detail struct {
	// ID is the filename stem, the authoritative identity (spec 3.4).
	ID string
	// Status drives the section and the glyph.
	Status model.Status
	// Priority is the primary sort key (0 highest).
	Priority int
	// Created is the secondary sort key, compared as the stored string. The
	// canonical timestamp form sorts lexicographically the same way it sorts
	// chronologically.
	Created string
	// Deps is mirrored into the `<- [deps]` suffix, sorted.
	Deps []string
	// ArchivedAt is the stored archive timestamp; presence means archived.
	ArchivedAt string
	// Title is the display title. The tracker stores no title field, so it is derived
	// from the body's first heading when the detail file is read.
	Title string
}

func detailFromMeta(meta *store.TicketMeta) *Detail {
	return &Detail{
		ID:         meta.ID,
		Status:     meta.Ticket.Status,
		Priority:   meta.Ticket.Priority,
		Created:    meta.Ticket.Created,
		Deps:       append([]string(nil), meta.Ticket.Deps...),
		ArchivedAt: meta.Ticket.Archived,
		Title:      firstHeading(meta.Body),
	}
}

func detailsFromMetas(metas []*store.TicketMeta) []*Detail {
	out := make([]*Detail, 0, len(metas))
	for _, m := range metas {
		out = append(out, detailFromMeta(m))
	}
	return out
}
