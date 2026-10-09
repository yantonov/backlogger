# backlogger — Design Specification

Status: decided (language: Go; license: Apache-2.0)
Supersedes: [`backlogger`](https://github.com/bobuk/backlogger) (`bl`, Python) and [`ticket`](https://github.com/wedow/ticket) (`tk`, Bash)
Goal: a single local task tracker that takes the best of both tools:
the **concurrency safety + low-friction single-file overview** of backlogger,
and the **per-ticket detail + dependency model + portable CLI** of ticket.

---

## 1. Goals

- Local-first, directory-scoped, git-friendly, no server, no daemon, no DB.
- Correct under concurrency **from day one** (multiple agents + a human may
  mutate the same backlog at the same time).
- Fast enough to scan/list thousands of tickets.
- One binary, no runtime dependencies (no Python, no jq, no bash).
- Agent-usable: stable visible IDs, a claim primitive (`start`), and
  machine-readable (`--json`) output.
- Human-usable: human-readable files; a TUI/GUI is deferred past the first
  iteration.

## 2. Non-goals

- No multi-machine / network filesystem coordination.
- No semantic/LLM ID generation, no external API calls of any kind.
- No accounts, ownership tokens, leases, or heartbeats.
- No sprint boards, estimates, time tracking.
- No hosted service or sync.
- No migration or import from other trackers — the tracker is new; there is no
  legacy backlog to convert.
- No packaging beyond GitHub Releases — source checkout, `go build`, and the
  release workflow (see §9). No Homebrew tap, AUR package, or `goreleaser`.

---

## 3. Storage model

Two-layer layout. A **single index file** for the fast, whole-backlog overview,
plus **one detail file per ticket**. Both are plain text in the repo.

```
<project>/
├── backlog.md            # index: one line per ticket, ordered, human-readable
└── .tickets/
    ├── <id>.md           # full detail + frontmatter per active/closed ticket
    ├── archive/
    │   └── <id>.md       # archived tickets (stored under the same ID)
    ├── attachments/      # only created when a ticket has attachments
    │   └── <id>__<name>  # e.g. login__screenshot.png, my-task__plan.md
    └── .lock             # single lock file (see §6)
```

`.tickets/archive/` holds only manually archived tickets (see §5.4). It exists
as a scan optimization: the hot paths never open archived files, so a large
archive never slows `list`/`available`/`started`/`blocked` down.
`.tickets/attachments/` is a flat, optional sidecar directory keyed by ID; it is
never scanned on the hot path (see §3.5).

### 3.1 `backlog.md` — the index

- A **committed** (git-tracked) derived view: one file that is present after
  `git clone`, so `list`/`available` need only read it instead of scanning the
  whole `.tickets/` directory on every call.
- Formally: `backlog.md == f(details)`, a **pure, deterministic function** of
  the detail files. It carries no information of its own; a conflict in it is
  always resolved by `bl reindex`, never by judgment.
  - **Determinism invariant (hard requirement):** `f` must be byte-stable —
    stable sort, stable field/line order, no embedded regeneration timestamps,
    stable newlines — so that `bl reindex` run twice produces no diff. Tested by
    a round-trip/idempotence test, else committed-index diffs become noise.
- A derived, ordered view of every ticket, for human readability and fast
  listing. Ticket order is derived from the sort `(priority, created, id)`.
  Because none of those keys change on a status/glyph update, a status change
  edits exactly one line and never reorders other tickets — keeping merges clean.
  (Manual reordering is out of scope: the index is derived, so it would need a
  stored order key such as a fractional `rank`.)
- **Derived, not authoritative**: its `#id` tokens are cached pointers into
  `.tickets/`. Regenerable at any time with `bl reindex` / `bl doctor fix`.
- One line per ticket, checkbox syntax for readability and diffs. Two
  sections in fixed order — Active, Closed — with a blank line between them:

  ```
  ## Active
  - [ ] Add login flow #login <- [design, ci]
  - [>] Design the API #design

  ## Closed
  - [x] Set up CI #ci
  ```

  A `<- [deps]` suffix (mirrored from the detail file) is present only when the
  ticket has dependencies. A closed ticket lives in `## Closed`. **Archived
  tickets are not in the index at all**: archive is detail/directory state, read
  on demand (see §5.4).

- Status glyphs: `[ ]` todo, `[>]` in progress (started), `[x]` done.
- The trailing `#id` is a cached pointer to the detail file; the filename stem
  is the authoritative identity (see §3.2). Everything before the ID is the
  title and may be edited freely.
- **Read model.** `list` and the recomputed views (`started`, `blocked`,
  `available`, `closed`) are **navigation views**: they exist only to find a
  ticket by ID/state so a caller can then act on it. They read **only** the
  index, in file order, and never open a detail file. They render exactly the
  fields already on an index line (glyph/status, title, `#id`, `<- [deps]`) and
  rely on `f` having already applied the `(priority, created, id)` order — file
  order *is* the sorted order, so a view never re-sorts and never inspects
  `priority`/`created`. They do not filter on priority and expose no other field.
  A caller that needs a ticket's details (priority, timestamps, attachments,
  body) calls `bl show <id>` (or `bl show <id> --json`); details are deliberately
  **not** part of the navigation surface.
  - **Archive is the one exception.** The default view, `--status`, and the
    dependency views never open an archived file. `list --archived` and
    `list --all` do: they read `.tickets/archive/`, because archived tickets are
    not in the index. Only those flags leave the index.
