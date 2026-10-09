package cli

// This file defines the frozen machine-readable schema for every read command.
// It is the single place the JSON shape lives, so a consumer can rely on the
// field names and their order. encoding/json emits struct fields in declaration
// order; no map is ever marshalled as the top level of a response, so field
// order is stable and deterministic.
//
// Every response carries a top-level "version" so a future change can be
// detected without guessing. Timestamps in the payload are always the stored
// frontmatter strings, never render-time now.

// jsonSchemaVersion is the version of the --json wire schema. Bump it only for
// an incompatible change; additive fields keep the same version.
const jsonSchemaVersion = 1

// JSON status tokens are the model.Status strings ("open", "in_progress",
// "closed") and are used verbatim. No separate enum is defined.

// jsonNavTicket is one ticket in a navigation view response (list and the
// dependency views). It is the lean index-line shape: id, status, title, deps,
// and archived. It deliberately carries no priority, timestamps, attachments,
// or hash; call `bl show <id> --json` for the full ticket. The
// field order is the wire order (spec 3.1/7).
type jsonNavTicket struct {
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	Title    string   `json:"title"`
	Deps     []string `json:"deps"`
	Archived bool     `json:"archived"`
}

// jsonTicket is the full ticket in a `show` response. The field order is the
// wire order. Empty optional strings use omitempty; the two list fields are
// always emitted as arrays (never null) so a consumer can iterate
// unconditionally. It is used only by jsonShow (spec 7).
type jsonTicket struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Priority    int      `json:"priority"`
	Deps        []string `json:"deps"`
	Attachments []string `json:"attachments"`
	Created     string   `json:"created"`
	Updated     string   `json:"updated"`
	ClosedAt    string   `json:"closed_at,omitempty"`
	Archived    bool     `json:"archived"`
	Hash        string   `json:"hash"`
}

// jsonList is the envelope for `list` and the dependency views
// (started/blocked/available/closed). Filter is present only when the command
// accepted filters and at least one was given. Tickets are the lean navigation
// shape; this response is never a way to fetch details (spec 7).
type jsonList struct {
	Version int             `json:"version"`
	Command string          `json:"command"`
	Filter  *jsonFilter     `json:"filter,omitempty"`
	Tickets []jsonNavTicket `json:"tickets"`
}

// jsonFilter records the options a filtered list ran with, so a consumer can
// tell what it asked for. It is omitted for unfiltered views. There is no
// priority field: the views do not filter on priority (spec 7).
type jsonFilter struct {
	Status   string `json:"status,omitempty"`
	Archived bool   `json:"archived,omitempty"`
	All      bool   `json:"all,omitempty"`
}

// jsonShow is the `show --json` response. It carries the full ticket plus the
// body and the derived content hash; the hash is the value a future --if-match
// would compare (spec 6.4). OCC itself is out of scope.
type jsonShow struct {
	Version int        `json:"version"`
	Command string     `json:"command"`
	Ticket  jsonTicket `json:"ticket"`
	Body    string     `json:"body"`
}

// jsonDepTree is the `dep tree --json` response. Lines are emitted in traversal
// order; each line records its depth and any note.
type jsonDepTree struct {
	Version int          `json:"version"`
	Command string       `json:"command"`
	Root    string       `json:"root"`
	Full    bool         `json:"full"`
	Lines   []jsonTreeLn `json:"lines"`
}

type jsonTreeLn struct {
	ID      string `json:"id"`
	Depth   int    `json:"depth"`
	Note    string `json:"note,omitempty"`
	Unknown bool   `json:"unknown"`
}

// jsonDepCycle is the `dep cycle --json` response. Cycle is empty when the
// graph is acyclic; it is rendered as "a -> b -> a" when present.
type jsonDepCycle struct {
	Version int    `json:"version"`
	Command string `json:"command"`
	Cycle   string `json:"cycle"`
}

// jsonDepOrder is the `dep order --json` response. Order is the stable
// topological sequence restricted to the selected tickets; it is always an
// array (never null) so an empty backlog is []. Unlike the dependency graph
// commands it carries no separate warnings field: unresolved deps are written
// to stderr exactly as the navigation views do, and the order itself is still
// computable (spec 5.2).
type jsonDepOrder struct {
	Version int               `json:"version"`
	Command string            `json:"command"`
	Order   []jsonOrderTicket `json:"order"`
}

// jsonOrderTicket is one ticket in a `dep order` response: the lean navigation
// shape plus its 1-based position in the sequence. It embeds jsonNavTicket, so
// the fields (id, status, title, deps, archived) flatten into the object and
// the wire order is position first, then the navigation fields.
type jsonOrderTicket struct {
	Position int `json:"position"`
	jsonNavTicket
}

// jsonAttachments is the `attachments --json` response. Entries are the
// authoritative links with on-disk presence; orphans are files present without
// a link.
type jsonAttachments struct {
	Version int              `json:"version"`
	Command string           `json:"command"`
	ID      string           `json:"id"`
	Entries []jsonAttachment `json:"entries"`
	Orphans []string         `json:"orphans"`
}

type jsonAttachment struct {
	Suffix  string `json:"suffix"`
	Present bool   `json:"present"`
}

// jsonDoctor is the `doctor` response. Issues are the sorted findings; the
// summary counts are included so a consumer need not count. `doctor` is a
// report-only command: it never repairs, so there is no fixes field.
type jsonDoctor struct {
	Version  int         `json:"version"`
	Command  string      `json:"command"`
	Issues   []jsonIssue `json:"issues"`
	Errors   int         `json:"errors"`
	Warnings int         `json:"warnings"`
	Clean    bool        `json:"clean"`
}

// jsonDoctorFix is the `doctor fix` response. Fixes lists the repairs applied
// in this run; issues are the findings that remain (the identity-level problems
// Fix cannot repair), re-computed after the repairs.
type jsonDoctorFix struct {
	Version  int         `json:"version"`
	Command  string      `json:"command"`
	Fixes    []jsonFix   `json:"fixes"`
	Issues   []jsonIssue `json:"issues"`
	Errors   int         `json:"errors"`
	Warnings int         `json:"warnings"`
	Clean    bool        `json:"clean"`
}

type jsonFix struct {
	Action string `json:"action"`
	ID     string `json:"id,omitempty"`
	Detail string `json:"detail"`
}

type jsonIssue struct {
	Severity string `json:"severity"`
	Check    string `json:"check"`
	ID       string `json:"id,omitempty"`
	Message  string `json:"message"`
}
