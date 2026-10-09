package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Paths holds the resolved paths of a backlog. The zero value is not usable;
// build one with NewPaths or Discover.
type Paths struct {
	// Root is the project root directory.
	Root string
	// TicketsDir is <root>/.tickets.
	TicketsDir string
	// ArchiveDir is <root>/.tickets/archive.
	ArchiveDir string
	// AttachmentsDir is <root>/.tickets/attachments.
	AttachmentsDir string
	// LockFile is <root>/.tickets/.lock (persistent, never deleted).
	LockFile string
	// IndexFile is <root>/backlog.md.
	IndexFile string
}

// ErrNoTickets is returned when a read command needs a .tickets/ directory that
// does not exist. Reads never create state (spec 7.1).
var ErrNoTickets = errors.New(".tickets directory not found; run `bl add` to create a backlog")

// NewPaths builds the path set for an explicit project root. It does not touch
// the filesystem.
func NewPaths(root string) *Paths {
	return &Paths{
		Root:           root,
		TicketsDir:     filepath.Join(root, ".tickets"),
		ArchiveDir:     filepath.Join(root, ".tickets", "archive"),
		AttachmentsDir: filepath.Join(root, ".tickets", "attachments"),
		LockFile:       filepath.Join(root, ".tickets", ".lock"),
		IndexFile:      filepath.Join(root, "backlog.md"),
	}
}

// Discover finds the project root that contains start and returns its paths.
// It walks up from start looking for an existing .tickets/ directory or a .git
// entry (directory or file, so linked worktrees are found too). If neither is
// found it uses start itself, which is where a write would create .tickets/.
//
// Discovery never creates or modifies anything.
func Discover(start string) (*Paths, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	root, err := findRoot(abs)
	if err != nil {
		return nil, err
	}
	return NewPaths(root), nil
}

func findRoot(start string) (string, error) {
	dir := start
	for {
		if isDir(filepath.Join(dir, ".tickets")) || exists(filepath.Join(dir, ".git")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start, nil
		}
		dir = parent
	}
}

// TicketsExist reports whether .tickets/ exists.
func (p *Paths) TicketsExist() bool { return isDir(p.TicketsDir) }

// RequireTickets returns ErrNoTickets when .tickets/ is missing. Read commands
// call this before reading (spec 7.1).
func (p *Paths) RequireTickets() error {
	if !p.TicketsExist() {
		return fmt.Errorf("%s: %w", p.TicketsDir, ErrNoTickets)
	}
	return nil
}

// EnsureTickets creates .tickets/ (and only .tickets/). It is called on the
// write path; archive/ and attachments/ are created lazily by their commands.
func (p *Paths) EnsureTickets() error {
	if err := os.MkdirAll(p.TicketsDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", p.TicketsDir, err)
	}
	return nil
}

// EnsureArchive creates .tickets/archive/ lazily.
func (p *Paths) EnsureArchive() error {
	if err := os.MkdirAll(p.ArchiveDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", p.ArchiveDir, err)
	}
	return nil
}

// EnsureAttachments creates .tickets/attachments/ lazily.
func (p *Paths) EnsureAttachments() error {
	if err := os.MkdirAll(p.AttachmentsDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", p.AttachmentsDir, err)
	}
	return nil
}

// exists reports whether path exists, following symlinks.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
