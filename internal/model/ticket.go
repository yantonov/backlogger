package model

import "time"

// TimestampLayout is the canonical timestamp encoding: UTC ISO-8601 with second
// precision and a literal Z suffix. The serializer emits timestamps in this
// form and never re-encodes them (spec 8.2/8.3).
const TimestampLayout = "2006-01-02T15:04:05Z"

// FormatTimestamp renders t in the canonical timestamp form, in UTC.
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(TimestampLayout)
}

// Status is the lifecycle state of a ticket. It is the value of the `status`
// frontmatter key and one of a fixed token set (spec 5.3).
type Status string

// The complete status token set. There are no other valid values.
const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusClosed     Status = "closed"
)

// Valid reports whether s is one of the three status tokens.
func (s Status) Valid() bool {
	switch s {
	case StatusOpen, StatusInProgress, StatusClosed:
		return true
	default:
		return false
	}
}

// Field defaults and bounds for the `priority` key.
const (
	DefaultStatus   Status = StatusOpen
	DefaultPriority        = 2
	MinPriority            = 0
	MaxPriority            = 4
)

// CanonicalFieldOrder is the order in which known keys are emitted. It is the
// single source of truth for the serializer's field order; unknown/user keys
// follow it in their original relative order.
var CanonicalFieldOrder = []string{
	"status",
	"priority",
	"deps",
	"attachments",
	"created",
	"updated",
	"closed-at",
	"archived",
}

// Extra is an unknown/user frontmatter key. It is opaque to the model: the
// parser does not understand its value and the serializer writes the original
// line back verbatim, so preservation is lossless (spec 8.2/8.3).
type Extra struct {
	// Key is the mapping key, used only for duplicate detection.
	Key string
	// Line is the full original `key: value` line, without the trailing
	// newline. It is written back byte-for-byte.
	Line string
}

// Ticket is the typed frontmatter of a single detail file. Identity is the
// filename stem and is deliberately not a field (spec 3.2).
//
// The core fields (Status, Priority, Deps, Attachments, Created, Updated) are
// always emitted by the serializer, even when empty. ClosedAt and Archived are
// emitted only when set.
type Ticket struct {
	Status      Status
	Priority    int
	Deps        []string
	Attachments []string
	Created     string
	Updated     string
	ClosedAt    string
	Archived    string
	Extra       []Extra
}

// DefaultTicket returns the field values a body-only detail file is treated as
// having: status open and priority 2, with empty lists and timestamps.
func DefaultTicket() Ticket {
	return Ticket{
		Status:   DefaultStatus,
		Priority: DefaultPriority,
	}
}

// IsActive reports whether the ticket is part of the active view (open or in
// progress). Closed and archived tickets are not active.
func (t Ticket) IsActive() bool {
	return t.Status == StatusOpen || t.Status == StatusInProgress
}

// ArchivedAt reports whether the ticket carries an archive timestamp. Presence
// of the field is the archive state (spec 5.4).
func (t Ticket) ArchivedAt() bool { return t.Archived != "" }
