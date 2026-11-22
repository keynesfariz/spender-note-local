---
title: Database Query Performance and go-migrate - Plan
type: feat
date: 2026-11-23
topic: db-query-performance-and-go-migrate
artifact_contract: ce-unified-plan/v1
product_contract_source: ce-brainstorm
execution: code
---

# Database Query Performance and go-migrate - Plan

## Goal Capsule

- **Objective:** Keep PortSpenderNote's account, transaction, and monthly spending APIs fast as data grows while replacing ad-hoc schema bootstrap logic with versioned, auto-running SQLite migrations.
- **Product Authority:** Requirements R1–R10 and Key Decisions in the Product Contract below govern behavior and scope; all API response shapes, sort orders, and financial calculations remain unchanged.
- **Open Blockers:** None.

---

## Product Contract

### Summary

Eliminate the `/accounts` N+1 query loop, streamline `/transactions` pagination and monthly spending summaries with missing composite indexes, and introduce `go-migrate` (`golang-migrate`) with embedded auto-migrations on startup using the latest database schema as the v1 baseline. Existing local databases auto-baseline seamlessly on boot, and the SQLite connection pool remains single-connection under WAL.

### Problem Frame

PortSpenderNote's SQLite layer currently runs O(N) queries when listing accounts (4–7 queries per account for reconciliations, balances, future counts, credit card billing cycles, and type-lock checks), executes redundant counts and per-row correlated subqueries on `/transactions`, performs four separate range scans for the monthly spending summary, and lacks composite indexes for common filter patterns. Separately, schema evolution relies on ad-hoc `CREATE TABLE IF NOT EXISTS` and `PRAGMA table_info` checks in `internal/db/db.go` rather than versioned migrations. Although current dataset sizes are small, addressing these hotspots proactively prevents scaling regressions and establishes a clean migration foundation before more schema changes land.

### Key Decisions

- **Include `go-migrate` in this same effort** — introduces versioned migrations now so new performance indexes ship as the first post-baseline migration rather than more ad-hoc startup SQL. Governs R7, R8, R9, R10. (session-settled: user-directed — chosen over deferring `go-migrate` to a separate follow-up: user wanted versioned migrations introduced alongside the query optimizations)
- **Embedded auto-migrations on startup with latest schema as v1 baseline** — eliminates manual CLI steps for local users and avoids reconstructing historical pre-baseline upgrades. Governs R7, R8, R9. (session-settled: user-approved — chosen over CLI-only or hybrid CLI migrations: zero manual steps on startup and clean v1 baseline from the latest schema)
- **Skip splitting SQLite WAL read/write connection pools** — `sqlDB.SetMaxOpenConns(1)` stays as-is because Next.js `goRoutes` already runs each API route in its own OS process, so UI reads on `/api/accounts` and `/api/transactions` already run concurrently with `/api/sync` under WAL. (session-settled: user-approved — chosen over splitting read-only and single-writer pools: avoids read-vs-write DB plumbing across callers for minimal intra-process gain)
- **Preserve all API contracts and financial math** — response payloads, ordering, balance calculations, and billing-cycle summaries stay behaviorally identical so this work is a pure performance and migration upgrade. Governs R1, R2, R3, R4, R5, R6.

### Requirements

**Accounts N+1 Elimination**

- R1. Listing accounts executes a small, constant number of SQL queries regardless of how many accounts exist, replacing the per-account query loop for latest reconciliations, balance totals, future transaction counts, credit card billing-cycle spend, and type-lock status.
- R2. Single-account mutation flows (create, update, confirm, reconcile) continue to return accurate `AccountSummary` values with identical balance, billing-cycle, and type-lock semantics.

**Transactions, Monthly Summary, and Indexes**

