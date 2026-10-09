package frontmatter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yantonov/backlogger/internal/model"
)

// ParseError is returned for any out-of-subset frontmatter. It is always loud:
// the parser never guesses or silently coerces a value (spec 8.2).
type ParseError struct {
	// Line is the 1-based line number inside the frontmatter block.
	Line int
	// Msg describes the offending construct.
	Msg string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("frontmatter line %d: %s", e.Line, e.Msg)
}

// Document is a parsed detail file: typed frontmatter plus the Markdown body
// that follows the closing fence.
type Document struct {
	Ticket model.Ticket
	Body   []byte
}

var (
	bareTokenRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
	intRe       = regexp.MustCompile(`^-?[0-9]+$`)
)

// Parse reads the frozen flat YAML subset from text. A file without a leading
// fence is a body-only ticket with default fields and is not an error (spec
// 8.2).
func Parse(text []byte) (*Document, error) {
	front, body, hasFront, err := splitFences(text)
	if err != nil {
		return nil, err
	}
	doc := &Document{Ticket: model.DefaultTicket(), Body: body}
	if !hasFront {
		return doc, nil
	}
	if err := parseBlock(front, &doc.Ticket); err != nil {
		return nil, err
	}
	return doc, nil
}

// Canonicalize parses text and re-serializes it in canonical form.
func Canonicalize(text []byte) ([]byte, error) {
	doc, err := Parse(text)
	if err != nil {
		return nil, err
	}
	return Serialize(doc), nil
}

func parseBlock(front []byte, t *model.Ticket) error {
	lines := strings.Split(string(front), "\n")
	seen := make(map[string]bool)
	for i, raw := range lines {
		lineNo := i + 1
		// The front block excludes the closing fence line; strings.Split may
		// still yield one empty trailing element when the block ends in \n.
		if i == len(lines)-1 && raw == "" {
			continue
		}
		line := strings.TrimSuffix(raw, "\r")
		if err := parseLine(line, lineNo, t, seen); err != nil {
			return err
		}
	}
	return nil
}

func parseLine(line string, lineNo int, t *model.Ticket, seen map[string]bool) error {
	if line == "" {
		return &ParseError{Line: lineNo, Msg: "empty lines are not allowed in frontmatter"}
	}
	if line[0] == ' ' || line[0] == '\t' {
		return &ParseError{Line: lineNo, Msg: "indented or nested maps are not allowed"}
	}
	if line[0] == '#' {
		return &ParseError{Line: lineNo, Msg: "comments are not allowed in frontmatter"}
	}

	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return &ParseError{Line: lineNo, Msg: "expected a key: value line"}
	}
	key := line[:colon]
	if strings.ContainsRune(key, '\t') {
		return &ParseError{Line: lineNo, Msg: "tab characters are not allowed in keys"}
	}
	if !bareTokenRe.MatchString(key) {
		return &ParseError{Line: lineNo, Msg: fmt.Sprintf("invalid key %q", key)}
	}
	if seen[key] {
		return &ParseError{Line: lineNo, Msg: fmt.Sprintf("duplicate key %q", key)}
	}
	seen[key] = true

	value := strings.TrimSpace(line[colon+1:])
	if value == "" {
		return &ParseError{Line: lineNo, Msg: fmt.Sprintf("key %q has no value (implicit null is not allowed)", key)}
	}

	switch key {
	case "status":
		if !model.Status(value).Valid() {
			return &ParseError{Line: lineNo, Msg: fmt.Sprintf("invalid status %q", value)}
		}
		t.Status = model.Status(value)
	case "priority":
		n, err := parsePriority(value)
		if err != nil {
			return &ParseError{Line: lineNo, Msg: err.Error()}
		}
		t.Priority = n
	case "deps":
		list, err := parseInlineList(value)
		if err != nil {
			return &ParseError{Line: lineNo, Msg: err.Error()}
		}
		t.Deps = list
	case "attachments":
		list, err := parseInlineList(value)
		if err != nil {
			return &ParseError{Line: lineNo, Msg: err.Error()}
		}
		t.Attachments = list
	case "created":
		s, err := parseScalar(value)
		if err != nil {
			return &ParseError{Line: lineNo, Msg: err.Error()}
		}
		t.Created = s
	case "updated":
		s, err := parseScalar(value)
		if err != nil {
			return &ParseError{Line: lineNo, Msg: err.Error()}
		}
		t.Updated = s
	case "closed-at":
		s, err := parseScalar(value)
		if err != nil {
			return &ParseError{Line: lineNo, Msg: err.Error()}
		}
		t.ClosedAt = s
	case "archived":
		s, err := parseScalar(value)
		if err != nil {
			return &ParseError{Line: lineNo, Msg: err.Error()}
		}
		t.Archived = s
	default:
		if err := validateValue(value); err != nil {
			return &ParseError{Line: lineNo, Msg: err.Error()}
		}
		t.Extra = append(t.Extra, model.Extra{Key: key, Line: line})
	}
	return nil
}

