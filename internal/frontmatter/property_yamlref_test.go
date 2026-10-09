//go:build yamlref

// Package frontmatter property test: every file the writer emits must parse
// identically under the reference YAML parser (gopkg.in/yaml.v3).
//
// This file is behind the `yamlref` build tag so the dependency is test-only
// and never enters the production build. Run it with:
//
//	go test -tags yamlref ./internal/frontmatter/...
package frontmatter

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestSerialize_RandomTicket_DecodesIdenticallyUnderYAMLv3 proves the "valid
// YAML subset" claim: the canonical writer output is parsed by a reference YAML
// parser and the decoded values match the ticket the writer was given (spec
// 10).
func TestSerialize_RandomTicket_DecodesIdenticallyUnderYAMLv3(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 500; i++ {
		doc := randomTicket(r)
		out := Serialize(doc)

		front := frontBlock(t, out)

		var decoded map[string]any
		if err := yaml.Unmarshal(front, &decoded); err != nil {
			t.Fatalf("iteration %d: yaml.v3 rejected writer output: %v\n%s", i, err, out)
		}

		want := expectedMap(doc)
		if !reflect.DeepEqual(decoded, want) {
			t.Fatalf("iteration %d: yaml.v3 decoded different values\n--- got ---\n%#v\n--- want ---\n%#v\n--- source ---\n%s", i, decoded, want, out)
		}
	}
}

func frontBlock(t *testing.T, text []byte) []byte {
	t.Helper()
	front, _, hasFront, err := splitFences(text)
	if err != nil {
		t.Fatalf("splitFences: %v", err)
	}
	if !hasFront {
		t.Fatalf("writer output has no frontmatter:\n%s", text)
	}
	return front
}

// expectedMap builds the map a reference YAML parser should produce for a
// ticket. Lists are sorted because the serializer sorts them.
func expectedMap(doc *Document) map[string]any {
	t := doc.Ticket
	m := map[string]any{
		"status":      string(t.Status),
		"priority":    t.Priority,
		"deps":        sortedAny(t.Deps),
		"attachments": sortedAny(t.Attachments),
		"created":     timestamp(t.Created),
		"updated":     timestamp(t.Updated),
	}
	if t.ClosedAt != "" {
		m["closed-at"] = timestamp(t.ClosedAt)
	}
	if t.Archived != "" {
		m["archived"] = timestamp(t.Archived)
	}
	for _, e := range t.Extra {
		// Re-parse each opaque line's value with our own scalar parser so the
		// expected value matches what we wrote.
		colon := indexByte(e.Line, ':')
		raw := e.Line[colon+1:]
		for len(raw) > 0 && raw[0] == ' ' {
			raw = raw[1:]
		}
		if len(raw) > 0 && raw[0] == '[' {
			list, err := parseInlineList(raw)
			if err != nil {
				continue
			}
			m[e.Key] = sortedAny(list)
			continue
		}
		s, err := parseScalar(raw)
		if err != nil {
			continue
		}
		m[e.Key] = s
	}
	return m
}

// timestamp renders a timestamp string the way a reference YAML parser sees
// it: yaml.v3 resolves ISO-8601 timestamps to time.Time.
func timestamp(s string) any {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t
}

func sortedAny(in []string) []any {
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	anyOut := make([]any, len(out))
	for i, v := range out {
		anyOut[i] = v
	}
	return anyOut
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
