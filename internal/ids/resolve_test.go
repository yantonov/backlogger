package ids_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/yantonov/backlogger/internal/ids"
)

// TestResolve_OperandTable_ResolvesExactUniqueOrFails covers exact-wins, unique
// substring, ambiguity, and not-found resolution (spec 5.1).
func TestResolve_OperandTable_ResolvesExactUniqueOrFails(t *testing.T) {
	candidates := []string{"login", "login-flow", "logout", "ci"}

	tests := []struct {
		name    string
		operand string
		want    string
		wantErr error
	}{
		{"exact match", "login", "login", nil},
		{"exact match is case insensitive", "LOGIN", "login", nil},
		{"exact wins over substring", "login", "login", nil},
		{"unique substring", "out", "logout", nil},
		{"unique string with dash", "flow", "login-flow", nil},
		{"ambiguous prefix", "log", "", ids.ErrAmbiguous},
		{"ambiguous substring", "o", "", ids.ErrAmbiguous},
		{"no match", "zzz", "", ids.ErrNotFound},
		{"empty operand", "   ", "", ids.ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ids.Resolve(candidates, tt.operand)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Resolve(%q) = (%q, %v), want error matching %v", tt.operand, got, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(%q) returned error: %v", tt.operand, err)
			}
			if got != tt.want {
				t.Fatalf("Resolve(%q) = %q, want %q", tt.operand, got, tt.want)
			}
		})
	}
}

// TestResolve_ExactIDThatIsAlsoSubstring_ReturnsExactMatch checks the explicit
// tie-break rule with a candidate set where the exact ID is also a substring of
// another.
func TestResolve_ExactIDThatIsAlsoSubstring_ReturnsExactMatch(t *testing.T) {
	got, err := ids.Resolve([]string{"a", "ab", "abc"}, "ab")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ab" {
		t.Fatalf("Resolve(ab) = %q, want the exact match ab", got)
	}
}

// TestResolve_AmbiguousOperand_ListsCandidatesSorted checks the error message
// names every candidate, sorted, whatever the input order.
func TestResolve_AmbiguousOperand_ListsCandidatesSorted(t *testing.T) {
	_, err := ids.Resolve([]string{"zeta", "alpha", "beta"}, "a")

	if !errors.Is(err, ids.ErrAmbiguous) {
		t.Fatalf("expected ErrAmbiguous, got %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "matches alpha, beta, zeta") {
		t.Fatalf("ambiguous error %q does not list alpha, beta, zeta in order", msg)
	}
}

// TestResolve_UppercaseOperand_ReturnsStoredForm proves the returned ID is the
// stored candidate, not the lowercased operand.
func TestResolve_UppercaseOperand_ReturnsStoredForm(t *testing.T) {
	got, err := ids.Resolve([]string{"my-task"}, "MY-TASK")
	if err != nil {
		t.Fatal(err)
	}
	if got != "my-task" {
		t.Fatalf("Resolve = %q, want stored form my-task", got)
	}
}
