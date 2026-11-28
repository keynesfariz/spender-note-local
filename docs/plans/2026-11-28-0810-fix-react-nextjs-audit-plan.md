---
title: React and Next.js audit follow-up
type: fix
date: 2026-11-28
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-brainstorm
execution: code
---

# React and Next.js audit follow-up

## Goal Capsule

**Objective.** Keep transaction entry and review consistent when users change their input, navigate, or work with a large statement.

**Product authority.** The user selected all seven findings from the [React and Next.js audit](../ideation/2026-11-28-react-nextjs-audit-ideation.html). Existing transaction, Combine, and Magic enrichment behavior governs compatibility. See [CONCEPTS.md](../../CONCEPTS.md).

## Product Contract

### Summary

Make manual import respect the latest input, report the submitted account, and refresh the views affected by a write. Make the card-payment picker paginate eligible transfers and have the Settings tab follow its URL. Start inspector details loading as soon as a user opens a transaction and make its patch rules independently verifiable.

### Requirements

**Manual Import**

- R1. The latest draft action wins when file reads overlap each other, a paste, or Start Over. Surface read failures for the active file and preserve the user's current draft.
- R2. While an import is pending, prevent changes to the submitted draft, target account, and wizard step. The receipt identifies the submitted account. A failed submission leaves the draft and selection available for retry.
- R3. Successful imports make transaction lists, spending totals, account balances, and category and tag counts eligible to refresh on the next read, including when a recently loaded view still has cached data.
- R4. Parse and normalize input when the user asks to preview pasted JSON or selects a file. Each unchanged draft and validation context share one result across validation and preview. Valid uploads still advance automatically, and existing row errors and selection rules remain available.

**Transfer matching and Settings**

- R5. Card-payment matches and their page totals include only transfers whose destination is a credit-card account. Ordinary transfer matching retains its existing candidate rules and ordering.
- R6. The Settings tab follows the current URL on direct links and same-route navigation, including browser history. Preserve explicit tab precedence, Gmail callback defaults and feedback, and the existing replace-history behavior for tab clicks.

**Transaction inspector**

- R7. Opening an inspector starts the selected transaction's detail request without waiting for the lazy drawer module on a cold open. Ordinary list rendering sends no detail requests. Preserve existing detail loading and error feedback.
- R8. Inspector patch construction can be checked independently of the component, with coverage for currency changes, timezone conversion, and omitted versus explicitly cleared fields. Preserve draft protection, review navigation, and API payload semantics.

### Acceptance Examples

- AE1. Covers R1. Start opening a slow file, then paste another statement or choose a second file. When the first read finishes, the current draft and step still reflect the later action. If the active file fails to read, the user sees an error and can try again.
- AE2. Covers R2–R3. Visit Accounts and Categories & Tags, then submit an import and try to change the account or step while it is pending. The receipt still identifies the chosen account. On success, revisiting the previously loaded views reflects the new balance and counts. On failure, the selected rows remain available.
- AE3. Covers R4. Paste invalid JSON and ask to preview it. See validation feedback, correct the input, and preview again. Upload valid JSON and verify that the same normalized result used to advance the wizard supplies the preview.
- AE4. Covers R5. Seed a transfer history where several pages of ordinary transfers precede two card payments. The card-payment picker shows both eligible transfers from its first page with an eligible-only total.
- AE5. Covers R6. Load a Gmail callback URL and verify its default tab and feedback. Navigate to a different explicit tab URL on the same route, then go back and verify that each displayed tab matches its URL.
- AE6. Covers R7. Open the inspector from a cold Transactions view. Observe the details request start before the drawer module finishes loading. Verify the selected transaction appears and a failed detail request shows the existing error feedback and reopening can recover.
- AE7. Covers R8. Save a draft with a currency change, a timezone-sensitive date, an untouched field, and a field explicitly cleared. Verify the outgoing patch preserves the existing meaning of each edit.