- **Write model.** Mutations update detail file(s) first, then regenerate the
  index **inside the same lock** (one extra atomic write), so the committed view
  advances in step with the authority on the happy path. Every mutating command —
  above all the compare-and-set `start` — re-reads the **authoritative detail
  file** under the lock and verifies state before changing it, so a stale index
  may *offer* a ticket that is already started but `start` still fails correctly
  (the same "refresh and retry" UX as backlogger); it can never cause a wrong
  write.
- **Merge handling.** Because the index is a pure view, conflicts are mechanical:
  resolve inputs, then run `bl reindex`. `doctor` verifies
  `backlog.md == f(details)` and fixes drift.

### 3.2 `.tickets/<id>.md` — the detail

- Frontmatter for structured fields, Markdown body for prose (description,
  notes) — borrowed from `ticket`. The frontmatter
  uses a **frozen, flat, YAML-compatible subset** (§8.2); it is valid YAML but
  the parser accepts only that subset and rejects everything else:

  ```markdown
  ---
  status: open
  priority: 2
  deps: [design, ci]
  attachments: [screenshot.png, plan.md]
  created: 2026-10-08T11:30:40Z
  updated: 2026-10-08T11:40:00Z
  closed-at: 2026-10-08T12:00:00Z
  archived: 2026-10-09T09:00:00Z
  ---
  # Add login flow

  ## Description
  ...
  ## Notes
  **2026-10-08T11:40:00Z**
  ...
  ```

  (`closed-at` and `archived` are emitted only when set; see
  §5.3. The writer emits canonical field order and no comments.)

- **The title is the body's first ATX heading, not a frontmatter field.** `add`
  writes the title as `# <title>`; `edit --title` rewrites it in place. `f`
  derives the display title from that heading, so no `title` key exists and the
  index stays a pure function of the detail bytes.

- **Identity is the filename stem, not a frontmatter field.** There is no
  `id:` key. `.tickets/login.md` *is* ticket `login`. This removes any
  possibility of frontmatter/filename drift and makes immutability enforceable
  by the filesystem (changing an ID is a file rename).
- The detail file is authoritative for all non-identity state; `backlog.md` is
  its derived view.
- **Write rule:** mutate detail file(s) first, then regenerate the index in the
  same lock. Details always win; `reindex` recreates the index from the detail
  files and is idempotent (`f(f(details)) == f(details)`). No transaction journal
  is required for index↔detail drift.
- Rationale: keeps the overview file cheap to read and git-diffable while giving
  each ticket a rich, independent, self-describing file; avoids rewriting the
  whole index body on every note edit.

### 3.3 Why not alternatives

- *Single file only (backlogger):* rich per-ticket metadata makes one file huge
  and every edit rewrites it; parallel detail edits serialize on one file.
- *Files only (ticket):* listing requires globbing + parsing every file; no
  cheap ordered overview; ordering is awkward.
- *The two-layer design keeps the hot path (`list`, `available`) as one small
  file read and the cold path (`show`, edit) as one small file.*

### 3.4 Identity

- **One authority: the filename stem.** `.tickets/<id>.md` defines the ID. There
  is no `id` field in frontmatter and the `#id` in the index is a derived cache.
- IDs are **immutable**: changing one is a file rename (`git mv`), not an edit.
  The tracker exposes no rename command; a wrong ID is superseded by a new ticket.
- IDs are **lowercase only** (see §5.1): they are filenames, so this is safe on
  case-insensitive filesystems (macOS, Windows) and eliminates case-collision
  ambiguity everywhere. Lowercase is enforced, not merely tolerated.

### 3.5 Attachments (optional)

- Flat sidecar dir `.tickets/attachments/`; files are named `<id>__<suffix>`,
  e.g. `login__screenshot.png`, `my-task__plan.md`.
- **Underscore is forbidden in IDs**, so the `__` delimiter is unambiguous: the
  ID is everything before the first `__`.
- **Suffix grammar.** A suffix is a strict filename subset: non-empty and
  containing only ASCII letters, digits, `.`, `-`, and `_`, and it must pass
  `filepath.Base`/`IsAbs`. Separators, volume prefixes, and the path elements
  `.`/`..` are rejected, so a suffix can never escape the attachments directory.
  A source path contributes only its base name.
