---
title: Magic enrichment can drop a tag created while a transaction form is open
date: 2026-11-25
category: ui-bugs
module: Transactions
problem_type: ui_bug
component: frontend
symptoms:
  - A tag created from Transaction details or Combine can disappear from the selection after Magic returns, even though it remains in the shared tag library.
root_cause: concurrency
resolution_type: code_fix
severity: medium
tags: [transactions, tags, magic-enrichment, async-state, race-condition]
---

# Magic enrichment can drop a tag created while a transaction form is open

## Problem

Inline tag creation and Magic enrichment could overlap in Transaction details and Combine. If Magic later applied a tag change calculated from an older form state, the new tag could disappear from the transaction draft. The library creation had already succeeded.

## Symptoms

- The new tag appears in the picker, then disappears from the selected tags when Magic applies its suggestions.
- The tag still exists in the shared library and can be selected again.

## What Didn't Work

The initial pending-state guard covered Save and Combine. It protected the submitted payload but left Magic available while the create request was in flight. Disabling the picker during enrichment only covered the opposite start order.

## Solution

- In both hosts, disable the Magic button and return early from `handleMagicEnrich` while `tagCreatePending` is true. Keep the picker disabled while enrichment is running. See [Transaction details](../../../components/transaction-inspector-drawer.tsx) and [Combine](../../../components/manual-merge-dialog.tsx).
- In `TagMultiSelect`, the mutation-level callbacks maintain the shared tag cache. The per-call callbacks select the returned ID and release the initiating form's pending state. This keeps the library update independent of the form session. See [tag picker](../../../components/tag-multi-select.tsx).

## Why This Works

Each host's `handleMagicEnrich` supplies a callback that reads the selected tag IDs from the render that started enrichment. `resolveMagicDraft` merges suggestions into those IDs and the host replaces the selection when the preview changes Tags. An ID appended by a pending tag creation can be absent from that earlier selection. After a successful create, the guard lets Magic start only once the new ID is in the selection. If Magic runs first, disabling the picker prevents a new create from starting. See [enrichment draft resolution](../../../components/magic-enrich.tsx).

## Prevention

When an async form action replaces a field using values captured at the start, check which other pending actions can still update that field. Guard both start orders or merge the result into the latest draft when the result arrives.

To check this interaction in both forms, delay the tag-create response, close just the picker, and verify Magic stays unavailable. Release the response, run Magic with a different tag suggestion, and verify the newly created tag remains selected. Reverse the order and verify the picker is disabled until Magic finishes.
