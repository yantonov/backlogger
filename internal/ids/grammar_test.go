package ids_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/yantonov/backlogger/internal/ids"
)

// TestValidate_GrammarTable_AcceptsOrRejectsPerRule covers every grammar rule
// in spec 5.1: length bounds, leading digit, underscore, uppercase, dot-prefix,
// reserved names, numeric-only, and minus placement (leading, trailing,
// doubled).
func TestValidate_GrammarTable_AcceptsOrRejectsPerRule(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr error // nil means valid
	}{
		// Length bounds.
		{"too short", "ab", ids.ErrGrammar},
		{"min length", "abc", nil},
		{"max length", strings.Repeat("a", 32), nil},
		{"too long", strings.Repeat("a", 33), ids.ErrGrammar},

		// Leading character.
		{"leading digit", "1abc", ids.ErrGrammar},
		{"leading letter", "a1bc", nil},
		{"leading minus", "-abc", ids.ErrGrammar},
		{"numeric only", "12345", ids.ErrGrammar},
		{"numeric only short", "123", ids.ErrGrammar},

		// Underscore is the attachment delimiter and is forbidden.
		{"underscore interior", "ab_c", ids.ErrGrammar},
		{"underscore leading", "_abc", ids.ErrGrammar},
		{"double underscore", "a__b", ids.ErrGrammar},

		// Uppercase is never tolerated in canonical form.
		{"uppercase interior", "abC", ids.ErrGrammar},
		{"uppercase leading", "Abc", ids.ErrGrammar},

		// Dot-prefixed names name reserved paths inside .tickets/.
		{"dot prefix", ".abc", ids.ErrGrammar},
		{"dot only", "...", ids.ErrGrammar},

		// Minus is allowed; leading minus is covered above.
		{"trailing minus", "abc-", nil},
		{"double minus", "a--b", nil},
		{"interior minus", "a-b-c", nil},

		// Reserved names.
		{"reserved archive", "archive", ids.ErrReserved},
		{"reserved attachments", "attachments", ids.ErrReserved},

		// Other forbidden bytes.
		{"space", "a b", ids.ErrGrammar},
		{"slash", "a/b", ids.ErrGrammar},
		{"non ascii", "abé", ids.ErrGrammar},
		{"empty", "", ids.ErrGrammar},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ids.Validate(tt.id)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate(%q) = %v, want nil", tt.id, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate(%q) = nil, want error matching %v", tt.id, tt.wantErr)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate(%q) = %v, want error matching %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

// The Normalize tests prove Normalize folds case and nothing else: a grammar
// violation other than case is never repaired.

func TestNormalize_MixedCaseID_FoldsToLowercase(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"login", "login"},
		{"LOGIN", "login"},
		{"Login", "login"},
		{"My-Task-42", "my-task-42"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ids.Normalize(tt.in)

			if err != nil {
				t.Fatalf("Normalize(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalize_NonCaseViolation_Rejects(t *testing.T) {
	for _, in := range []string{"ABC_DEF", "1ABC", "AB", "_AB", ".AB", "ARCHIVE", "ATTACHMENTS"} {
		t.Run(in, func(t *testing.T) {
			_, err := ids.Normalize(in)

			if err == nil {
				t.Fatalf("Normalize(%q) = nil error, want rejection", in)
			}
		})
	}
}

// The reserved-name tests check the reserved set explicitly, since these
// names are grammatical but name reserved paths.

func TestValidate_ReservedName_ReturnsErrReserved(t *testing.T) {
	for _, name := range []string{"archive", "attachments"} {
		t.Run(name, func(t *testing.T) {
			err := ids.Validate(name)

			if !errors.Is(err, ids.ErrReserved) {
				t.Fatalf("Validate(%q) = %v, want ErrReserved", name, err)
			}
		})
	}
}

// TestNormalize_ReservedNameInAnyCase_ReturnsErrReserved proves case folding
// is not a way around the reserved set.
func TestNormalize_ReservedNameInAnyCase_ReturnsErrReserved(t *testing.T) {
	for _, name := range []string{"archive", "attachments", "ARCHIVE", "Attachments"} {
		t.Run(name, func(t *testing.T) {
			_, err := ids.Normalize(name)

			if !errors.Is(err, ids.ErrReserved) {
				t.Fatalf("Normalize(%q) = %v, want ErrReserved", name, err)
			}
		})
	}
}

// The CheckAvailable tests guard the duplicate-explicit-ID contract
// (spec 5.1).

func existsLogin(id string) (bool, error) { return id == "login", nil }

func TestCheckAvailable_TakenIDInOtherCase_ReturnsErrDuplicate(t *testing.T) {
	err := ids.CheckAvailable("LOGIN", existsLogin)

	if !errors.Is(err, ids.ErrDuplicate) {
		t.Fatalf("CheckAvailable(LOGIN) = %v, want ErrDuplicate", err)
	}
}

func TestCheckAvailable_FreeID_ReturnsNil(t *testing.T) {
	err := ids.CheckAvailable("new-id", existsLogin)

	if err != nil {
		t.Fatalf("CheckAvailable(new-id) = %v, want nil", err)
	}
}

func TestCheckAvailable_BadGrammar_ReturnsErrGrammar(t *testing.T) {
	err := ids.CheckAvailable("bad_id", existsLogin)

	if !errors.Is(err, ids.ErrGrammar) {
		t.Fatalf("CheckAvailable(bad_id) = %v, want ErrGrammar", err)
	}
}

func TestCheckAvailable_NilPredicate_ReturnsError(t *testing.T) {
	err := ids.CheckAvailable("new-id", nil)

	if err == nil || !strings.Contains(err.Error(), "nil existence predicate") {
		t.Fatalf("CheckAvailable with nil predicate = %v, want the nil-predicate error", err)
	}
}

// TestExists_UppercaseID_QueriesLowercaseForm proves the existence helper
// canonicalizes before asking.
func TestExists_UppercaseID_QueriesLowercaseForm(t *testing.T) {
	var seen string
	exists := func(id string) (bool, error) { seen = id; return true, nil }
	got, err := ids.Exists("LOGIN", exists)
	if err != nil {
		t.Fatal(err)
	}
	if !got || seen != "login" {
		t.Fatalf("Exists(LOGIN) = (%v, seen %q), want (true, login)", got, seen)
	}
}