- R3. Paginated transaction listing avoids redundant range-count queries when no extra filters are active beyond the date range.
- R4. Paginated transaction listing resolves per-row payment method hints and source roles in batched lookups for the page's transaction IDs rather than executing correlated subqueries per row.
- R5. Monthly spending summary calculation eliminates redundant table scans over the same month window while preserving identical totals, foreign currency counts, and category, merchant, and account breakdowns.
- R6. Missing composite indexes are added for hot query paths, including `transactions(currency, occurred_at_utc)` for monthly summary and currency-filtered range queries and `raw_emails(sender_id, internal_date_ms)` for sender high-water-mark sync queries.

**Versioned Migrations with `go-migrate`**

- R7. Database schema initialization and upgrades use `go-migrate` with SQL migration files embedded into the Go binary via `go:embed` and executed automatically during `OpenDatabase`, staying CGO-free with the existing pure-Go SQLite driver (`modernc.org/sqlite`).
- R8. Migration v1 represents the current latest database schema baseline, and the new performance indexes from R6 ship as migration v2.
- R9. Existing local databases created before `go-migrate` (which already have the current schema tables but no `schema_migrations` table) automatically baseline at v1 on startup and immediately apply v2+ with zero manual intervention.
- R10. Startup migration execution serializes safely across concurrent OS processes (such as parallel Next.js `goRoutes` boots on first page load) so concurrent startups never race, fail with `SQLITE_BUSY`, or leave `schema_migrations` in a dirty state.

### Scope Boundaries

- Splitting `sql.DB` into separate read-only and single-writer WAL pools (`SetMaxOpenConns(1)` stays unchanged).
- Building a standalone migration CLI binary or requiring manual CLI migration commands.
- Reconstructing historical pre-baseline migrations (older `gguf` / pre-`reviewed` / pre-`statement_day` table rebuilds).
- Frontend changes, UI caching, or API response schema changes.

### Dependencies / Assumptions

- Existing local databases that users care about have already booted at least once on current `main`, so their schema matches the v1 baseline before `go-migrate` auto-baselines them.
- In-memory SQLite test databases (`:memory:`) must continue to initialize cleanly through `OpenDatabase(":memory:", false)` using the same embedded migrations.

### Sources / Research

- `internal/db/db.go` — current `schemaSQL`, `migrateSchema`, `runImmediateMigration`, `SetMaxOpenConns(1)`, and existing index definitions.
- `internal/domain/accounts.go` and `internal/domain/accounts_balance.go` — `listAccountsAt` and `buildAccountSummaryAt` N+1 query paths.
- `internal/domain/transactions.go` — `ListTransactions` (3 queries + 2 correlated subqueries per row) and `GetMonthlySpendingSummary` (4 separate range scans).
- `internal/syncrunner/runner.go` — unindexed `SELECT MAX(internal_date_ms) FROM raw_emails WHERE sender_id = ?`.
- `docs/solutions/database-issues/sqlite-table-rebuild-begin-immediate-goroutes.md` — documented multi-process `goRoutes` startup race and `BEGIN IMMEDIATE` serialization pattern.
- `go.mod` — pure-Go SQLite stack (`github.com/glebarez/sqlite`, `modernc.org/sqlite`).

---

## Planning Contract

### Key Technical Decisions

