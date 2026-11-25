---
title: Address the React Performance Audit - Plan
type: perf
date: 2026-11-25
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-plan-bootstrap
execution: code
---

# Address the React Performance Audit - Plan

## Goal Capsule

**Objective.** Open and search spending views promptly, including when the local LLM is unavailable or the database contains a large email history.

**Means.** Remove the LLM probe from common reads, bound list work and payloads, defer optional interface code, and isolate frequent updates from expensive renders.

**Authority.** The Product Contract governs behavior. The Planning Contract governs implementation choices within it. Existing saved data and documented route, selection, and accessibility behavior govern compatibility.

**Execution profile.** One PR with independently verifiable units. Use a disposable database and a controllable local LLM stub for integration verification.

**Stop condition.** The requirements and verification thresholds pass. Record the measured outcomes and any evidence-gated finding whose proposed change was unnecessary.

## Product Contract

### Summary

Make Transactions and Accounts independent of model discovery. Make ignored-email search and transaction detail load only the data needed for the current view. Reduce initial JavaScript and repeated rendering while preserving navigation, editing, and triage behavior.

### Problem Frame

Transactions currently waits for an LLM probe indirectly through the settings query. Optional calendar and dialog code loads up front. Ignored-email pagination limits the response only after the Go handler has loaded and processed the entire ignored history. The previous audit also identified avoidable work during typing, polling, calendar updates, and tag enrichment.

### Requirements

**Reads and data**

- R1. Transactions, Skipped, Accounts, and non-LLM settings remain usable when the LLM is offline. Both transaction tabs support independent list and locale reads. The Local LLM tab retains saved and draft URL model discovery and offline feedback.
- R2. Ignored-email pagination and search retain normalized sender matching, global sender options, literal search semantics, filtered counts, and stable ordering. Load body text only when expanded in the UI.
- R3. Upgraded databases present the same skipped transactions and counts on the first configured read. Warm reads avoid legacy backfill checks.
- R4. Transaction account selectors retain IDs, labels, and historical selection fallbacks. The inspector retains source counts, email previews, enrichment, and splitting.

**Loading and interactions**

- R5. Cold loads download optional calendar, dialog, drawer, and tab code only when needed or intentionally prefetched. The active view remains accessible on direct entry and keyboard navigation.
- R6. Start primary reads earlier where supported by the installed Go route runtime. Keep the existing same-origin browser fetch path available on direct entry and on prefetch failure.
- R7. Transaction and ignored-email inputs update immediately. Display results for the latest criteria and keep transaction URL, date scope, paging, and selection behavior consistent.
- R8. Keep sync progress, calendar keyboard focus, form draft protection, and list behavior intact while reducing unrelated renders.
- R9. Apply the audit’s bounded JavaScript and render cleanups where the original semantics hold. Verify performance-sensitive changes with observable behavior or build evidence.

### Acceptance Examples

- AE1. Covers R1. With a model server that accepts a connection and never answers, opening Transactions or Accounts yields data and creates zero model-server requests. Opening Local LLM triggers model discovery and shows its timeout feedback.
- AE2. Covers R2 and R7. Two ignored messages with tied timestamps, mixed-case sender headers and a search term containing a literal percent sign return deterministic pages and the full sender option set. Rapid typing followed by clearing the field leaves the default results visible.
- AE3. Covers R3. Two Go route processes read a legacy database at the same time. Both see consistent triage results, including after retrying a transient failure during backfill.
- AE4. Covers R4 and R8. Open an email-backed transaction, edit a field, let a background refresh finish, switch tabs, expand an email, and split it where permitted. The draft follows the existing reset rules and the source count refreshes.
- AE5. Covers R5 and R8. Navigate a date picker by keyboard, select the first date of a range, and complete the range. Focus and component identity survive the intermediate render.

## Planning Contract

### Key Technical Decisions

