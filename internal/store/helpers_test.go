package store_test

import (
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// newTicket builds an active detail for id with the given body.
func newTicket(id, body string, now time.Time) *store.TicketMeta {
	ts := model.FormatTimestamp(now)
	return &store.TicketMeta{
		ID: id,
		Ticket: model.Ticket{
			Status:   model.StatusOpen,
			Priority: model.DefaultPriority,
			Created:  ts,
			Updated:  ts,
		},
		Body: []byte(body),
	}
}

func mustBegin(t *testing.T, paths *store.Paths, opts store.TxOptions) *store.Tx {
	t.Helper()
	tx, err := store.Begin(paths, opts)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	return tx
}
