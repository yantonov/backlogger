package frontmatter

import (
	"bytes"
	"testing"

	"github.com/yantonov/backlogger/internal/model"
)

// The unknown-key tests verify that unknown/user keys survive a parse ->
// rewrite byte-for-byte and in their original relative order (spec 8.3 / 12).

// unknownKeysDoc has three unknown keys interleaved with the known ones.
var unknownKeysDoc = []byte("---\n" +
	"status: open\n" +
	"priority: 2\n" +
	"deps: []\n" +
	"attachments: []\n" +
	"zebra: first\n" +
	"created: 2026-10-08T11:30:40Z\n" +
	"updated: 2026-10-08T11:30:40Z\n" +
	"alpha: \"quoted value\"\n" +
	"omega: [b, a]\n" +
	"---\n" +
	"body\n")

// unknownKeyLines are unknownKeysDoc's unknown lines in file order.
var unknownKeyLines = []string{"zebra: first", "alpha: \"quoted value\"", "omega: [b, a]"}

func TestParse_UnknownKeys_CapturesThemVerbatimInOrder(t *testing.T) {
	doc, err := Parse(unknownKeysDoc)

	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Ticket.Extra) != len(unknownKeyLines) {
		t.Fatalf("got %d extra keys, want %d: %+v", len(doc.Ticket.Extra), len(unknownKeyLines), doc.Ticket.Extra)
	}
	for i, want := range unknownKeyLines {
		if doc.Ticket.Extra[i].Line != want {
			t.Fatalf("extra[%d].Line = %q, want %q", i, doc.Ticket.Extra[i].Line, want)
		}
	}
}

func TestSerialize_ParsedUnknownKeys_EmitsThemVerbatimInOrder(t *testing.T) {
	doc, err := Parse(unknownKeysDoc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	out := Serialize(doc)

	last := -1
	for _, want := range unknownKeyLines {
		i := bytes.Index(out, []byte(want+"\n"))
		if i < 0 {
			t.Fatalf("serialized output lost %q:\n%s", want, out)
		}
		if i < last {
			t.Fatalf("unknown key %q moved before an earlier one:\n%s", want, out)
		}
		last = i
	}
}

// TestSerialize_DefaultTicket_EmitsCoreKeysAndOmitsUnsetConditionalKeys
// verifies the core-field invariant: status, priority, deps, attachments,
// created, and updated are emitted even when empty, and conditional keys are
// omitted when unset.
func TestSerialize_DefaultTicket_EmitsCoreKeysAndOmitsUnsetConditionalKeys(t *testing.T) {
	doc := &Document{Ticket: model.DefaultTicket(), Body: []byte("body\n")}
	out := Serialize(doc)
	for _, key := range []string{"status:", "priority:", "deps:", "attachments:", "created:", "updated:"} {
		if !bytes.Contains(out, []byte(key)) {
			t.Errorf("core key %q missing from output:\n%s", key, out)
		}
	}
	for _, key := range []string{"closed-at:", "archived:"} {
		if bytes.Contains(out, []byte(key)) {
			t.Errorf("conditional key %q emitted while unset:\n%s", key, out)
		}
	}
	if !bytes.Contains(out, []byte("created: \"\"\n")) {
		t.Errorf("empty timestamp should serialize as an explicit empty string:\n%s", out)
	}
}

// quotingCases pairs a scalar with its serialized form: quotes are added only
// when the scalar is not a safe bare token.
var quotingCases = []struct {
	name, in, want string
}{
	{"bare_token", "JIRA-123", "JIRA-123"},
	{"space", "hello world", `"hello world"`},
	{"bool_word", "true", `"true"`},
	{"empty", "", `""`},
	{"colon", "a:b", "a:b"},
	{"embedded_quote", "has\"quote", `"has\"quote"`},
}

// TestQuoteIfNeeded_Scalar_QuotesOnlyUnsafeTokens verifies quotes are added
// only when a scalar is not a safe bare token.
func TestQuoteIfNeeded_Scalar_QuotesOnlyUnsafeTokens(t *testing.T) {
	for _, tc := range quotingCases {
		t.Run(tc.name, func(t *testing.T) {
			got := quoteIfNeeded(tc.in)

			if got != tc.want {
				t.Errorf("quoteIfNeeded(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseScalar_QuotedScalar_RoundTrips verifies a serialized scalar parses
// back to the original value.
func TestParseScalar_QuotedScalar_RoundTrips(t *testing.T) {
	for _, tc := range quotingCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseScalar(quoteIfNeeded(tc.in))

			if err != nil {
				t.Fatalf("parseScalar(%s): %v", tc.want, err)
			}
			if got != tc.in {
				t.Errorf("round-trip of %q = %q", tc.in, got)
			}
		})
	}
}