- KTD1. Add lightweight locale and saved-settings read paths. Keep the existing full settings GET and probe contract for compatibility. Use separate TanStack query keys and invalidate only the settings slices affected by each mutation. This governs R1.
- KTD2. Filter and page ignored emails in SQLite using the same Go sender normalization and case folding through deterministic scalar functions registered with the existing driver before it opens connections. Use literal substring matching and the current ordering. Keep page, count, and sender options in one read snapshot. This governs R2.
- KTD3. Keep the compatibility backfill in the domain layer behind a per-route-process successful completion gate. Set it only after the configured backfill succeeds. Retain the existing immediate transaction and candidate recheck for concurrent processes. This governs R3.
- KTD4. Add narrow read models alongside existing endpoints. Keep list and initial detail responses small for the new UI paths, and key body queries by the owning email and transaction where applicable. This governs R2 and R4.
- KTD5. Use conditional rendering at client-side dynamic import boundaries. Preload optional modules on explicit pointer or keyboard intent where that improves the first interaction. Preserve exit animations and reset semantics where controlled wrappers currently provide them. This governs R5.
- KTD6. Use intent-driven TanStack prefetch with the same canonical query keys as the views. The installed Next.js BFF guide documents that build-time Server Components cannot fetch app route handlers through a listening server. Only adopt runtime server prefetch if a supported data path starts reads earlier without brittle loopback or extra work. This governs R6.
- KTD7. Keep SQLite query work ordered within its single connection. Combine or avoid independent statements where useful and preserve consistent reads. This governs R2 and R4.

### Assumptions

- Existing local API response shapes are compatibility surfaces. New browser paths can use narrower responses, with current callers continuing on existing contracts.
- The current sync writer populates the signature needed for new triage candidates. Verify this across all writers before enabling the warm-read gate. Billing statement rows retain their existing exclusion.
- Bundle sizes from the audit identify where to split. Measure a fresh baseline and candidate under the same build settings before claiming savings.

### System-Wide Impact

The two list handlers run in separate Go processes even though they share SQLite. Check the local Go route reference and the repository migration notes before changing initialization. The database already uses one connection per process. Keep source email bodies within the same local API boundary as the existing detail views. Keep the same generated Go-to-TypeScript workflow for changed response types.

- `node_modules/next/dist/docs/01-app/03-api-reference/03-file-conventions/route-go.md`
- `node_modules/next/dist/docs/01-app/02-guides/backend-for-frontend.md`
- `docs/solutions/database-issues/sqlite-table-rebuild-begin-immediate-goroutes.md`
- `docs/solutions/ui-bugs/base-ui-select-items-label-resolution.md`
- `docs/solutions/ui-bugs/base-ui-button-render-link-semantics.md`
- `docs/solutions/logic-errors/go-loadlocation-local-pseudo-timezone-breaks-js-intl.md`

## Implementation Units

### U1. Remove model discovery from common reads

- **Covers** R1. **Dependencies** None.
- **Files** `internal/domain/settings.go`, `internal/domain/settings_test.go`, `app/api/settings/route.go`, `app/api/transactions/route.go`, `internal/domain/transactions.go`, `internal/domain/transactions_filters_test.go`, `lib/api/transactions.ts`, `cmd/gen-ts-types/main.go`, `lib/api/settings.ts`, `components/transactions-view.tsx`, `components/accounts-view.tsx`, `components/settings-view.tsx`, settings tab components.
- **Approach** Apply KTD1 using small GET modes on the current settings route. Fetch only timezone and currency for transaction and account callers. Leave country catalog and currency lock in saved settings. Give the list query an explicit server-resolved local-today mode using the locale that Go already reads. Preserve omitted-date API semantics for existing callers. Start the active list query independently of locale. Match the current local-today default, including from-only and to-only deep links. Preserve rejection of the Go Local pseudo-timezone. Load the status and model list in Local LLM and preserve explicit draft-URL probing. Invalidate transaction, skipped, dashboard, and other locale-dependent queries when locale changes. Return the resolved effective range, timezone, and configuration state with the new list mode. Treat unconfigured results as stale on setup. Validate cached relative ranges against the current timezone and local date on navigation, and refetch at the next local-day boundary while the view is open. Preserve direct entry to all settings tabs.
- **Verification** Extend settings tests with a counting LLM stub. Exercise configured and unconfigured locale, warm-cache setup and timezone updates, midnight rollover including DST, all-dates and one-sided URL ranges, settings updates, current and draft URL probes, and offline feedback in the production build.

### U2. Bound ignored-email work and defer email bodies

