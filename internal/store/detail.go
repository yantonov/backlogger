package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yantonov/backlogger/internal/frontmatter"
	"github.com/yantonov/backlogger/internal/lock"
	"github.com/yantonov/backlogger/internal/model"
)

// ErrNotFound is returned when a detail file does not exist.
var ErrNotFound = errors.New("ticket not found")

// IsNotFound reports whether err is or wraps ErrNotFound.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsLockTimeout reports whether err is a lock acquisition timeout. A caller
// that sees this has made no changes (spec 6.1).
func IsLockTimeout(err error) bool { return lock.IsTimeout(err) }

// TicketMeta describes a detail file's identity and parsed contents.
type TicketMeta struct {
	// ID is the filename stem, which is the authoritative identity (spec 3.4).
	ID string
	// Archived reports whether the file lives under .tickets/archive/.
	Archived bool
	// Ticket is the parsed frontmatter.
	Ticket model.Ticket
	// Body is the Markdown body after the closing fence.
	Body []byte
	// Path is the absolute path of the detail file.
	Path string
}

// ActivePath returns the path of an active ticket's detail file.
func (p *Paths) ActivePath(id string) string {
	return filepath.Join(p.TicketsDir, id+".md")
}

// ArchivedPath returns the path of an archived ticket's detail file.
func (p *Paths) ArchivedPath(id string) string {
	return filepath.Join(p.ArchiveDir, id+".md")
}

// DetailPath returns the detail path for id, choosing the archive directory
// when archived is true.
func (p *Paths) DetailPath(id string, archived bool) string {
	if archived {
		return p.ArchivedPath(id)
	}
	return p.ActivePath(id)
}

// ReadDetail reads and parses one detail file. A read error on an existing
// file is propagated and is never treated as an empty backlog (spec 6.2). A
// missing file returns ErrNotFound wrapped with the path.
func (p *Paths) ReadDetail(id string) (*TicketMeta, error) {
	path := p.ActivePath(id)
	meta, err := p.readDetailAt(path)
	if err == nil {
		return meta, nil
	}
	if errors.Is(err, ErrNotFound) {
		// Fall back to the archive directory: a ticket is identified by its
		// stem wherever it lives.
		archivedPath := p.ArchivedPath(id)
		if archived, aerr := p.readDetailAt(archivedPath); aerr == nil {
			return archived, nil
		} else if !errors.Is(aerr, ErrNotFound) {
			return nil, aerr
		}
	}
	return nil, err
}

func (p *Paths) readDetailAt(path string) (*TicketMeta, error) {
	data, err := readFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := frontmatter.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &TicketMeta{
		ID:       stem(path),
		Archived: isArchivePath(p, path),
		Ticket:   doc.Ticket,
		Body:     doc.Body,
		Path:     path,
	}, nil
}

// WriteDetail serializes meta and writes it atomically, preserving the file
// mode of an existing file. It does not touch backlog.md; that is the
// transaction helper's job.
func (p *Paths) WriteDetail(meta *TicketMeta) error {
	if meta == nil {
		return errors.New("nil TicketMeta")
	}
	path := meta.Path
	if path == "" {
		path = p.DetailPath(meta.ID, meta.Archived)
	}
	data := frontmatter.Serialize(&frontmatter.Document{Ticket: meta.Ticket, Body: meta.Body})
	if err := writeFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("write detail %s: %w", meta.ID, err)
	}
	return nil
}

// WriteDetailMoved writes meta at the location implied by its `archived` field
// and removes the file at its previous path when the location changes. A file
// with a non-empty archived field lives under .tickets/archive/; an empty field
// lives directly under .tickets/. Keeping the location and the field in step is
// the archive invariant (spec 5.4).
//
// A move never overwrites: when the destination already holds a file, the ID
// is duplicated in active and archive, and the call fails without touching
// either copy. Which copy is right is the user's decision.
func (p *Paths) WriteDetailMoved(meta *TicketMeta) error {
	if meta == nil {
		return errors.New("nil TicketMeta")
	}
	wantArchived := meta.Ticket.Archived != ""
	dest := p.DetailPath(meta.ID, wantArchived)
	old := meta.Path
	if old != "" && old != dest {
		if _, err := os.Lstat(dest); err == nil {
			return fmt.Errorf("ticket %s exists in both %s and %s; keep one copy by hand (bl doctor reports the duplicate)", meta.ID, old, dest)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", dest, err)
		}
	}
	if wantArchived {
		if err := p.EnsureArchive(); err != nil {
			return err
		}
	}
	meta.Archived = wantArchived
	meta.Path = dest
	if err := p.WriteDetail(meta); err != nil {
		return err
	}
	if old != "" && old != dest {
		if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", old, err)
		}
	}
	return nil
}

