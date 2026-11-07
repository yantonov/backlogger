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

Also agree the **branch strategy** before the first claim. Ask for it the same
way you ask for the agent tool and model. Offer three strategies, each with its
trade-off:

- **Stacked ticket branches**: one branch per ticket, each cut from the previous.
  This suits a run where each ticket gets its own review. It leaves one branch
  per ticket to manage.
- **Integration branch**: one commit per ticket directly on `master`, or on
  whatever branch the repo integrates to. This suits a solo run reviewed at the
  end. It keeps history linear, with no branches to fold.
- **Hybrid**: stacked ticket branches folded into the integration branch at a
  boundary you name.

Record the strategy next to the filter, so a resumed session keeps it.

Done when the filter names at least one open ticket and you wrote the strategy
down.

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

3. **Branch.** Follow the strategy agreed at scope time.

   One invariant holds in all three strategies: the new work descends from the
   previous ticket's commit. Stacked branches show that commit in `git log`. An
   integration branch contains it.

   - *Stacked branches:* cut from the previous ticket's branch. When a branch for
     this ID exists, use it as-is.
   - *Integration branch:* commit directly on it.
   - *Hybrid:* cut from the previous ticket's branch until the agreed boundary,
     then follow `## Consolidate`.

   Resuming after a fold: when the previous ticket's branch is gone, its commit
   is in the integration branch. Cut from there. Treat the missing branch as a
   completed fold once `git merge-base --is-ancestor <sha> <integration>`
   passes.

   Done when `git log` shows the previous ticket's commit in the new work's
   ancestry.

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

   Before you spawn, print one line: the ticket, the agent and model, and that
   the session now waits on them. A delegation that runs for minutes reads as a
   hang when the human sees no line first.

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
   `started` and record the rest: `bl edit <id> --note "…"`.
   Done when the ticket state and the branch agree.

8. **Report.** Print one line for the human as the ticket closes, then return to
   step 1. A ticket is the unit of progress, so this line fires on each close and
   nowhere finer. It is the heartbeat that shows the run is alive, not the
   end-of-run report.

       [<done>/<total>] <id> closed · <sha> · gate <green|red> · next: <id>

   When the ticket stays `started`, close the same line with the remaining work
   in one sentence. Print the line on a red gate too: a stalled ticket is the one
   the human needs to see early.
   Done when the line carries all five fields.

The commits stay local. Push and review come after the slice.

## Output

The run ends when `bl available` holds no in-scope ticket. Report the strategy you
used, the integration branch the work sits on, and one commit per ticket. Report
that `.tickets/` and `backlog.md` agree, and that each unfinished ticket sits
`started` with the reason. Keep every report to the step 8 line: the id, the
commit, the gate state, and the next id.

A slice of more than five tickets also reports at each slice boundary: when a
root of the dependency tree finishes, and before work the human may want to
redirect, such as a large refactor, a schema change, or any item on the escalate
list. The per-ticket line is the heartbeat. The boundary report is the place the
human steers.

## Consolidate

Only for the stacked and hybrid strategies. Consolidate at the boundary named at
scope time, or at the end of the slice.

- Fold with `git merge --ff-only <tip-branch>` when the chain is linear. A
  fast-forward makes the history read as ticket order. A merge commit buries that
  order.
- Run the gate before the merge and again after it. A green branch proves the
  branch. It does not prove the merge.
- Delete a folded branch after `git branch --merged` lists it. Deleting an
  unmerged branch discards work.
- Bookkeeping adds noise. Commits to `.tickets/` and `backlog.md` land wherever
  checkout happened to be, so a fold can carry a commit that is not a ticket.
  Either commit bookkeeping on the integration branch or accept it in history.

Done when the gate is green on the integration branch and you either deleted each
folded branch or listed it as retained.

## Escalate to human (stop and ask)

- The filter matches no open ticket, or matches work outside the design doc.
- The ticket body and the design doc disagree, or an acceptance criterion has
  no observable result.
- The ticket needs a dependency, a runtime package, or a schema change that the
  body does not name.
- The gate stays red unless a decided choice changes.
