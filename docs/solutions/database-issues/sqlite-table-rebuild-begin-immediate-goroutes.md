---
title: SQLite table-rebuild migrations under Next.js goRoutes must use BEGIN IMMEDIATE and re-check DDL inside the lock
date: 2026-11-23
category: database-issues
module: SQLite Startup Migrations
problem_type: database_issue
component: database
symptoms:
  - "Multiple Next.js goRoutes API processes boot concurrently on first page load and call OpenDatabase in parallel"
  - "GORM DEFERRED transactions allow two processes to observe legacy sqlite_master DDL before either acquires SQLite's write lock"
  - "Concurrent table-rebuild migrations race on CREATE TABLE *_new and DROP TABLE, failing startup with table-already-exists or SQLITE_BUSY errors"
root_cause: concurrency
resolution_type: migration
severity: high
tags: [sqlite, strict-tables, migrations, nextjs-goroutes, concurrency, gorm, go]
---

# SQLite table-rebuild migrations under Next.js goRoutes must use BEGIN IMMEDIATE and re-check DDL inside the lock

## Problem
In PortSpenderNote, SQLite tables use `STRICT` with inline `CHECK` constraints (`internal/db/db.go`), which SQLite cannot alter in place with `ALTER TABLE`. Updating constraints (such as migrating `app_settings` from `gguf` to `openai_compatible` or adding `'syncing'` to `sync_runs.phase`) requires a table-rebuild migration (`CREATE TABLE *_new`, `INSERT ... SELECT`, `DROP TABLE`, `ALTER TABLE RENAME`). Under Next.js `experimental: { goRoutes: true }`, loading a page like `/settings` fires multiple parallel API route requests (`/api/settings`, `/api/senders`, `/api/gmail/status`, `/api/accounts`, `/api/sync`), each running in its own Go process and calling `db.OpenDatabase` on startup.

## Symptoms
- Opening the app after a schema-constraint upgrade triggers parallel `OpenDatabase` -> `migrateSchema` calls across multiple Go route processes.
- Using GORM's `gdb.Transaction(...)` (which opens a `DEFERRED` transaction) allows two processes to read `sqlite_master` concurrently, see the legacy DDL, and race when creating `*_new` or dropping the original table.

## What Didn't Work
- **Wrapping the table rebuild in `gdb.Transaction(...)` with a pre-check outside the transaction:** `gdb.Transaction` starts a `DEFERRED` transaction that does not acquire SQLite's write lock until the first write statement. Two processes both pass the `sqlite_master` check and collide at `CREATE TABLE app_settings_new`.
- **Trying to bind a raw `*sql.Conn` back into `gorm.Session{ConnPool: conn}`:** GORM's `gorm.Session` struct does not expose a `ConnPool` field, and `gdb.Statement.Context` can be `nil` during `OpenDatabase` initialization.

## Solution
Use `runImmediateMigration` in `internal/db/db.go` for any SQLite table-rebuild migration:

1. Acquire a dedicated `*sql.Conn` from `sqlDB.Conn(context.Background())` and execute `BEGIN IMMEDIATE` to take SQLite's reserved write lock immediately (respecting `PRAGMA busy_timeout = 5000`).
2. Re-read `sqlite_master` (`getTableCreateSQLConn`) *after* `BEGIN IMMEDIATE` succeeds so any process that waited on the lock observes the already-migrated schema and exits cleanly as a no-op.
3. Run `DROP TABLE IF EXISTS <table>_new` before creating the replacement table, copy rows, drop the old table, rename, recreate indexes, and `COMMIT` (or `ROLLBACK` on any error).

```go
func runImmediateMigration(gdb *gorm.DB, fn func(ctx context.Context, conn *sql.Conn) error) error {
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin immediate migration tx: %w", err)
	}
	if err := fn(ctx, conn); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return fmt.Errorf("commit migration tx: %w", err)
	}
	return nil
}
```

## Why This Works
`BEGIN IMMEDIATE` serializes writers across separate OS processes via SQLite's file lock before any schema inspection runs. Combined with `busy_timeout(5000)` and the post-lock `sqlite_master` re-check, the first Go route process completes the atomic rebuild and commits while all concurrent route processes wait briefly, see the updated DDL inside their own lock, and no-op without touching `*_new`.

## Prevention
- Always route future SQLite `STRICT`/`CHECK` table-rebuild migrations in `internal/db/db.go` through `runImmediateMigration` with a double-checked `sqlite_master` guard inside the lock.
- Add legacy-schema upgrade and idempotency tests in `internal/db/db_test.go` using a real temp file (not only `:memory:`) to verify reopen idempotency.

## Related Issues
- `internal/db/db.go` (`runImmediateMigration`, `migrateAppSettingsTable`, `migrateSyncRunsTable`)
- `internal/db/db_test.go` (`TestOpenDatabase_MigratesLegacyGgufAppSettingsAndSyncRuns`)
