---
title: Cloning review selection breaks all transaction inspector actions
date: 2026-11-27
category: ui-bugs
module: Transactions
problem_type: ui_bug
component: frontend
symptoms:
  - "Save changes, Save changes & next, Mark reviewed, Previous, Next, and Close stop responding in the transaction inspector drawer."
  - "Only actions outside useTransactionReview (such as Magic LLM) still work."
root_cause: logic_error
resolution_type: code_fix
severity: high
tags: [transactions, react, ref-identity, async-state, inspector-drawer, navigation]
---

# Cloning review selection breaks all transaction inspector actions

## Problem

When `useTransactionReview` resolves a transaction's position asynchronously (for example, locating an item in the unfiltered ledger when a day filter is active), cloning `session.selection` to attach the resolved position silently breaks every action routed through `review` — including saving, marking reviewed, navigating Previous/Next, and closing the drawer.

## Symptoms

- Open a transaction from a date-filtered ledger view.
- Click **Save changes**, **Save changes & next**, **Mark reviewed**, **Previous**, **Next**, or the **Close** button: nothing happens and no error is shown.
- **Magic LLM** still works because it does not go through `useTransactionReview`.

## Root Cause

`useTransactionReview` guards `run()` against stale async callbacks using object reference identity:

```ts
if (
  !selection ||
  openSelection.current !== selection ||
  pendingSelection.current === selection
) {
  return
}
```

When `open(id)` resolved the position asynchronously and wrote `selection: { ...previous.selection, position: found }` into React state, `session.selection` became a new object reference while `openSelection.current` still pointed to the original `nextSelection`. Every subsequent `review.write()` and `review.navigate()` call saw `openSelection.current !== selection` and returned early. Even updating `openSelection.current` alongside `setSession` remained fragile across Fast Refresh when the drawer was already open.

## Solution

Keep `session.selection` immutable for the lifetime of an open transaction and store asynchronously resolved metadata on `ReviewSession` instead of replacing `selection` in [use-transaction-review.ts](../../../hooks/use-transaction-review.ts):

```ts
type ReviewSession = {
  selection: Selection
  pending: boolean
  error?: string
  savedPosition?: ReviewPosition
  resolvedPosition?: ReviewPosition
}

const applyFound = (found: ReviewPosition) => {
  setSession((previous) =>
    previous?.selection === nextSelection
      ? { ...previous, resolvedPosition: found }
      : previous
  )
}
```

Derive `position` with `session?.resolvedPosition ?? selection.position`, and let `close()` clear any active `selection` (`if (selection) select(null)`).

## Prevention

Treat `Selection` in `useTransactionReview` as an immutable identity token — never clone or replace `session.selection` for the same open transaction. Attach any post-open async state (such as `savedPosition` or `resolvedPosition`) as sibling fields on `ReviewSession` so `openSelection.current === selection` stays intact.

## Related Issues

- [Transaction discard prompt reopens after navigating back](transaction-discard-prompt-reopens-after-navigation.md) covers another state-lifecycle issue in Transaction details.