- **Covers** R2. **Dependencies** None.
- **Files** `internal/domain/triage.go`, `internal/domain/triage_test.go`, `internal/domain/triage_signature.go`, `app/api/triage/ignored/route.go`, `lib/api/triage.ts`, `components/settings/triage-rules-tab.tsx`, generated API types.
- **Approach** Characterize existing search before applying KTD2 and KTD4. Push sort, count, and page bounds into SQL. Keep the available sender query independent of active filters and skip loading bodies for the UI summary mode. Fetch a body on expansion and scope it to an email whose raw status and extraction triage state still qualify it for the ignored list. Preserve full bodies in the existing response mode. Bound page arithmetic before calculating offsets.
- **Verification** Cover Unicode, malformed headers, percent and underscore, empty criteria, tied dates, out-of-range and huge pages, and removal of the final item on a page. Confirm the UI request returns only the requested page, queries do not project bodies for off-page rows, and expansion is the first body request. Verify that a promoted email no longer returns a body through the ignored preview.

### U3. Remove repeated legacy triage checks from warm reads

- **Covers** R3. **Dependencies** None.
- **Files** `internal/domain/triage.go`, `internal/domain/transactions.go`, `internal/domain/triage_test.go`, `internal/domain/transactions_test.go`, `internal/db/tx.go`, `internal/syncrunner/runner.go` for writer verification.
- **Approach** Apply KTD3 at the configured list entry points, above database initialization. Keep direct backfill callable for existing tests. Scope successful completion to the opened database lifetime. Leave failure retryable and retain locale/category-dependent normalization. Verify all regular write paths build complete signatures before removing the recurring fallback.
- **Verification** Use a file-backed legacy fixture and two database handles for the concurrency regression. Cover fresh and unconfigured databases, legacy rows with missing extraction or signature, previously ignored rows, and rollback followed by retry. Cancel the initiating request after the backfill obtains its write lock and verify a subsequent read can use the same handle. After successful setup, subsequent configured list calls issue zero backfill SQL.

### U4. Narrow account selector and inspector payloads

- **Covers** R4. **Dependencies** None.
- **Files** `internal/domain/accounts.go`, `internal/domain/accounts_test.go`, `internal/domain/transactions.go`, `internal/domain/transactions_test.go`, `app/api/accounts/route.go`, `app/api/transactions/[id]/route.go`, `lib/api/accounts.ts`, `lib/api/transactions.ts`, `components/transactions-view.tsx`, `components/transaction-inspector-drawer.tsx`, generated API types.
- **Approach** Apply KTD4 to serve account ID/name options without balance computation. Preserve the existing selector order and fallback labels. For the inspector, return the editable transaction, source count, and source metadata. Fetch body text on expansion and omit raw LLM JSON from the new UI payload. Enforce transaction-to-source membership in the body read itself. Invalidate detail and source entries after mutations that can change source membership or visibility.
- **Verification** Verify renamed and historical accounts, unassigned rows, transactions with zero or multiple sources, enrichment without expanded emails, and split invalidation. Verify that detached sources and mismatched transaction/source IDs return no body. Confirm account-option requests skip the account balance scan and collapsed details transfer no body or raw LLM JSON.

### U5. Split optional interface code and protect calendar identity

- **Covers** R5 and the calendar portion of R8. **Dependencies** None.
- **Files** `components/transactions-view.tsx`, `components/accounts-view.tsx`, `components/settings-view.tsx`, `components/date-picker.tsx`, `components/ui/calendar.tsx`, `components/app-shell.tsx`.
- **Approach** Apply KTD5 to the audited transaction dialogs, skipped panel, account dialogs, secondary settings tabs, and calendar. Separate the trigger from the calendar body. Lift DayPicker slot component types to module scope and provide locale without recreating the slot type. Preserve the installed Base UI selection and label contract.
- **Verification** Inspect initial production scripts for Accounts and Transactions, and inspect requests when each split path is first used. Exercise direct Settings tab links, pointer and keyboard open, focus return, calendar range selection, exit transitions, and reopen/reset behavior. Compare gzipped initial script bytes with the baseline.

### U6. Start primary queries earlier and cancel superseded searches