- **Authoritative link is the `attachments:` list in the ticket frontmatter**
  (e.g. `attachments: [screenshot.png, plan.md]`). The filename is only storage;
  `doctor` checks frontmatter ↔ files (missing entries, orphans).
- Created only when a ticket actually has attachments; the directory itself is
  optional.
- **Never scanned on the hot path** — `list`/`available`/`started`/`blocked` and
  `reindex` ignore it entirely. Attachments are read only by on-demand commands
  (`bl show --attachments`, `bl attachments`, `bl attach`, `bl detach`).
- Commands: `bl attach <id> <file...>`, `bl detach <id> <file>`, under the same
  lock; `attach` copies the file in and records it in frontmatter.
- Reserved names `archive` and `attachments` cannot be IDs (see §5.1).

---

## 4. Language: Go

Go is chosen for a free Windows cross-compile with no C toolchain,
low-effort multi-OS builds, and fast iteration on the parsing/locking/graph code.
(The state-machine and DAG correctness are not a discriminator — a correct
implementation is possible in either language.)

Consequences the rest of this spec depends on:

- §6 locking must work on Linux, macOS, and Windows (`flock` / `LockFileEx`).
- The frozen flat YAML subset (§8.2) is **hand-rolled** (no full YAML library) so
  `f(details)` stays byte-stable and dependency-free.
- The only runtime dependency is the lock library (`gofrs/flock`); `yaml.v3` is a
  test-only dependency.

---

## 5. Tickets, IDs, and dependencies

### 5.1 IDs

- **Locally generated only** (no LLM). Short, human-friendly, unique.
- Grammar: start with an ASCII letter; then `[a-z0-9-]`; length 3–32.
  **Minus (`-`) is allowed; underscore (`_`) is forbidden** so that `__` is
  reserved as the attachment-name delimiter (see §3.5).
  Purely numeric IDs are forbidden (avoids collision with line numbers).
  **Lowercase is mandatory.**
- **Reserved names:** `archive`, `attachments`, and any dot-prefixed name are
  forbidden as IDs (they name reserved paths inside `.tickets/`).
- The ID **is the filename stem** (`.tickets/<id>.md`); see §3.4. Source of
  truth is the filesystem, not a frontmatter field.
- Generation: random base-32/`nanoid`-style token (e.g. 5–6 chars), then
  **uniqueness checked** against `.tickets/` under the lock. The generator takes
  an injectable existence predicate rather than a directory, so `ids` stays free
  of `internal/store` (no import cycle) and the caller composes the
  active+archive uniqueness check.
- **All ID input is lowercased before use**: `--id`, a trailing `#id`, and
  every operand/lookup (`bl show LOGIN` == `bl show login`). An explicit ID
  that, after lowercasing, violates the grammar is rejected with an error; a
  duplicate explicit ID is an error (no silent overwrite). Grammar violations
  other than case are never silently repaired.
  - Case folding applies to ASCII only. An explicit `--id` is never trimmed of
    whitespace (so `" login"` is rejected), while an interactive operand lookup
    (`bl show " login "`) is trimmed and lowercased before matching.
- Canonical form: stored IDs, index `#id` tokens, and `deps` references
  are always the lowercased form.
- IDs are **immutable** for the life of the ticket (survive status change,
  reorder, archive). Changing an ID would be a file rename, which it does not
  expose as a command.
- Partial-ID matching: if an operand is not an exact ID, match by unique
  substring of the filename stem; ambiguous → error listing candidates.

### 5.2 Dependencies

- `deps: [id, ...]` = this ticket is blocked until each dep is `closed`.
- Direction: `dep A B` means **A depends on B** (`A -> B`).
- Used by `blocked` (active, ≥1 dep open/in_progress) and `available`
  (`status = open` AND all deps closed, so nothing already started is offered).
  `started` is the claimed set: `status = in_progress`, with no dependency
  filter (a claim is independent of the graph). These views are **recomputed,
  never stored**.
- Commands: `dep add`, `dep rm`, `dep tree [--full]`, `dep cycle`,
  `dep order`.
- **Cycle policy: hard refuse.** `dep add` computes in the
  same lock whether `B` already reaches `A`; if the new edge would close a cycle
  (including self-loops `dep A A`), the command **fails with an error naming the
  cycle**, e.g. `would create cycle: a -> b -> a`, and writes nothing. The
  dependency graph is therefore an invariant DAG.
  - The traversal reads detail files of the would-be dependents (not covered by
    the index); it is O(V+E) and only runs on the rare `dep add`.
  - A cycle can still enter via a race or a manual file edit, so `dep cycle`
    and a `doctor` cycle check remain and must not be removed.
  - `dep tree` uses a visited set regardless, so it terminates even if the graph
    is somehow corrupted.
- **An unresolved dep blocks and warns.** A dep ID that names no known ticket is
  surfaced as a warning (never silently ignored) and counts as *not satisfied*,
  so `available`/`blocked` treat a broken reference as blocking rather than
  silently unblocking.
