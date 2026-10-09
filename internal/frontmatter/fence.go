package frontmatter

import "bytes"

// fence is the fixed frontmatter delimiter line. It appears once at byte 0
// (the opening fence) and once more at the end of the block (the closing
// fence). Only a line that is exactly these three bytes closes the block.
const fence = "---"

// splitFences separates the frontmatter block from the Markdown body.
//
// Frontmatter starts at byte 0 with "---\n" and ends at the next line that is
// exactly "---". A CRLF variant of either fence is accepted so that files
// checked out on Windows are not mistaken for body-only files. The body after
// the closing fence is returned verbatim and may contain "---" freely. A file
// without a leading fence has no frontmatter and its whole content is the
// body; this is not an error (spec 8.2).
func splitFences(text []byte) (front, body []byte, hasFront bool, err error) {
	rest, ok := trimOpenFence(text)
	if !ok {
		return nil, text, false, nil
	}

	lineStart := 0
	for lineStart <= len(rest) {
		nl := bytes.IndexByte(rest[lineStart:], '\n')
		lineEnd := len(rest)
		next := len(rest)
		if nl >= 0 {
			lineEnd = lineStart + nl
			next = lineEnd + 1
		}
		if isFenceLine(rest[lineStart:lineEnd]) {
			return rest[:lineStart], rest[next:], true, nil
		}
		if nl < 0 {
			break
		}
		lineStart = next
	}
	return nil, nil, false, &ParseError{Line: 1, Msg: "opening frontmatter fence is not closed"}
}

// trimOpenFence consumes a leading "---\n" or "---\r\n" and returns the rest.
func trimOpenFence(text []byte) ([]byte, bool) {
	if bytes.HasPrefix(text, []byte(fence+"\n")) {
		return text[len(fence)+1:], true
	}
	if bytes.HasPrefix(text, []byte(fence+"\r\n")) {
		return text[len(fence)+2:], true
	}
	return nil, false
}

// isFenceLine reports whether line is exactly the closing fence, allowing a
// trailing carriage return from CRLF input.
func isFenceLine(line []byte) bool {
	return bytes.Equal(line, []byte(fence)) || bytes.Equal(line, []byte(fence+"\r"))
}