func parsePriority(value string) (int, error) {
	if !intRe.MatchString(value) {
		return 0, fmt.Errorf("priority must be an integer, got %q", value)
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("priority must be an integer, got %q", value)
	}
	if n < model.MinPriority || n > model.MaxPriority {
		return 0, fmt.Errorf("priority %d is out of range %d-%d", n, model.MinPriority, model.MaxPriority)
	}
	return n, nil
}

// validateValue checks that value is one of the five lexical forms without
// binding it to a typed model field. It is used for unknown/user keys.
func validateValue(value string) error {
	if strings.HasPrefix(value, "[") {
		_, err := parseInlineList(value)
		return err
	}
	_, err := parseScalar(value)
	return err
}

// parseScalar decodes a bare scalar or a quoted string. It rejects the
// out-of-subset bare words (bool-ish and null) so the parser never guesses.
func parseScalar(value string) (string, error) {
	if strings.HasPrefix(value, `"`) {
		return parseQuoted(value)
	}
	if !bareTokenRe.MatchString(value) {
		return "", fmt.Errorf("invalid scalar %q", value)
	}
	if isForbiddenBareWord(value) {
		return "", fmt.Errorf("bare word %q is not allowed (no booleans or null)", value)
	}
	return value, nil
}

// isForbiddenBareWord reports whether value is a YAML bool-ish or null word.
func isForbiddenBareWord(value string) bool {
	switch strings.ToLower(value) {
	case "yes", "no", "on", "off", "true", "false", "null":
		return true
	default:
		return false
	}
}

// parseInlineList parses `[ scalar (, scalar)* ]`, allowing an empty list.
// It consumes the whole value and rejects any trailing content.
func parseInlineList(value string) ([]string, error) {
	if !strings.HasPrefix(value, "[") {
		return nil, fmt.Errorf("expected an inline list, got %q", value)
	}
	if !strings.HasSuffix(value, "]") {
		return nil, fmt.Errorf("inline list %q is not closed on one line", value)
	}
	inner := value[1 : len(value)-1]
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return nil, nil
	}

	var out []string
	i := 0
	for {
		for i < len(inner) && inner[i] == ' ' {
			i++
		}
		if i >= len(inner) {
			return nil, fmt.Errorf("inline list %q has a trailing comma", value)
		}
		item, next, err := parseListElement(inner, i)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
		i = next
		for i < len(inner) && inner[i] == ' ' {
			i++
		}
		if i >= len(inner) {
			return out, nil
		}
		if inner[i] != ',' {
			return nil, fmt.Errorf("inline list %q has trailing content", value)
		}
		i++
	}
}

// parseListElement parses one bare or quoted element starting at index i.
func parseListElement(inner string, i int) (string, int, error) {
	if inner[i] == '"' {
		end := i + 1
		for end < len(inner) {
			if inner[end] == '\\' {
				end += 2
				continue
			}
			if inner[end] == '"' {
				s, err := parseQuoted(inner[i : end+1])
				if err != nil {
					return "", 0, err
				}
				return s, end + 1, nil
			}
			end++
		}
		return "", 0, fmt.Errorf("quoted list element is not closed")
	}
	end := i
	for end < len(inner) && inner[end] != ',' && inner[end] != ']' {
		end++
	}
	token := strings.TrimSpace(inner[i:end])
	if token == "" {
		return "", 0, fmt.Errorf("empty list element")
	}
	s, err := parseScalar(token)
	if err != nil {
		return "", 0, err
	}
	return s, end, nil
}

// parseQuoted decodes a double-quoted string with backslash escapes. Any
// content after the closing quote is rejected.
func parseQuoted(value string) (string, error) {
	if len(value) < 2 || value[0] != '"' {
		return "", fmt.Errorf("invalid quoted string %q", value)
	}
	var b strings.Builder
	for i := 1; i < len(value); i++ {
		c := value[i]
		switch c {
		case '"':
			if i != len(value)-1 {
				return "", fmt.Errorf("unexpected content after quoted string: %q", value)
			}
			return b.String(), nil
		case '\\':
			i++
			if i >= len(value) {
				return "", fmt.Errorf("dangling escape in quoted string %q", value)
			}
			switch value[i] {
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				return "", fmt.Errorf("unsupported escape \\%c", value[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", fmt.Errorf("unterminated quoted string %q", value)
}
