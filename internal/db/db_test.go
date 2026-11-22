package db

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOpenDatabase_FreshSchemaDefaultsAndIndexes(t *testing.T) {
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

	var mig struct {
		Version int  `gorm:"column:version"`
		Dirty   bool `gorm:"column:dirty"`
	}
	if err := gdb.Raw(`SELECT version, dirty FROM schema_migrations`).Scan(&mig).Error; err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if mig.Version != 2 || mig.Dirty {
		t.Fatalf("expected schema_migrations version=2 dirty=false, got version=%d dirty=%v", mig.Version, mig.Dirty)
	}

	for _, idxName := range []string{"idx_transactions_currency_occurred", "idx_raw_emails_sender_date"} {
		var found string
		if err := gdb.Raw(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, idxName).Scan(&found).Error; err != nil {
			t.Fatalf("check index %s: %v", idxName, err)
		}
		if found != idxName {
			t.Fatalf("expected index %s to exist", idxName)
		}
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

func TestOpenDatabase_AutoBaselinesExistingDatabaseAndAppliesV2(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "existing.db")

	rawDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open raw existing db: %v", err)
	}

	baselineSQL, err := migrationsFS.ReadFile("migrations/000001_baseline.up.sql")
	if err != nil {
		t.Fatalf("read baseline sql: %v", err)
	}
	if err := rawDB.Exec(string(baselineSQL)).Error; err != nil {
		t.Fatalf("seed baseline schema: %v", err)
	}
	if err := rawDB.Exec(`
		INSERT INTO app_settings (id, country_code, timezone, currency, llm_provider, llm_base_url, llm_timeout_ms, sync_batch_limit)
		VALUES (1, 'ID', 'Asia/Jakarta', 'IDR', 'openai_compatible', ?, 45000, 25)`, DefaultLlmBaseURL).Error; err != nil {
		t.Fatalf("seed app_settings: %v", err)
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
	if s.LlmTimeoutMs != 45000 || s.SyncBatchLimit != 25 {
		t.Fatalf("expected timeout=45000 and batch=25 preserved, got timeout=%d batch=%d", s.LlmTimeoutMs, s.SyncBatchLimit)
	}

	var mig struct {
		Version int  `gorm:"column:version"`
		Dirty   bool `gorm:"column:dirty"`
	}
	if err := migrated.Raw(`SELECT version, dirty FROM schema_migrations`).Scan(&mig).Error; err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if mig.Version != 2 || mig.Dirty {
		t.Fatalf("expected version=2 dirty=false, got version=%d dirty=%v", mig.Version, mig.Dirty)
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

func TestOpenDatabase_ConcurrentStartupMigrationsSerializeCleanly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent.db")

	const workers = 6
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gdb, err := OpenDatabase(dbPath, false)
			if err != nil {
				errs <- err
				return
			}
			if sqlDB, err := gdb.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent OpenDatabase failed: %v", err)
	}
}
