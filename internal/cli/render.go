package cli

import "strings"

// Document body layout.
//
// The body is one opaque prose block. The title is a required frontmatter field
// (spec 5.3), so the body carries no structural element at all: the tracker
// never parses headings inside it, which means a `# ` line inside a shell fence
// is just text and can never be mistaken for the ticket title. Both add and edit
// manipulate the body through the helpers below.

// newBody renders a fresh ticket body: the description when one was given. The
// description is stored verbatim.
func newBody(description string) []byte {
	var b strings.Builder
	writeProse(&b, description)
	return []byte(b.String())
}

// writeProse writes text as the body's single prose block.
func writeProse(b *strings.Builder, text string) {
	if text == "" {
		return
	}
	b.WriteString(strings.TrimRight(text, "\n"))
	b.WriteString("\n")
}

// setDescription replaces the whole body verbatim. An empty text empties it. The
// payload is never scanned for headings, so any `# ` or `## ` line in it is
// stored as text.
func setDescription(text string) []byte {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return []byte(strings.TrimRight(text, "\n") + "\n")
}

// appendProse adds text to the end of the body as a separate paragraph. The
// payload is stored verbatim.
func appendProse(body []byte, text string) []byte {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return body
	}
	lines := append(splitLines(body), "", text)
	return joinLines(lines)
}

// splitLines splits body into lines, dropping a single trailing newline so the
// result round-trips through joinLines.
func splitLines(body []byte) []string {
	s := strings.TrimSuffix(string(body), "\n")
	if s == "" {
		return nil
	}
	raw := strings.Split(s, "\n")
	for i := range raw {
		raw[i] = strings.TrimSuffix(raw[i], "\r")
	}
	return raw
}

// joinLines joins lines with "\n" and a trailing newline. An empty line list
// yields empty bytes.
func joinLines(lines []string) []byte {
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}
