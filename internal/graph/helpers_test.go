package graph

import (
	"time"

	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// ts is a fixed created timestamp so view ordering is deterministic.
var ts = model.FormatTimestamp(time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC))

func meta(id string, status model.Status, priority int, deps ...string) *store.TicketMeta {
	return &store.TicketMeta{
		ID: id,
		Ticket: model.Ticket{
			Status:   status,
			Priority: priority,
			Created:  ts,
			Updated:  ts,
			Deps:     deps,
		},
		Body: []byte("# " + id + "\n"),
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
