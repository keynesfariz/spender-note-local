---
title: Reuse shared shadcn controls in feature forms
date: 2026-11-26
last_updated: 2026-11-26
category: design-patterns
module: UI Components
problem_type: design_pattern
component: frontend
severity: low
applies_when:
  - Building or changing a create or edit form
  - Adding a form field or a form entry point
tags: [shadcn, base-ui, select, date-picker, drawer, forms, create-edit, input-group]
---

# Reuse shared shadcn controls in feature forms

## Context

Add Transaction used a native select for transaction type. The Transfer form used native selects and `datetime-local` inputs. Accounts Transfer used the shared drawer content and a separate button that mounted the drawer in its open state. The user noticed that the controls looked different and that the drawer opened differently from other account drawers.

## Guidance

1. **Share the create and edit components.** Start with one form component for both modes. If the surrounding workflows differ, reuse the same field components in both. Inventory both modes first, then use props for differences such as initial values, labels, required fields, and disabled state. Keep mode-specific validation, draft updates, and submit behavior with the caller.
2. **Use the shared shadcn Drawer for forms by default.** Use [Drawer](../../../components/ui/drawer.tsx) and reuse [CreateFormDrawer](../../../components/detail-drawer-content.tsx) for the standard form layout. For a form with its own open button, use a `DrawerTrigger` under the same drawer root as the form. Keep that root mounted while closing and run teardown from `onOpenChangeComplete`. Creating or renaming one tag uses the same shared shadcn `Dialog`. Use a drawer for a bulk tag form if that flow is added.
3. **Use shadcn components for every field.** Start with the shared components in `components/ui/`. If a component is missing, add it from the project's shadcn registry configuration so future forms can reuse it.

| Field | Shared component |
| --- | --- |
| Choices | [Select](../../../components/ui/select.tsx). Pass one option array to `items` and use it to render the `SelectItem` children. |
| Single-line input | [Input](../../../components/ui/input.tsx) |
| Input with an icon | Use `InputGroup` with an `InputGroupInput` and put the icon in `InputGroupAddon`. [shadcn Input Group](https://ui.shadcn.com/docs/components/base/input-group#icon) |
| Multiline input | [Textarea](../../../components/ui/textarea.tsx) |
| Date or date and time | [DatePicker](../../../components/date-picker.tsx) uses the shared shadcn Calendar. Supply its `time` prop for date and time. The form owns timezone conversion and decides what clearing means. |

## Why This Matters

A feature can use the shared drawer markup and still bypass the normal open and close lifecycle if it mounts the root already open from an external button. The trigger and root pairing gives the drawer an opening control to restore focus to. Waiting until close completes gives the exit animation time to finish before cleanup.

## When to Apply

Use this pattern when adding controls to a feature form or introducing a second entry point for an existing form. In linted TypeScript files, the [ESLint config](../../../eslint.config.mjs) catches bare `select` elements and literal `date` or `datetime-local` types on `input` and `Input`. For drawer changes, review the trigger hierarchy and check that dismissal and successful submission finish the closing animation and restore focus in the host flow.

## Examples

[AccountsView](../../../components/accounts-view.tsx) uses one [AccountFormDialog](../../../components/account-form-dialog.tsx) for create and edit. The create and edit transaction flows both render [TransactionForm and TransactionFormFields](../../../components/transaction-form.tsx) and supply their own draft and save behavior.

[Accounts Transfer](../../../components/accounts-view.tsx) passes a `DrawerTrigger` to [TransferFormDialog](../../../components/transfer-form-dialog.tsx). The form uses `DatePicker` for transaction time and both posting times. The posting fields are clearable, and blank posting times fall back to the transaction time. When callers supply success or close callbacks, the transfer form handles them after the drawer finishes closing.

## Related

- [Base UI Select option labels](../ui-bugs/base-ui-select-items-label-resolution.md) explains why the closed trigger needs option data.
- [Inventory fields before sharing create and edit forms](inventory-fields-before-sharing-create-edit-forms.md) covers keeping validation and draft behavior with each caller.
