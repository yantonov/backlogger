package ids

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrNotFound is returned by Resolve when the operand is neither an exact ID
// nor a prefix/substring of any candidate.
var ErrNotFound = errors.New("ticket not found")

// ErrAmbiguous is returned by Resolve when the operand matches more than one
// candidate. The error message lists the candidates.
var ErrAmbiguous = errors.New("ambiguous ticket ID")

// Resolve maps an operand to one ID from the candidate list. The rules are
// (spec 5.1):
//
//   - The operand is lowercased before comparison.
//   - An exact match always wins, even if it would also be an ambiguous
//     substring of other IDs.
//   - Otherwise a unique substring of a filename stem (candidate) resolves.
//   - Multiple matches return ErrAmbiguous with the sorted candidate list.
//   - No match returns ErrNotFound with the operand.
//
// The returned ID is the candidate in its stored (canonical) form, not the
// lowercased operand, so callers can trust it as a filename stem.
func Resolve(candidates []string, operand string) (string, error) {
	op := asciiLower(strings.TrimSpace(operand))
	if op == "" {
		return "", fmt.Errorf("%w: empty operand", ErrNotFound)
	}

	for _, c := range candidates {
		if c == op {
			return c, nil
		}
	}

	matches := make([]string, 0, 1)
	for _, c := range candidates {
		if strings.Contains(c, op) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w: %q", ErrNotFound, operand)
	case 1:
		return matches[0], nil
	default:
		sort.Strings(matches)
		return "", fmt.Errorf("%w: %q matches %s", ErrAmbiguous, operand, strings.Join(matches, ", "))
	}
}