- Cost: deps are authoritative in the detail file and **mirrored into the index**
  by `f` (`display: <- [deps]`). Because the index is committed and derived, the
  hot views do no detail scan. Every active/closed ticket's current glyph is in
  the index, so dependency satisfaction is decidable without reading details:
  `available` requires each mirrored dep's glyph to be `x` (closed). An
  archived ticket is definitionally closed and is not in the index, so the view
  path lists the archive names (cheap, no detail read) and treats every archived
  ID as satisfied.
  - `list`, `list --status`, `available`, `started`, `blocked`, `closed` are
    **index-only** (plus the name-only archive listing for dependency
    satisfaction).
  - `list --archived`/`--all` read the archived detail files (they are not in
    the index).
  - `show` and the graph commands (`dep tree`, `dep cycle`, `dep order`) read
    detail files.
  - `dep add` (CAS + cycle check) and `doctor` read detail files.
- **`dep order` is a derived ordering, not a navigation view.** It returns a
  stable topological order of the **active** tickets (open or in progress), with
  the ticket sort `(priority, created, id)` as the only tiebreak among
  independent tickets. It therefore **re-sorts** where a view may not: a
  low-priority blocker must precede the ticket it unblocks, so the index's
  committed order is not enough. It is a graph command, so it reads the
  authoritative detail files (like `dep tree`) and never writes; it is not bound
  by the "views never re-sort" rule (§3.1/§7).
  - Like `dep tree`, the answer is recomputed from the detail files every time
    and never stored.
  - **Closed and archived tickets are never listed.** A closed ticket imposes no
    constraint on the work that remains, and an archived ticket is closed by
    definition, so neither belongs in an ordering of what is left to do. They
    are still read/known as blockers — a dep on a closed or archived ticket is
    satisfied, not an unresolved warning — but they are filtered from the
    output. Closed details are read because they live beside the active ones;
    archived IDs are known by name only, so the archive directory is never
    opened (§5.4).
  - An unresolved dep is ignored for ordering (it is already warned and must
    not stall the sort); warnings are reported only for the active tickets the
    view lists, so a closed ticket's broken dep adds no noise. A cycle
    introduced by a race or a manual edit is impossible for the tickets that
    can still be drained, and any ticket left in a cycle is appended in
    `(priority, created, id)` order, so the walk always terminates and never
    drops a ticket.
- **Mirroring is safe** precisely because the index is a pure view: it is never
  read back as authoritative for deps. `dep add`/`dep rm` update the detail under
  the lock, then regenerate the index; a manual index edit is corrected by
  `reindex` and flagged by `doctor`.
- Dep lists are rendered in **deterministic (sorted) order** so `f` stays
  byte-stable (idempotence invariant, §3.1).

### 5.3 Field set

Core keys are **always emitted**, even when empty:

| Field | Type | Values / default | Role |
|---|---|---|---|
| `status` | enum | `open` (default) \| `in_progress` \| `closed` | state; drives views |
| `priority` | int | `0`–`4`, default `2`, 0 highest | sort key (invisible in list) |
| `deps` | id list | `[]` | blocking graph |
| `attachments` | string list | `[]` | file links (see §3.5) |
| `created` | timestamp | set on create | sort tiebreak |
| `updated` | timestamp | bumped on any mutation | last-touched |

Conditional keys, emitted **only when set**:

| Field | Type | Set when | Role |
|---|---|---|---|
| `closed-at` | timestamp | on `done` | age filter |
| `archived` | timestamp | on `archive` | archive state (see §5.4) |

**Dropped** (out of scope): `type`, `assignee`, `parent`, `links`,
`tags`. Rationale: they add schema and validation surface without behavior.
There is **no ownership** (§6.5), no hierarchy (deps cover ordering), no
symmetric relations, and no free-form label vocabulary (tags); arbitrary keys
remain *preserved* on rewrite (§8.3) but are not first-class or filterable.

- `updated` = last mutation, **including** `start`/`done`/`dep` changes. It is
  display/age only; it is never a concurrency token (see §6.4).
- **Types map to the frozen YAML subset (§8.2):** `enum` → fixed token set, `int`
  → integer, `timestamp`/`string` → scalar (bare or quoted), `id list`/`string
  list` → inline list. There is **no boolean type** in the schema or the subset.
- Unknown/user keys are preserved on rewrite (§8.3) but not schema-validated or
  filterable.
- Filenames (identity) and the title are **not** fields and never appear in
  frontmatter; the title is the body's first ATX heading (§3.2).

### 5.4 Archive

- **Archive is stored state in the detail file**, not a derived view: the
  `archived` field holds an ISO-8601 timestamp; presence = archived (`updated`
  and `closed-at` stay available for sorting).
