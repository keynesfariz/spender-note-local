---
title: Base UI Select Unfocused Label Display - Plan
type: fix
date: 2026-11-22
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-plan-bootstrap
execution: code
---

# Base UI Select Unfocused Label Display - Plan

## Goal Capsule

- **Objective:** Every select dropdown in the app displays its human-readable option label in the closed and unfocused trigger state instead of raw IDs or sentinel values, while showing its placeholder when no option is selected.
- **Means:** Pass Base UI's `items` prop to every `Select` root and make `items` required on the shared `Select` wrapper so omissions fail typecheck (KTD1, KTD2).
- **Authority:** Product Contract governs user-visible behavior; Planning Contract governs technical mechanism; Implementation Units execute within both.
- **Execution profile:** Single pull request; three implementation units in two stages (`U1`, followed by `U2` and `U3` in parallel or either order).
- **Stop conditions:** Stop and escalate if any `<Select>` requires rich non-string trigger rendering that conflicts with `items`, or if a dynamic option list can hold a persisted ID not present in the fetched options and needs a product-defined fallback string.
- **Ownership:** Implementing agent (`ce-work`) completes all units and verification gates; user reviews and merges.

## Product Contract

### Summary

Update the shared `Select` wrapper and all 18 `<Select>` instances across 8 UI components so closed/unfocused triggers always render the human-readable option label instead of raw values, UUIDs, or sentinel keys, and empty states render their configured placeholder.

### Problem Frame

Across the dashboard, transactions list, settings page, and all account/transaction dialogs, every `<Select>` component displays its raw underlying value (such as `credit_card`, `card_last4`, `all`, `__unassigned__`, `__none__`, account UUIDs, month numbers, or country codes) when the dropdown is closed or unfocused. In `@base-ui/react/select` 1.9.0, `SelectPrimitive.Value` resolves labels from the root `items` prop via `resolveSelectedLabel`; when `items` is omitted, it falls back to stringifying the raw `value`. Additionally, `MergeAccountDialog` omits a `placeholder` prop on `<SelectValue />`, and several selects pass `""` rather than Base UI's `null` empty-value convention when unselected.

### Requirements

**Label Display**
- R1. Every `<Select>` trigger across the application displays the matching human-readable option label on initial render, while open, and after closing/blurring, never raw IDs or sentinel values (`all`, `unassigned`, `__unassigned__`, `__none__`, `credit_card`, `card_last4`, `auto`, `card_charge`, `receipt`, month numbers, country codes, or account UUIDs).
- R2. Every `<Select>` with a `placeholder` displays that placeholder when no value is selected, rather than rendering an empty string or raw value.
- R3. Option labels shown in the closed trigger match the corresponding `<SelectItem>` text in the open dropdown.

**Regression Prevention**
- R4. Omitting the label map/list on any future `<Select>` usage fails static typechecking (`bun run typecheck`) so the Base UI raw-ID fallback cannot silently recur.

### Scope Boundaries

- **In scope:** `components/ui/select.tsx` and all 18 `<Select>` instances across the 8 consumer files under `components/`.
- **Not built (considered):** Auto-extracting labels in `Select` by walking React `children` — rejected because `React.Children` traversal is brittle with fragments, wrappers, and dynamic renders, whereas Base UI's native `items` prop is the documented API; revisit only if a future consumer needs dynamic children without an upfront option list.
- **Not built (considered):** Adding a new frontend unit-test runner (Vitest / React Testing Library) solely for this fix — out of scope for a lightweight bug fix; the repo currently has no JS/TS component test harness, so verification uses `bun run typecheck`, `bun run lint`, `bun run build`, and browser smoke checks.
- **Deferred to Follow-Up Work:** Documenting the Base UI `Select` `items` convention in `docs/solutions/ui-bugs/` (via `ce-compound` after verification).

## Planning Contract

### Key Technical Decisions

- KTD1. **Pass `items` to `<Select>` using `{ value, label }[]` arrays or `Record<string, string>` maps instead of per-call `<SelectValue>` render functions.** Verified in `node_modules/@base-ui/react/select/value/SelectValue.mjs` and `node_modules/@base-ui/react/internals/resolveValueLabel.mjs`: `SelectPrimitive.Value` reads `items` from root store synchronously on every render (initial, closed, unfocused, SSR), and sharing the same option list between `items` and `<SelectItem>` keeps trigger and dropdown labels in sync (R1, R3).
- KTD2. **Require `items` on the `Select` export in `components/ui/select.tsx`.** Wrapping `SelectPrimitive.Root` with `items` required in its TypeScript props makes any omitted label mapping a compile-time error under `bun run typecheck` (R4) without changing runtime behavior.
- KTD3. **Normalize unselected string values to `null` and ensure empty-capable selects have placeholders.** Aligning unselected string states (`""`) to `value || null` matches Base UI's `null` empty-value convention (`dashboard-view.tsx` month/year, `settings-view.tsx` country/timezone/model, `merge-account-dialog.tsx` destination account) while keeping existing state types intact, and adding `placeholder="Select destination account..."` to `MergeAccountDialog` ensures its initial empty state renders a placeholder instead of a blank trigger (R2).

