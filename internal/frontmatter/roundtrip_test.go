package frontmatter

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// testdataDir is the repository-root fixtures directory for this package.
const testdataDir = "../../testdata/frontmatter"

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// TestSerializeOfParse_GoodFixture_EqualsCanonicalize proves
// serialize(parse(x)) == canonicalize(x) on the good fixture (spec 8.2).
func TestSerializeOfParse_GoodFixture_EqualsCanonicalize(t *testing.T) {
	in := readFixture(t, "good.md")
	doc, err := Parse(in)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := Serialize(doc)

	canon, err := Canonicalize(in)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if !bytes.Equal(got, canon) {
		t.Fatalf("serialize(parse(x)) != canonicalize(x)\n--- serialize ---\n%s\n--- canonicalize ---\n%s", got, canon)
	}
}

// TestRoundTrip_SerializedFixture_IsByteIdenticalAndKeepsContent proves a
// second parse -> serialize pass over serialized output changes no bytes and
// keeps the ticket fields and the body.
func TestRoundTrip_SerializedFixture_IsByteIdenticalAndKeepsContent(t *testing.T) {
	doc, err := Parse(readFixture(t, "good.md"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	first := Serialize(doc)

	doc2, err := Parse(first)
	if err != nil {
		t.Fatalf("Parse(serialized): %v", err)
	}
	second := Serialize(doc2)

	if !bytes.Equal(first, second) {
		t.Fatalf("round-trip is not byte-identical\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	if !reflect.DeepEqual(doc.Ticket, doc2.Ticket) {
		t.Fatalf("ticket changed across round-trip:\n%+v\n%+v", doc.Ticket, doc2.Ticket)
	}
	if !bytes.Equal(doc.Body, doc2.Body) {
		t.Fatalf("body changed across round-trip:\n%q\n%q", doc.Body, doc2.Body)
	}
}

// TestCanonicalize_CanonicalFixture_IsFixedPoint proves canonicalizing an
// already canonical file changes no bytes: the second pass is a no-op.
func TestCanonicalize_CanonicalFixture_IsFixedPoint(t *testing.T) {
	canon, err := Canonicalize(readFixture(t, "good.md"))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	once, err := Canonicalize(canon)
	if err != nil {
		t.Fatalf("Canonicalize(canon): %v", err)
	}
	if !bytes.Equal(canon, once) {
		t.Fatalf("second canonicalize pass changed bytes\n%s\n%s", canon, once)
	}
}

// TestCanonicalize_GoodFixture_EmitsCanonicalFrontmatter is the golden check
// of the canonical form: known keys in fixed order, lists sorted, unknown keys
// after them.
func TestCanonicalize_GoodFixture_EmitsCanonicalFrontmatter(t *testing.T) {
	canon, err := Canonicalize(readFixture(t, "good.md"))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	wantPrefix := "---\n" +
		"status: open\n" +
		"priority: 2\n" +
		"deps: [ci, design]\n" +
		"attachments: [plan.md, screenshot.png]\n" +
		"created: 2026-10-08T11:30:40Z\n" +
		"updated: 2026-10-08T11:40:00Z\n" +
		"custom-key: some-value\n" +
		"---\n"
	if !bytes.HasPrefix(canon, []byte(wantPrefix)) {
		t.Fatalf("canonical output has unexpected frontmatter\nwant prefix:\n%s\ngot:\n%s", wantPrefix, canon)
	}
}
