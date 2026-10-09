package attach

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Delimiter separates the ticket ID from the attachment suffix in a sidecar
// filename. Underscore is forbidden in IDs (spec 5.1), so the first occurrence
// of "__" is an unambiguous boundary: the ID is everything before it and the
// suffix is everything after it (spec 3.5).
const Delimiter = "__"

// ErrInvalidSuffix wraps every rejected attachment suffix. A suffix names a
// file inside .tickets/attachments/, so it must never be able to escape that
// directory.
var ErrInvalidSuffix = errors.New("invalid attachment suffix")

// Entry is one attachment of a ticket: the authoritative suffix and whether a
// matching sidecar file currently exists on disk.
type Entry struct {
	// Suffix is the frontmatter/storage suffix, e.g. "screenshot.png".
	Suffix string
	// Present reports whether the sidecar file exists.
	Present bool
}

// LinkIssues describes the disagreements between a ticket's authoritative
// `attachments:` list and the files present in the sidecar directory. It is the
// doctor support value.
type LinkIssues struct {
	// ID is the ticket the issues belong to.
	ID string
	// Missing lists suffixes that are linked in frontmatter but have no file.
	Missing []string
	// Orphans lists sidecar files that exist but are not linked in frontmatter.
	Orphans []string
}

// Empty reports whether there is no disagreement to report.
func (l LinkIssues) Empty() bool { return len(l.Missing) == 0 && len(l.Orphans) == 0 }

// ValidateSuffix reports whether suffix is safe to use as the part of a sidecar
// filename after the ID delimiter.
//
// A valid suffix is non-empty and contains only ASCII letters, digits, dot,
// dash, and underscore. Separators, drive/volume prefixes, and the special path
// elements "." and ".." are rejected, so a suffix can never escape the
// attachments directory (no path traversal).
func ValidateSuffix(suffix string) error {
	if suffix == "" {
		return fmt.Errorf("%w: suffix is empty", ErrInvalidSuffix)
	}
	if suffix == "." || suffix == ".." {
		return fmt.Errorf("%w: %q is a path element", ErrInvalidSuffix, suffix)
	}
	for _, r := range suffix {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			// Allowed.
		default:
			return fmt.Errorf("%w: %q contains %q", ErrInvalidSuffix, suffix, r)
		}
	}
	// filepath.Base is OS-aware and catches separators and volume prefixes that
	// the rune scan cannot see on every platform (for example `C:file` on
	// Windows).
	if filepath.Base(suffix) != suffix || filepath.IsAbs(suffix) {
		return fmt.Errorf("%w: %q is not a plain filename", ErrInvalidSuffix, suffix)
	}
	return nil
}

// SourceSuffix returns the valid suffix implied by a source path: its base
// name. An invalid base name is rejected with ErrInvalidSuffix.
func SourceSuffix(src string) (string, error) {
	suffix := filepath.Base(src)
	if err := ValidateSuffix(suffix); err != nil {
		return "", fmt.Errorf("%q: %w", src, err)
	}
	return suffix, nil
}

// Name returns the sidecar filename for id and suffix.
func Name(id, suffix string) string {
	return id + Delimiter + suffix
}

// Split parses a sidecar filename into its ID and suffix at the first "__"
// delimiter. It reports ok=false when the name has no delimiter or either half
// is empty.
func Split(name string) (id, suffix string, ok bool) {
	i := strings.Index(name, Delimiter)
	if i <= 0 || i+len(Delimiter) >= len(name) {
		return "", "", false
	}
	return name[:i], name[i+len(Delimiter):], true
}

// Path returns the absolute sidecar path for id and suffix. It does not touch
// the filesystem.
func Path(dir, id, suffix string) string {
	return filepath.Join(dir, Name(id, suffix))
}

