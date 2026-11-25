---
title: Transaction discard prompt reopens after navigating back
date: 2026-11-25
category: ui-bugs
module: Transactions
problem_type: ui_bug
component: frontend
symptoms:
  - The discard prompt closes after moving to another transaction and reappears when returning to the original transaction.
root_cause: wrong_api
resolution_type: code_fix
severity: low
tags: [transactions, base-ui, alert-dialog, controlled-state, navigation, drafts]
---

# Transaction discard prompt reopens after navigating back

## Problem

Confirming **Discard and move** in Transaction details could leave the pending confirmation in memory. The old prompt reappeared on return to the transaction, even though its draft was gone.

## Symptoms

- Edit a transaction, navigate to a neighboring transaction, and confirm the discard. The drawer shows the neighbor and the prompt disappears.
- Navigate back to the original transaction. The prompt reappears and blocks interaction with its details.

## What Didn't Work

The original confirm handler started navigation. Moving away hid the controlled dialog because the IDs differed. That change in visibility left the pending request intact, so the IDs matched again when the reviewer came back.

## Solution

Clear the pending discard in the confirm handler before calling navigation. Keep the transaction ID guard for the request that is currently pending. [TransactionInspectorDrawer](../../../components/transaction-inspector-drawer.tsx)

```tsx
onClick={() => {
  setDiscard(null)
  if (discardDirection) void review.navigate(discardDirection)
}}
```

## Why This Works

`AlertDialogAction` renders the project's `Button`, so its click handler owns any state change needed to close a controlled alert. `AlertDialogCancel` uses the close primitive. [Alert dialog wrappers](../../../components/ui/alert-dialog.tsx)

In `TransactionInspectorDrawer`, the render derives `discardDirection` from the pending request and the current transaction ID. Clearing the request on confirmation makes that value empty even if the same transaction becomes current again. The confirmed direction is still available in the handler's closure. [TransactionInspectorDrawer](../../../components/transaction-inspector-drawer.tsx)

## Prevention

In controlled confirmations, read the local action wrapper and decide at which event the pending request should be cleared. For a discard that has already been confirmed, clear it in the action handler before any asynchronous navigation.

The Chromium regression check edited a note, chose **Keep editing** once, then confirmed **Discard and move** on a second attempt. The check verified that the note was absent from the saved transaction, returned to that transaction, and closed the drawer without another discard prompt. Include the return to the original transaction when checking this flow. A check that ends on the neighboring transaction misses the stale prompt.

## Related Issues

- [Magic enrichment can drop a tag created while a transaction form is open](magic-enrichment-drops-pending-tag-selection.md) covers another pending-state issue in Transaction details.
- [Base UI Button render prop and link semantics](base-ui-button-render-link-semantics.md) shows another case where the local wrapper's rendered primitive determines behavior.
