package index_test

import (
	"path/filepath"
	"testing"

	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

func fixtureRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("testdata", "golden")
}

func fixturePaths(t *testing.T) *store.Paths {
	t.Helper()
	return store.NewPaths(fixtureRoot(t))
}

func newTempPaths(t *testing.T) *store.Paths {
	t.Helper()
	paths := store.NewPaths(t.TempDir())
	if err := paths.EnsureTickets(); err != nil {
		t.Fatal(err)
	}
	return paths
}

func detail(id string, status model.Status, priority int, created, title string, deps ...string) *index.Detail {
	return &index.Detail{
		ID:       id,
		Status:   status,
		Priority: priority,
		Created:  created,
		Title:    title,
		Deps:     deps,
	}
}

func ids(entries []index.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}
