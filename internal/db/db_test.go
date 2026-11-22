package db

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOpenDatabase_FreshSchemaDefaultsAndSyncingPhase(t *testing.T) {
	gdb, err := OpenDatabase(":memory:", false)
	if err != nil {
		t.Fatalf("open fresh db: %v", err)
	}

	var s AppSetting
	if err := gdb.First(&s, 1).Error; err != nil {
		t.Fatalf("load app_settings: %v", err)
	}
	if s.LlmProvider != "openai_compatible" {
		t.Fatalf("expected llm_provider=openai_compatible, got %q", s.LlmProvider)
	}
	if s.LlmBaseURL != DefaultLlmBaseURL {
		t.Fatalf("expected llm_base_url=%q, got %q", DefaultLlmBaseURL, s.LlmBaseURL)
	}
	if s.LlmModel != nil {
		t.Fatalf("expected llm_model=nil, got %q", *s.LlmModel)
	}

	run := SyncRun{
		ID:           "run-syncing-1",
		Status:       "running",
		Phase:        "syncing",
		LlmProvider:  "openai_compatible",
		LlmModel:     "qwen2.5",
		StartedAtUtc: "2026-11-23T00:00:00Z",
	}
	if err := gdb.Create(&run).Error; err != nil {
		t.Fatalf("expected sync_runs to accept phase='syncing', got error: %v", err)
	}
}

func TestOpenDatabase_MigratesLegacyGgufAppSettingsAndSyncRuns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	rawDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open raw legacy db: %v", err)
	}

	legacySQL := `
		CREATE TABLE app_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			country_code TEXT,
			timezone TEXT,
			currency TEXT,
			llm_provider TEXT NOT NULL DEFAULT 'gguf' CHECK (llm_provider IN ('gguf')),
			llm_model_path TEXT,
			llm_context_size INTEGER NOT NULL DEFAULT 8192 CHECK (llm_context_size > 0),
			llm_timeout_ms INTEGER NOT NULL DEFAULT 60000 CHECK (llm_timeout_ms > 0),
			sync_batch_limit INTEGER NOT NULL DEFAULT 50 CHECK (sync_batch_limit >= 1 AND sync_batch_limit <= 500)
		) STRICT;
		INSERT INTO app_settings (id, country_code, timezone, currency, llm_provider, llm_model_path, llm_context_size, llm_timeout_ms, sync_batch_limit)
		VALUES (1, 'ID', 'Asia/Jakarta', 'IDR', 'gguf', 'hf_Qwen.gguf', 16384, 45000, 25);

		CREATE TABLE sync_runs (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'partial_error', 'failed', 'cancelled', 'interrupted')),
			phase TEXT NOT NULL CHECK (phase IN ('preflight', 'fetching', 'extracting', 'done')),
			llm_provider TEXT NOT NULL,
			llm_model TEXT NOT NULL,
			fetched_count INTEGER NOT NULL DEFAULT 0,
			cleaned_count INTEGER NOT NULL DEFAULT 0,
			imported_count INTEGER NOT NULL DEFAULT 0,
			merged_count INTEGER NOT NULL DEFAULT 0,
			skipped_count INTEGER NOT NULL DEFAULT 0,
			failed_count INTEGER NOT NULL DEFAULT 0,
			has_more_emails INTEGER NOT NULL DEFAULT 0 CHECK (has_more_emails IN (0, 1)),
			provisional_accounts_created INTEGER NOT NULL DEFAULT 0,
			error_message TEXT,
			started_at_utc TEXT NOT NULL CHECK (length(started_at_utc) = 20 AND started_at_utc LIKE '%Z'),
			finished_at_utc TEXT
		) STRICT;
		INSERT INTO sync_runs (id, status, phase, llm_provider, llm_model, imported_count, started_at_utc, finished_at_utc)
		VALUES ('legacy-run-1', 'completed', 'done', 'gguf', 'hf_Qwen.gguf', 3, '2026-11-22T01:00:00Z', '2026-11-22T01:01:00Z');
	`
	if err := rawDB.Exec(legacySQL).Error; err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if sqlDB, err := rawDB.DB(); err == nil {
		_ = sqlDB.Close()
	}

	migrated, err := OpenDatabase(dbPath, false)
	if err != nil {
		t.Fatalf("open migrated db: %v", err)
	}

	var s AppSetting
	if err := migrated.First(&s, 1).Error; err != nil {
		t.Fatalf("load migrated app_settings: %v", err)
	}
	if s.CountryCode == nil || *s.CountryCode != "ID" || s.Timezone == nil || *s.Timezone != "Asia/Jakarta" || s.Currency == nil || *s.Currency != "IDR" {
		t.Fatalf("expected locale preserved, got %+v", s)
	}
	if s.LlmProvider != "openai_compatible" || s.LlmBaseURL != DefaultLlmBaseURL || s.LlmModel != nil {
		t.Fatalf("expected llm settings migrated cleanly, got %+v", s)
	}
	if s.LlmTimeoutMs != 45000 || s.SyncBatchLimit != 25 {
		t.Fatalf("expected timeout=45000 and batch=25 preserved, got timeout=%d batch=%d", s.LlmTimeoutMs, s.SyncBatchLimit)
	}

	var legacyRun SyncRun
	if err := migrated.First(&legacyRun, "id = ?", "legacy-run-1").Error; err != nil {
		t.Fatalf("expected legacy sync_run preserved: %v", err)
	}
	if legacyRun.ImportedCount != 3 {
		t.Fatalf("expected legacy imported_count=3, got %d", legacyRun.ImportedCount)
	}

	if err := migrated.Create(&SyncRun{
		ID:           "new-syncing-run",
		Status:       "running",
		Phase:        "syncing",
		LlmProvider:  "openai_compatible",
		LlmModel:     "llama-3",
		StartedAtUtc: "2026-11-23T00:05:00Z",
	}).Error; err != nil {
		t.Fatalf("expected migrated sync_runs to accept phase='syncing': %v", err)
	}

	if sqlDB, err := migrated.DB(); err == nil {
		_ = sqlDB.Close()
	}

	// Second open must be idempotent.
	reopened, err := OpenDatabase(dbPath, false)
	if err != nil {
		t.Fatalf("reopen already-migrated db: %v", err)
	}
	if sqlDB, err := reopened.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
