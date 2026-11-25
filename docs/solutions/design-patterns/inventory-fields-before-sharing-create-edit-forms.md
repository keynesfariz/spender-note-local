---
title: Inventory all fields before sharing create and edit forms
date: 2026-11-25
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

[TransactionFormFields](../../../components/transaction-form-fields.tsx) renders Merchant as a required name for manual entry and Remark as editable display text in the inspector. The [add dialog](../../../components/add-transaction-dialog.tsx) supplies a date only and renders Note inline. The [inspector](../../../components/transaction-inspector-drawer.tsx) supplies a date and time to one picker and renders the shared Note field in its own tab. Each caller handles UTC conversion when its submit payload includes the date.

## When to apply

Before calling a field extraction complete, walk the original inventory in both forms. Verify that each field has a shared implementation or a concrete reason to be implemented locally. Check differences in layout, validation, and submitted values against each caller.

The transaction forms also illustrate why draft behavior belongs with the caller. The inspector disables Magic while a tag create is pending because both actions can change the selected tags. When moving a picker, preserve the callback that lets its caller coordinate that work. See [the tag creation and Magic enrichment race](../ui-bugs/magic-enrichment-drops-pending-tag-selection.md).
