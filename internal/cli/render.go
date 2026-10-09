package cli

import (
	"strings"
)

// Document body layout.
//
// The tracker stores no `title` frontmatter field; the title is the body's first
// ATX heading (spec 3.2), so `add` writes it as `# <title>` at the top of the
// body. Structured prose
// lives in `## <Section>` blocks below it. Both add and edit manipulate the
// body through the helpers below so the layout stays consistent and is a pure
// function of the bytes.
const (
	sectionDescription = "Description"
	sectionNotes       = "Notes"
)

// newBody renders a fresh ticket body: the title heading followed by any
// non-empty sections, in canonical order (Description, Notes). Empty sections
// are omitted.
func newBody(title string, sections []bodySection) []byte {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString("\n")
	writeSections(&b, sections)
	return []byte(b.String())
}

// bodySection is one named prose block. An empty Text is not emitted.
type bodySection struct {
	Name string
	Text string
}

func writeSections(b *strings.Builder, sections []bodySection) {
	for _, s := range sections {
		if s.Text == "" {
			continue
		}
		b.WriteString("\n## ")
		b.WriteString(s.Name)
		b.WriteString("\n")
		b.WriteString(strings.TrimRight(s.Text, "\n"))
		b.WriteString("\n")
	}
}

// bodyTitle returns the text of the first ATX heading in body, or "".
func bodyTitle(body []byte) string {
	for _, line := range splitLines(body) {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		text := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		text = strings.TrimSpace(strings.Trim(text, "#"))
		if text != "" {
			return text
		}
	}
	return ""
}

// setBodyTitle replaces the first ATX heading with `# title`, or prepends one
// when the body has no heading. Other content is preserved verbatim.
func setBodyTitle(body []byte, title string) []byte {
	lines := splitLines(body)
	heading := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
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

// setSection replaces or appends a `## name` section. An empty text removes the
// section when it exists (and does nothing when it does not). When appendMode
// is true and the section exists, text is appended to it on a new line instead
// of replacing the existing prose.
func setSection(body []byte, name, text string, appendMode bool) []byte {
	lines := splitLines(body)
	start, end := sectionBounds(body, name)

	if start < 0 {
		if text == "" {
			return joinLines(lines)
		}
		out := append([]string(nil), lines...)
		out = append(out, "", "## "+name, strings.TrimRight(text, "\n"))
		return joinLines(out)
	}

	if text == "" {
		// Remove the heading and its block, plus the blank separator directly
		// before it when present.
		from := start
		for from > 0 && strings.TrimSpace(lines[from-1]) == "" {
			from--
		}
		out := append([]string(nil), lines[:from]...)
		out = append(out, lines[end:]...)
		return joinLines(out)
	}

	merged := text
	if appendMode {
		existing := strings.TrimSpace(strings.Join(lines[start+1:end], "\n"))
		if existing != "" {
			merged = existing + "\n\n" + strings.TrimRight(text, "\n")
		}
	}
	block := strings.Split("## "+name+"\n"+strings.TrimRight(merged, "\n"), "\n")
	out := append([]string(nil), lines[:start]...)
	out = append(out, block...)
	out = append(out, lines[end:]...)
	return joinLines(out)
}

// sectionBounds returns the [start, end) line range of the `## name` section,
// where start is the heading line and end is the line before the next `## `
// heading (or len(lines)). It returns (-1, -1) when the section is absent.
func sectionBounds(body []byte, name string) (int, int) {
	lines := splitLines(body)
	heading := "## " + name
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, heading) {
			start = i
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	return start, end
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
