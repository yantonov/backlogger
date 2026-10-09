// Package store resolves project paths and owns atomic detail reads and writes.
//
// Writes go to a temp file in the same directory, are fsynced, then replaced
// with os.replace so readers always see a whole old or whole new file. A read
// error on an existing file is never treated as an empty backlog.
package store