### How This Work Fits Together

<!-- ce-section: work-relationships -->

R1–R4 share the import draft lifecycle and can be verified as one workflow. Transfer matching and Settings have independent acceptance criteria. Inspector loading and patch construction touch the same feature and can be verified independently.

### Scope Boundaries

- Cover the seven audit findings through the app's existing routes and workflows. Preserve the query library, data architecture, and persisted transaction format.
- Keep preview virtualization, background parsing workers, and Lucide bundle changes outside this batch. Revisit them if profiling supports the cost.

## Planning Contract

### Key Technical Decisions

- Keep import draft state local to the wizard. Record the input and validation context that produced a preview. Pasted input validates on Preview, and file input validates when the latest read resolves. A stale result immediately becomes ineligible for submission. Current-context revalidation is required before proceeding if locale or taxonomy changes.
- Use a monotonic input revision for file reads. Advance it on file selection, paste/edit, and Start Over. Compare before applying either success or failure. Read the current validation context at completion so a file opened before metadata arrives can still validate against the latest data.
- Pass the account ID, display label, and selected payloads as explicit mutation variables. Preserve one submission in flight across wizard entry points. Use the shared transaction invalidation policy on success and render the receipt from the submitted variables and server count.
- Add an optional `destination_account_type=credit_card` filter to the existing transaction list query. Validate the supported value and use a parameterized correlated `EXISTS` predicate on the destination account. The shared filter must work in count queries that lack the row query's destination join. Apply it to filtered rows and totalCount. Preserve date-scope counters and ordinary ledger semantics.
- Read Settings tab and callback values from the live URL using the same shallow URL-state approach as Categories & Tags. Wrap the query-reading subtree in Suspense if the route becomes static. Keep callback parameters through initial render and preserve the current tab-click behavior of clearing callback feedback.
- Start a prefetch using `transactionInspectorQuery(id)` in the review open path before the drawer mounts. Keep the same query definition in the drawer so an in-flight request can be shared and a fresh response reused. Leave nearby-row prefetching outside this change.

### High-Level Technical Design

```text
Paste/edit ──→ draft revision ── Preview ──→ validate ──→ preview
File select ─→ draft revision ── file read ── current revision? ──→ validate
Preview ─────→ submitted snapshot ──→ pending ──→ receipt + invalidate
                                              └─→ error + editable preview

Card picker ─→ list query key including destination type ─→ Go route
             └─ eligible rows and count ← shared SQL predicate ←┘

Inspector open ─┬─→ drawer module ──→ useQuery(same key)
                └─→ detail prefetch ─→ TanStack Query cache

Settings URL ──→ explicit tab or callback default ──→ rendered tab
Tab click ─────→ shallow URL replace ────────────────┘
```

## Implementation Units

### U1. Make import draft and submission ownership explicit

**Covers** R1, R2, R4. **Dependencies** None.

**Files** `components/manual-import-view.tsx`, `lib/manual-import.ts` if parser changes are needed, and existing import tests where pure behavior can be exercised.

- Route both Preview entry points through the same validation action. Keep Preview actionable for nonempty input so malformed JSON can surface its error. Invalidate selection for a changed draft or validation context. Keep upload errors visible and allow retrying the same file.
- Guard late reads by revision. Handle paste, file selection, and Start Over consistently. Block all draft, account, and step controls during a pending import, including step navigation and Back. Protect the submit handler against a second activation before the UI updates.
- Keep the mutation tied to its submit snapshot. On error retain the user's preview and row selection. On success use the submitted account label and returned count in the receipt.
- **Verification** Exercise AE1–AE3 with two out-of-order file reads, an intervening edit, a failed read, a failed submission, and success. Check that an unchanged valid upload is normalized once and that a changed validation context cannot submit an old preview.

### U2. Refresh dependent views after import

**Covers** R3. **Dependencies** U1.