- **Archiving is a manual command.** The command may *select candidates* using
  status/age logic evaluated at invocation time, but the decision is then frozen
  in the file. Time is never part of regeneration, so the pure-index invariant
  `backlog.md == f(details)` holds and `reindex` stays idempotent.
  - `bl archive <id>...` — archive specific tickets.
  - `bl archive --closed-older-than 30d` — select closed tickets whose
    `closed-at` is older than the threshold. The threshold is strict (strictly
    older than the cutoff) and parsed from the stored canonical `closed-at`; an
    unparseable or empty `closed-at` never qualifies.
  - `--dry-run` lists what *would* be archived (uses the clock; touches nothing).
  - A batch is all-or-nothing: every named ID (or every selected candidate) is
    validated before the first write, so one ineligible ID changes nothing.
- **Constraint:** only `status: closed` tickets may be archived; archiving an
  open ticket is an error. `reopen` clears `archived` and `closed-at`.
- **Storage:** archived tickets live in `.tickets/archive/<id>.md` (same ID, no
  rename of identity). `archive` moves the file there and sets the field;
  `unarchive` moves it back and clears the field — both in the **same locked
  transaction**. Every mutation routes through the store's location-aware write,
  so the location always follows the stored `archived` field: `reopen` also
  moves an archived file back, not only `unarchive`.
  `doctor` verifies location↔field agreement (a file in `archive/` must have
  `archived`, and vice versa).
  A move never overwrites: when the same ID already has a file at the
  destination (a hand edit or a bad merge), `archive` and `unarchive` fail,
  touch neither copy, and point to `bl doctor`; which copy is right is the
  user's decision.
- **Scan optimization:** active operations read only `.tickets/*.md` and never
  open archived files. For dependencies, an archived ticket is definitionally
  `closed`, so `blocked` treats archive membership as "closed" by
  listing archive names (never opening the files). `bl list --archived`,
  `bl list --all`, and `doctor` read the archived details.
- **Index rendering:** `f` groups tickets into Active / Closed from the stored
  fields. **Archived tickets are omitted from the index entirely** — archive is
  detail/directory state, not index state. `bl list` hides them by default,
  `bl list --archived` shows only them, and `bl list --all` shows active, closed,
  and archived; the archived set is read from `.tickets/archive/` on demand.
- **No `purge` command** — the metaphor was confusing and nothing needs moving
  beyond the archive subdir. `archive` replaces it.

---

## 6. Concurrency

Borrow backlogger's proven transaction model; it is the core.

### 6.1 Lock

- One lock file per backlog: `<project>/.tickets/.lock` (persistent, never
  deleted — avoids inode-race on delete/recreate).
- Advisory **exclusive** lock via `flock(LOCK_EX|LOCK_NB)` on POSIX and
  `LockFileEx` on Windows, with a bounded retry loop and a timeout (default 5s).
- Timeout → clear error, no changes.
- Lock scope = "the whole read → verify → modify → write cycle" for every
  mutating operation. Never hold the lock during user interaction or any slow
  external work (there are no network calls, but the rule stands for future hooks).

### 6.2 Atomic writes

- Write to a temp file in the same directory, `fsync`, then `os.replace`/rename.
- Readers always see a whole old or whole new file, never a partial one.
- A read error on an existing file must NOT be treated as "empty backlog"
  — otherwise a transient error could clobber data.
- **Stale temp-file sweep.** A writer killed before it can clean up (for
  example `SIGKILL`) may leave a temp file behind. The next transaction removes
  every `.<name>.tmp-*` file in `.tickets/`, `.tickets/archive/`, and
  `.tickets/attachments/`, immediately after acquiring the lock and before any
  read/write. The lock proves no live writer owns a temp file, so every match is
  stale. The name cannot collide with real data (IDs may not start with a dot;
  an attachment is `<id>__<suffix>` with no leading dot), directories are never
  removed, and a temp file is never listed as a ticket. Read paths do not take
  the lock and do not sweep. Read paths never create temporary files.
- **Windows open-reader caveat.** On Windows, `os.ReadFile` opens without
  `FILE_SHARE_DELETE`, so a concurrent `os.Rename` onto the same path can fail
  with `Access is denied`. The store's readers therefore use a Windows-only
  shared read (`FILE_SHARE_READ|WRITE|DELETE`) and the atomic replace retries
  briefly. POSIX needs neither.

### 6.3 Status transitions

The three statuses form a strict, guarded cycle. Every transition is a
compare-and-set inside the lock: it succeeds only from the stated state and
writes nothing on any other state.

```
open --start--> in_progress --done--> closed --reopen--> open
                     ^                      |
                     +--------reopen--------+
```

- `start <id>`: `open → in_progress`. The claim primitive. Succeed **only** if
  the ticket is currently todo (`[ ]`); any other state → nonzero exit, no
  change. Racing starts → exactly one success.
- `done <id>`: `in_progress → closed` (sets `closed-at`). Work must be started
  first, so an open ticket cannot be closed directly.
