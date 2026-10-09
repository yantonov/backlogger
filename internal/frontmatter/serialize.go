package frontmatter

import (
	"sort"
	"strconv"
	"strings"
)

// Serialize renders a document in canonical form. The output is byte-stable:
// fixed field order, one field per line, inline lists sorted for deps and
// attachments, quotes only when a scalar is not a safe bare token, and no
// comments. Unknown/user keys are written back verbatim after the known keys,
// in their original relative order.
func Serialize(doc *Document) []byte {
	var b strings.Builder
	b.WriteString(fence)
	b.WriteByte('\n')

	t := doc.Ticket
	writeEnum(&b, "status", string(t.Status))
	writeInt(&b, "priority", t.Priority)
	writeList(&b, "deps", t.Deps)
	writeList(&b, "attachments", t.Attachments)
	writeScalar(&b, "created", t.Created)
	writeScalar(&b, "updated", t.Updated)
	if t.ClosedAt != "" {
		writeScalar(&b, "closed-at", t.ClosedAt)
	}
	if t.Archived != "" {
		writeScalar(&b, "archived", t.Archived)
	}
	for _, e := range t.Extra {
		b.WriteString(e.Line)
		b.WriteByte('\n')
	}

	b.WriteString(fence)
	b.WriteByte('\n')
	b.Write(doc.Body)
	return []byte(b.String())
}

func writeEnum(b *strings.Builder, key, value string) {
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteByte('\n')
}

func writeInt(b *strings.Builder, key string, value int) {
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(strconv.Itoa(value))
	b.WriteByte('\n')
}

// writeList emits an always-present inline list. Elements are sorted so the
// output does not depend on input order.
func writeList(b *strings.Builder, key string, values []string) {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	b.WriteString(key)
	b.WriteString(": [")
	for i, v := range sorted {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quoteIfNeeded(v))
	}
	b.WriteString("]\n")
}

// writeScalar emits a scalar that is always present. An empty value is emitted
// as a quoted empty string so the line stays a valid subset value (a bare
// empty value would be implicit null, which the parser rejects).
func writeScalar(b *strings.Builder, key, value string) {
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(quoteIfNeeded(value))
	b.WriteByte('\n')
}

// quoteIfNeeded returns token unchanged when it is a safe bare token, or a
// double-quoted, backslash-escaped form otherwise. A token that would be
// invalid or ambiguous as a YAML plain scalar (trailing colon, bool-ish word,
// empty) is quoted.
func quoteIfNeeded(token string) string {
	if token != "" && bareTokenRe.MatchString(token) &&
		!isForbiddenBareWord(token) && !strings.HasSuffix(token, ":") {
		return token
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(token); i++ {
		switch c := token[i]; c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
