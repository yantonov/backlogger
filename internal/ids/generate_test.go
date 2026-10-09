package ids_test

import (
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/yantonov/backlogger/internal/ids"
)

// TestGenerate_RandomDraws_AreValidUnreservedAndUnique is the generation
// property test: with a seeded RNG and an in-memory directory predicate, many
// draws are always grammar-valid, never reserved, and never duplicated (spec
// 5.1).
func TestGenerate_RandomDraws_AreValidUnreservedAndUnique(t *testing.T) {
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic test RNG
	seen := map[string]bool{}
	exists := func(id string) (bool, error) { return seen[id], nil }

	const iterations = 2000
	for i := 0; i < iterations; i++ {
		id, err := ids.Generate(exists, rng)
		if err != nil {
			t.Fatalf("iteration %d: Generate returned error: %v", i, err)
		}
		if err := ids.Validate(id); err != nil {
			t.Fatalf("iteration %d: generated invalid ID %q: %v", i, id, err)
		}
		if len(id) < 5 || len(id) > 6 {
			t.Fatalf("iteration %d: generated ID %q has length %d, want 5-6", i, id, len(id))
		}
		if !isLetter(id[0]) {
			t.Fatalf("iteration %d: generated ID %q does not start with a letter", i, id)
		}
		if seen[id] {
			t.Fatalf("iteration %d: generated duplicate ID %q", i, id)
		}
		seen[id] = true
	}
}

// TestGenerate_EveryIDTaken_ReturnsErrGenerateExhausted proves Generate never
// returns a taken ID: a predicate that reports everything taken forces retries
// and eventually the typed exhaustion error, rather than a duplicate.
func TestGenerate_EveryIDTaken_ReturnsErrGenerateExhausted(t *testing.T) {
	rng := rand.New(rand.NewSource(7)) //nolint:gosec // deterministic test RNG
	exists := func(string) (bool, error) { return true, nil }
	_, err := ids.Generate(exists, rng)
	if !errors.Is(err, ids.ErrGenerateExhausted) {
		t.Fatalf("Generate with all-taken predicate = %v, want ErrGenerateExhausted", err)
	}
}

// TestGenerate_DirExistsWithFirstDrawTaken_SkipsExistingFile exercises
// DirExists against a real directory: the first token a seeded RNG draws
// already exists as a detail file, so Generate must skip it.
func TestGenerate_DirExistsWithFirstDrawTaken_SkipsExistingFile(t *testing.T) {
	dir := t.TempDir()
	never := func(string) (bool, error) { return false, nil }
	taken, err := ids.Generate(never, rand.New(rand.NewSource(3))) //nolint:gosec // deterministic test RNG
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, taken+".md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	id, err := ids.Generate(ids.DirExists(dir), rand.New(rand.NewSource(3))) //nolint:gosec // deterministic test RNG

	if err != nil {
		t.Fatal(err)
	}
	if id == taken {
		t.Fatalf("Generate returned %q, which exists on disk", id)
	}
	if err := ids.Validate(id); err != nil {
		t.Fatalf("generated invalid ID %q: %v", id, err)
	}
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