- `reopen <id>`: `in_progress | closed → open` (clears `closed-at` and
  `archived`). It is the inverse of both `start` and `done`, so it is also how an
  in-progress ticket is un-claimed.
- There is no other status verb: the statuses are fully reachable through the
  guarded cycle above. A missing or wrong precondition is a nonzero exit that
  changes nothing.
- No expected-revision flags (see §6.4). Structural edits (status, deps)
  that target a specific ID are guarded by the lock and the relevant state check;
  free-text edits are last-writer-wins.

### 6.4 Revision / conflict detection

- **No `rev` and no optimistic-concurrency flags.**
  Correctness is already guaranteed by the whole-cycle lock (§6.1) plus the
  compare-and-set `start` (§6.3): concurrent writers serialize and racing claims
  resolve to exactly one winner. No lost updates, no corruption, no double-claim.
- The only gap a `rev`/ETag could close is *editing a ticket you did not start*
  based on a stale read. The agent protocol (`start` before work) makes this the
  exception, so it is not worth a stored field or flags.
- `updated` is for display and age only. It is **never** a concurrency token or
  an ownership/identity proxy (no owners exist; see §6.5).
- If OCC is wanted later, prefer a **derived content hash returned by
  `show --json` + `--if-match`** (no new schema), rather than a stored `rev`.
- **No transaction journal is needed.** Because the detail file is the sole
  authority and the index is derived, a crash can only leave the index stale,
  never inconsistent. `reindex` / `bl doctor fix` regenerates it idempotently.
- Corruption/repair (§5.2/§5.4/§3.5): `doctor` validates filename/ID integrity,
  uniqueness, index↔detail consistency, dep integrity, cycles, archive
  location↔field agreement, and attachment links, and prints the findings as
  JSON. `doctor fix` repairs every finding that has a lossless,
  identity-preserving rewrite and then re-reports: it regenerates `backlog.md`,
  moves an archive file to match its stored `archived` field, drops dependency
  references that name no ticket, and reconciles attachment links (dropping a
  link whose file is gone, adopting an orphan file). Findings that would change
  ticket identity — a malformed filename, the same ID in active and archive, an
  unparseable detail — and dependency cycles are left for a human and remain in
  the report. Neither copy of a duplicated ID gets any detail repair: two files
  under one identity usually mean a manual edit, and which copy is right is the
  user's decision. Both commands exit nonzero while any finding remains and zero when
  clean, so a script can gate on the exit code. `doctor fix` has no dry-run and
  no confirmation prompt: the backlog is committed, so git history is the undo.

### 6.5 Ownership

Explicitly **not** in scope: no owners, leases, or heartbeat. `start` protects the
state transition; `done` does not authenticate the caller (the same honest
boundary as backlogger, which the index design borrows from).

---

## 7. Command surface

```
Core
  bl [help]                Print usage (no args = usage)
  bl list [--status S] [--archived] [--all] [--json] [-v|-vv]
  bl add <title> [--id ID] [-d desc] [-p prio]
  bl show <id> [--attachments] [--json]
  bl edit <id> [fields...]
  bl start <id>            Atomically claim (open -> in progress)
  bl done <id> / reopen <id>

Dependencies
  bl dep add <id> <dep>    A depends on B
  bl dep rm  <id> <dep>
  bl dep tree [--full] <id> [--json]
  bl dep cycle [--json]
  bl dep order [--json] [-v|-vv]   topo order of active work, then (priority, created, id)

Views
  bl started [--json] [-v|-vv]   in progress (claimed)
  bl blocked [--json] [-v|-vv]   active, some dep open
  bl available [--json] [-v|-vv] open AND deps satisfied (claimable)
  bl closed [--json] [-v|-vv]    the Closed section in committed order

Archive
  bl archive <id>... [--dry-run]        archive specific closed tickets
  bl archive --closed-older-than D [--dry-run]
  bl unarchive <id>             clear archived state, move file back

Attachments
  bl attach <id> <file...>      copy file into .tickets/attachments/
  bl detach <id> <file>         remove an attachment
  bl attachments <id> [--json]  list a ticket's attachments

Maintenance
  bl doctor                     validate invariants (JSON report)
  bl doctor fix                 repair every safe finding, then report
  bl reindex                    regenerate backlog.md from the detail files
  bl version
  bl help

Hooks (future, out of scope)
  bl run                        drive an agent over the backlog
  # no TUI/GUI in the first iteration; revisit as explicit `bl tui` later
```

Partial-ID matching everywhere. `--json` on the read commands for agents;
`--json` is **explicit only** — output never depends on whether stdout is a tty.
`doctor` is the one exception: its report is always JSON (there is no human
format for a findings list), and it does not accept a `--json` flag.

