// Package ids implements the ticket ID grammar, case folding, generation, and
// partial-ID resolution.
//
// IDs are lowercase filenames stems: start with an ASCII letter, then
// [a-z0-9-], length 3-32, with underscore and the reserved names archive and
// attachments forbidden.
package ids
