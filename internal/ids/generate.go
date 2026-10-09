package ids

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	cryptorand "crypto/rand"
)

// Generation parameters (spec 5.1): a nanoid-style base32 token, 5 or 6
// characters long, always starting with a letter.
const (
	minGenerated = 5
	maxGenerated = 6
	maxAttempts  = 1000
)

// base32Alphabet is the 32-symbol generation alphabet: the lowercase ASCII
// letters followed by the digits 2-7. It is exactly 32 symbols, so a random
// byte taken modulo 32 is unbiased.
const base32Alphabet = "abcdefghijklmnopqrstuvwxyz234567"

// letterCount is the size of the leading-letter prefix of base32Alphabet.
const letterCount = 26

// ErrGenerateExhausted is returned when no unused token was found within
// maxAttempts. With a 32-symbol alphabet and 4-5 random symbols the chance is
// negligible; it exists so the loop can never spin forever.
var ErrGenerateExhausted = errors.New("ids: could not generate a unique ticket ID")

// Generate returns a fresh, unused ticket ID. The token is a nanoid-style
// base32 string 5-6 characters long that starts with a letter, so it always
// satisfies Validate. exists is consulted for every candidate; a nil predicate
// is an error. rng supplies the randomness and may be nil, in which case
// crypto/rand is used. Injecting rng makes generation deterministic in tests.
//
// Generate never returns a reserved name or a token that already exists
// according to exists (spec 5.1).
func Generate(exists ExistsFunc, rng io.Reader) (string, error) {
	if exists == nil {
		return "", errors.New("ids: nil existence predicate")
	}
	if rng == nil {
		rng = cryptorand.Reader
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		id, err := randomToken(rng)
		if err != nil {
			return "", err
		}
		if err := Validate(id); err != nil {
			// randomToken guarantees the grammar; this guards against a
			// future change to the alphabet.
			return "", fmt.Errorf("ids: generated invalid token %q: %w", id, err)
		}
		taken, err := exists(id)
		if err != nil {
			return "", fmt.Errorf("ids: check %q availability: %w", id, err)
		}
		if !taken {
			return id, nil
		}
	}
	return "", ErrGenerateExhausted
}

// randomToken draws one candidate from rng. The first symbol comes from the
// letter-only prefix; the remaining 4-5 symbols come from the full alphabet.
func randomToken(rng io.Reader) (string, error) {
	length := minGenerated
	b := make([]byte, 1)
	if _, err := io.ReadFull(rng, b); err != nil {
		return "", fmt.Errorf("ids: read randomness: %w", err)
	}
	if b[0]%2 == 1 {
		length = maxGenerated
	}
	out := make([]byte, length)
	// First symbol: a letter, drawn without modulo bias via rejection.
	for {
		if _, err := io.ReadFull(rng, b); err != nil {
			return "", fmt.Errorf("ids: read randomness: %w", err)
		}
		// 26 divides 234 (9*26), so values below 234 map uniformly.
		if int(b[0]) < letterCount*9 {
			out[0] = base32Alphabet[int(b[0])%letterCount]
			break
		}
	}
	for i := 1; i < length; i++ {
		if _, err := io.ReadFull(rng, b); err != nil {
			return "", fmt.Errorf("ids: read randomness: %w", err)
		}
		// 256 is a multiple of 32, so the direct modulo is unbiased.
		out[i] = base32Alphabet[int(b[0])%len(base32Alphabet)]
	}
	return string(out), nil
}

// DirExists returns an ExistsFunc backed by a tickets directory: it reports
// whether <dir>/<id>.md exists. The predicate is deliberately filesystem-only
// so callers can pass either the live .tickets/ directory or a test directory;
// they can compose it with a store-supplied check that also covers archive/.
func DirExists(dir string) ExistsFunc {
	return func(id string) (bool, error) {
		_, err := os.Stat(filepath.Join(dir, id+".md"))
		if err == nil {
			return true, nil
		}
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", filepath.Join(dir, id+".md"), err)
	}
}