// ListArchiveIDs returns the stems of the archived detail files without opening
// any of them. Partial-ID resolution and the doctor use it to know archive
// membership without reading the file (spec 5.4).
func (p *Paths) ListArchiveIDs() ([]string, error) {
	return p.listIDs(p.ArchiveDir)
}

// CandidateIDs returns the stems of every detail file (active and archived)
// without opening any file. Partial-ID resolution uses it so that even a
// command that never reads archive contents can still resolve an archived ID.
func (p *Paths) CandidateIDs() ([]string, error) {
	seen := make(map[string]bool)
	for _, dir := range []string{p.TicketsDir, p.ArchiveDir} {
		ids, err := p.listIDs(dir)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			seen[id] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// listIDs returns the stems of the *.md files directly inside dir, sorted. A
// missing directory is treated as empty.
func (p *Paths) listIDs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read directory %s: %w", dir, err)
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(ids)
	return ids, nil
}

// WriteDetailBytes writes an already-serialized detail body atomically.
func (p *Paths) WriteDetailBytes(path string, data []byte) error {
	return writeFileAtomic(path, data, 0o644)
}

// WriteIndex writes backlog.md atomically. The index is a derived view, so a
// crash can only leave it stale, never partially written (spec 3.1/6.2).
func (p *Paths) WriteIndex(data []byte) error {
	return writeFileAtomic(p.IndexFile, data, 0o644)
}

// ReadDetailBytes reads a detail file verbatim. It is used where the caller
// needs the raw bytes (for example hashing) and shares the read-error contract
// of ReadDetail: a missing file is ErrNotFound, any other error propagates.
func (p *Paths) ReadDetailBytes(path string) ([]byte, error) {
	data, err := readFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// ListActive returns the metadata of every active detail file, sorted by ID.
// The attachments/ and archive/ directories are never scanned here.
func (p *Paths) ListActive() ([]*TicketMeta, error) {
	return p.listDir(p.TicketsDir)
}

// ListArchived returns the metadata of every archived detail file, sorted by ID.
func (p *Paths) ListArchived() ([]*TicketMeta, error) {
	return p.listDir(p.ArchiveDir)
}

// ListAll returns active then archived tickets, each group sorted by ID.
func (p *Paths) ListAll() ([]*TicketMeta, error) {
	active, err := p.ListActive()
	if err != nil {
		return nil, err
	}
	archived, err := p.ListArchived()
	if err != nil {
		return nil, err
	}
	return append(active, archived...), nil
}

// listDir reads every *.md file directly inside dir. A missing directory is
// treated as empty (its absence is not a data-loss error). Any other error
// propagates.
func (p *Paths) listDir(dir string) ([]*TicketMeta, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read directory %s: %w", dir, err)
	}
	var out []*TicketMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		meta, err := p.readDetailAt(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// CoreHash returns a stable content hash of a detail file. It is derived from
// the canonical serialization of the parsed document, so an unrelated rewrite
// of the body does not change it only when the body is unchanged as well. It is
// the value a future --if-match would compare (spec 6.4); OCC itself is out of
// scope.
func (p *Paths) CoreHash(meta *TicketMeta) string {
	if meta == nil {
		return ""
	}
	data := frontmatter.Serialize(&frontmatter.Document{Ticket: meta.Ticket, Body: meta.Body})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func stem(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".md")
}

func isArchivePath(p *Paths, path string) bool {
	rel, err := filepath.Rel(p.ArchiveDir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
