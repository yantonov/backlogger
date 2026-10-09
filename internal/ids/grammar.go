package ids

import (
	"errors"
	"fmt"
)

// Length bounds for a ticket ID (spec 5.1).
const (
	MinLength = 3
	MaxLength = 32
)

// Sentinel errors. Callers match them with errors.Is.
var (
	// ErrGrammar wraps every grammar violation (length, alphabet, casing).
	ErrGrammar = errors.New("invalid ticket ID")
	// ErrReserved wraps the reserved names archive and attachments.
	ErrReserved = errors.New("reserved ticket ID")
	// ErrDuplicate is returned when an explicit ID is already taken.
	ErrDuplicate = errors.New("duplicate ticket ID")
)

// reservedNames cannot be IDs because they name paths inside .tickets/
// (spec 5.1). The lookup is on the lowercased, validated form.
var reservedNames = map[string]bool{
	"archive":     true,
	"attachments": true,
}

// Validate reports whether id is a legal ticket ID in canonical (lowercase)
// form. The grammar is ^[a-z][a-z0-9-]{2,31}$ (spec 5.1). Underscore and
// uppercase are rejected, and the reserved names archive and attachments are
// rejected. Validate never repairs its input; use Normalize for case folding.
func Validate(id string) error {
	if len(id) < MinLength || len(id) > MaxLength {
		return fmt.Errorf("%w: %q must be %d-%d characters", ErrGrammar, id, MinLength, MaxLength)
	}
	if id[0] == '.' {
		return fmt.Errorf("%w: %q: dot-prefixed names are reserved", ErrGrammar, id)
	}
	if !isASCIILetter(id[0]) {
		return fmt.Errorf("%w: %q must start with an ASCII letter", ErrGrammar, id)
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			// Legal.
		case c == '_':
			return fmt.Errorf("%w: %q: underscore is forbidden (attachment delimiter)", ErrGrammar, id)
		case c >= 'A' && c <= 'Z':
			return fmt.Errorf("%w: %q: uppercase is not allowed; IDs are lowercase", ErrGrammar, id)
		default:
			return fmt.Errorf("%w: %q: character %q is not allowed", ErrGrammar, id, c)
		}
	}
	if reservedNames[id] {
		return fmt.Errorf("%w: %q is reserved", ErrReserved, id)
	}
	return nil
}

// Normalize lowercases s and then validates it. Case is the only thing ever
// repaired: any other grammar violation is returned as an error, never silently
// rewritten (spec 5.1).
func Normalize(s string) (string, error) {
	id := asciiLower(s)
	if err := Validate(id); err != nil {
		return "", err
	}
	return id, nil
}

// ExistsFunc reports whether a ticket ID is already in use. The store supplies
// it (for example by listing .tickets/). ids never imports the filesystem, so
// the uniqueness check stays injectable and testable.
type ExistsFunc func(id string) (bool, error)

// Exists normalizes and validates id, then asks exists whether the canonical
// form is taken. A grammar violation is returned before the predicate runs.
func Exists(id string, exists ExistsFunc) (bool, error) {
	canonical, err := Normalize(id)
	if err != nil {
		return false, err
	}
	if exists == nil {
		return false, errors.New("ids: nil existence predicate")
	}
	return exists(canonical)
}

// CheckAvailable is the duplicate guard for explicit IDs: it rejects an ID that
// is not grammatical and one that already exists, so callers never silently
// overwrite a ticket (spec 5.1).
func CheckAvailable(id string, exists ExistsFunc) error {
	canonical, err := Normalize(id)
	if err != nil {
		return err
	}
	if exists == nil {
		return errors.New("ids: nil existence predicate")
	}
	taken, err := exists(canonical)
	if err != nil {
		return fmt.Errorf("ids: check %q availability: %w", canonical, err)
	}
	if taken {
		return fmt.Errorf("%w: %q already exists", ErrDuplicate, canonical)
	}
	return nil
}

// asciiLower lowercases only the ASCII letters A-Z. Non-ASCII bytes are left
// untouched so the subsequent character scan rejects them instead of folding
// them into a legal ASCII character.
func asciiLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
