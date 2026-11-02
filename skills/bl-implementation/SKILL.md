---
name: bl-implementation
description: "Work a slice of a `bl` backlog end to end: one ticket at a time, one local commit per ticket. Triggers on 'work the backlog', 'implement the tickets', and a request to close several tickets in sequence."
---

# Implementing a backlog slice

`bl` holds the claim protocol and `bl-decompose` cuts the work. This skill turns
cut tickets into local commits: pick, claim, branch, delegate, verify, commit,
close, repeat.

## Fix the scope

A **slice** is the set of tickets this run implements. Agree on it before the
first claim and write down its filter: a label, a component, or a list of IDs.
Every step then answers one question with that filter: in scope or out.

Done when the filter names at least one open ticket.

## Sources of truth

- **Ticket detail:** the `.tickets/<id>.md` body.
- **Ticket state:** `backlog.md`. Read it to see state, never to choose order.
- **Design:** the project design doc, usually `docs/spec.md`. When the body and
  the doc disagree, stop and ask.
- **Dependencies:** one way. The slice may depend on finished work. Nothing the
  slice builds points back into the slice. An edge back into finished work means
  the scope is wrong.

## The loop

Work one ticket at a time. Start ticket *n+1* only after you close ticket *n*.
That sequence keeps the history linear.

1. **Pick.** Run `bl available`. Take the first in-scope ticket. The list is
   dependency-ordered, so it beats line order in `backlog.md`.
   Done when you hold one in-scope ID.

2. **Claim.** Run `bl start <id>`.
   Done when the command exits 0. On a non-zero exit the ticket moved. Run
   `bl available` again and pick from the new list.

3. **Branch.** Cut the ticket branch from the previous ticket's branch. The slice
   then forms one line, and one branch per ticket records the sequence. Name the
   branch after the ticket ID. When a branch for that ID exists, use it as-is.
   Done when `git log` on that branch shows the previous ticket's commit.

4. **Delegate.** Spawn one subagent per ticket in a separate agent process, which
   keeps this session small. Ask the user which agent tool and model to use, and
   name both in the subagent prompt. Any agent CLI works, for example
   `pi -p`, `claude -p`, or `codex exec`, so ask rather than assume the one you
   run in.

   The prompt carries the ticket ID, the ticket body, the repo, the branch, the
   design-doc pointer, and one sentence that the subagent updates this ticket's
   problem. It also carries two standing requirements:
   - Write tests for all new logic.
   - Write each comment to give the reason, not to restate the code.

   Done when the subagent exits with the ticket changes in the worktree.

5. **Verify.** Run the project gate. Where the project defines one, it is
   `make check`. Run the ticket acceptance criteria too.
   Done when the gate is green, including the tests that existed before this
   ticket, and every acceptance criterion passes.

6. **Commit.** The parent session commits. The subagent does not. One commit per
   ticket:

       <one-line subject>

       <details>

       subtask: <backlog-id>

   Done when `git log -1` shows that message and `git status` is clean.

7. **Close.** Run `bl done <id>`. For a partly finished ticket, leave it
   `started` and record the rest: `bl edit <id> --note "…"`. Then return to
   step 1.
   Done when the ticket state and the branch agree.

The commits stay local. Push and review come after the slice.

## Output

The run ends when `bl available` holds no in-scope ticket. Report one local
branch and one commit per ticket, `.tickets/` and `backlog.md` in sync, and each
ticket left `started` with the reason.

## Escalate to human (stop and ask)

- The filter matches no open ticket, or matches work outside the design doc.
- The ticket body and the design doc disagree, or an acceptance criterion has
  no observable result.
- The ticket needs a dependency, a runtime package, or a schema change that the
  body does not name.
- The gate stays red unless a decided choice changes.