**Views are navigation-only.** `list` and the dependency views return the same
fields the index line carries — `id`, `status` (glyph), `title`, `deps` — plus
`archived` (`true` only for a ticket read from `list --archived`/`--all`, so an
archived ticket is distinguishable from a merely closed one), and nothing else.
`--json` echoes that lean shape; it is not a way to fetch ticket details. To
read a ticket, call `bl show <id> --json`. The views have no priority filter and
`closed` has no `--limit`, so they stay pure index reads (§3.1, §5.2).

`dep order` renders the same lean shape and the same `-v`/`-vv` ladder as a
view, but it is a **graph command, not a view**: it recomputes a topological
order from the detail files (like `dep tree`) rather than echoing the committed
index order, so it is the one read that reorders (§5.2). Its `--json` adds a
`position` field to each lean ticket.

**Text verbosity (`-v` / `-vv`).** Every navigation view (`list`, `started`,
`blocked`, `available`, `closed`) renders text at one of three levels, modeled
on `git branch`. The default is deliberately minimal because the primary use is
to grab an ID and act on it:

| level | flags | line |
|---|---|---|
| 0 (default) | none | bare ID, one per line (`abc`) |
| 1 | `-v` | ID + title (`abc Fix the widget`) |
| 2 | `-vv` | the canonical index line (`- [ ] Fix the widget #abc <- [dep]`) |

- The default is the bare filename stem, never `#id`: operands resolve as bare
  IDs (`ids.Resolve` does not strip `#`), so the output pipes directly —
  `bl available | head -n1 | xargs bl start`.
- `-vv` reproduces `RenderEntry` byte-for-byte, so it is the level that mirrors
  `backlog.md`.
- `-vv` wins when both flags are given, in either order.
- These are flags, not command aliases; the levels above are the only values
  (`-vvv` is an unknown flag).
- `--json` is unaffected: when it is passed, the level is ignored and the lean
  JSON shape is emitted.

### 7.1 Defaults and no-implicit-branching rules

- `bl` with no args prints usage; unknown command prints usage and exits nonzero.
- `bl list` defaults to the **active view** (open + in_progress), in the
  committed index order `(priority, created, id)` — the order `f` already
  rendered, so the view does not re-sort. Closed/archived are hidden unless
  requested (`--status closed`, `--archived`, or `--all`). The list carries no
  field beyond what an index line shows (see §7 "Views are navigation-only").
- Navigation views print bare IDs by default; `-v`/`-vv` widen the line (see §7).
- Empty backlog prints `(backlog is empty)` and exits 0.
- **Read vs write on a missing `.tickets/`:** read commands error with guidance;
  write commands (`add`) create the directory. No command silently creates state
  on a read, and a read error is never treated as an empty backlog (see §6.2).
- Single-letter aliases are **not** part of the command surface (namespacing cost); commands are
  spelled out. The `-v`/`-vv` verbosity flags are the one deliberately
  git-shaped exception.

---

## 8. File format details

### 8.1 Index line grammar

```
^(\s*-\s*\[)([ >x])(\]\s*)(.*)$
```

One glyph group over space (`open`), `>` (`in_progress`), and `x` (`closed`).
Trailing ID token: `(?:^|\s)#([a-z][a-z0-9-]{2,})\s*$` (lowercase; minus only).
Title is everything before the ID.

### 8.2 Detail frontmatter — frozen flat YAML subset

**No full YAML library.** The frontmatter is a **closed, flat,
YAML-compatible subset** — valid YAML that a strict parser can read, but the
`bl` parser implements only the subset and **rejects** any out-of-subset input
with a clean error (never a silent misread). Rationale: a full YAML dependency
is non-deterministic and heavyweight, and would break the byte-stability
invariant `backlog.md == f(details)` (§3.1); a fully custom format would cost
the same to parse and lose Markdown-frontmatter compatibility for nothing.

**The subset has no booleans and no nesting.** The complete field set (§5.3)
needs only five lexical forms:

| Form | Matches | Used by |
|---|---|---|
| enum | one of a fixed token set | `status` |
| int | `-?[0-9]+` | `priority` |
| bare scalar | `[A-Za-z0-9][A-Za-z0-9._:-]*` | timestamps, IDs |
| inline list | `[` scalar (`,` scalar)* `]` | `deps`, `attachments` |
| quoted string | `"…"` (backslash escapes) | any scalar needing it |

The parser is therefore a flat line loop: split `key: rest` at the first colon,
parse `rest` as one of the forms above. No indentation tracking, no nesting
stack, no state machine.

**Fence rules.**
- Frontmatter starts at byte 0 with `---\n` and ends at the next line that is
exactly `---`.
- The Markdown body after that fence may contain `---` freely (it is prose).
- A file without a leading fence is treated as a body-only ticket with default
  fields; it is never treated as a parse error that loses data.

**Parser MUST reject (error, do not guess):**
- block scalars `|`, `>`; anchors/aliases `&`/`*`; merge keys `<<`
- flow maps `{}`; any nested/indented map
- multi-line arrays (lists are inline only)
- implicit `null`/`~`; bool-ish bare words (`yes`, `no`, `on`, `off`, `true`,
  `false`) — there are no booleans in the subset
