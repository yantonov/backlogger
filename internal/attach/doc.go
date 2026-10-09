// Package attach manages the optional flat attachment sidecar directory.
//
// Files are stored as .tickets/attachments/<id>__<suffix>; the authoritative
// link is the attachments list in the ticket frontmatter. The directory is
// never scanned on the hot path.
package attach