- KTD1. **Embedded `golang-migrate/migrate/v4` with `iofs` and pure-Go `database/sqlite`** — uses `embed.FS` under `internal/db/migrations/` with `github.com/golang-migrate/migrate/v4/source/iofs` and `github.com/golang-migrate/migrate/v4/database/sqlite` (`sqlite.WithInstance`), keeping the build CGO-free. Governs R7, R8.
- KTD2. **Cross-process advisory file lock around startup migrations** — for file-backed SQLite databases, `OpenDatabase` acquires an exclusive `syscall.Flock` on `<dbPath>.migrate.lock` before checking `schema_migrations`, auto-baselining existing pre-`go-migrate` databases (`Force(1)` when `app_settings` exists without `schema_migrations`), and running `m.Up()`. In-memory databases skip the file lock and run `m.Up()` directly. Governs R9, R10.
- KTD3. **Constant-query batching for `listAccountsAt`** — replaces the per-account loop in `internal/domain/accounts.go` / `accounts_balance.go` with batched queries for latest reconciliations, balance/future/type-lock aggregates, and credit card billing-cycle spend, while keeping `buildAccountSummaryAt` intact for single-account writes (create, update, confirm, reconcile). Governs R1, R2.
- KTD4. **Batched page enrichment and single-scan monthly summary in `internal/domain/transactions.go`** — `ListTransactions` skips the second `COUNT(*)` when `whereClause == rangeWhere` and resolves `payment_method_raw` and `sourceRoles` in 2 batched `IN (...)` queries over the page's transaction IDs instead of 2 correlated subqueries per row. `GetMonthlySpendingSummary` fetches the month's transactions in a single indexed range scan (with `LEFT JOIN accounts`) and computes totals, foreign count, and category/merchant/account breakdowns in Go. Governs R3, R4, R5.
- KTD5. **Composite indexes in migration v2** — `000002_performance_indexes.up.sql` adds `idx_transactions_currency_occurred ON transactions(currency, occurred_at_utc)` and `idx_raw_emails_sender_date ON raw_emails(sender_id, internal_date_ms)`. Governs R6, R8.

---

## Implementation Units

### U1. Embedded `go-migrate` foundation and v1/v2 migrations

- **Goal:** Replace ad-hoc `schemaSQL` and `migrateSchema` in `internal/db/db.go` with embedded `golang-migrate/migrate/v4` migrations (v1 baseline schema, v2 performance indexes), cross-process flock serialization, and seamless auto-baselining for existing local databases.
- **Files:** `go.mod`, `go.sum`, `internal/db/db.go`, `internal/db/migrations/000001_baseline.up.sql`, `internal/db/migrations/000001_baseline.down.sql`, `internal/db/migrations/000002_performance_indexes.up.sql`, `internal/db/migrations/000002_performance_indexes.down.sql`, `internal/db/db_test.go`, `internal/domain/extraction_test.go`, `internal/domain/accounts_test.go`.
- **Verification:** Fresh `:memory:` and file DBs apply v1 and v2 cleanly; pre-`go-migrate` file DBs auto-baseline at v1 and apply v2; concurrent `OpenDatabase` calls succeed without `SQLITE_BUSY` or dirty state; `go test ./internal/db/...`.

### U2. Eliminate `/accounts` N+1 queries

- **Goal:** Refactor `listAccountsAt` in `internal/domain/accounts.go` and `internal/domain/accounts_balance.go` so listing N accounts runs a small constant number of SQL queries while preserving exact balance, future count, billing cycle, and type-lock results.
- **Files:** `internal/domain/accounts.go`, `internal/domain/accounts_balance.go`, `internal/domain/accounts_test.go`.
- **Verification:** Existing account tests pass, plus a new test verifying query count stays constant across multiple accounts and matches single-account `buildAccountSummaryAt` results.

### U3. Optimize `ListTransactions` and `GetMonthlySpendingSummary`

- **Goal:** Eliminate redundant filtered `COUNT(*)` when unfiltered, replace per-row correlated subqueries with batched `IN (...)` lookups for the page's transaction IDs, and consolidate `GetMonthlySpendingSummary`'s 4 range scans into 1 indexed scan with Go aggregation.
- **Files:** `internal/domain/transactions.go`, `internal/domain/transactions_test.go`.
- **Verification:** Existing transaction and monthly summary tests pass, plus tests verifying identical outputs for filtered/unfiltered pagination, payment method precedence, source roles, and monthly summary breakdowns.

---

## Verification Contract

| Check | Command | Scope |
|---|---|---|
| Go unit & integration tests | `go test ./...` | U1, U2, U3 |
| Go vet | `go vet ./...` | U1, U2, U3 |
| TypeScript / Next.js typecheck | `npm run typecheck` (if present) | Repo sanity |

---

## Definition of Done

- All Implementation Units U1–U3 implemented and verified with `go test ./...` and `go vet ./...` passing.
- Code simplified and reviewed, with all eligible findings applied and committed locally.