### Sources & Research

- `node_modules/@base-ui/react/select/value/SelectValue.mjs` — `SelectValue` resolves display text via `resolveSelectedLabel(value, items, itemToStringLabel)` and shows `placeholder` only when `!hasSelectedValue`.
- `node_modules/@base-ui/react/internals/resolveValueLabel.mjs` — When `items` is absent, `resolveSelectedLabel` falls back to `stringifyAsLabel(value)`, printing the raw `value`. Supports both `Record<string, ReactNode>` and `ReadonlyArray<{ value, label }>`.
- `components/ui/select.tsx` — Currently re-exports `Select = SelectPrimitive.Root` directly without requiring `items`.
- `docs/solutions/ui-bugs/base-ui-button-render-link-semantics.md` — Prior Base UI 1.9.0 pitfall in this repo establishing the precedent of enforcing correct Base UI usage at the component boundary.

## Implementation Units

### U1. Enforce `items` prop on shared `Select` wrapper

- **Goal:** Make `items` a required prop on `Select` in `components/ui/select.tsx` so Base UI always receives a label source and future omissions fail `bun run typecheck`.
- **Requirements:** R1, R4
- **Dependencies:** None
- **Files:**
  - Modify: `components/ui/select.tsx`
- **Approach:**
  1. Replace the bare `const Select = SelectPrimitive.Root` alias with a typed wrapper function `Select` that intersects `SelectPrimitive.Root.Props<Value, Multiple>` with `{ items: NonNullable<SelectPrimitive.Root.Props<Value, Multiple>['items']> }` (avoiding `Omit<..., 'items'>` generic assignability issues when forwarding props).
  2. Preserve full generic inference for `Value` and `Multiple` so `onValueChange` callbacks in consumers keep their inferred value types.
- **Patterns to follow:** `components/ui/select.tsx` existing `SelectGroup`/`SelectTrigger` wrapper style.
- **Test scenarios:**
  - Happy path: Passing `items` as a `ReadonlyArray<{ value: string; label: string }>` or `Record<string, string>` compiles cleanly and preserves `onValueChange` value typing.
  - Edge case: Omitting `items` on any `<Select>` fails `bun run typecheck`.
- **Verification:** `bun run typecheck` fails exclusively on consumer `<Select>` sites that do not yet pass `items` (confirming the constraint is active before U2/U3).

### U2. Wire `items` and null-empty values in view-level selects

- **Goal:** Supply `items` to all 9 `<Select>` instances in dashboard, transaction list, and settings views, and normalize empty string values to `null` where placeholders are used.
- **Requirements:** R1, R2, R3
- **Dependencies:** U1
- **Files:**
  - Modify: `components/dashboard-view.tsx`
  - Modify: `components/transaction-list.tsx`
  - Modify: `components/settings-view.tsx`
- **Approach:**
  1. `components/dashboard-view.tsx` (2 selects): Pass `items={MONTH_OPTIONS}` with `value={activeParts.month || null}` for Month; derive `yearItems` (`{ value: yr, label: yr }[]`) with `value={activeParts.year || null}` for Year.
  2. `components/transaction-list.tsx` (2 selects): Build `categoryItems` (`all` → `"All categories"` plus `DEFAULT_CATEGORIES`) and `accountItems` (`all` → `"All accounts"`, `unassigned` → `"Unassigned"`, plus `accounts` `id` → `name`) via `useMemo` and render `<SelectItem>` from the same arrays so labels cannot drift.
  3. `components/settings-view.tsx` (5 selects): Build memoized `items` arrays for Country (`c.code` → `${c.name} (${c.currency})`, `value={countryCode || null}`), Timezone (`tz` → `tz`, `value={timezone || null}`), Sender role (`auto` → `"Auto-detect role"`, `card_charge` → `"Card charge alert"`, `receipt` → `"Merchant receipt"`), Default account (`NO_DEFAULT_ACCOUNT` → `"Default account (optional)"` plus `accounts` `id` → `name`), and Local GGUF model (`m.relativePath` → `${m.relativePath} (${formatBytes(m.sizeBytes)})`, `value={modelPath || null}`).
- **Execution note:** No frontend component test runner is installed; verify via `bun run typecheck`, `bun run build`, and browser smoke checks on `/`, `/transactions`, and `/settings`.
- **Patterns to follow:** `MONTH_OPTIONS` in `components/dashboard-view.tsx` (`{ value, label }[]` single source of truth).
- **Test scenarios:**
  - Happy path: Dashboard month select shows `"November"` (not `"11"`), transaction filters show `"All categories"` / `"All accounts"` / account name (not `"all"` or account UUID), settings selects show `"Auto-detect role"`, `"Default account (optional)"`, country name with currency, and model path with size while closed and unfocused.
  - Edge case: When country, timezone, or GGUF model is unset (`""`), the closed trigger renders its placeholder (`"Select country..."`, `"Select timezone..."`, `"Select a local GGUF model..."`).
  - Edge case: Changing a filter or setting selection and blurring/closing the dropdown keeps the chosen human-readable label displayed.