// Exists reports whether the sidecar file for id and suffix is present.
func Exists(dir, id, suffix string) (bool, error) {
	_, err := os.Stat(Path(dir, id, suffix))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// FilesFor returns the suffixes of every sidecar file in dir whose ID is id,
// sorted. A missing directory is empty, not an error; the directory is optional
// (spec 3.5). Files for other tickets never appear.
func FilesFor(dir, id string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read attachments directory %s: %w", dir, err)
	}
	prefix := id + Delimiter
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(name, prefix)
		if suffix == "" {
			continue
		}
		out = append(out, suffix)
	}
	sort.Strings(out)
	return out, nil
}

// Entries returns the ticket's authoritative attachment list paired with
// on-disk presence, sorted by suffix. It is the read model for
// `bl attachments` and `bl show --attachments`.
func Entries(dir, id string, linked []string) ([]Entry, error) {
	out := make([]Entry, 0, len(linked))
	for _, suffix := range uniqueSorted(linked) {
		present, err := Exists(dir, id, suffix)
		if err != nil {
			return nil, err
		}
		out = append(out, Entry{Suffix: suffix, Present: present})
	}
	return out, nil
}

// CheckLinks compares the authoritative attachment list of one ticket with the
// sidecar files on disk and returns the missing links and orphan files. It is
// the doctor support function for the attachment invariant (spec 3.5); the full
// doctor check.
func CheckLinks(dir, id string, linked []string) (LinkIssues, error) {
	onDisk, err := FilesFor(dir, id)
	if err != nil {
		return LinkIssues{}, err
	}
	linkedSet := make(map[string]bool, len(linked))
	for _, suffix := range linked {
		linkedSet[suffix] = true
	}
	diskSet := make(map[string]bool, len(onDisk))
	for _, suffix := range onDisk {
		diskSet[suffix] = true
	}

	issues := LinkIssues{ID: id}
	for _, suffix := range uniqueSorted(linked) {
		if !diskSet[suffix] {
			issues.Missing = append(issues.Missing, suffix)
		}
	}
	for _, suffix := range onDisk {
		if !linkedSet[suffix] {
			issues.Orphans = append(issues.Orphans, suffix)
		}
	}
	return issues, nil
}

// Copy stores the contents of src as <id>__<suffix> in dir and returns the
// suffix. The directory is created lazily, so it exists only once a ticket
// actually has an attachment (spec 3.5). The write is atomic: a temp file in
// the same directory is fsynced and renamed over the destination, so a reader
// never sees a partial attachment.
//
// Copying the same ticket + suffix again replaces the stored bytes. The ID is
// part of the filename, so a copy can never collide with or overwrite a
// different ticket's attachment.
func Copy(dir, id, src string) (string, error) {
	suffix, err := SourceSuffix(src)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create attachments directory %s: %w", dir, err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("read attachment %s: %w", src, err)
	}
	perm := os.FileMode(0o644)
	if fi, err := os.Stat(src); err == nil {
		perm = fi.Mode().Perm()
	}
	dst := Path(dir, id, suffix)
	if filepath.Dir(dst) != filepath.Clean(dir) {
		return "", fmt.Errorf("%w: %q escapes %s", ErrInvalidSuffix, suffix, dir)
	}
	if err := writeFileAtomic(dst, data, perm); err != nil {
		return "", err
	}
	return suffix, nil
}

// Remove deletes the sidecar file for id and suffix. It reports whether a file
// was actually removed; a missing file is not an error (detach may be fixing
// only the frontmatter link).
func Remove(dir, id, suffix string) (bool, error) {
	if err := ValidateSuffix(suffix); err != nil {
		return false, err
	}
	err := os.Remove(Path(dir, id, suffix))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("remove attachment %s: %w", Path(dir, id, suffix), err)
}

// uniqueSorted returns values with duplicates removed and sorted, so rendering
// is deterministic even for a hand-edited frontmatter list.
func uniqueSorted(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// writeFileAtomic writes data to path via a temp file in the same directory, an
// fsync, and a rename. It mirrors the store's detail write so an attachment is
// never partially visible.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write attachment temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync attachment temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return fmt.Errorf("chmod attachment temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close attachment temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace attachment %s: %w", path, err)
	}
	return nil
}
