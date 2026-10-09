# AGENTS.md

## Project

`bl` is a local-first, directory-scoped task tracker: one committed index
(`backlog.md`) derived from one detail file per ticket under `.tickets/`. It is
correct under concurrent writers and git-friendly.

Stack: Go. Module `github.com/yantonov/backlogger`, binary `bl`.
Install and run: see `README.md`.

## Commands

The `Makefile` is the source of truth for build and test targets; read it, do
not restate it.

- **Full verification: `make check`.** Run it before every commit.

## Definition of Done

Done when the acceptance criteria pass, the gate is green, `docs/spec.md` and
`docs/testing.md` reflect the change, and there is exactly one commit in the
repo's commit style.

Not done: "code is written", "looks right", "tests pass on another box".

## Hard constraints (MUST / MUST NOT)

- **MUST: one mutation path.** Lock -> read detail -> verify -> write detail
  atomically -> regenerate `backlog.md` -> release. Every write goes through it.
- **MUST: propagate read errors.** An error on an existing file is an error,
  never an empty backlog.
- **MUST: keep `f(details)` byte-stable.** Stable sort, fixed field order,
  timestamps read from stored fields only, deterministic iteration.
- **MUST: key `--json` off the flag, not the tty.** A pipe changes nothing.
- **MUST NOT: ship a runtime dependency.** Only `gofrs/flock` ships; `yaml.v3`
  is test-only; everything else is stdlib.
- **MUST: create state only in `add`.** Reads on a missing `.tickets/` error
  with guidance.
- **MUST: name IDs as lowercase filename stems.** Underscore and the names
  `archive`/`attachments` are reserved.
- **MUST: commit the `.tickets/` backlog.**

## Work rules

- **WIP = 1:** one coherent unit of work, one commit, bisectable history.
- Keep the layering `cli -> store -> {model, frontmatter, index, graph, ids,
  lock}`; `model` and `frontmatter` depend on nothing internal.
- Every mutation goes through the one mutation path above. Reads are index-only
  except `show`, graph commands, and `doctor`.
- **Commit style:** one-line imperative subject, no trailing period; an optional
  *why* body wrapped at ~72 columns. Plain text only: no attribution, no
  `Co-authored-by`, no tool signatures.

Example commit:

    Add frozen YAML frontmatter subset parser

    The parser is a flat line loop over the five lexical forms in spec 8.2 and
    rejects every out-of-subset construct loudly, so f(details) stays byte-stable.

### Package layout

    cmd/bl/main.go   thin entry: wire deps, dispatch, exit codes
    internal/model/        Ticket struct, Status enum, canonical field order
    internal/frontmatter/  frozen flat YAML subset: parse + canonical serialize
    internal/ids/          grammar, generation, lowercasing, partial resolution
    internal/lock/         cross-platform exclusive advisory lock + timeout
    internal/store/        paths, atomic write, detail read/write, tx glue
    internal/index/        f(details): render + parse backlog.md, idempotence
    internal/graph/        deps, cycle detect, tree, unresolved warnings
    internal/attach/       sidecar copy/remove/list
    internal/doctor/       invariants + fix
    internal/cli/          command handlers, human + --json rendering

## Escalate to human (stop and ask)

Stop and ask when a decision is unresolved in `docs/spec.md` and the default is
unsafe; the gate cannot go green without changing a decided choice (module path,
storage model, index semantics); the work needs a runtime dependency beyond the
lock library; or the request conflicts with `docs/spec.md` in a way that drops an
invariant.

## Details (read when needed)

- `docs/spec.md` — the design spec (behavior, invariants, command surface).
  **Source of truth.** Read the section you are about to change.
- `docs/testing.md` — spec test categories mapped to concrete tests. Read before
  writing a test; update it when you add one.