- **Verification:** All 9 selects in these three views typecheck cleanly and display human-readable labels (or placeholders when empty) while unfocused.

### U3. Wire `items` and null-empty values in dialog selects

- **Goal:** Supply `items` to all 9 `<Select>` instances across the 5 dialog components so every form and merge dialog displays human-readable labels when closed/unfocused.
- **Requirements:** R1, R2, R3
- **Dependencies:** U1
- **Files:**
  - Modify: `components/account-form-dialog.tsx`
  - Modify: `components/merge-account-dialog.tsx`
  - Modify: `components/add-transaction-dialog.tsx`
  - Modify: `components/transaction-inspector-dialog.tsx`
  - Modify: `components/manual-merge-dialog.tsx`
- **Approach:**
  1. `components/account-form-dialog.tsx` (2 selects): Define static `ACCOUNT_TYPE_OPTIONS` (`credit_card` → `"Credit Card"`, `debit_card` → `"Debit Card"`, `digital_wallet` → `"Digital Wallet"`) and `IDENTIFIER_KIND_OPTIONS` (`card_last4` → `"Card last 4"`, `bank_account` → `"Bank account"`, `alias` → `"Alias / Wallet"`), pass as `items`, and map `<SelectItem>` from them.
  2. `components/merge-account-dialog.tsx` (1 select): Build memoized `targetItems` from `eligibleTargets` (`target.id` → `${target.name} (${ACCOUNT_TYPE_LABELS[target.type]} · ...)`), pass `items={targetItems}` and `value={targetId || null}`, add `placeholder="Select destination account..."` to `<SelectValue />`, and render `<SelectItem>` from `targetItems`.
  3. `components/add-transaction-dialog.tsx`, `components/transaction-inspector-dialog.tsx`, `components/manual-merge-dialog.tsx` (6 selects total — Category and Account in each): Build `categoryItems` from `DEFAULT_CATEGORIES` and memoized `accountItems` (`UNASSIGNED_ACCOUNT` → `"Unassigned"` plus `accounts` `id` → `name`), pass as `items`, and render `<SelectItem>` from them. In `TransactionInspectorDialog` and `ManualMergeDialog`, if the current `accountId` is neither `UNASSIGNED_ACCOUNT` nor present in `accounts` (e.g., while `accounts` is still loading or references a stale account), include a fallback entry in `accountItems` using the transaction's `account_name` (or `"Unknown account"` if null) so the trigger never renders a raw UUID.
- **Execution note:** Verify via `bun run typecheck`, `bun run build`, and browser smoke checks opening each dialog.
- **Patterns to follow:** `ACCOUNT_TYPE_LABELS` in `components/merge-account-dialog.tsx` and `{ value, label }[]` pattern from U2.
- **Test scenarios:**
  - Happy path: Opening Account Form dialog shows `"Credit Card"` and `"Card last 4"` (not `"credit_card"` / `"card_last4"`); opening Add Transaction, Transaction Inspector, and Manual Merge dialogs shows `"Unassigned"` or the account name (not `"__unassigned__"` or account UUID); opening Merge Account dialog shows the formatted destination account label (not its UUID).
  - Edge case: When `targetId` is initially empty in `MergeAccountDialog`, the trigger displays `"Select destination account..."` and updates to the full formatted label once a target is selected.
  - Edge case: When `TransactionInspectorDialog` or `ManualMergeDialog` opens before `accounts` has resolved, the Account trigger displays the transaction's `account_name` (or `"Unknown account"`) rather than the raw account UUID.
  - Edge case: Editing a transaction assigned to an account, switching to `"Unassigned"`, and blurring displays `"Unassigned"`.
- **Verification:** All 9 dialog selects typecheck cleanly and display human-readable labels while closed/unfocused.

## Verification Contract

| Gate | Command | Scope |
|---|---|---|
| Typecheck | `bun run typecheck` | Verifies `U1` required `items` constraint and all 18 `<Select>` call sites in `U2` and `U3` |
| Lint | `bun run lint` | Ensures no ESLint regressions in modified components |
| Production build | `bun run build` | Confirms Next.js SSR/client compilation succeeds across all routes |
| Browser smoke check | Start `bun run dev` and inspect `/`, `/transactions`, `/accounts`, `/settings`, and dialogs | Visually confirms closed/unfocused triggers display labels (never IDs/sentinels) and placeholders work when unset |

## Definition of Done

- All 3 implementation units (`U1`, `U2`, `U3`) are complete and every `<Select>` in the repository passes `items`.
- `bun run typecheck`, `bun run lint`, and `bun run build` pass with zero errors.
- Manual/browser check confirms no `<Select>` renders a raw ID or sentinel key when closed/unfocused, and placeholders render when values are empty.
- No abandoned or experimental code remains in the working diff.
