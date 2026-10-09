package index_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// TestBuild_GoldenFixture_MatchesGoldenIndex renders f from the fixture detail
// files and compares it byte-for-byte with the committed golden backlog.md.
// This pins the exact section layout, ordering, glyphs, trailing #id tokens,
// and the deps suffix.
func TestBuild_GoldenFixture_MatchesGoldenIndex(t *testing.T) {
	paths := fixturePaths(t)

	got, err := index.Build(paths)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want, err := os.ReadFile(filepath.Join(fixtureRoot(t), "backlog.md"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("rendered index does not match golden.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestReindex_AlreadyReindexedFixture_RewritesSameGoldenBytes copies the
// fixture into a writable temp dir, runs reindex twice, and proves the two runs
// produce byte-identical files (and match the golden). This is the spec-12 "two
// reindex runs produce no diff" requirement.
func TestReindex_AlreadyReindexedFixture_RewritesSameGoldenBytes(t *testing.T) {
	root := t.TempDir()
	copyTree(t, fixtureRoot(t), root)
	paths := store.NewPaths(root)

	if err := index.Reindex(paths); err != nil {
		t.Fatalf("first reindex: %v", err)
	}
	first, err := os.ReadFile(paths.IndexFile)
	if err != nil {
		t.Fatalf("read index after first reindex: %v", err)
	}

	if err := index.Reindex(paths); err != nil {
		t.Fatalf("second reindex: %v", err)
	}
	second, err := os.ReadFile(paths.IndexFile)
	if err != nil {
		t.Fatalf("read index after second reindex: %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Fatalf("two reindex runs differ.\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	want, err := os.ReadFile(filepath.Join(fixtureRoot(t), "backlog.md"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(second, want) {
		t.Fatalf("reindex output differs from golden.\n--- got ---\n%s\n--- want ---\n%s", second, want)
	}
}

// TestCommit_InjectedIndexReindex_WritesIndexWithNewTicket proves the store
// transaction helper, with the index ReindexFunc injected, regenerates
// backlog.md inside the same lock as the detail write (spec 3.1 write model).
func TestCommit_InjectedIndexReindex_WritesIndexWithNewTicket(t *testing.T) {
	root := t.TempDir()
	paths := store.NewPaths(root)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	tx, err := store.Begin(paths, store.TxOptions{Create: true, Reindex: index.ReindexFunc()})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	meta := &store.TicketMeta{
		ID: "login",
		Ticket: model.Ticket{
			Status:   model.StatusOpen,
			Priority: 1,
			Created:  model.FormatTimestamp(now),
			Updated:  model.FormatTimestamp(now),
		},
		Body: []byte("# Add login flow\n"),
	}
	if err := tx.WriteDetail(meta); err != nil {
		t.Fatalf("write detail: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := os.ReadFile(paths.IndexFile)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	want := "## Active\n- [ ] Add login flow #login\n\n## Closed\n"
	if string(got) != want {
		t.Fatalf("index after commit:\n%q\nwant:\n%q", got, want)
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy tree: %v", err)
	}
}
