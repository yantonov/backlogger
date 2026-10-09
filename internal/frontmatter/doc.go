// Package frontmatter parses and serializes the frozen, flat, YAML-compatible
// frontmatter subset defined in the design spec section 8.2.
//
// The parser implements only the five lexical forms (enum, int, bare scalar,
// inline list, quoted string) and rejects every out-of-subset construct loudly.
// The serializer emits canonical, byte-stable output so the derived index is
// reproducible.
package frontmatter
