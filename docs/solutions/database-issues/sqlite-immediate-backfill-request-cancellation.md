---
title: Let SQLite backfill transactions finish after HTTP cancellation
date: 2026-11-25
category: database-issues
module: Triage Backfill
problem_type: best_practice
component: database
severity: high
applies_when:
  - "A Go route starts a compatibility backfill before a request-scoped SQLite read"
  - "The backfill uses an explicit BEGIN IMMEDIATE on a pinned connection"
tags: [sqlite, gorm, go, backfill, request-cancellation, rollback]
---

# Let SQLite backfill transactions finish after HTTP cancellation

## Context

When Transactions and Skipped began passing HTTP cancellation to their SQLite reads, review found that the same context could reach the legacy triage backfill. A disconnect after the backfill acquired its write lock could cancel the deferred rollback along with the write. The fix keeps the cancellation boundary inside the domain operation, where the read and the compatibility write diverge. See [Transactions GET](../../../app/api/transactions/route.go) and [Skipped GET](../../../app/api/triage/skipped/route.go).

## Guidance

- Keep the HTTP context on the ordinary list queries. Give the one-time compatibility write an independent context from the start of the transaction through commit or rollback. In the triage path, `TriageBackfill.ensure` uses `context.Background()` for `BackfillSkippedEmailTriageData`. [Backfill gate](../../../internal/domain/triage_backfill.go)
- Mark readiness after the backfill succeeds so failures can be retried. A process-local gate skips repeated checks in that route process. Keep the candidate recheck inside the SQLite write lock because another route process can run the same backfill. [Backfill implementation](../../../internal/domain/triage.go)

## Why This Matters

`RunImmediateTx` sends `BEGIN IMMEDIATE`, `COMMIT`, and `ROLLBACK` as SQL on a pinned GORM connection. With raw `BEGIN`, there is no managed `sql.Tx` to roll back automatically when the request context is canceled. If the same canceled context reaches the deferred `ROLLBACK`, the connection can return to the pool with cleanup unfinished. This repository allows one open SQLite connection per database handle, so that failure can affect the next request as well. [Transaction helper](../../../internal/db/tx.go) · [Database setup](../../../internal/db/db.go)

## When to Apply

Use this boundary for idempotent compatibility work that must finish independently of the request that started it. Keep user-driven operations under their own cancellation semantics.

## Examples

`TestTriageBackfill_RetryAfterCanceledRequest` cancels the initiating context after the lock is acquired. A SQLite trigger forces the update to fail, and the test requires that specific trigger error. It then removes the trigger and retries on the same database handle. That proves the backfill reached the write after cancellation and the connection stayed usable after rollback. [Regression test](../../../internal/domain/triage_backfill_test.go)

## Related

- [SQLite table-rebuild locking across Go route processes](sqlite-table-rebuild-begin-immediate-goroutes.md)
