---
title: Inventory all fields before sharing create and edit forms
date: 2026-11-25
last_updated: 2026-11-26
category: design-patterns
module: Transactions
problem_type: design_pattern
component: frontend
severity: low
applies_when:
  - Extracting shared fields from related create and edit forms
tags: [transactions, forms, react, refactoring]
---

# Inventory all fields before sharing create and edit forms

## Context

The transaction refactor used [AccountFormDialog](../../../components/account-form-dialog.tsx) as the model for sharing create and edit field markup. The first pass extracted the fields with very similar markup and left Merchant, Remark, and occurred time in their callers. Merchant and Remark already had the same input structure. The shared component could handle their different labels, requiredness, placeholders, and placement. Each form could supply its own change handler.

## Guidance

List the fields in both forms before choosing the shared component's props. For each field, record where it renders, how it's labeled, when it's required, and how the caller uses the value. When a field differs in presentation or requiredness, configure those differences in the shared component. Keep each caller's draft updates and submit behavior with the caller.

When an entire section is optional, group its inputs in one optional prop. Use the prop's presence to render both the tab trigger and its panel. An empty section can still be available, so keep its count separate from the decision to show it.

[TransactionFormFields](../../../components/transaction-form.tsx) renders Remark and the date and time picker in both the [add dialog](../../../components/add-transaction-dialog.tsx) and the [inspector](../../../components/transaction-inspector-drawer.tsx). Both callers use `TransactionForm` for the Note tab and handle UTC conversion when their submit payload includes the date.

The add dialog previously hid Source emails with a flag and still passed a dummy count. It now omits `sourceTab`. The inspector supplies that prop even when its count is zero, so the tab stays available and shows an empty message when selected. When the inspector has source emails, it supplies the list as the tab content.

## When to apply

Before calling a field extraction complete, walk the original inventory in both forms. Verify that each field has a shared implementation or a concrete reason to be implemented locally. Check differences in layout, validation, and submitted values against each caller. For optional sections, verify all applicable states: omitted, visible and empty, and populated.

The transaction forms also illustrate why draft behavior belongs with the caller. The inspector disables Magic while a tag create is pending because both actions can change the selected tags. When moving a picker, preserve the callback that lets its caller coordinate that work. See [the tag creation and Magic enrichment race](../ui-bugs/magic-enrichment-drops-pending-tag-selection.md).
