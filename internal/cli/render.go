package cli

import (
	"strings"
)

// Document body layout.
//
// The tracker stores no `title` frontmatter field; the title is the body's first
// ATX heading (spec 3.2), so `add` writes it as `# <title>` at the top of the
// body. Everything below that heading is one opaque prose block: the tracker
// never parses headings inside it, so a description is stored verbatim and a
// `## ` line the writer supplied is just text. Both add and edit manipulate the
// body through the helpers below so the layout stays consistent and is a pure
// function of the bytes.

// newBody renders a fresh ticket body: the title heading followed by the
// description when one was given. The description is stored verbatim.
func newBody(title, description string) []byte {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n")
	writeProse(&b, description)
	return []byte(b.String())
}

func writeProse(b *strings.Builder, text string) {
	if text == "" {
		return
	}
	b.WriteString("\n")
	b.WriteString(strings.TrimRight(text, "\n"))
	b.WriteString("\n")
}

// bodyTitle returns the text of the body's title heading, or "". Only a
// single-`#` heading is the title; a `## ` line is prose (spec 3.2).
func bodyTitle(body []byte) string {
	for _, line := range splitLines(body) {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "# ") {
			continue
		}
		if text := strings.TrimSpace(strings.Trim(trimmed, "#")); text != "" {
			return text
		}
	}
	return ""
}

// setBodyTitle replaces the title heading with `# title`, or prepends one when
// the body has no title heading. Other content is preserved verbatim.
func setBodyTitle(body []byte, title string) []byte {
	lines := splitLines(body)
	heading := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			heading = i
			break
		}
	}
	replacement := "# " + title
	if heading < 0 {
		out := []string{replacement}
		out = append(out, lines...)
		return joinLines(out)
	}
	lines[heading] = replacement
	return joinLines(lines)
}

// proseStart returns the line index where the description begins: the line
// after the body's title heading, or 0 when the body has no heading. The title
// is the only structure the tracker reads out of a body, so everything after it
// is prose (spec 3.2).
func proseStart(lines []string) int {
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			return i + 1
		}
	}
	return 0
}

// setDescription replaces the whole description verbatim, keeping the title
// heading. An empty text removes the description. The payload is never scanned
// for headings, so any `## ` line in it is stored as text.
func setDescription(body []byte, text string) []byte {
	lines := splitLines(body)
	start := proseStart(lines)
	out := append([]string(nil), lines[:start]...)
	if strings.TrimSpace(text) != "" {
		out = append(out, "", strings.TrimRight(text, "\n"))
	}
	return joinLines(out)
}

// appendProse adds text to the end of the body as a separate paragraph, keeping
// the title and the existing description. The payload is stored verbatim.
func appendProse(body []byte, text string) []byte {
	lines := splitLines(body)
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return joinLines(lines)
	}
	return joinLines(append(append([]string(nil), lines...), "", text))
}

// splitLines splits body into lines, dropping a single trailing newline so the
// result round-trips through joinLines. A CR is trimmed per line.
func splitLines(body []byte) []string {
	s := string(body)
	s = strings.TrimSuffix(s, "\n")
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

// ensureHeading guarantees the body starts with an ATX heading. It is used
// after edit operations that might otherwise leave a headingless body.
func ensureHeading(body []byte, id string) []byte {
	if bodyTitle(body) != "" {
		return body
	}
	return setBodyTitle(body, id)
}
