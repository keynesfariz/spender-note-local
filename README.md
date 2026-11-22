# PortSpenderNote

A local-first personal spending tracker. It pulls transaction emails from Gmail, extracts structured spending data using a local GGUF LLM, groups transactions into accounts, and presents monthly dashboards, category breakdowns, and reconciliation tools.

## Why We Build It

Most expense trackers either require manual entry for every purchase or ask you to hand over bank credentials and forward financial emails to third-party cloud servers. PortSpenderNote automates spending tracking from the bank, card, and receipt emails you already receive—while keeping both your transaction database and LLM extraction completely local on your machine.

## Tech Stack

- **Frontend:** Next.js 16 (App Router), React 19, TypeScript, Tailwind CSS v4, shadcn/ui + Base UI (`@base-ui/react`), TanStack Query
- **Backend:** Go API routes (`app/api/`, `internal/syncrunner`, `internal/gmail`, `internal/domain`, `internal/db`)
- **Database:** SQLite (WAL mode) via GORM + `glebarez/sqlite` (`data/spender.db`)
- **Local LLM Extraction:** Direct OpenAI-compatible local server (defaults to `llama.app` at `http://localhost:9931/v1`)
- **Runtime:** Bun (`bun run dev`, `bun run build`, `bun run typecheck`)

## TODO

Ordered by priority.

### P0: Correctness and Reliability

1. [x] **Fix:** wrong amounts displayed and valid amounts raising "invalid amount" errors
2. [x] **Feat:** email fetching with configurable `sync_batch_limit` (default `50`), Gmail 403/429 rate-limit retry, and incremental progress saving *(deferred out of P0: running email fetching concurrently with LLM parsing)*
3. [x] **Fix:** `senders.last_sync_at_utc` and `senders.last_internal_date_ms` staying `NULL` under batch limits, and high-water mark cursor calculation across multi-batch backfills

### P1: Data Model Decisions

4. [ ] **Design:** transaction merging setting:
   - Option 1: current merging mechanism
   - Option 2: merge only on equal amounts within a set time window (also the fallback when no accounts exist)
5. [ ] **Design:** merge suggestions and behavior — show the email address in the transaction list when a transaction has exactly one
6. [x] **Design:** auto-create provisional accounts (`status = 'provisional'`, `initial_balance_minor = NULL`) during extraction (`CreateProvisionalAccountForExtraction`), with confirm and merge flows (`MergeAccounts` / `MergeAccountDialog`) that move transactions, identifiers, senders, and reconciliations to the destination account
7. [ ] **Design:** mark two transactions (from email or manual) as a transfer (`from → to`)
8. [ ] **Design:** customizable tracking cycle in Settings — a newly added email sender is tracked from the first day of the current cycle

### P2: Features

9. [ ] **Feat:** Category and Tag CRUD
10. [ ] **Feat:** scheduled transactions (e.g., payday on the 24th) without creating duplicates
11. [ ] **UI:** tabbed Settings (to organize cycle, merging, and category options)

### P3: Refactors & Improvements

12. [ ] **Sync:** fair/round-robin per-sender batching so one high-volume sender does not consume the entire `sync_batch_limit`
13. [ ] **Refactor:** SQL and general cleanup
