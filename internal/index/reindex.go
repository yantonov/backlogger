package index

import (
	"github.com/yantonov/backlogger/internal/store"
)

// Details reads every detail file (active and archived) from paths and maps it
// to the pure index view. A read error on an existing file propagates and is
// never treated as an empty backlog (spec 6.2). A missing .tickets/ is an
// error; reindex is a write path and never invents state.
func Details(paths *store.Paths) ([]*Detail, error) {
	if err := paths.RequireTickets(); err != nil {
		return nil, err
	}
	metas, err := paths.ListAll()
	if err != nil {
		return nil, err
	}
	return detailsFromMetas(metas), nil
}

// Build reads all detail files and returns the index bytes f(details). It does
// not write anything; it is the pure read half of reindex and the value the
// golden and idempotence tests compare.
func Build(paths *store.Paths) ([]byte, error) {
	details, err := Details(paths)
	if err != nil {
		return nil, err
	}
	return Render(details), nil
}

// Reindex reads every detail file, computes f(details), and writes backlog.md
// atomically. It is safe to call inside a store transaction (the caller holds
// the lock); it never acquires the lock itself, so it composes with any caller
// that already owns it (spec 3.1/6.1).
func Reindex(paths *store.Paths) error {
	data, err := Build(paths)
	if err != nil {
		return err
	}
	return paths.WriteIndex(data)
}

// ReindexFunc is the store injection point: a ReindexFunc with the index
// implementation. The CLI passes this to store.TxOptions so Commit
// regenerates backlog.md inside the same lock as the detail write.
func ReindexFunc() store.ReindexFunc {
	return Reindex
}

// ParseIndex reads backlog.md and parses it for the fast read path. A missing
// index is not an error (the caller falls back to a reindex); any other read
// error propagates.
func ParseIndex(paths *store.Paths) ([]Entry, error) {
	data, err := paths.ReadDetailBytes(paths.IndexFile)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return Parse(data), nil
}