- duplicate keys; tab indentation; trailing content after a value that is not a
  valid list form

**Writer rules (canonical, byte-stable).**
- Fixed field order; one field per line; lists inline `[a, b]`, **sorted** for
  `deps`/`attachments` (§5.2).
- Quotes only when a scalar is not a safe bare token (e.g. a title with spaces),
  and always for a token ending in `:` (a trailing colon is invalid or ambiguous
  YAML under a reference parser).
- No comments in emitted frontmatter; timestamps/ints/enums/IDs are bare.
- Timestamps are always UTC ISO-8601 `YYYY-MM-DDTHH:MM:SSZ` (never re-encoded
  by the parser).

**Unknown/user keys stay opaque.** The parser types only the known keys; every
other `key: rawvalue` line is preserved as an **opaque string** and written back
verbatim. The parser therefore never has to understand arbitrary YAML, and
preservation (§8.3) is trivial and lossless. Unknown keys are never
schema-validated and never filterable.

### 8.3 Serialization stability

- Keep field order stable; one field per line; lists inline `[a, b]`.
- Preserve unknown and user fields on rewrite verbatim (§8.2).
- Round-trip must be byte-identical: `serialize(parse(text)) == canonicalize(text)`,
  and a second `f` pass produces no diff (idempotence, §3.1).
- File mode preserved across atomic replace (backlogger does this).

---

## 9. Distribution & license

- Single static binary, built with `CGO_ENABLED=0` by `make build` and the
  release workflow. Distribution is via GitHub only — source checkout,
  `go build`, and tagged GitHub Releases. Creating a release triggers
  `.github/workflows/release.yml`, which builds `bl` for linux/windows/darwin
  (`386`, `amd64`, `arm64`) and uploads one archive per target as a release
  asset. No Homebrew tap, AUR package, or `goreleaser` pipeline. Local
  installation is `make install` or `bin/install.sh` (build from source);
  `bin/download.sh` fetches the latest tagged archive for the current OS.
- **Version identity.** A release build embeds a version string (from
  `git describe --tags --always --dirty`) into the binary; `bl version` prints
  it on one stable line (e.g. `bl v0.1.0-3-gabc1234`). A plain `go build` with
  no version injection reports `bl dev`. The release workflow passes the tag
  as `-X github.com/yantonov/backlogger/internal/cli.Version=<tag>`. The string
  is never read from the backlog and never affects `f(details)`.
- Licensed under the **Apache License 2.0**.
- **No extension mechanism**: every mutation stays inside the
  one locked, validated code path. No plugin API is published before the core is
  stable. The seam is *reserved* but unimplemented: unknown `bl <cmd>` errors
  with usage; a future version may add subprocess commands / post-commit hooks
  that mutate **only by calling back into `bl`** (never by editing files).

---

## 10. Testing strategy

- Unit tests for lock, atomic write, ID uniqueness, parser/serializer round-trip.
- **Frontmatter subset tests (§8.2):**
  - Property test: every file the writer emits parses identically under a
    reference YAML parser (`yaml.v3`, **test-only dependency**). This proves
    "valid YAML subset"; it does not assert it.
  - Failing-loud test: out-of-subset fixtures (`|`, `&x`, `{}`, nested maps,
    `no`/`true`, duplicate keys, tabs) each produce a clean error, never a
    silently coerced value.
  - Preservation test: unknown/user keys survive a parse→rewrite byte-for-byte.
  - Byte-stability test: `f` run twice produces no diff (idempotence, §3.1).
- **Multi-process concurrency tests:**
  - N parallel `add` → all present exactly once, unique IDs, no lost lines.
  - Two parallel `start` of one free ticket → exactly one success.
  - Kill a process mid-transaction → file is whole (old or new); next op works.
  - Lock timeout returns a clean error and leaves the file untouched.
- Property tests: parser ↔ serializer idempotence for arbitrary titles/fields.
- CLI tests for the command surface: in-process tests plus a subprocess suite
  that drives the built binary. See `docs/testing.md` for the spec-item → test
  mapping.

---

## 11. Borrowed-by-design summary

From **backlogger**:
- `flock` + `.lock` file, whole-cycle transaction, bounded timeout.
- Atomic `os.replace`, never treat read errors as empty.
- Compare-and-set `start`; guarded status transitions; expected-revision edits.
- Visible trailing `#id`; index/archive in one readable file; `doctor`.
- Multi-process tests; honest "no ownership/lease" boundary.

From **ticket**:
- One file per ticket with YAML frontmatter + Markdown prose sections.
- Metadata (priority, created/updated).
- Dependencies (`deps`), `started`/`blocked`, `dep tree`/`cycle`.
- Partial-ID matching; single static binary, distributed via GitHub.
- Plugin/extension spark (deferred).