**Files** `components/manual-import-view.tsx`, `lib/api/transactions.ts` for the existing helper.

- Replace the local transaction and spending invalidations with `invalidateTransactionRelatedQueries` on successful import. Keep the server response as the source of the receipt.
- **Verification** Warm Accounts and Categories & Tags caches before importing into an existing account with an existing category and tag. Revisit immediately after success and verify all three views refresh. Verify a failed import leaves the draft available and skips the success invalidation.

### U3. Paginate eligible card-payment transfers

**Covers** R5. **Dependencies** None.

**Files** `components/transfer-form-dialog.tsx`, `lib/api/transactions.ts`, `app/api/transactions/route.go`, `internal/domain/transactions.go`, `internal/domain/transactions_filters_test.go`.

- Thread the optional filter through serialization, query-key identity, route parsing, and domain filtering. Add the shared predicate before count and page selection. Preserve the existing `kind=transfer` and sort order.
- Set the filter only for card-payment matching and remove the page-local destination filtering. Reset to page one when the matching mode changes within a mounted picker, if that transition is supported by its caller.
- **Verification** Seed mixed transfers spanning multiple pages, including a transfer *from* a credit card. Assert the card-payment rows and total match destination eligibility, pages have stable ordering, and ordinary transfer results remain unchanged. Verify the UI pager uses the filtered total.

### U4. Make the Settings tab URL-owned

**Covers** R6. **Dependencies** None.

**Files** `app/settings/page.tsx`, `components/settings-view.tsx`. Reference `components/categories-tags-view.tsx` and installed Next.js navigation and search-params docs.

- Replace the initial-prop state copy with a tab derived from live query values. Use a shallow replace for tab clicks and clear the callback parameters in the same update. Preserve tab precedence and the existing default for invalid or absent tab values.
- **Verification** Cover direct tab links, both Gmail callback outcomes, explicit tab plus callback values, same-route history navigation, and tab clicks that clear callback feedback. Use the production build to verify any required Suspense boundary.

### U5. Overlap inspector code and data loading

**Covers** R7. **Dependencies** None.

**Files** `hooks/use-transaction-review.ts`, `lib/api/transaction-inspector.ts`, `components/transaction-inspector-drawer.tsx`. Reference the dynamic boundary in `components/transactions-view.tsx`.

- Start prefetch for the explicit transaction ID on open and let the drawer subscribe to the canonical key. Keep review-position lookup independent of the detail request. Review existing loading and retry behavior and retain it.
- **Verification** Use a cold module and query cache. Confirm that the detail request starts before the drawer module finishes and that a successful open produces one effective detail request. Test failure recovery and quick opens of distinct transactions. Check that loading the list alone sends no detail requests.

### U6. Extract the inspector patch calculation

**Covers** R8. **Dependencies** None. Apply after U5 when editing the same drawer file.

**Files** `components/transaction-inspector-drawer.tsx`, a colocated pure patch helper and focused test.

- Move the existing patch decisions and validation into a pure function taking the current draft, baseline, and transaction context. Keep status messages, guard checks, the mutation, and Save and Next sequencing in the form.
- **Verification** Check that the serialized payload preserves omission, explicit nulls, and empty tag selections. Cover income receiving currency, conversion to local currency, a changed currency baseline, timezone conversion, and incomplete date-time input.

## Verification Contract

Run `bun run lint`, `bun run typecheck`, the relevant `bun test` cases, `go test ./internal/domain`, and `bun run build` after implementation. Use a disposable local database for workflow checks. Use a cold browser request trace for U5 and compare import parser calls with a representative large statement for U1. Record the observed request ordering and parse counts rather than estimating time savings.

## Definition of Done

Every requirement R1–R8 has evidence from its implementation unit and acceptance examples. New tests cover the transfer predicate and the extracted patch semantics. Existing import, transfer, Settings callback, and inspector save flows still pass their compatibility checks.
