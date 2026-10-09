---
name: bl-decompose
description: "Decompose a large topic or ticket into independently closable tasks sized S/M/L/XL. Prefer S and M over L and XL. Triggers on 'decompose', 'break down', 'split this ticket', 'S/M/L/XL', 'size this', 'chunk this', and when a ticket crosses subsystems."
---

# Task decomposition

## Sizes

- **S:** one atomic change, one commit. Under an hour.
- **M:** one coherent change, a few files. Single commit or short stack. A few hours.
- **L:** a change that crosses subsystems. Multiple commits in order. A day.
- **XL:** an epic: several L tasks, unclear boundary, unstable scope. Days to a week.

The **decomposition threshold** sits between M and L. An L ticket survives review
only when it cannot be split further. An XL ticket is a sign to split before
starting.

## Why S and M win

- **Closed sooner.** Each close commits the work, regenerates the backlog, and
  moves state. Long-running tickets drift.
- **Reviewed in one sitting.** A reviewer holds an M. An L forces paging a diff.
- **Reverted independently.** A stack of S commits means one bad change reverts
  without unpicking the rest.
- **Resumed after interruption.** An S finishes in a session. An L loses
  context.

## Method

1. **List verifiable outcomes.** Start from the acceptance criteria. Write each
   piece as "when this is done, X is true." A piece without a clear
   done-condition is still too large.

2. **Size each piece.**
   - One commit closes it → S.
   - One subsystem, a few files → M.
   - Crosses subsystems → L. Split it.
   - Contains its own sub-plan → XL. Restart from step 1 with this piece as the
     whole.

3. **Split L and XL pieces by the done-condition.**
   - **By layer:** data model, then storage, then CLI, then rendering. Each
     layer closes on its own.
   - **By path:** happy path first, error handling second, edge cases third.
     Ship the happy path, then harden.
   - **By capability:** add the flag (one ticket), wire it to the store (next),
     render it in output (third).
   - **By the test:** one ticket delivers the test and the minimal code. The
     next adds another test and the code it demands.

4. **Sequence the pieces.** The output is a DAG: which tickets block which.
   S tickets rarely need internal dependencies. M tickets may depend on one S.
   A chain of four or more dependencies signals over-decomposition or work that
   is genuinely sequential.

5. **File each piece.** Create one `bl` ticket per piece. Wire dependencies so
   `bl available` surfaces the next unblocked piece. The `bl` skill covers the
   exact commands.

## When to keep a ticket whole

- An S that is already one commit. Sub-tickets add ceremony.
- An M whose pieces touch the same three lines. Re-merging costs more than
  splitting.
- Work where intermediate states cannot be verified on their own.

## Checklist

Each piece must satisfy all four:

- **Closable:** `bl done` leaves a consistent repo.
- **Verifiable:** the done-condition is a statement that can be checked.
- **Sized:** S or M. L and XL pieces split again.
- **Independent:** claimable without holding every other ticket.
