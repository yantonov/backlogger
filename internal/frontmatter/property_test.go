package frontmatter

import (
	"bytes"
	"go/parser"
	"go/token"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yantonov/backlogger/internal/model"
)

// randomString builds an arbitrary scalar that exercises quoting and escaping.
// It always starts with a letter so the value is unambiguously a string under
// a reference YAML parser rather than being coerced to a number or date.
func randomString(r *rand.Rand, maxLen int) string {
	const alphabet = "abcXYZ019 ._-:/\\\"\t\n"
	const first = "abcXYZ"
	n := r.Intn(maxLen)
	var b strings.Builder
	if n > 0 {
		b.WriteByte(first[r.Intn(len(first))])
		n--
	}
	for i := 0; i < n; i++ {
		b.WriteByte(alphabet[r.Intn(len(alphabet))])
	}
	return b.String()
}

func randomKey(r *rand.Rand) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789-"
	n := 1 + r.Intn(10)
	var b strings.Builder
	b.WriteByte(alphabet[r.Intn(26)])
	for i := 1; i < n; i++ {
		b.WriteByte(alphabet[r.Intn(len(alphabet))])
	}
	return b.String()
}

func randomTicket(r *rand.Rand) *Document {
	statuses := []model.Status{model.StatusOpen, model.StatusInProgress, model.StatusClosed}
	t := model.Ticket{
		Status:   statuses[r.Intn(len(statuses))],
		Priority: r.Intn(model.MaxPriority + 1),
	}
	for i := 0; i < r.Intn(5); i++ {
		t.Deps = append(t.Deps, randomString(r, 12))
	}
	for i := 0; i < r.Intn(5); i++ {
		t.Attachments = append(t.Attachments, randomString(r, 12))
	}
	t.Created = "2026-10-08T11:30:40Z"
	t.Updated = "2026-10-08T11:40:00Z"
	if r.Intn(2) == 0 {
		t.ClosedAt = "2026-10-08T12:00:00Z"
	}
	if r.Intn(2) == 0 {
		t.Archived = "2026-10-09T09:00:00Z"
	}
	used := make(map[string]bool)
	for i := 0; i < r.Intn(4); i++ {
		key := randomKey(r)
		if isKnownKey(key) || used[key] {
			continue
		}
		used[key] = true
		val := randomString(r, 8)
		t.Extra = append(t.Extra, model.Extra{Key: key, Line: key + ": " + quoteIfNeeded(val)})
	}
	return &Document{Ticket: t, Body: []byte("# body\n\n---\n\nprose\n")}
}

func isKnownKey(k string) bool {
	for _, known := range model.CanonicalFieldOrder {
		if k == known {
			return true
		}
	}
	return false
}

// TestSerializeOfParse_RandomTicket_IsFixedPointAndKeepsBody is the
// arbitrary-fields property test: for any generated ticket, serialize -> parse
// -> serialize is a fixed point and the body is preserved (spec 10).
func TestSerializeOfParse_RandomTicket_IsFixedPointAndKeepsBody(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 500; i++ {
		doc := randomTicket(r)
		out := Serialize(doc)

		parsed, err := Parse(out)
		if err != nil {
			t.Fatalf("iteration %d: serializer emitted unparseable frontmatter: %v\n%s", i, err, out)
		}
		out2 := Serialize(parsed)
		if !bytes.Equal(out, out2) {
			t.Fatalf("iteration %d: serialize(parse(x)) != x\n--- first ---\n%q\n--- second ---\n%q", i, out, out2)
		}
		if !bytes.Equal(parsed.Body, doc.Body) {
			t.Fatalf("iteration %d: body not preserved\n%q\n%q", i, parsed.Body, doc.Body)
		}
	}
}

// TestCanonicalize_RandomTicket_IsFixedPoint checks that canonicalize is
// idempotent for arbitrary generated files.
func TestCanonicalize_RandomTicket_IsFixedPoint(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for i := 0; i < 300; i++ {
		doc := randomTicket(r)
		first, err := Canonicalize(Serialize(doc))
		if err != nil {
			t.Fatalf("iteration %d: Canonicalize: %v", i, err)
		}
		second, err := Canonicalize(first)
		if err != nil {
			t.Fatalf("iteration %d: second Canonicalize: %v", i, err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("iteration %d: canonicalize is not a fixed point", i)
		}
	}
}

// TestImports_ProductionGoFiles_ExcludeYAML enforces the test-only status of
// the reference YAML parser: no non-test file in the module may import a yaml
// package.
func TestImports_ProductionGoFiles_ExcludeYAML(t *testing.T) {
	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range f.Imports {
			if strings.Contains(imp.Path.Value, "yaml") {
				t.Errorf("production file %s imports %s", path, imp.Path.Value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}
