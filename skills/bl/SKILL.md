---
name: bl
description: "Work a `bl` backlog: pick up, claim, close, and file tickets; manage dependencies. Triggers on a repo with `.tickets/` or `backlog.md`, the `bl` command, and 'backlog', 'next ticket', 'claim', 'close', 'file a ticket', 'blocked', 'dependency'."
---

# Working a `bl` backlog

`bl` is a local task tracker. The command surface is `bl help` — run it, don't
cache it. This skill covers the protocol `bl help` cannot: how to claim work so
concurrent writers stay correct, and the rules the command surface assumes.

## The claim loop

Every ticket follows this path: open → claimed → closed. Only `bl add` creates
a ticket; only `bl start` claims it; only `bl done` closes it. Every mutation
goes through `bl`, which holds the lock, verifies state, writes the detail file
atomically, and regenerates `backlog.md`. Editing files by hand races other
writers.

1. **Find.** Run `bl available` — open tickets with all dependencies met,
   ordered best-first. Done when you have the top ID.
2. **Claim.** Run `bl start <id>`.
   - Done when the command exits 0. The ticket is now yours.
   - On exit ≠ 0, the ticket moved under you. Re-run `bl available` and claim
     what is there now. Never retry blindly and never write the file to force it.
3. **Read.** Run `bl show <id>` (`--json` when you need structured output).
   Done when you understand the description, notes, and dependencies.
4. **Do the work.**
5. **Close.** Run `bl done <id>` when finished. If you cannot finish, leave the
   ticket `started` and record what remains: `bl edit <id> --note "…"`.

## Filing work

- `bl add "Title"` returns the generated ID. Priority is `-p 0` (highest)
  through `4`; default `2`.
- File discovered work as its own ticket instead of holding it in your head.

## Relating tickets

- `bl dep add A B` means *A depends on B*. A stays out of `available` until B
  is closed.
- `bl dep add` refuses an edge that would create a cycle.

## Reading the backlog

- IDs are lowercase filename stems. They abbreviate to any unique substring
  (`bl show log` finds `login`; an ambiguous prefix is an error listing the
  candidates). Operands take the bare ID, not `#id`.
- Views print bare IDs by default: `bl available | head -n1 | xargs bl start`
  works. Add `-v` for ID + title, `-vv` for the full index line.
- Pass `--json` when you need to parse. Piping never changes the format.
- Reads on a project with no `.tickets/` error with guidance. Only `bl add`
  creates a backlog.
- When something looks inconsistent, run `bl doctor`.