- **Covers** R6 and R7. **Dependencies** U1, U2, U3, U4, U5.
- **Files** `components/app-shell.tsx`, `components/dashboard-view.tsx`, `components/transactions-view.tsx`, `components/transaction-list.tsx`, `components/settings/triage-rules-tab.tsx`, `lib/api/client.ts`, `lib/api/transactions.ts`, `lib/api/triage.ts`, `app/transactions/page.tsx`, `app/categories/page.tsx`.
- **Approach** Apply KTD6 to route-intent prefetch. Reuse the server-resolved range mode from U1 and the view’s exact query key. Verify whether safe direct-entry server prefetch is supported and record the finding. Debounce the request-driving search value and apply its page reset in the same update. Pass the query signal to fetch. Apply request context to ordinary Go reads while allowing the one-time backfill transaction to finish or roll back independently. Preserve immediate clearing, URL back/forward restores, page reset, and selection clearing.
- **Verification** Observe a prefetched route enter with one data request per key. Repeat with a prefetch acquired just before local midnight and consumed after it. Verify that the date picker and rows agree. Start on page 2 or later. On rapid typing under slow network, check input response, aborted superseded requests, filtered paging, back/forward restoration, and stale responses. Confirm a fresh production build succeeds without an app server listening during build. Record direct-entry results separately from intent-prefetched navigation.

### U7. Isolate frequent render updates and apply bounded cleanups

- **Covers** R8 and R9. **Dependencies** U4, U5, U6.
- **Files** `components/app-shell.tsx`, `components/sync-toast.tsx`, `components/transaction-inspector-drawer.tsx`, `components/transaction-list.tsx`, `components/transactions-view.tsx`, `components/categories-tags-view.tsx`, `components/skipped-transactions-list.tsx`, `components/add-transaction-dialog.tsx`, `components/manual-merge-dialog.tsx`, `components/accounts-view.tsx`, `components/settings/locale-settings-tab.tsx`, `components/settings/triage-rules-tab.tsx`, `components/tag-multi-select.tsx`, `components/magic-enrich.tsx`, `components/theme-provider.tsx`, `lib/api/index.ts` and its value-import consumers.
- **Approach** Select the shell's sync boolean. Initialize the inspector draft from loaded data and retain its clean-refresh and dirty-draft rules. Keep closed-dialog computations inactive and share selection derivation. Isolate row trees from search and dialog state. Apply direct API imports, stable empty values, cheap derived expressions, single-pass collection operations, memoized skipped-category maps, set membership, length-first array comparison, immutable sort, a stable theme listener, and SVG animation wrappers where audited. Check the icon registry in the build and keep ordinary imports for packages already optimized by Next.js. Defer picker-only icon code if the build shows that list rendering currently brings it in.
- **Verification** Use render counts to confirm sync updates with an unchanged boolean leave the shell header alone, and unrelated dialog input edits leave list rows alone. Exercise tag ordering and duplicates, dirty draft refresh, date and time defaults, effective selection, and the theme hotkey. Keep virtualization and offscreen containment conditional on a profile showing remaining list cost.

## Verification Contract

- Record baseline and candidate production builds with `bun run build`. Verify the initial Accounts and Transactions scripts exclude the optional calendar module and their gzipped script totals each decrease. Use the same build settings and record raw byte deltas.
- Use a disposable configured database and a stalled LLM stub to prove the zero model-request criterion for non-LLM flows. Capture cold and warm request ordering, plus an intent-prefetched navigation. Verify that the active list request begins while a deliberately stalled locale request is still pending.
- Verify ignored-email counts, order, and page and sender options against a fixture with multiple pages and large email bodies. Capture query shape and response sizes. Reproduce the legacy-first-read and warm-read scenarios on the same fixture.
- Run `go test -count=1 ./internal/... ./cmd/...` and vet changed Go route files individually. Run `bun run check:types`, `bun run typecheck`, `bun run lint`, `bun test`, and the production build. Follow `docs/solutions/workflow-issues/nextjs-goroutes-bracket-dirs-break-go-test-and-vet.md` for dynamic route paths.
- Smoke-test the dashboard, both Transactions tabs, Accounts, Categories & Tags, and direct Settings tab links. Include keyboard date picking, dialog reset, draft refresh, email expansion, split, promote, undo, and offline model discovery.

## Definition of Done

- R1–R9 are satisfied and each unit has a recorded verification result.
- The cold-load request traces, production script deltas, ignored-email behavior, and legacy backfill results support the claimed improvements.
- Existing route modes and saved data round-trip through the verified flows.
- Any evidence-gated optimization is resolved with a measurement or a concrete runtime constraint. Remove experimental code that did not contribute to the result.
