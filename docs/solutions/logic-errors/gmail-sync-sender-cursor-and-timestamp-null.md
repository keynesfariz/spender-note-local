---
title: Gmail sync leaves sender last_sync_at_utc and last_internal_date_ms NULL under batch limits
date: 2026-11-21
category: logic-errors
module: Gmail Sync Runner
problem_type: logic_error
component: background_job
symptoms:
  - "senders.last_sync_at_utc and senders.last_internal_date_ms remain NULL after multiple completed Gmail sync runs"
  - "Settings view displays 'Never synced' for senders that already have fetched and extracted rows in raw_emails"
  - "After a multi-batch backfill finishes, last_internal_date_ms can be pinned to the oldest batch's timestamp or remain NULL instead of advancing to the newest fetched email"
root_cause: logic_error
resolution_type: code_fix
severity: medium
tags: [gmail-sync, pagination, cursor, high-water-mark, sqlite, go]
---

# Gmail sync leaves sender last_sync_at_utc and last_internal_date_ms NULL under batch limits

## Problem
When a sender has more unfetched Gmail messages than `app_settings.sync_batch_limit`, repeated sync runs fetch and extract emails for that sender while leaving both `senders.last_sync_at_utc` and `senders.last_internal_date_ms` as `NULL`. Even once the multi-batch backfill finishes, the cursor logic only looked at the final (oldest) batch's `maxInternalDateMs`, risking a stale or `NULL` high-water mark cursor.

## Symptoms
- Multiple `sync_runs` finish with `status = 'completed'`, `fetched_count > 0`, and `has_more_emails = 1`, yet `senders.last_sync_at_utc` and `senders.last_internal_date_ms` remain `NULL`.
- The Settings UI (`components/settings/settings-view.tsx`) shows `Never synced` for a sender that already has extracted `raw_emails`.
- When `sync_batch_limit` is small (e.g., `5`), the first enabled sender alphabetically (`runFullSyncJob` orders enabled senders by `email ASC`) consumes the entire run's `remainingBudget`, so later senders are not reached until earlier ones catch up.

## What Didn't Work
- **Naively removing the `!result.HasMore` guard for both columns:** Gmail's `messages.list` API returns messages in reverse chronological order (newest first). Advancing `last_internal_date_ms` to the first batch's `maxInternalDateMs` while `result.HasMore` is `true` would set the next sync's `after:` filter (`overlapStartMs = *sender.LastInternalDateMs - cursorOverlapMs`) past all older unfetched emails between `backfill_from_utc` and the first batch, permanently skipping them.

## Solution
Decouple `last_sync_at_utc` from `last_internal_date_ms` in `updateSenderSyncState` (`internal/syncrunner/runner.go`), called from `runFullSyncJob` after a sender fetch succeeds without cancellation or error:

1. Always update `last_sync_at_utc` to `domain.NowUtcIsoSeconds()`, even when `hasMore` is `true`.
2. Keep `last_internal_date_ms` gated on `!hasMore` so multi-batch backfills never skip older emails.
3. When `!hasMore` is `true`, compute `nextCursor` as the maximum of `sender.LastInternalDateMs`, the current run's `maxInternalDateMs`, and `SELECT MAX(internal_date_ms) AS max_ms FROM raw_emails WHERE sender_id = ?` so the cursor lands on the true high-water mark across all prior batches.

**Before (`internal/syncrunner/runner.go` in `runFullSyncJob`):**
```go
if !result.HasMore {
	nextCursor := sender.LastInternalDateMs
	if maxInternalDateMs != nil {
		v := *maxInternalDateMs
		if nextCursor != nil && *nextCursor > v {
			v = *nextCursor
		}
		nextCursor = &v
	}
	gdb.Model(&db.Sender{}).Where("id = ?", sender.ID).Updates(map[string]any{
		"last_internal_date_ms": nextCursor,
		"last_sync_at_utc":      domain.NowUtcIsoSeconds(),
	})
}
```

**After (`internal/syncrunner/runner.go` in `updateSenderSyncState`):**
```go
func updateSenderSyncState(gdb *gorm.DB, sender db.Sender, maxInternalDateMs *int64, hasMore bool) {
	updates := map[string]any{
		"last_sync_at_utc": domain.NowUtcIsoSeconds(),
	}
	if !hasMore {
		nextCursor := sender.LastInternalDateMs
		if maxInternalDateMs != nil {
			v := *maxInternalDateMs
			if nextCursor == nil || v > *nextCursor {
				nextCursor = &v
			}
		}
		var maxRow struct {
			MaxMs *int64 `gorm:"column:max_ms"`
		}
		gdb.Raw(`SELECT MAX(internal_date_ms) AS max_ms FROM raw_emails WHERE sender_id = ?`, sender.ID).Scan(&maxRow)
		if maxRow.MaxMs != nil {
			v := *maxRow.MaxMs
			if nextCursor == nil || v > *nextCursor {
				nextCursor = &v
			}
		}
		updates["last_internal_date_ms"] = nextCursor
	}
	gdb.Model(&db.Sender{}).Where("id = ?", sender.ID).Updates(updates)
}
```

## Why This Works
- **`last_sync_at_utc` vs. `last_internal_date_ms` have different invariants:** `last_sync_at_utc` records when the sender was last successfully synced (safe to update on every completed sender fetch, including partial batches). `last_internal_date_ms` is a high-water mark cursor used to compute the `after:` epoch second filter in `runFullSyncJob` (`internal/syncrunner/runner.go`), so it can only advance once `gmail.FetchSenderMessages` (`internal/gmail/client.go`) exhausts all unseen messages (`HasMore == false`).
- **Newest-first pagination requires querying stored `raw_emails` on completion:** Because `gmail.FetchSenderMessages` pages newest-first and skips already-known `gmail_message_id` values via `KnownMessageIDs`, earlier batch-capped runs persist the newest emails while the final `!HasMore` run persists the oldest remaining emails (or zero emails if the previous run hit the exact boundary). Taking `MAX(internal_date_ms)` from `raw_emails` for that `sender_id` ensures the cursor advances to the true newest message across the entire backfill, including after a user moves `backfill_from_utc` earlier (which resets `last_internal_date_ms` to `nil` in `domain.UpdateSender` in `internal/domain/settings.go`).

## Prevention
- **Regression tests:** `internal/syncrunner/runner_test.go` verifies `updateSenderSyncState` against an in-memory SQLite database (`db.OpenDatabase(":memory:", false)`) across four cases:
  - Partial batch (`hasMore == true`) sets `last_sync_at_utc` while leaving `last_internal_date_ms` unchanged/`NULL`.
  - Final batch (`hasMore == false`) with older emails advances `last_internal_date_ms` to the higher timestamp stored in `raw_emails` from an earlier batch.
  - Final batch (`hasMore == false`) with `maxInternalDateMs == nil` still advances `last_internal_date_ms` to the stored `raw_emails` high-water mark.
  - Sender with zero emails leaves `last_internal_date_ms` as `NULL` while updating `last_sync_at_utc`.
- **Cursor design rule for reverse-chronological APIs:** Never couple an observational "last synced" timestamp to a pagination-completion cursor guard, and never advance a lower-bound `after:` cursor before all pages in the current window are exhausted.

## Related Issues
- None.
