---
title: Base UI Select displays raw value or ID instead of label when closed
date: 2026-11-22
category: ui-bugs
module: UI Components
problem_type: ui_bug
component: frontend
symptoms:
  - "Closed or unfocused `<Select>` triggers display raw enum keys, sentinel strings, or UUIDs (e.g., `credit_card`, `card_last4`, `all`, `__unassigned__`, `__none__`, account UUIDs, month numbers, country codes) instead of human-readable `<SelectItem>` labels."
  - "Account selects in dialogs flash a raw account UUID when a transaction loads before the `accounts` list finishes fetching."
root_cause: wrong_api
resolution_type: code_fix
severity: medium
framework_version: "@base-ui/react 1.9.0 / next 16.5.0-canary.60"
tags: [base-ui, shadcn, select, items-prop, labels]
---

# Base UI Select displays raw value or ID instead of label when closed

## Problem
Every `<Select>` in the app displayed its raw underlying value (enum keys like `credit_card`, sentinel strings like `__unassigned__` or `all`, month numbers, country codes, or account UUIDs) in the closed/unfocused trigger instead of the human-readable option label shown inside `<SelectItem>`.

## Symptoms
- Closed `<SelectTrigger>` elements across Dashboard, Transactions, Settings, and all account/transaction dialogs showed raw values/IDs instead of option labels.
- In dialogs initialized from a transaction (`TransactionInspectorDialog`, `ManualMergeDialog`), the Account select could also fall back to a raw account UUID while `accounts` was still loading or if a referenced account was missing.

## What Didn't Work
- **Relying on `<SelectItem>` children alone (Radix UI mental model):** Unlike Radix UI Select, `@base-ui/react/select` does not read closed trigger labels from unmounted `<SelectItem>` children.
- **Walking React `children` inside `Select` to scrape `<SelectItem>` labels:** Fragile with fragments, helper components, and dynamic renders; Base UI already provides a first-class `items` prop on `SelectPrimitive.Root`.

## Solution
Pending merge on branch `fix/base-ui-select-labels`:

1. **Require `items` on the shared `Select` wrapper** in `components/ui/select.tsx` so omitting label data fails `bun run typecheck` at compile time while preserving `<Value, Multiple>` generic inference:

```tsx
function Select<Value, Multiple extends boolean | undefined = false>(
  props: SelectPrimitive.Root.Props<Value, Multiple> & {
    items: NonNullable<SelectPrimitive.Root.Props<Value, Multiple>["items"]>
  }
) {
  return <SelectPrimitive.Root {...props} />
}
```

2. **Pass a single `{ value, label }[]` source of truth** to both `<Select items={...}>` and `<SelectItem>` across all 18 `<Select>` call sites (8 components):

```tsx
const ACCOUNT_TYPE_OPTIONS: ReadonlyArray<{
  value: AccountType;
  label: string;
}> = [
  { value: 'credit_card', label: 'Credit Card' },
  { value: 'debit_card', label: 'Debit Card' },
  { value: 'digital_wallet', label: 'Digital Wallet' },
];

<Select items={ACCOUNT_TYPE_OPTIONS} value={type} onValueChange={...}>
  <SelectTrigger className="w-full" aria-label="Account type">
    <SelectValue />
  </SelectTrigger>
  <SelectContent>
    {ACCOUNT_TYPE_OPTIONS.map((opt) => (
      <SelectItem key={opt.value} value={opt.value}>
        {opt.label}
      </SelectItem>
    ))}
  </SelectContent>
</Select>
```

3. **Handle async/missing IDs and empty states:**
   - In `components/transaction-inspector-dialog.tsx` and `components/manual-merge-dialog.tsx`, append a fallback entry to `accountItems` using the transaction's `account_name` (or `'Unknown account'`) whenever `accountId` is neither `UNASSIGNED_ACCOUNT` nor present in `accounts`.
   - Normalize empty unselected values to `null` (`value={countryCode || null}`, `value={targetId || null}`, etc.) to match Base UI's `null` empty-value convention, and provide `placeholder` on `<SelectValue />` where an empty state is reachable (such as `components/merge-account-dialog.tsx`).

## Why This Works
- In `node_modules/@base-ui/react/select/value/SelectValue.mjs`, `SelectValue` resolves the displayed label via `resolveSelectedLabel(value, items, itemToStringLabel)`.
- In `node_modules/@base-ui/react/internals/resolveValueLabel.mjs`, `resolveSelectedLabel` looks up `value` in `items` (supporting both `Record<string, ReactNode>` and `ReadonlyArray<{ value, label }>`); when `items` is omitted or does not contain `value`, it falls back to `stringifyAsLabel(value)`, rendering the raw value string.
- Requiring `items` on `Select` in `components/ui/select.tsx` turns accidental omissions into `tsc --noEmit` errors instead of silent UI regressions.

## Prevention
- **Preserve the `items` requirement when updating `components/ui/select.tsx`:** Regenerating `components/ui/select.tsx` via shadcn CLI will revert `Select` back to `const Select = SelectPrimitive.Root`; keep the typed wrapper requiring `items`.
- **Share one option array between `items` and `<SelectItem>`:** Base UI types `items` and `SelectItem` `value` as `any`, so generating both from one `{ value, label }[]` array prevents compile-time-invisible label drift.
- **Add a fallback entry for async/stale IDs:** Whenever a controlled `value` can arrive before its option list loads, include a fallback `{ value, label }` entry so `resolveSelectedLabel` never falls back to a raw UUID.

## Related Issues
- `docs/solutions/ui-bugs/base-ui-button-render-link-semantics.md` — sibling `@base-ui/react` 1.9.0 API pitfall in `components/ui/`.
